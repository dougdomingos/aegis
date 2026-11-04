// Package rules provides the shared rule syntax validation enforced by both
// the Orchestrator and the Sentinel.
package rules

import (
	"net"
	"regexp"
	"strings"

	"dougdomingos.com/aegis/internal/common/errors"
)

// domainRegex validates the shape of a DOMAIN rule value.
var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// Rule carries the wire-neutral fields of a filtering rule, without any
// policy association.
type Rule struct {
	// Type is the rule classification (DOMAIN or IP).
	Type string

	// Value is the targeted resource (e.g., IP address, CIDR, domain name).
	Value string

	// Protocol is the optional targeted network protocol (e.g., TCP, UDP).
	Protocol *string

	// Port is the optional targeted network port.
	Port *int
}

// Validate checks whether the provided rule holds a valid configuration,
// returning the same errors the Orchestrator uses for stored rules.
func Validate(rule Rule) error {
	if strings.TrimSpace(rule.Type) == "" {
		return errors.ErrRuleTypeRequired
	}

	if strings.TrimSpace(rule.Value) == "" {
		return errors.ErrRuleValueRequired
	}

	switch rule.Type {
	case "DOMAIN":
		if rule.Protocol != nil || rule.Port != nil {
			return errors.ErrInvalidFieldForRuleType
		}

		if !domainRegex.MatchString(rule.Value) {
			return errors.ErrMalformedRuleValue
		}

	case "IP":
		if net.ParseIP(rule.Value) == nil {
			return errors.ErrMalformedRuleValue
		}

		if rule.Port != nil && (*rule.Port < 0 || *rule.Port > 65535) {
			return errors.ErrInvalidRulePortValue
		}

	default:
		return errors.ErrInvalidRuleType
	}

	return nil
}
