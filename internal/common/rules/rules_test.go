package rules_test

import (
	"errors"
	"testing"

	ruleErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/common/rules"
)

func TestRules_Validate_WithValidDomainRule_ReturnsNil(t *testing.T) {
	rule := rules.Rule{Type: "DOMAIN", Value: "example.com"}

	if err := rules.Validate(rule); err != nil {
		t.Errorf("Validate(%+v) = %v, want nil", rule, err)
	}
}

func TestRules_Validate_WithValidIPRule_ReturnsNil(t *testing.T) {
	rule := rules.Rule{Type: "IP", Value: "127.0.0.1", Protocol: ptr("TCP"), Port: ptr(443)}

	if err := rules.Validate(rule); err != nil {
		t.Errorf("Validate(%+v) = %v, want nil", rule, err)
	}
}

func TestRules_Validate_WithIPRuleVariations_ReturnsExpectedResult(t *testing.T) {
	testCases := map[string]struct {
		value   string
		wantErr error
	}{
		"ipv4 cidr":             {value: "10.0.0.0/8", wantErr: nil},
		"ipv6 cidr":             {value: "2001:db8::/32", wantErr: nil},
		"cidr prefix too large": {value: "10.0.0.0/33", wantErr: ruleErrors.ErrMalformedRuleValue},
		"single ip":             {value: "10.0.0.1", wantErr: nil},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rule := rules.Rule{Type: "IP", Value: tc.value}
			err := rules.Validate(rule)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Validate(%+v) = %v, want %v", rule, err, tc.wantErr)
			}
		})
	}
}

func TestRules_Validate_WithMissingType_ReturnsErrRuleTypeRequired(t *testing.T) {
	testCases := map[string]struct {
		rule rules.Rule
	}{
		"empty type": {rule: rules.Rule{Type: "", Value: "example.com"}},
		"blank type": {rule: rules.Rule{Type: "   ", Value: "example.com"}},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := rules.Validate(tc.rule)
			if !errors.Is(err, ruleErrors.ErrRuleTypeRequired) {
				t.Errorf("Validate(%+v) = %v, want ErrRuleTypeRequired", tc.rule, err)
			}
		})
	}
}

func TestRules_Validate_WithMissingValue_ReturnsErrRuleValueRequired(t *testing.T) {
	testCases := map[string]struct {
		rule rules.Rule
	}{
		"empty value": {rule: rules.Rule{Type: "DOMAIN", Value: ""}},
		"blank value": {rule: rules.Rule{Type: "IP", Value: "   "}},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := rules.Validate(tc.rule)
			if !errors.Is(err, ruleErrors.ErrRuleValueRequired) {
				t.Errorf("Validate(%+v) = %v, want ErrRuleValueRequired", tc.rule, err)
			}
		})
	}
}

func TestRules_Validate_WithDomainCarryingProtocol_ReturnsErrInvalidFieldForRuleType(t *testing.T) {
	testCases := map[string]struct {
		rule rules.Rule
	}{
		"protocol set":    {rule: rules.Rule{Type: "DOMAIN", Value: "example.com", Protocol: ptr("TCP")}},
		"port set":        {rule: rules.Rule{Type: "DOMAIN", Value: "example.com", Port: ptr(80)}},
		"both fields set": {rule: rules.Rule{Type: "DOMAIN", Value: "example.com", Protocol: ptr("TCP"), Port: ptr(80)}},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := rules.Validate(tc.rule)
			if !errors.Is(err, ruleErrors.ErrInvalidFieldForRuleType) {
				t.Errorf("Validate(%+v) = %v, want ErrInvalidFieldForRuleType", tc.rule, err)
			}
		})
	}
}

func TestRules_Validate_WithMalformedDomain_ReturnsErrMalformedRuleValue(t *testing.T) {
	testCases := map[string]struct {
		value string
	}{
		"no dot":      {value: "localhost"},
		"spaces":      {value: "not a domain"},
		"leading dot": {value: ".example.com"},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rule := rules.Rule{Type: "DOMAIN", Value: tc.value}
			err := rules.Validate(rule)
			if !errors.Is(err, ruleErrors.ErrMalformedRuleValue) {
				t.Errorf("Validate(%+v) = %v, want ErrMalformedRuleValue", rule, err)
			}
		})
	}
}

func TestRules_Validate_WithMalformedIP_ReturnsErrMalformedRuleValue(t *testing.T) {
	testCases := map[string]struct {
		value string
	}{
		"not an address": {value: "999.999.999.999"},
		"domain value":   {value: "example.com"},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rule := rules.Rule{Type: "IP", Value: tc.value}
			err := rules.Validate(rule)
			if !errors.Is(err, ruleErrors.ErrMalformedRuleValue) {
				t.Errorf("Validate(%+v) = %v, want ErrMalformedRuleValue", rule, err)
			}
		})
	}
}

func TestRules_Validate_WithPortOutOfRange_ReturnsErrInvalidRulePortValue(t *testing.T) {
	testCases := map[string]struct {
		port int
	}{
		"negative":     {port: -1},
		"too high":     {port: 65536},
		"far too high": {port: 70000},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			rule := rules.Rule{Type: "IP", Value: "127.0.0.1", Port: ptr(tc.port)}
			err := rules.Validate(rule)
			if !errors.Is(err, ruleErrors.ErrInvalidRulePortValue) {
				t.Errorf("Validate(%+v) = %v, want ErrInvalidRulePortValue", rule, err)
			}
		})
	}
}

func TestRules_Validate_WithUnknownType_ReturnsErrInvalidRuleType(t *testing.T) {
	rule := rules.Rule{Type: "MAC", Value: "00:11:22:33:44:55"}

	err := rules.Validate(rule)
	if !errors.Is(err, ruleErrors.ErrInvalidRuleType) {
		t.Errorf("Validate(%+v) = %v, want ErrInvalidRuleType", rule, err)
	}
}

// ptr returns a pointer to the provided value.
func ptr[T any](v T) *T {
	return &v
}
