package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"dougdomingos.com/aegis/internal/common/auth"
	machineErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/schemas"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
	"dougdomingos.com/aegis/internal/orchestrator/store"
)

// MachineService provides all operations needed to manage machine
// authentication in the system.
type MachineService struct {

	// store provides database operations related to machines.
	store domain.MachineStore

	// propagations provides the latest propagation state of each machine for
	// the status views.
	propagations domain.PropagationStore

	// labKey is the shared HMAC key used to verify machine authentication
	// challenges.
	labKey []byte

	// sessionSecret is the HMAC signing secret used for session tokens.
	sessionSecret string

	// sessionTTL is the lifetime of an issued session token.
	sessionTTL time.Duration

	// stalenessGrace is the duration a machine remains displayed as CONNECTED
	// after its last heartbeat.
	stalenessGrace time.Duration

	// nonces tracks the pending authentication challenges per machine IP.
	nonces *store.NonceStore

	// now is the clock used to resolve the current time.
	now func() time.Time
}

// NewMachineService creates a new service instance with the provided store
// providers and authentication secrets.
func NewMachineService(machineStore domain.MachineStore, propagationStore domain.PropagationStore, labKey []byte, sessionSecret string, sessionTTL time.Duration, stalenessGrace time.Duration, now func() time.Time) *MachineService {
	return &MachineService{
		store:          machineStore,
		propagations:   propagationStore,
		labKey:         labKey,
		sessionSecret:  sessionSecret,
		sessionTTL:     sessionTTL,
		stalenessGrace: stalenessGrace,
		nonces:         store.NewNonceStore(now),
		now:            now,
	}
}

// SyncMachines reconciles the stored machines with the provided accepted
// sources, upserting each configured machine and pruning any whose IP is no
// longer configured.
func (service *MachineService) SyncMachines(ctx context.Context, machines []domain.Machine) error {
	return service.store.Sync(ctx, machines)
}

// ListMachines returns all registered machines within the database.
func (service *MachineService) ListMachines(ctx context.Context) ([]schemas.MachineOutputSchema, error) {
	machines, err := service.store.List(ctx)
	if err != nil {
		return nil, err
	}

	propagations, err := service.propagations.ListLatest(ctx)
	if err != nil {
		return nil, err
	}

	latestByMachine := make(map[int64]domain.Propagation, len(propagations))
	for _, propagation := range propagations {
		latestByMachine[propagation.MachineID] = propagation
	}

	output := make([]schemas.MachineOutputSchema, 0, len(machines))
	for i := range machines {
		schema := mapMachineToOutputSchema(&machines[i], service.now(), service.stalenessGrace)

		if propagation, exists := latestByMachine[machines[i].ID]; exists {
			schema.Propagation = mapPropagationToOutputSchema(&propagation)
		} else {
			schema.Propagation = pendingPropagationOutput()
		}

		output = append(output, *schema)
	}

	return output, nil
}

// GetPropagationStatus returns the latest propagation state of the machine. A machine
// that never received a message reports the PENDING representation instead of
// an error; an unknown machine returns ErrMachineNotFound.
func (service *MachineService) GetPropagationStatus(ctx context.Context, machineID int64) (*schemas.PropagationOutputSchema, error) {
	machine, err := service.store.GetByID(ctx, machineID)
	if err != nil {
		return nil, err
	}

	if machine == nil {
		return nil, machineErrors.ErrMachineNotFound
	}

	propagation, err := service.propagations.LatestByMachine(ctx, machineID)
	if err != nil {
		return nil, err
	}

	if propagation == nil {
		pending := pendingPropagationOutput()
		return &pending, nil
	}

	mapped := mapPropagationToOutputSchema(propagation)
	return &mapped, nil
}

// InitiateAuthentication starts an authentication challenge for the provided
// IP, returning a single-use nonce to be signed by the machine.
func (service *MachineService) InitiateAuthentication(ctx context.Context, payload schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error) {
	if strings.TrimSpace(payload.IP) == "" {
		return nil, machineErrors.ErrMachineIPRequired
	}

	nonce, expiresAt := service.nonces.Issue(payload.IP)

	return &schemas.InitiateOutputSchema{
		Nonce:     nonce,
		ExpiresAt: expiresAt,
	}, nil
}

// Authenticate validates that the provided signature authenticates the pending
// challenge for the configured source, marks the machine as connected, and
// issues a session token.
func (service *MachineService) Authenticate(ctx context.Context, payload schemas.AuthenticateMachineSchema) (*schemas.AuthenticateOutputSchema, error) {
	if strings.TrimSpace(payload.IP) == "" {
		return nil, machineErrors.ErrMachineIPRequired
	}

	nonce, ok := service.nonces.Lookup(payload.IP)
	if !ok {
		return nil, machineErrors.ErrInvalidCredentials
	}

	signature, err := hex.DecodeString(payload.Signature)
	if err != nil {
		return nil, machineErrors.ErrInvalidCredentials
	}

	if !auth.Verify(service.labKey, auth.CanonicalPayload(payload.IP, nonce), signature) {
		return nil, machineErrors.ErrInvalidCredentials
	}

	machine, err := service.store.GetByIP(ctx, payload.IP)
	if err != nil {
		return nil, err
	}

	if machine == nil {
		return nil, machineErrors.ErrInvalidCredentials
	}

	service.nonces.Consume(payload.IP)

	mac := payload.MAC
	if mac == "" {
		mac = machine.MAC
	}

	connected, err := service.store.Touch(ctx, machine.ID, mac, domain.ConnectedStatus)
	if err != nil {
		return nil, err
	}

	sessionToken, expiresAt, err := auth.SignSessionToken(connected.ID, service.sessionSecret, service.sessionTTL, service.now())
	if err != nil {
		return nil, err
	}

	return &schemas.AuthenticateOutputSchema{
		SessionToken: sessionToken,
		ExpiresAt:    expiresAt,
		Machine:      *mapMachineToOutputSchema(connected, service.now(), service.stalenessGrace),
	}, nil
}

// Heartbeat validates the provided session token and refreshes the
// connectivity state and last-seen timestamp of the machine it identifies.
func (service *MachineService) Heartbeat(ctx context.Context, token string) error {
	machineID, err := auth.ParseSessionToken(token, service.sessionSecret, service.now())
	if err != nil {
		return err
	}

	_, err = service.store.Touch(ctx, machineID, "", domain.ConnectedStatus)
	if errors.Is(err, machineErrors.ErrMachineNotFound) {
		return fmt.Errorf("%w: the machine is no longer registered", machineErrors.ErrSessionTokenInvalid)
	}

	return err
}

// mapMachineToOutputSchema converts the Machine entity format into the output
// schema provided by this service, deriving the displayed status from the
// freshness of the machine's last-seen timestamp. Returns nil if the provided
// machine is nil.
func mapMachineToOutputSchema(machine *domain.Machine, now time.Time, grace time.Duration) *schemas.MachineOutputSchema {
	if machine == nil {
		return nil
	}

	status := domain.DisconnectedStatus
	if machine.LastSeenAt != nil && now.Sub(*machine.LastSeenAt) <= grace {
		status = domain.ConnectedStatus
	}

	return &schemas.MachineOutputSchema{
		ID:         machine.ID,
		Name:       machine.Name,
		IP:         machine.IP,
		MAC:        machine.MAC,
		GroupID:    machine.GroupID,
		GroupName:  machine.GroupName,
		Status:     string(status),
		LastSeenAt: machine.LastSeenAt,
		CreatedAt:  machine.CreatedAt,
	}
}

// mapPropagationToOutputSchema converts the Propagation entity format into the
// output schema provided by this service, deriving the acknowledgement duration
// when both delivery timestamps are present.
func mapPropagationToOutputSchema(propagation *domain.Propagation) schemas.PropagationOutputSchema {
	output := schemas.PropagationOutputSchema{
		Sequence:      propagation.Sequence,
		Kind:          string(propagation.Kind),
		Status:        string(propagation.Status),
		PolicyID:      propagation.PolicyID,
		PolicyName:    propagation.PolicyName,
		PolicyVersion: propagation.PolicyVersion,
		PolicyType:    propagation.PolicyType,
		SentAt:        propagation.SentAt,
		AckedAt:       propagation.AckedAt,
		NackReason:    propagation.NackReason,
	}

	if propagation.SentAt != nil && propagation.AckedAt != nil {
		duration := propagation.AckedAt.Sub(*propagation.SentAt).Milliseconds()
		output.DurationMS = &duration
	}

	return output
}

// pendingPropagationOutput returns the representation of a machine that never
// received a propagation.
func pendingPropagationOutput() schemas.PropagationOutputSchema {
	return schemas.PropagationOutputSchema{
		Sequence: 0,
		Kind:     "",
		Status:   string(domain.PropagationPending),
		PolicyID: nil,
	}
}
