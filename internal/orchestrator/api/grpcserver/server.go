// Package grpcserver exposes the Orchestrator's authenticated gRPC
// propagation stream, where Sentinels receive rule packages and return
// acknowledgements.
package grpcserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	"dougdomingos.com/aegis/internal/common/auth"
	machineErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/pkg/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// propagationController declares the propagation operations the gRPC stream
// consumes, owned here so the adapter never depends on the concrete service.
type propagationController interface {
	Register(ctx context.Context, machineID int64, sender propagation.MessageSender) (uint64, error)
	Unregister(machineID int64, connID uint64)
	DeliverSnapshot(ctx context.Context, machineID int64)
	Acknowledge(ctx context.Context, machineID int64, ack propagation.Ack)
}

// PropagationServer implements the pb.PropagationServer contract,
// authenticating each stream with a session token before registering its
// sender with the propagation controller.
type PropagationServer struct {
	pb.UnimplementedPropagationServer
	service       propagationController
	sessionSecret string
	logger        *slog.Logger
}

// NewPropagationServer creates a new PropagationServer instance. A nil logger
// discards every record.
func NewPropagationServer(service propagationController, sessionSecret string, logger *slog.Logger) *PropagationServer {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &PropagationServer{
		service:       service,
		sessionSecret: sessionSecret,
		logger:        logger,
	}
}

// Connect serves one Sentinel propagation stream: it authenticates the session
// token from the incoming metadata, registers the machine's sender, delivers
// the initial snapshot, and forwards acknowledgements until the stream ends.
func (server *PropagationServer) Connect(stream pb.Propagation_ConnectServer) error {
	token, err := bearerToken(stream.Context())
	if err != nil {
		return status.Error(codes.Unauthenticated, "invalid session token")
	}

	machineID, err := auth.ParseSessionToken(token, server.sessionSecret, time.Now())
	if err != nil {
		return status.Error(codes.Unauthenticated, "invalid session token")
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	sender := newStreamSender(stream, cancel)

	connID, err := server.service.Register(ctx, machineID, sender)
	if err != nil {
		if errors.Is(err, machineErrors.ErrMachineNotFound) {
			return status.Error(codes.Unauthenticated, "invalid session token")
		}

		return status.Error(codes.Internal, "failed to register stream")
	}

	defer server.service.Unregister(machineID, connID)

	server.logger.Info("propagation stream established", "machine_id", machineID)

	server.service.DeliverSnapshot(ctx, machineID)

	for {
		// The receive runs in a goroutine so the loop also watches the
		// handler's context: when a wedged send abandons this stream, the
		// cancelled context ends the loop, and returning finalizes the
		// stream — unwinding the pending receive and any abandoned send.
		type received struct {
			ack *pb.Ack
			err error
		}

		receiving := make(chan received, 1)
		go func() {
			ack, err := stream.Recv()
			receiving <- received{ack: ack, err: err}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-receiving:
			if result.err != nil {
				if errors.Is(result.err, io.EOF) {
					return nil
				}

				return result.err
			}

			server.service.Acknowledge(ctx, machineID, fromProtoAck(result.ack))
		}
	}
}

// bearerToken extracts the session token from the incoming `authorization`
// metadata, accepting a case-insensitive `Bearer` prefix.
func bearerToken(ctx context.Context) (string, error) {
	incoming, found := metadata.FromIncomingContext(ctx)
	if !found {
		return "", errors.New("missing authorization metadata")
	}

	for _, value := range incoming.Get("authorization") {
		if len(value) >= 7 && strings.EqualFold(value[:7], "bearer ") {
			return strings.TrimSpace(value[7:]), nil
		}
	}

	return "", errors.New("authorization metadata is not a bearer token")
}
