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
	queryDeleteMachineByID      = "DELETE FROM machines WHERE id = ?"
	queryLinkMachineToGroup     = "UPDATE machines SET group_id = ? WHERE id = ?"
	queryUnlinkMachineFromGroup = "UPDATE machines SET group_id = NULL WHERE id = ?"
	queryTouchMachine           = "UPDATE machines SET mac = CASE WHEN ? = '' THEN mac ELSE ? END, status = ?, last_seen_at = CURRENT_TIMESTAMP WHERE id = ?"
	queryCountMachinesByIP      = "SELECT COUNT(*) FROM machines WHERE ip = ?"
	queryGetMachineLinkByIP     = "SELECT id, group_id FROM machines WHERE ip = ?"
	queryInsertGroupIfAbsent    = "INSERT OR IGNORE INTO groups (name) VALUES (?)"
	queryGetGroupIDByName       = "SELECT id FROM groups WHERE name = ?"
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

func (store *MachineStore) RemoveByID(ctx context.Context, id int64) error {
	result, err := store.executor.Exec(ctx, queryDeleteMachineByID, id)
	if err != nil {
		return fmt.Errorf("failed to remove machine %d: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%w: id %d", errors.ErrMachineNotFound, id)
	}

	return nil
}

// Import atomically registers or renames each provided machine, creates a
// group for every unknown lab, and links each machine with a non-empty lab to
// its group. Machines absent from the entries are kept.
func (store *MachineStore) Import(ctx context.Context, entries []domain.MachineImport) (*domain.MachineImportResult, error) {
	result := &domain.MachineImportResult{}

	err := store.executor.WithTx(ctx, func(tx *sql.Tx) error {
		groupIDs, err := ensureImportGroups(ctx, tx, entries, result)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			if err := importMachine(ctx, tx, entry, groupIDs, result); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
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

// ensureImportGroups resolves the ID of every unique non-empty lab of the
// entries, creating the groups that do not exist yet, using the provided
// transaction.
func ensureImportGroups(ctx context.Context, tx *sql.Tx, entries []domain.MachineImport, result *domain.MachineImportResult) (map[string]int64, error) {
	groupIDs := make(map[string]int64)

	for _, entry := range entries {
		if entry.Lab == "" {
			continue
		}

		if _, resolved := groupIDs[entry.Lab]; resolved {
			continue
		}

		inserted, err := tx.ExecContext(ctx, queryInsertGroupIfAbsent, entry.Lab)
		if err != nil {
			return nil, fmt.Errorf("failed to create group %q: %w", entry.Lab, err)
		}

		rowsAffected, err := inserted.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("failed to check rows affected: %w", err)
		}

		result.GroupsCreated += int(rowsAffected)

		var groupID int64
		if err := tx.QueryRowContext(ctx, queryGetGroupIDByName, entry.Lab).Scan(&groupID); err != nil {
			return nil, fmt.Errorf("failed to resolve group %q: %w", entry.Lab, err)
		}

		groupIDs[entry.Lab] = groupID
	}

	return groupIDs, nil
}

// importMachine registers or renames the machine of the entry and links it to
// the group of its lab when the lab is not empty, using the provided
// transaction.
func importMachine(ctx context.Context, tx *sql.Tx, entry domain.MachineImport, groupIDs map[string]int64, result *domain.MachineImportResult) error {
	machine := entry.Machine

	var existent int
	if err := tx.QueryRowContext(ctx, queryCountMachinesByIP, machine.IP).Scan(&existent); err != nil {
		return fmt.Errorf("failed to check machine %q: %w", machine.IP, err)
	}

	if _, err := tx.ExecContext(ctx, queryUpsertMachine, machine.Name, machine.IP); err != nil {
		return fmt.Errorf("failed to upsert machine %q: %w", machine.IP, err)
	}

	if existent == 0 {
		result.MachinesCreated++
	} else {
		result.MachinesUpdated++
	}

	if entry.Lab == "" {
		return nil
	}

	var machineID int64
	var currentGroupID sql.NullInt64
	if err := tx.QueryRowContext(ctx, queryGetMachineLinkByIP, machine.IP).Scan(&machineID, &currentGroupID); err != nil {
		return fmt.Errorf("failed to resolve machine %q: %w", machine.IP, err)
	}

	groupID := groupIDs[entry.Lab]
	if currentGroupID.Valid && currentGroupID.Int64 == groupID {
		return nil
	}

	if _, err := tx.ExecContext(ctx, queryLinkMachineToGroup, groupID, machineID); err != nil {
		return fmt.Errorf("failed to link machine %q to group %q: %w", machine.IP, entry.Lab, err)
	}

	result.RelinkedIDs = append(result.RelinkedIDs, machineID)

	return nil
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
