package propagation_test

import (
	"encoding/json"
	"testing"

	"dougdomingos.com/aegis/internal/common/propagation"
)

func TestPropagation_Message_JSONRoundTrip_PreservesPackage(t *testing.T) {
	protocol := "TCP"
	port := 443
	message := propagation.Message{
		Epoch:         "abcdef0123456789abcdef0123456789",
		Sequence:      7,
		MachineID:     42,
		Kind:          propagation.KindPackage,
		PolicyID:      3,
		PolicyName:    "lab-whitelist",
		PolicyVersion: 2,
		PolicyType:    propagation.PolicyTypeWhitelist,
		Rules: []propagation.Rule{
			{Type: "IP", Value: "10.0.0.1", Protocol: &protocol, Port: &port},
			{Type: "DOMAIN", Value: "example.com"},
		},
	}

	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("json.Marshal() = %v, want nil", err)
	}

	var decoded propagation.Message
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() = %v, want nil", err)
	}

	if decoded.Epoch != message.Epoch {
		t.Errorf("Epoch = %q, want %q", decoded.Epoch, message.Epoch)
	}
	if decoded.Sequence != message.Sequence {
		t.Errorf("Sequence = %d, want %d", decoded.Sequence, message.Sequence)
	}
	if decoded.MachineID != message.MachineID {
		t.Errorf("MachineID = %d, want %d", decoded.MachineID, message.MachineID)
	}
	if decoded.Kind != message.Kind {
		t.Errorf("Kind = %q, want %q", decoded.Kind, message.Kind)
	}
	if decoded.PolicyID != message.PolicyID {
		t.Errorf("PolicyID = %d, want %d", decoded.PolicyID, message.PolicyID)
	}
	if decoded.PolicyName != message.PolicyName {
		t.Errorf("PolicyName = %q, want %q", decoded.PolicyName, message.PolicyName)
	}
	if decoded.PolicyVersion != message.PolicyVersion {
		t.Errorf("PolicyVersion = %d, want %d", decoded.PolicyVersion, message.PolicyVersion)
	}
	if decoded.PolicyType != message.PolicyType {
		t.Errorf("PolicyType = %q, want %q", decoded.PolicyType, message.PolicyType)
	}

	if len(decoded.Rules) != 2 {
		t.Fatalf("Rules = %d entries, want 2", len(decoded.Rules))
	}
	if decoded.Rules[0].Protocol == nil || *decoded.Rules[0].Protocol != protocol {
		t.Errorf("Rules[0].Protocol = %v, want %q", decoded.Rules[0].Protocol, protocol)
	}
	if decoded.Rules[0].Port == nil || *decoded.Rules[0].Port != port {
		t.Errorf("Rules[0].Port = %v, want %d", decoded.Rules[0].Port, port)
	}
	if decoded.Rules[1].Protocol != nil || decoded.Rules[1].Port != nil {
		t.Errorf("Rules[1] optional fields = %v/%v, want nil/nil", decoded.Rules[1].Protocol, decoded.Rules[1].Port)
	}
}

func TestPropagation_Message_JSONRoundTrip_PreservesRevoke(t *testing.T) {
	message := propagation.Message{
		Epoch:     "abcdef0123456789abcdef0123456789",
		Sequence:  8,
		MachineID: 42,
		Kind:      propagation.KindRevoke,
	}

	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("json.Marshal() = %v, want nil", err)
	}

	var decoded propagation.Message
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() = %v, want nil", err)
	}

	if decoded.Kind != propagation.KindRevoke {
		t.Errorf("Kind = %q, want %q", decoded.Kind, propagation.KindRevoke)
	}
	if decoded.PolicyID != 0 || decoded.PolicyName != "" || decoded.PolicyVersion != 0 || decoded.PolicyType != "" {
		t.Errorf("policy metadata = %+v, want zero values", decoded)
	}
	if len(decoded.Rules) != 0 {
		t.Errorf("Rules = %d entries, want 0", len(decoded.Rules))
	}
}

func TestPropagation_Kind_Constants_HaveWireValues(t *testing.T) {
	testCases := map[string]struct {
		kind propagation.Kind
		want string
	}{
		"package": {kind: propagation.KindPackage, want: "PACKAGE"},
		"revoke":  {kind: propagation.KindRevoke, want: "REVOKE"},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			if string(tc.kind) != tc.want {
				t.Errorf("kind = %q, want %q", string(tc.kind), tc.want)
			}
		})
	}
}

func TestPropagation_AckStatus_Constants_HaveWireValues(t *testing.T) {
	testCases := map[string]struct {
		status propagation.AckStatus
		want   string
	}{
		"acked":  {status: propagation.AckAcked, want: "ACKED"},
		"nacked": {status: propagation.AckNacked, want: "NACKED"},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			if string(tc.status) != tc.want {
				t.Errorf("status = %q, want %q", string(tc.status), tc.want)
			}
		})
	}
}

func TestPropagation_PolicyType_Constants_HaveWireValues(t *testing.T) {
	testCases := map[string]struct {
		policyType string
		want       string
	}{
		"whitelist": {policyType: propagation.PolicyTypeWhitelist, want: "WHITELIST"},
		"blacklist": {policyType: propagation.PolicyTypeBlacklist, want: "BLACKLIST"},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			if tc.policyType != tc.want {
				t.Errorf("policyType = %q, want %q", tc.policyType, tc.want)
			}
		})
	}
}
