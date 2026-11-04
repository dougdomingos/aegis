package service_test

import (
	"context"
	"sync"
	"time"

	"dougdomingos.com/aegis/internal/orchestrator/domain"
)

// MockPropagationStore provides a thread-safe in-memory implementation of
// domain.PropagationStore, so the propagation worker's audit transitions are
// observable from tests running concurrently with it.
//
// Injectable error fields abort the respective call: EpochErr for Epoch,
// CreateErr for Create, MarkSentErr for MarkSent, LatestByMachineErr for
// LatestByMachine, and ListLatestErr for ListLatest.
type MockPropagationStore struct {
	mu sync.Mutex

	EpochValue         string
	EpochErr           error
	CreateErr          error
	MarkSentErr        error
	LatestByMachineErr error
	ListLatestErr      error

	Rows        map[int64][]domain.Propagation
	CreateCalls int
}

// NewMockPropagationStore initializes a new MockPropagationStore instance with
// a stable test epoch.
func NewMockPropagationStore() *MockPropagationStore {
	return &MockPropagationStore{
		EpochValue: "abcdef0123456789abcdef0123456789",
		Rows:       make(map[int64][]domain.Propagation),
	}
}

// Create assigns the next per-machine sequence, encodes the payload, and
// stores the row as PENDING.
func (mock *MockPropagationStore) Create(ctx context.Context, propagation domain.Propagation, encode func(sequence int64) (string, error)) (*domain.Propagation, error) {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	if mock.CreateErr != nil {
		return nil, mock.CreateErr
	}

	mock.CreateCalls++
	sequence := int64(len(mock.Rows[propagation.MachineID]) + 1)

	payload, err := encode(sequence)
	if err != nil {
		return nil, err
	}

	created := propagation
	created.Sequence = sequence
	created.Status = domain.PropagationPending
	created.Payload = payload
	created.CreatedAt = time.Now()
	mock.Rows[propagation.MachineID] = append(mock.Rows[propagation.MachineID], created)

	return &created, nil
}

// MarkSent transitions the matching row to SENT.
func (mock *MockPropagationStore) MarkSent(ctx context.Context, machineID, sequence int64, at time.Time) error {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	if mock.MarkSentErr != nil {
		return mock.MarkSentErr
	}

	index, exists := mock.rowIndexLocked(machineID, sequence)
	if !exists {
		return nil
	}

	mock.Rows[machineID][index].Status = domain.PropagationSent
	mock.Rows[machineID][index].SentAt = &at

	return nil
}

// MarkFailed transitions the matching row to FAILED with the given reason.
func (mock *MockPropagationStore) MarkFailed(ctx context.Context, machineID, sequence int64, reason string) error {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	index, exists := mock.rowIndexLocked(machineID, sequence)
	if !exists {
		return nil
	}

	mock.Rows[machineID][index].Status = domain.PropagationFailed
	mock.Rows[machineID][index].NackReason = reason

	return nil
}

// Acknowledge applies the terminal status to the matching row; a missing row
// is silently ignored.
func (mock *MockPropagationStore) Acknowledge(ctx context.Context, machineID, sequence int64, status domain.PropagationStatus, reason string, at time.Time) error {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	index, exists := mock.rowIndexLocked(machineID, sequence)
	if !exists {
		return nil
	}

	mock.Rows[machineID][index].Status = status
	mock.Rows[machineID][index].AckedAt = &at
	mock.Rows[machineID][index].NackReason = reason

	return nil
}

// LatestByMachine returns the row with the highest sequence of the machine.
func (mock *MockPropagationStore) LatestByMachine(ctx context.Context, machineID int64) (*domain.Propagation, error) {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	if mock.LatestByMachineErr != nil {
		return nil, mock.LatestByMachineErr
	}

	rows := mock.Rows[machineID]
	if len(rows) == 0 {
		return nil, nil
	}

	latest := rows[len(rows)-1]
	return &latest, nil
}

// ListLatest returns the highest row of every machine that has one.
func (mock *MockPropagationStore) ListLatest(ctx context.Context) ([]domain.Propagation, error) {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	if mock.ListLatestErr != nil {
		return nil, mock.ListLatestErr
	}

	list := make([]domain.Propagation, 0, len(mock.Rows))
	for _, rows := range mock.Rows {
		if len(rows) == 0 {
			continue
		}
		list = append(list, rows[len(rows)-1])
	}

	return list, nil
}

// Epoch returns the configured epoch value or error.
func (mock *MockPropagationStore) Epoch(ctx context.Context) (string, error) {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	if mock.EpochErr != nil {
		return "", mock.EpochErr
	}

	return mock.EpochValue, nil
}

// RowAt returns a copy of the row of the given machine and sequence.
func (mock *MockPropagationStore) RowAt(machineID, sequence int64) (domain.Propagation, bool) {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	index, exists := mock.rowIndexLocked(machineID, sequence)
	if !exists {
		return domain.Propagation{}, false
	}

	return mock.Rows[machineID][index], true
}

// CreateCallCount returns how many times Create was invoked.
func (mock *MockPropagationStore) CreateCallCount() int {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	return mock.CreateCalls
}

func (mock *MockPropagationStore) rowIndexLocked(machineID, sequence int64) (int, bool) {
	for i, row := range mock.Rows[machineID] {
		if row.Sequence == sequence {
			return i, true
		}
	}

	return 0, false
}
