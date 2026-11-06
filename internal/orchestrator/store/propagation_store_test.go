package store_test

import (
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/orchestrator/domain"
	"dougdomingos.com/aegis/internal/orchestrator/store"
)

func TestPropagationStore_Create_WithMultipleMachines_AssignsIndependentSequences(t *testing.T) {
	ctx, propagationStore := arrangeStoreTest(t, store.NewPropagationStore)

	first, err := propagationStore.Create(ctx, packagePropagation(101), encodePayload)
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if first.Sequence != 1 {
		t.Errorf("Create() sequence = %d, want 1", first.Sequence)
	}

	second, err := propagationStore.Create(ctx, packagePropagation(101), encodePayload)
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if second.Sequence != 2 {
		t.Errorf("Create() sequence = %d, want 2", second.Sequence)
	}

	other, err := propagationStore.Create(ctx, packagePropagation(102), encodePayload)
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if other.Sequence != 1 {
		t.Errorf("Create() sequence = %d, want 1", other.Sequence)
	}
}

func TestPropagationStore_Create_WithNewRow_StartsPendingAndStoresPayload(t *testing.T) {
	ctx, propagationStore := arrangeStoreTest(t, store.NewPropagationStore)

	created, err := propagationStore.Create(ctx, packagePropagation(101), encodePayload)
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}

	if created.Status != domain.PropagationPending {
		t.Errorf("Status = %q, want %q", created.Status, domain.PropagationPending)
	}
	if created.Payload != "payload-1" {
		t.Errorf("Payload = %q, want %q", created.Payload, "payload-1")
	}
	if created.SentAt != nil {
		t.Errorf("SentAt = %v, want nil", created.SentAt)
	}
	if created.AckedAt != nil {
		t.Errorf("AckedAt = %v, want nil", created.AckedAt)
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want the store-assigned timestamp")
	}
	if created.ID == 0 {
		t.Error("ID = 0, want the store-assigned identifier")
	}
}

func TestPropagationStore_Create_WhenEncoderFails_RollsBack(t *testing.T) {
	ctx, propagationStore := arrangeStoreTest(t, store.NewPropagationStore)
	encoderErr := errors.New("encoder failure")

	_, err := propagationStore.Create(ctx, packagePropagation(101), func(int64) (string, error) {
		return "", encoderErr
	})
	if err == nil {
		t.Fatal("Create() = nil, want the encoder error")
	}

	latest, err := propagationStore.LatestByMachine(ctx, 101)
	if err != nil {
		t.Fatalf("LatestByMachine() = %v, want nil", err)
	}
	if latest != nil {
		t.Errorf("LatestByMachine() = %+v, want no row after rollback", latest)
	}
}

func TestPropagationStore_MarkSent_UpdatesOnlyTargetRow(t *testing.T) {
	ctx, propagationStore := arrangeStoreTest(t, store.NewPropagationStore)

	if _, err := propagationStore.Create(ctx, packagePropagation(101), encodePayload); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if _, err := propagationStore.Create(ctx, packagePropagation(102), encodePayload); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}

	sentAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := propagationStore.MarkSent(ctx, 101, 1, sentAt); err != nil {
		t.Fatalf("MarkSent() = %v, want nil", err)
	}

	target, err := propagationStore.LatestByMachine(ctx, 101)
	if err != nil {
		t.Fatalf("LatestByMachine() = %v, want nil", err)
	}
	if target.Status != domain.PropagationSent {
		t.Errorf("Status = %q, want %q", target.Status, domain.PropagationSent)
	}
	if target.SentAt == nil || !target.SentAt.Equal(sentAt) {
		t.Errorf("SentAt = %v, want %v", target.SentAt, sentAt)
	}

	untouched, err := propagationStore.LatestByMachine(ctx, 102)
	if err != nil {
		t.Fatalf("LatestByMachine() = %v, want nil", err)
	}
	if untouched.Status != domain.PropagationPending {
		t.Errorf("Status = %q, want %q", untouched.Status, domain.PropagationPending)
	}
	if untouched.SentAt != nil {
		t.Errorf("SentAt = %v, want nil", untouched.SentAt)
	}
}

func TestPropagationStore_MarkFailed_SetsReasonAndLeavesAckedAtNil(t *testing.T) {
	ctx, propagationStore := arrangeStoreTest(t, store.NewPropagationStore)

	if _, err := propagationStore.Create(ctx, packagePropagation(101), encodePayload); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}

	sentAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := propagationStore.MarkSent(ctx, 101, 1, sentAt); err != nil {
		t.Fatalf("MarkSent() = %v, want nil", err)
	}
	if err := propagationStore.MarkFailed(ctx, 101, 1, "send timeout"); err != nil {
		t.Fatalf("MarkFailed() = %v, want nil", err)
	}

	latest, err := propagationStore.LatestByMachine(ctx, 101)
	if err != nil {
		t.Fatalf("LatestByMachine() = %v, want nil", err)
	}

	if latest.Status != domain.PropagationFailed {
		t.Errorf("Status = %q, want %q", latest.Status, domain.PropagationFailed)
	}
	if latest.NackReason != "send timeout" {
		t.Errorf("NackReason = %q, want %q", latest.NackReason, "send timeout")
	}
	if latest.AckedAt != nil {
		t.Errorf("AckedAt = %v, want nil", latest.AckedAt)
	}
	if latest.SentAt == nil || !latest.SentAt.Equal(sentAt) {
		t.Errorf("SentAt = %v, want %v", latest.SentAt, sentAt)
	}
}

func TestPropagationStore_Acknowledge_WithMatchingRow_SetsStatusAndTimestamps(t *testing.T) {
	testCases := map[string]struct {
		status     domain.PropagationStatus
		reason     string
		wantStatus domain.PropagationStatus
	}{
		"acked": {
			status:     domain.PropagationAcked,
			wantStatus: domain.PropagationAcked,
		},
		"nacked": {
			status:     domain.PropagationNacked,
			reason:     "stale sequence",
			wantStatus: domain.PropagationNacked,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx, propagationStore := arrangeStoreTest(t, store.NewPropagationStore)

			if _, err := propagationStore.Create(ctx, packagePropagation(101), encodePayload); err != nil {
				t.Fatalf("Create() = %v, want nil", err)
			}

			sentAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			if err := propagationStore.MarkSent(ctx, 101, 1, sentAt); err != nil {
				t.Fatalf("MarkSent() = %v, want nil", err)
			}

			ackedAt := time.Date(2026, 1, 1, 12, 0, 2, 500_000_000, time.UTC)
			if err := propagationStore.Acknowledge(ctx, 101, 1, tc.status, tc.reason, ackedAt); err != nil {
				t.Fatalf("Acknowledge() = %v, want nil", err)
			}

			latest, err := propagationStore.LatestByMachine(ctx, 101)
			if err != nil {
				t.Fatalf("LatestByMachine() = %v, want nil", err)
			}

			if latest.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", latest.Status, tc.wantStatus)
			}
			if latest.AckedAt == nil || !latest.AckedAt.Equal(ackedAt) {
				t.Errorf("AckedAt = %v, want %v", latest.AckedAt, ackedAt)
			}
			if latest.NackReason != tc.reason {
				t.Errorf("NackReason = %q, want %q", latest.NackReason, tc.reason)
			}
			if latest.SentAt == nil || !latest.SentAt.Equal(sentAt) {
				t.Errorf("SentAt = %v, want %v", latest.SentAt, sentAt)
			}
		})
	}
}

func TestPropagationStore_Acknowledge_WithMissingRow_ReturnsNil(t *testing.T) {
	ctx, propagationStore := arrangeStoreTest(t, store.NewPropagationStore)

	testCases := map[string]struct {
		machineID int64
		sequence  int64
	}{
		"unknown machine":     {machineID: 999, sequence: 1},
		"superseded sequence": {machineID: 101, sequence: 2},
		"unknown both":        {machineID: 888, sequence: 5},
	}

	if _, err := propagationStore.Create(ctx, packagePropagation(101), encodePayload); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := propagationStore.Acknowledge(ctx, tc.machineID, tc.sequence, domain.PropagationAcked, "", time.Now())
			if err != nil {
				t.Errorf("Acknowledge() = %v, want nil", err)
			}
		})
	}
}

func TestPropagationStore_LatestByMachine_ReturnsHighestSequence(t *testing.T) {
	ctx, propagationStore := arrangeStoreTest(t, store.NewPropagationStore)

	for expected := int64(1); expected <= 3; expected++ {
		if _, err := propagationStore.Create(ctx, packagePropagation(101), encodePayload); err != nil {
			t.Fatalf("Create() = %v, want nil", err)
		}
	}

	latest, err := propagationStore.LatestByMachine(ctx, 101)
	if err != nil {
		t.Fatalf("LatestByMachine() = %v, want nil", err)
	}

	if latest.Sequence != 3 {
		t.Errorf("Sequence = %d, want 3", latest.Sequence)
	}

	missing, err := propagationStore.LatestByMachine(ctx, 777)
	if err != nil {
		t.Fatalf("LatestByMachine() = %v, want nil", err)
	}
	if missing != nil {
		t.Errorf("LatestByMachine() = %+v, want nil for unknown machine", missing)
	}
}

func TestPropagationStore_ListLatest_ReturnsOnlyHighestSequencePerMachine(t *testing.T) {
	ctx, propagationStore := arrangeStoreTest(t, store.NewPropagationStore)

	seeds := map[int64]int{
		101: 2,
		102: 1,
		103: 3,
	}

	for machineID, count := range seeds {
		for i := 0; i < count; i++ {
			if _, err := propagationStore.Create(ctx, packagePropagation(machineID), encodePayload); err != nil {
				t.Fatalf("Create() = %v, want nil", err)
			}
		}
	}

	rows, err := propagationStore.ListLatest(ctx)
	if err != nil {
		t.Fatalf("ListLatest() = %v, want nil", err)
	}

	want := map[int64]int64{101: 2, 102: 1, 103: 3}
	if len(rows) != len(want) {
		t.Fatalf("ListLatest() = %d rows, want %d", len(rows), len(want))
	}

	for _, row := range rows {
		if row.Sequence != want[row.MachineID] {
			t.Errorf("machine %d sequence = %d, want %d", row.MachineID, row.Sequence, want[row.MachineID])
		}
		if row.Payload != "" {
			t.Errorf("machine %d payload = %q, want empty (status reads must not load payloads)", row.MachineID, row.Payload)
		}
	}
}

func TestPropagationStore_Epoch_ReturnsStableThirtyTwoHexCharacters(t *testing.T) {
	ctx, propagationStore := arrangeStoreTest(t, store.NewPropagationStore)

	epoch, err := propagationStore.Epoch(ctx)
	if err != nil {
		t.Fatalf("Epoch() = %v, want nil", err)
	}

	matched, _ := regexp.MatchString(`^[0-9a-f]{32}$`, epoch)
	if !matched {
		t.Errorf("Epoch() = %q, want 32 lowercase hex characters", epoch)
	}

	again, err := propagationStore.Epoch(ctx)
	if err != nil {
		t.Fatalf("Epoch() = %v, want nil", err)
	}
	if again != epoch {
		t.Errorf("Epoch() = %q, want stable %q", again, epoch)
	}
}

func TestPropagationStore_Epoch_AfterMetadataDeletion_ReturnsNewValue(t *testing.T) {
	ctx, db := arrangeStoreDB(t)
	propagationStore := store.NewPropagationStore(db)

	epoch, err := propagationStore.Epoch(ctx)
	if err != nil {
		t.Fatalf("Epoch() = %v, want nil", err)
	}

	if _, err := db.ExecContext(ctx, "DELETE FROM system_metadata"); err != nil {
		t.Fatalf("DELETE FROM system_metadata = %v, want nil", err)
	}

	regenerated, err := propagationStore.Epoch(ctx)
	if err != nil {
		t.Fatalf("Epoch() = %v, want nil", err)
	}
	if regenerated == epoch {
		t.Errorf("Epoch() = %q, want a new value after metadata deletion", regenerated)
	}

	matched, _ := regexp.MatchString(`^[0-9a-f]{32}$`, regenerated)
	if !matched {
		t.Errorf("Epoch() = %q, want 32 lowercase hex characters", regenerated)
	}
}

// Helpers for propagation store scenarios (arrangeStoreDB and arrangeStoreTest
// are declared in helpers_test.go).

func packagePropagation(machineID int64) domain.Propagation {
	policyID := int64(3)
	return domain.Propagation{
		MachineID:     machineID,
		Kind:          domain.PropagationPackage,
		PolicyID:      &policyID,
		PolicyName:    "lab-whitelist",
		PolicyVersion: 1,
		PolicyType:    "WHITELIST",
	}
}

func encodePayload(sequence int64) (string, error) {
	return fmt.Sprintf("payload-%d", sequence), nil
}
