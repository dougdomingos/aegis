package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"dougdomingos.com/aegis/internal/auth"
	"dougdomingos.com/aegis/internal/domain"
	"dougdomingos.com/aegis/internal/errors"
	"dougdomingos.com/aegis/internal/schemas"
	"dougdomingos.com/aegis/internal/store"
)

// MachineService provides all operations needed to manage machine
// authentication in the system.
type MachineService struct {

	// store provides database operations related to machines.
	store domain.MachineStore

	// labKey is the shared HMAC key used to verify machine authentication
	// challenges.
	labKey []byte

	// sessionSecret is the HMAC signing secret used for session tokens.
	sessionSecret string

	// sessionTTL is the lifetime of an issued session token.
	sessionTTL time.Duration

	// nonces tracks the pending authentication challenges per machine IP.
	nonces *store.NonceStore

	// now is the clock used to resolve the current time.
	now func() time.Time
}

// NewMachineService creates a new service instance with the provided store
// provider and authentication secrets.
func NewMachineService(machineStore domain.MachineStore, labKey []byte, sessionSecret string, sessionTTL time.Duration, now func() time.Time) *MachineService {
	return &MachineService{
		store:         machineStore,
		labKey:        labKey,
		sessionSecret: sessionSecret,
		sessionTTL:    sessionTTL,
		nonces:        store.NewNonceStore(now),
		now:           now,
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

	output := make([]schemas.MachineOutputSchema, 0, len(machines))
	for i := range machines {
		output = append(output, *mapMachineToOutputSchema(&machines[i]))
	}

	return output, nil
}

// InitiateAuthentication starts an authentication challenge for the provided
// IP, returning a single-use nonce to be signed by the machine.
func (service *MachineService) InitiateAuthentication(ctx context.Context, payload schemas.InitiateAuthenticationSchema) (*schemas.InitiateOutputSchema, error) {
	if strings.TrimSpace(payload.IP) == "" {
		return nil, errors.ErrMachineIPRequired
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
		return nil, errors.ErrMachineIPRequired
	}

	nonce, ok := service.nonces.Lookup(payload.IP)
	if !ok {
		return nil, errors.ErrInvalidCredentials
	}

	signature, err := hex.DecodeString(payload.Signature)
	if err != nil {
		return nil, errors.ErrInvalidCredentials
	}

	if !auth.Verify(service.labKey, auth.CanonicalPayload(payload.IP, nonce), signature) {
		return nil, errors.ErrInvalidCredentials
	}

	machine, err := service.store.GetByIP(ctx, payload.IP)
	if err != nil {
		return nil, err
	}

	if machine == nil {
		return nil, errors.ErrInvalidCredentials
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

	sessionToken, expiresAt, err := issueSessionToken(connected.ID, service.sessionSecret, service.sessionTTL, service.now())
	if err != nil {
		return nil, err
	}

	return &schemas.AuthenticateOutputSchema{
		SessionToken: sessionToken,
		ExpiresAt:    expiresAt,
		Machine:      *mapMachineToOutputSchema(connected),
	}, nil
}

// issueSessionToken builds an HMAC-SHA256-signed session token encoding the
// machine ID and its expiry, and returns the token along with the expiry
// timestamp.
func issueSessionToken(machineID int64, secret string, ttl time.Duration, now time.Time) (string, time.Time, error) {
	expiresAt := now.Add(ttl)
	payload := fmt.Sprintf("%d:%d", machineID, expiresAt.Unix())

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))

	token := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return token, expiresAt, nil
}

// mapMachineToOutputSchema converts the Machine entity format into the output
// schema provided by this service. Returns nil if the provided machine is
// nil.
func mapMachineToOutputSchema(machine *domain.Machine) *schemas.MachineOutputSchema {
	if machine == nil {
		return nil
	}

	return &schemas.MachineOutputSchema{
		ID:         machine.ID,
		Name:       machine.Name,
		IP:         machine.IP,
		MAC:        machine.MAC,
		Status:     string(machine.Status),
		LastSeenAt: machine.LastSeenAt,
		CreatedAt:  machine.CreatedAt,
	}
}
