package grpcserver

import (
	"testing"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/pkg/pb"
)

func TestConvert_ToProtoMessage_WithPackage_MapsPolicyAndRules(t *testing.T) {
	protocol := "TCP"
	port := 443

	message := propagation.Message{
		Epoch:         "epoch-abc",
		Sequence:      2,
		MachineID:     42,
		Kind:          propagation.KindPackage,
		PolicyID:      7,
		PolicyName:    "lab-whitelist",
		PolicyVersion: 3,
		PolicyType:    propagation.PolicyTypeWhitelist,
		Rules: []propagation.Rule{
			{Type: "IP", Value: "10.0.0.1", Protocol: &protocol, Port: &port},
			{Type: "DOMAIN", Value: "example.com"},
		},
	}

	converted, err := toProtoMessage(message)
	if err != nil {
		t.Fatalf("toProtoMessage() = %v, want nil", err)
	}

	if converted.Epoch != "epoch-abc" {
		t.Errorf("Epoch = %q, want %q", converted.Epoch, "epoch-abc")
	}
	if converted.Sequence != 2 {
		t.Errorf("Sequence = %d, want 2", converted.Sequence)
	}
	if converted.MachineId != 42 {
		t.Errorf("MachineId = %d, want 42", converted.MachineId)
	}

	packagePayload, ok := converted.Payload.(*pb.ServerMessage_Policy)
	if !ok {
		t.Fatalf("Payload = %T, want *pb.ServerMessage_Policy", converted.Payload)
	}

	if packagePayload.Policy.PolicyId != 7 {
		t.Errorf("PolicyId = %d, want 7", packagePayload.Policy.PolicyId)
	}
	if packagePayload.Policy.PolicyName != "lab-whitelist" {
		t.Errorf("PolicyName = %q, want %q", packagePayload.Policy.PolicyName, "lab-whitelist")
	}
	if packagePayload.Policy.PolicyVersion != 3 {
		t.Errorf("PolicyVersion = %d, want 3", packagePayload.Policy.PolicyVersion)
	}
	if packagePayload.Policy.PolicyType != propagation.PolicyTypeWhitelist {
		t.Errorf("PolicyType = %q, want %q", packagePayload.Policy.PolicyType, propagation.PolicyTypeWhitelist)
	}

	if len(packagePayload.Policy.Rules) != 2 {
		t.Fatalf("Rules = %d, want 2", len(packagePayload.Policy.Rules))
	}

	ipRule := packagePayload.Policy.Rules[0]
	if ipRule.Protocol == nil || *ipRule.Protocol != "TCP" {
		t.Errorf("IP rule Protocol = %v, want TCP", ipRule.Protocol)
	}
	if ipRule.Port == nil || *ipRule.Port != 443 {
		t.Errorf("IP rule Port = %v, want 443", ipRule.Port)
	}

	domainRule := packagePayload.Policy.Rules[1]
	if domainRule.Protocol != nil || domainRule.Port != nil {
		t.Errorf("domain rule optional fields = %v/%v, want nil/nil", domainRule.Protocol, domainRule.Port)
	}
}

func TestConvert_ToProtoMessage_WithRevoke_MapsRevoke(t *testing.T) {
	message := propagation.Message{
		Epoch:     "epoch-abc",
		Sequence:  3,
		MachineID: 42,
		Kind:      propagation.KindRevoke,
	}

	converted, err := toProtoMessage(message)
	if err != nil {
		t.Fatalf("toProtoMessage() = %v, want nil", err)
	}

	if _, ok := converted.Payload.(*pb.ServerMessage_Revoke); !ok {
		t.Fatalf("Payload = %T, want *pb.ServerMessage_Revoke", converted.Payload)
	}
}

func TestConvert_ToProtoMessage_WithUnknownKind_ReturnsError(t *testing.T) {
	_, err := toProtoMessage(propagation.Message{Kind: propagation.Kind("BOGUS")})
	if err == nil {
		t.Fatal("toProtoMessage() = nil, want an error for an unknown kind")
	}
}

func TestConvert_FromProtoAck_WithKnownStatuses_MapsToDomain(t *testing.T) {
	testCases := map[string]struct {
		status pb.AckStatus
		want   propagation.AckStatus
	}{
		"acked":       {status: pb.AckStatus_ACK_STATUS_ACKED, want: propagation.AckAcked},
		"nacked":      {status: pb.AckStatus_ACK_STATUS_NACKED, want: propagation.AckNacked},
		"unspecified": {status: pb.AckStatus_ACK_STATUS_UNSPECIFIED, want: propagation.AckStatus("")},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ack := fromProtoAck(&pb.Ack{Sequence: 5, Status: tc.status, Reason: "because"})

			if ack.Sequence != 5 {
				t.Errorf("Sequence = %d, want 5", ack.Sequence)
			}
			if ack.Status != tc.want {
				t.Errorf("Status = %q, want %q", ack.Status, tc.want)
			}
			if ack.Reason != "because" {
				t.Errorf("Reason = %q, want %q", ack.Reason, "because")
			}
		})
	}
}
