package errors

import "errors"

var (
	// ErrRuleNotFound is returned when the requested rule is not found within the
	// database.
	ErrRuleNotFound = errors.New("requested rule not found")

	// ErrRuleTypeRequired is returned when the type of a rule is required but not
	// provided by the payload
	ErrRuleTypeRequired = errors.New("must specify rule type")

	// ErrRuleTypeRequired is returned when the action of a rule is required but
	// not provided by the payload
	ErrRuleActionRequired = errors.New("must specify rule action")

	// ErrRuleTypeRequired is returned when the value of a rule is required but
	// not provided by the payload
	ErrRuleValueRequired = errors.New("must specify rule value")

	// ErrMalformedRuleValue is returned when the value of a rule does not match
	// the expected format for its type.
	ErrMalformedRuleValue = errors.New("rule value is malformed")

	// ErrInvalidFieldForRuleType is returned when the payload provided defines
	// a field that is not supported by the rule type (e.g., a domain rule which
	// declares a port field).
	ErrInvalidFieldForRuleType = errors.New("field is not supported by this rule type")

	// ErrInvalidRulePortValue is returned when the payload provided defines
	// a port field with an invalid configuration (e.g., negative ports,
	// out-of-range values).
	ErrInvalidRulePortValue = errors.New("rule port must be a number between 0 and 65536")
)
