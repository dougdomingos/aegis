package grpcserver

import (
	"fmt"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/pkg/pb"
)

// toProtoMessage converts one transport-neutral message into its wire
// representation, selecting the payload branch by kind. Unknown kinds return
// an error.
func toProtoMessage(message propagation.Message) (*pb.ServerMessage, error) {
	converted := &pb.ServerMessage{
		Epoch:     message.Epoch,
		Sequence:  message.Sequence,
		MachineId: message.MachineID,
	}

	switch message.Kind {
	case propagation.KindPackage:
		converted.Payload = &pb.ServerMessage_Policy{
			Policy: &pb.PolicyPackage{
				PolicyId:      message.PolicyID,
				PolicyName:    message.PolicyName,
				PolicyVersion: int32(message.PolicyVersion),
				PolicyType:    message.PolicyType,
				Rules:         toProtoRules(message.Rules),
			},
		}

	case propagation.KindRevoke:
		converted.Payload = &pb.ServerMessage_Revoke{Revoke: &pb.Revoke{}}

	default:
		return nil, fmt.Errorf("unknown propagation kind %q", message.Kind)
	}

	return converted, nil
}

// toProtoRules maps every wire rule into its proto counterpart, preserving the
// optionality of protocol and port.
func toProtoRules(rules []propagation.Rule) []*pb.Rule {
	converted := make([]*pb.Rule, 0, len(rules))
	for _, rule := range rules {
		protoRule := &pb.Rule{Type: rule.Type, Value: rule.Value}

		if rule.Protocol != nil {
			protocol := *rule.Protocol
			protoRule.Protocol = &protocol
		}

		if rule.Port != nil {
			port := int32(*rule.Port)
			protoRule.Port = &port
		}

		converted = append(converted, protoRule)
	}

	return converted
}

// fromProtoAck converts one proto acknowledgement into its transport-neutral
// representation. Unknown statuses map to the zero value.
func fromProtoAck(ack *pb.Ack) propagation.Ack {
	converted := propagation.Ack{
		Sequence: ack.GetSequence(),
		Reason:   ack.GetReason(),
	}

	switch ack.GetStatus() {
	case pb.AckStatus_ACK_STATUS_ACKED:
		converted.Status = propagation.AckAcked
	case pb.AckStatus_ACK_STATUS_NACKED:
		converted.Status = propagation.AckNacked
	default:
		converted.Status = propagation.AckStatus("")
	}

	return converted
}
