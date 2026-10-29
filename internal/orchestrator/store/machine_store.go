package store

import (
	"context"
	"database/sql"
	"fmt"

	"dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
	"dougdomingos.com/aegis/internal/orchestrator/infra/query"
)

const (
	queryUpsertMachine = `INSERT INTO machines (name, ip, status) VALUES (?, ?, 'DISCONNECTED')
		ON CONFLICT(ip) DO UPDATE SET name = excluded.name`
	queryGetMachineByIP = `SELECT m.id, m.name, m.ip, m.mac, m.status, m.last_seen_at, m.created_at, m.group_id, g.name
		FROM machines m LEFT JOIN groups g ON g.id = m.group_id WHERE m.ip = ?`
	queryGetMachineByID = `SELECT m.id, m.name, m.ip, m.mac, m.status, m.last_seen_at, m.created_at, m.group_id, g.name
		FROM machines m LEFT JOIN groups g ON g.id = m.group_id WHERE m.id = ?`
	queryGetAllMachines = `SELECT m.id, m.name, m.ip, m.mac, m.status, m.last_seen_at, m.created_at, m.group_id, g.name
		FROM machines m LEFT JOIN groups g ON g.id = m.group_id`
	queryListMachinesByGroup = `SELECT m.id, m.name, m.ip, m.mac, m.status, m.last_seen_at, m.created_at, m.group_id, g.name
		FROM machines m LEFT JOIN groups g ON g.id = m.group_id WHERE m.group_id = ?`
	queryDeleteMachineByIP      = "DELETE FROM machines WHERE ip = ?"
	queryLinkMachineToGroup     = "UPDATE machines SET group_id = ? WHERE id = ?"
	queryUnlinkMachineFromGroup = "UPDATE machines SET group_id = NULL WHERE id = ?"
	queryTouchMachine           = "UPDATE machines SET mac = CASE WHEN ? = '' THEN mac ELSE ? END, status = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?"
)

// MachineStore manages all database-related operations over machines.
type MachineStore struct {
	executor *query.QueryExecutor[domain.Machine]
}

// NewMachineStore creates a new MachineStore instance.
func NewMachineStore(db *sql.DB) *MachineStore {
	return &MachineStore{
		executor: query.NewQueryExecutor(db, mapRowToMachine),
	}
}

// GetByIP retrieves the machine whose IP matches the provided argument.
func (store *MachineStore) GetByIP(ctx context.Context, ip string) (*domain.Machine, error) {
	return store.executor.QueryOne(ctx, queryGetMachineByIP, ip)
}

func (store *MachineStore) GetByID(ctx context.Context, id int64) (*domain.Machine, error) {
	return store.executor.QueryOne(ctx, queryGetMachineByID, id)
}

func (store *MachineStore) LinkToGroup(ctx context.Context, machineID int64, groupID int64) (*domain.Machine, error) {
	result, err := store.executor.Exec(ctx, queryLinkMachineToGroup, groupID, machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to link machine %d to group %d: %w", machineID, groupID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return nil, fmt.Errorf("%w: id %d", errors.ErrMachineNotFound, machineID)
	}

	return store.GetByID(ctx, machineID)
}

func (store *MachineStore) UnlinkFromGroup(ctx context.Context, machineID int64) (*domain.Machine, error) {
	result, err := store.executor.Exec(ctx, queryUnlinkMachineFromGroup, machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to unlink machine %d: %w", machineID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return nil, fmt.Errorf("%w: id %d", errors.ErrMachineNotFound, machineID)
	}

	return store.GetByID(ctx, machineID)
}

func (store *MachineStore) ListByGroup(ctx context.Context, groupID int64) ([]domain.Machine, error) {
	return store.executor.QueryMany(ctx, queryListMachinesByGroup, groupID)
}

// List retrieves all registered machines within the database.
func (store *MachineStore) List(ctx context.Context) ([]domain.Machine, error) {
	return store.executor.QueryMany(ctx, queryGetAllMachines)
}

// Upsert registers a machine or updates its name when its IP already exists,
// preserving the current connection state.
func (store *MachineStore) Upsert(ctx context.Context, machine domain.Machine) (*domain.Machine, error) {
	if _, err := store.executor.Exec(ctx, queryUpsertMachine, machine.Name, machine.IP); err != nil {
		return nil, fmt.Errorf("failed to upsert machine %q: %w", machine.IP, err)
	}

	return store.GetByIP(ctx, machine.IP)
}

// RemoveByIP removes the machine whose IP matches the provided argument.
func (store *MachineStore) RemoveByIP(ctx context.Context, ip string) error {
	if _, err := store.executor.Exec(ctx, queryDeleteMachineByIP, ip); err != nil {
		return fmt.Errorf("failed to remove machine %q: %w", ip, err)
	}

	return nil
}

// Sync reconciles the stored machines with the provided accepted sources,
// upserting each configured machine and pruning any whose IP is no longer
// configured, atomically.
func (store *MachineStore) Sync(ctx context.Context, machines []domain.Machine) error {
	configured := make(map[string]bool, len(machines))

	return store.executor.WithTx(ctx, func(tx *sql.Tx) error {
		for _, machine := range machines {
			if _, err := tx.ExecContext(ctx, queryUpsertMachine, machine.Name, machine.IP); err != nil {
				return fmt.Errorf("failed to upsert machine %q: %w", machine.IP, err)
			}

			configured[machine.IP] = true
		}

		existent, err := listMachines(ctx, tx)
		if err != nil {
			return fmt.Errorf("failed to list machines for pruning: %w", err)
		}

		for _, machine := range existent {
			if !configured[machine.IP] {
				if _, err := tx.ExecContext(ctx, queryDeleteMachineByIP, machine.IP); err != nil {
					return fmt.Errorf("failed to prune machine %q: %w", machine.IP, err)
				}
			}
		}

		return nil
	})
}

// Touch updates the connection state and last-seen timestamp of the machine
// whose ID matches the provided argument.
func (store *MachineStore) Touch(ctx context.Context, id int64, mac string, status domain.MachineStatus) (*domain.Machine, error) {
	result, err := store.executor.Exec(ctx, queryTouchMachine, mac, mac, status, id)
	if err != nil {
		return nil, fmt.Errorf("failed to touch machine %d: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return nil, fmt.Errorf("%w: id %d", errors.ErrMachineNotFound, id)
	}

	return store.executor.QueryOne(ctx, queryGetMachineByID, id)
}

// listMachines retrieves all registered machines within the database using the
// provided transaction.
func listMachines(ctx context.Context, tx *sql.Tx) ([]domain.Machine, error) {
	rows, err := tx.QueryContext(ctx, queryGetAllMachines)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var machines []domain.Machine

	for rows.Next() {
		machine, err := mapRowToMachine(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to map row: %w", err)
		}

		machines = append(machines, machine)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return machines, nil
}

// mapRowToMachine maps a scan function to a domain.Machine struct.
func mapRowToMachine(scan query.ScanFunc) (domain.Machine, error) {
	var machine domain.Machine
	var groupID sql.NullInt64
	var groupName sql.NullString

	if err := scan(&machine.ID, &machine.Name, &machine.IP, &machine.MAC, &machine.Status, &machine.LastSeenAt, &machine.CreatedAt, &groupID, &groupName); err != nil {
		return domain.Machine{}, err
	}

	if groupID.Valid {
		id := groupID.Int64
		machine.GroupID = &id
	}

	machine.GroupName = groupName.String

	return machine, nil
}
