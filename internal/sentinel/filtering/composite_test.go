package filtering_test

import (
	"errors"
	"reflect"
	"testing"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/filtering"
)

func TestComposite_Apply_WithSuccessfulBackends_AppliesPacketsThenResolver(t *testing.T) {
	// Arrange
	calls := []string{}
	filter := filtering.NewComposite(&packetMock{calls: &calls}, &resolverMock{calls: &calls})

	// Act
	err := filter.Apply(validSnapshot())

	// Assert
	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}
	if want := []string{"packets.Apply", "resolver.Update"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}

func TestComposite_Apply_WithInvalidSnapshot_TouchesNeitherBackend(t *testing.T) {
	// Arrange
	calls := []string{}
	filter := filtering.NewComposite(&packetMock{calls: &calls}, &resolverMock{calls: &calls})
	snapshot := validSnapshot()
	snapshot.PolicyType = "UNKNOWN"

	// Act
	err := filter.Apply(snapshot)

	// Assert
	if err == nil {
		t.Fatalf("Apply() error = nil, want validation error")
	}
	if len(calls) != 0 {
		t.Errorf("calls = %v, want none so no backend is left half-applied", calls)
	}
}

func TestComposite_Apply_WithPacketFailure_NeverTouchesResolver(t *testing.T) {
	// Arrange
	calls := []string{}
	packetErr := errors.New("nft failed")
	filter := filtering.NewComposite(&packetMock{calls: &calls, applyErr: packetErr}, &resolverMock{calls: &calls})

	// Act
	err := filter.Apply(validSnapshot())

	// Assert
	if !errors.Is(err, packetErr) {
		t.Fatalf("Apply() error = %v, want %v", err, packetErr)
	}
	if want := []string{"packets.Apply"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}

func TestComposite_Clear_WithSuccessfulBackends_ClearsBoth(t *testing.T) {
	// Arrange
	calls := []string{}
	filter := filtering.NewComposite(&packetMock{calls: &calls}, &resolverMock{calls: &calls})

	// Act
	err := filter.Clear()

	// Assert
	if err != nil {
		t.Fatalf("Clear() error = %v, want nil", err)
	}
	if want := []string{"packets.Clear", "resolver.Clear"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}

func TestComposite_Clear_WithPacketFailure_StillClearsResolver(t *testing.T) {
	// Arrange
	calls := []string{}
	packetErr := errors.New("nft failed")
	filter := filtering.NewComposite(&packetMock{calls: &calls, clearErr: packetErr}, &resolverMock{calls: &calls})

	// Act
	err := filter.Clear()

	// Assert
	if !errors.Is(err, packetErr) {
		t.Fatalf("Clear() error = %v, want %v", err, packetErr)
	}
	if want := []string{"packets.Clear", "resolver.Clear"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}

// packetMock records PacketFilter calls.
type packetMock struct {
	calls    *[]string
	applyErr error
	clearErr error
}

// Apply records the call and returns the configured error.
func (m *packetMock) Apply(filtering.PolicySnapshot) error {
	*m.calls = append(*m.calls, "packets.Apply")
	return m.applyErr
}

// Clear records the call and returns the configured error.
func (m *packetMock) Clear() error {
	*m.calls = append(*m.calls, "packets.Clear")
	return m.clearErr
}

// resolverMock records ResolverFilter calls.
type resolverMock struct {
	calls *[]string
}

// Update records the call.
func (m *resolverMock) Update(filtering.PolicySnapshot) {
	*m.calls = append(*m.calls, "resolver.Update")
}

// Clear records the call.
func (m *resolverMock) Clear() {
	*m.calls = append(*m.calls, "resolver.Clear")
}

// validSnapshot returns a well-formed WHITELIST snapshot.
func validSnapshot() filtering.PolicySnapshot {
	return filtering.PolicySnapshot{PolicyType: propagation.PolicyTypeWhitelist}
}
