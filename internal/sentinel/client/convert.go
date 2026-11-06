package client

import (
	"fmt"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/pkg/pb"
)

// toProtoAck converts one transport-neutral acknowledgement into its wire
// representation. Unknown statuses map to the unspecified enum value.
func toProtoAck(ack propagation.Ack) *pb.Ack {
	converted := &pb.Ack{
		Sequence: ack.Sequence,
		Reason:   ack.Reason,
		Status:   pb.AckStatus_ACK_STATUS_UNSPECIFIED,
	}

	switch ack.Status {
	case propagation.AckAcked:
		converted.Status = pb.AckStatus_ACK_STATUS_ACKED
	case propagation.AckNacked:
		converted.Status = pb.AckStatus_ACK_STATUS_NACKED
	}

	return converted
}

// fromProtoServerMessage converts one server message into its transport-neutral
// representation, mapping the payload branch to the message kind. Payloads of
// unknown branches return an error.
func fromProtoServerMessage(message *pb.ServerMessage) (*propagation.Message, error) {
	converted := &propagation.Message{
		Epoch:     message.GetEpoch(),
		Sequence:  message.GetSequence(),
		MachineID: message.GetMachineId(),
	}

	switch payload := message.GetPayload().(type) {
	case *pb.ServerMessage_Policy:
		converted.Kind = propagation.KindPackage
		converted.PolicyID = payload.Policy.GetPolicyId()
		converted.PolicyName = payload.Policy.GetPolicyName()
		converted.PolicyVersion = int(payload.Policy.GetPolicyVersion())
		converted.PolicyType = payload.Policy.GetPolicyType()
		converted.Rules = fromProtoRules(payload.Policy.GetRules())

	case *pb.ServerMessage_Revoke:
		converted.Kind = propagation.KindRevoke

	default:
		return nil, fmt.Errorf("propagation message of machine %d sequence %d carries no known payload", message.GetMachineId(), message.GetSequence())
	}

	return converted, nil
}

// fromProtoRules maps every wire rule into its transport-neutral counterpart,
// preserving the optionality of protocol and port.
func fromProtoRules(rules []*pb.Rule) []propagation.Rule {
	converted := make([]propagation.Rule, 0, len(rules))
	for _, rule := range rules {
		wireRule := propagation.Rule{Type: rule.GetType(), Value: rule.GetValue()}

		if rule.Protocol != nil {
			protocol := rule.GetProtocol()
			wireRule.Protocol = &protocol
		}

		if rule.Port != nil {
			port := int(rule.GetPort())
			wireRule.Port = &port
		}

		converted = append(converted, wireRule)
	}

	return converted
}
