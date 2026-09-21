package domain

// IPRuleBuilder provides a builder pattern for constructing IP-based rules.
type IPRuleBuilder struct {
	rule Rule
}

// NewIPRule initializes a new IPRuleBuilder instance with action and target value.
func NewIPRule(action RuleAction, value string) *IPRuleBuilder {
	return &IPRuleBuilder{
		rule: Rule{
			Type:   IPRuleType,
			Action: action,
			Value:  value,
		},
	}
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

// NewDomainRule initializes a new DomainRuleBuilder instance with action and target value.
func NewDomainRule(action RuleAction, value string) *DomainRuleBuilder {
	return &DomainRuleBuilder{
		rule: Rule{
			Type:   DomainRuleType,
			Action: action,
			Value:  value,
		},
	}
}

// Build constructs and returns the finalized Domain Rule instance.
func (b *DomainRuleBuilder) Build() Rule {
	return b.rule
}
