package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"dougdomingos.com/aegis/internal/orchestrator/domain"
	"dougdomingos.com/aegis/internal/orchestrator/infra/query"
)

const (
	// metadataEpochKey is the system_metadata key holding the propagation epoch.
	metadataEpochKey = "propagation_epoch"

	queryNextPropagationSequence = "SELECT COALESCE(MAX(sequence), 0) + 1 FROM propagations WHERE machine_id = ?"
	queryCreatePropagation       = `INSERT INTO propagations
		(machine_id, sequence, kind, policy_id, policy_name, policy_version, policy_type, status, payload)
		VALUES (?,?,?,?,?,?,?,?,?)`
	queryGetPropagationByID = `SELECT id, machine_id, sequence, kind, policy_id, policy_name, policy_version,
		policy_type, status, sent_at, acked_at, nack_reason, payload, created_at
		FROM propagations WHERE id = ?`
	queryMarkPropagationSent    = "UPDATE propagations SET status = 'SENT', sent_at = ? WHERE machine_id = ? AND sequence = ?"
	queryMarkPropagationFailed  = "UPDATE propagations SET status = 'FAILED', nack_reason = ? WHERE machine_id = ? AND sequence = ?"
	queryAcknowledgePropagation = `UPDATE propagations
		SET status = ?, acked_at = ?, nack_reason = ? WHERE machine_id = ? AND sequence = ?`
	queryLatestPropagationByMachine = `SELECT id, machine_id, sequence, kind, policy_id, policy_name, policy_version,
		policy_type, status, sent_at, acked_at, nack_reason, created_at
		FROM propagations WHERE machine_id = ? ORDER BY sequence DESC LIMIT 1`
	queryListLatestPropagations = `SELECT p.id, p.machine_id, p.sequence, p.kind, p.policy_id, p.policy_name,
		p.policy_version, p.policy_type, p.status, p.sent_at, p.acked_at, p.nack_reason, p.created_at
		FROM propagations p
		JOIN (SELECT machine_id, MAX(sequence) AS max_sequence FROM propagations GROUP BY machine_id) latest
			ON latest.machine_id = p.machine_id AND latest.max_sequence = p.sequence`
	queryGetSystemMetadata    = "SELECT value FROM system_metadata WHERE key = ?"
	queryInsertSystemMetadata = "INSERT OR IGNORE INTO system_metadata (key, value) VALUES (?, ?)"
)

// PropagationStore manages all database-related operations over the
// propagation audit log.
type PropagationStore struct {
	db       *sql.DB
	executor *query.QueryExecutor[domain.Propagation]
}

// NewPropagationStore creates a new PropagationStore instance.
func NewPropagationStore(db *sql.DB) *PropagationStore {
	return &PropagationStore{
		db:       db,
		executor: query.NewQueryExecutor(db, mapRowToPropagationStatus),
	}
}

func (store *PropagationStore) Create(ctx context.Context, propagation domain.Propagation, encode func(sequence int64) (string, error)) (*domain.Propagation, error) {
	var created *domain.Propagation

	err := store.executor.WithTx(ctx, func(tx *sql.Tx) error {
		var sequence int64
		if err := tx.QueryRowContext(ctx, queryNextPropagationSequence, propagation.MachineID).Scan(&sequence); err != nil {
			return fmt.Errorf("failed to assign propagation sequence for machine %d: %w", propagation.MachineID, err)
		}

		payload, err := encode(sequence)
		if err != nil {
			return fmt.Errorf("failed to encode propagation payload for machine %d sequence %d: %w", propagation.MachineID, sequence, err)
		}

		result, err := tx.ExecContext(ctx, queryCreatePropagation,
			propagation.MachineID,
			sequence,
			propagation.Kind,
			propagation.PolicyID,
			propagation.PolicyName,
			propagation.PolicyVersion,
			propagation.PolicyType,
			domain.PropagationPending,
			payload,
		)
		if err != nil {
			return fmt.Errorf("failed to create propagation for machine %d: %w", propagation.MachineID, err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to retrieve created propagation id: %w", err)
		}

		row := tx.QueryRowContext(ctx, queryGetPropagationByID, id)
		record, err := mapRowToPropagation(row.Scan)
		if err != nil {
			return fmt.Errorf("failed to retrieve created propagation: %w", err)
		}

		created = &record
		return nil
	})

	if err != nil {
		return nil, err
	}

	return created, nil
}

func (store *PropagationStore) MarkSent(ctx context.Context, machineID, sequence int64, at time.Time) error {
	if _, err := store.executor.Exec(ctx, queryMarkPropagationSent, at, machineID, sequence); err != nil {
		return fmt.Errorf("failed to mark propagation %d of machine %d as sent: %w", sequence, machineID, err)
	}

	return nil
}

func (store *PropagationStore) MarkFailed(ctx context.Context, machineID, sequence int64, reason string) error {
	if _, err := store.executor.Exec(ctx, queryMarkPropagationFailed, reason, machineID, sequence); err != nil {
		return fmt.Errorf("failed to mark propagation %d of machine %d as failed: %w", sequence, machineID, err)
	}

	return nil
}

func (store *PropagationStore) Acknowledge(ctx context.Context, machineID, sequence int64, status domain.PropagationStatus, reason string, at time.Time) error {
	if _, err := store.executor.Exec(ctx, queryAcknowledgePropagation, status, at, reason, machineID, sequence); err != nil {
		return fmt.Errorf("failed to acknowledge propagation %d of machine %d: %w", sequence, machineID, err)
	}

	return nil
}

func (store *PropagationStore) LatestByMachine(ctx context.Context, machineID int64) (*domain.Propagation, error) {
	return store.executor.QueryOne(ctx, queryLatestPropagationByMachine, machineID)
}

func (store *PropagationStore) ListLatest(ctx context.Context) ([]domain.Propagation, error) {
	return store.executor.QueryMany(ctx, queryListLatestPropagations)
}

func (store *PropagationStore) Epoch(ctx context.Context) (string, error) {
	var epoch string

	err := store.db.QueryRowContext(ctx, queryGetSystemMetadata, metadataEpochKey).Scan(&epoch)
	if err == nil {
		return epoch, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("failed to retrieve propagation epoch: %w", err)
	}

	generated, genErr := generateEpoch()
	if genErr != nil {
		return "", genErr
	}

	if _, err := store.db.ExecContext(ctx, queryInsertSystemMetadata, metadataEpochKey, generated); err != nil {
		return "", fmt.Errorf("failed to persist propagation epoch: %w", err)
	}

	// Re-select: a concurrent inserter may have won the INSERT OR IGNORE race,
	// and every caller must observe the same value.
	if err := store.db.QueryRowContext(ctx, queryGetSystemMetadata, metadataEpochKey).Scan(&epoch); err != nil {
		return "", fmt.Errorf("failed to retrieve propagation epoch: %w", err)
	}

	return epoch, nil
}

// generateEpoch returns 32 lowercase hex characters from 16 random bytes.
func generateEpoch() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate propagation epoch: %w", err)
	}

	return hex.EncodeToString(buf), nil
}

func mapRowToPropagation(scan query.ScanFunc) (domain.Propagation, error) {
	var propagation domain.Propagation
	if err := scan(
		&propagation.ID,
		&propagation.MachineID,
		&propagation.Sequence,
		&propagation.Kind,
		&propagation.PolicyID,
		&propagation.PolicyName,
		&propagation.PolicyVersion,
		&propagation.PolicyType,
		&propagation.Status,
		&propagation.SentAt,
		&propagation.AckedAt,
		&propagation.NackReason,
		&propagation.Payload,
		&propagation.CreatedAt); err != nil {

		return domain.Propagation{}, err
	}

	return propagation, nil
}

func mapRowToPropagationStatus(scan query.ScanFunc) (domain.Propagation, error) {
	var propagation domain.Propagation
	if err := scan(
		&propagation.ID,
		&propagation.MachineID,
		&propagation.Sequence,
		&propagation.Kind,
		&propagation.PolicyID,
		&propagation.PolicyName,
		&propagation.PolicyVersion,
		&propagation.PolicyType,
		&propagation.Status,
		&propagation.SentAt,
		&propagation.AckedAt,
		&propagation.NackReason,
		&propagation.CreatedAt); err != nil {

		return domain.Propagation{}, err
	}

	return propagation, nil
}
