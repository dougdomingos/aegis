package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/pkg/pb"
)

// errStreamWedged reports a sender whose stream was abandoned after a send
// outlived its context. The stream is cancelled and never written again, so a
// wedged peer cannot block the propagation worker or accept a concurrent
// write.
var errStreamWedged = errors.New("propagation stream wedged")

// streamSender adapts the gRPC stream to the propagation.MessageSender
// contract. Sends are serialized with a mutex (the bidi stream permits only
// one active sender) and any failure abandons the stream: the context handed
// to the controller is cancelled, which ends the handler's receive loop and
// tears the stream down, and the sender is marked dead so no later delivery
// ever writes to the same stream. The in-flight write is left to unwind on
// its own behind the dead flag.
type streamSender struct {
	stream pb.Propagation_ConnectServer
	mu     sync.Mutex
	cancel context.CancelFunc
	dead   bool
}

// newStreamSender builds a sender over the provided stream and cancel func.
func newStreamSender(stream pb.Propagation_ConnectServer, cancel context.CancelFunc) *streamSender {
	return &streamSender{stream: stream, cancel: cancel}
}

// Send converts and delivers one message over the stream, honouring the
// caller's context: when ctx expires before the write completes, the send is
// abandoned and a context error is returned, so one unresponsive machine
// never blocks the caller beyond its deadline.
func (sender *streamSender) Send(ctx context.Context, message propagation.Message) error {
	sender.mu.Lock()
	defer sender.mu.Unlock()

	if sender.dead {
		return errStreamWedged
	}

	protoMessage, err := toProtoMessage(message)
	if err != nil {
		sender.abandon()
		return err
	}

	sent := make(chan error, 1)
	go func() {
		sent <- sender.stream.Send(protoMessage)
	}()

	select {
	case err := <-sent:
		if err != nil {
			sender.abandon()
			return err
		}

		return nil
	case <-ctx.Done():
		// A send completing in the same instant as the deadline is not a
		// timeout: prefer the real outcome over a spurious context error.
		select {
		case err := <-sent:
			if err != nil {
				sender.abandon()
				return err
			}

			return nil
		default:
		}

		sender.abandon()
		return fmt.Errorf("propagation send did not complete: %w", ctx.Err())
	}
}

// abandon marks the sender dead and cancels the stream context. It must be
// called with the sender's mutex held.
func (sender *streamSender) abandon() {
	sender.dead = true
	sender.cancel()
}
