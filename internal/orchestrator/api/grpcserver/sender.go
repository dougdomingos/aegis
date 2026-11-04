package grpcserver

import (
	"context"
	"sync"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/pkg/pb"
)

// streamSender adapts the gRPC stream to the service.MessageSender contract.
// Sends are serialized with a mutex (the bidi stream permits only one active
// sender) and any failure cancels the stream context, unblocking the Recv
// loop so the handler can unregister and the client can reconnect.
type streamSender struct {
	stream pb.Propagation_ConnectServer
	mu     sync.Mutex
	cancel context.CancelFunc
}

// newStreamSender builds a sender over the provided stream and cancel func.
func newStreamSender(stream pb.Propagation_ConnectServer, cancel context.CancelFunc) *streamSender {
	return &streamSender{stream: stream, cancel: cancel}
}

// Send converts and delivers one message over the stream.
func (sender *streamSender) Send(ctx context.Context, message propagation.Message) error {
	sender.mu.Lock()
	defer sender.mu.Unlock()

	protoMessage, err := toProtoMessage(message)
	if err != nil {
		sender.cancel()
		return err
	}

	if err := sender.stream.Send(protoMessage); err != nil {
		sender.cancel()
		return err
	}

	return nil
}
