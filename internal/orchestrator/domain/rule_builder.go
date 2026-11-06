package domain

// IPRuleBuilder provides a builder pattern for constructing IP-based rules.
type IPRuleBuilder struct {
	rule Rule
}

// NewIPRule initializes a new IPRuleBuilder instance with the target value.
func NewIPRule(value string) *IPRuleBuilder {
	return &IPRuleBuilder{
		rule: Rule{
			Type:  IPRuleType,
			Value: value,
		},
	}
}

// WithPolicy sets the policy that owns the rule.
func (b *IPRuleBuilder) WithPolicy(policyID int64) *IPRuleBuilder {
	b.rule.PolicyID = policyID
	return b
}

// WithProtocol sets the targeted network protocol for an IP rule.
func (b *IPRuleBuilder) WithProtocol(p string) *IPRuleBuilder {
	b.rule.Protocol = &p
	return b
}

// WithPort sets the targeted network port for an IP rule.
func (b *IPRuleBuilder) WithPort(p int) *IPRuleBuilder {
	b.rule.Port = &p
	return b
}

// Build constructs and returns the finalized IP Rule instance.
func (b *IPRuleBuilder) Build() Rule {
	return b.rule
}

// DomainRuleBuilder provides a builder pattern for constructing domain-based rules.
type DomainRuleBuilder struct {
	rule Rule
}

// NewDomainRule initializes a new DomainRuleBuilder instance with the target value.
func NewDomainRule(value string) *DomainRuleBuilder {
	return &DomainRuleBuilder{
		rule: Rule{
			Type:  DomainRuleType,
			Value: value,
		},
	}
}

// WithPolicy sets the policy that owns the rule.
func (b *DomainRuleBuilder) WithPolicy(policyID int64) *DomainRuleBuilder {
	b.rule.PolicyID = policyID
	return b
}

// Build constructs and returns the finalized Domain Rule instance.
func (b *DomainRuleBuilder) Build() Rule {
	return b.rule
}
