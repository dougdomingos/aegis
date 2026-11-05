package domain_test

import (
	"errors"
	"testing"

	ruleErrors "dougdomingos.com/aegis/internal/common/errors"
	"dougdomingos.com/aegis/internal/orchestrator/domain"
)

func TestRule_Validate_WithoutPolicyID_ReturnsErrRulePolicyRequired(t *testing.T) {
	rule := domain.Rule{
		Type:     domain.DomainRuleType,
		Value:    "example.com",
		Protocol: nil,
		Port:     nil,
	}

	err := rule.Validate()
	if !errors.Is(err, ruleErrors.ErrRulePolicyRequired) {
		t.Errorf("Validate() = %v, want ErrRulePolicyRequired", err)
	}
}
