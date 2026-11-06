package filtering

import (
	"fmt"

	"dougdomingos.com/aegis/internal/common/propagation"
)

// Filter applies and clears the local packet and DNS filtering. It is the
// contract the propagation runner consumes.
type Filter interface {
	Apply(snapshot PolicySnapshot) error
	Clear() error
}

// PacketFilter is the packet-level backend, satisfied by the nftables executor.
type PacketFilter interface {
	Apply(snapshot PolicySnapshot) error
	Clear() error
}

// ResolverFilter is the DNS backend, satisfied by the embedded resolver. Its
// operations cannot fail, so the composite validates a snapshot before
// swapping anything.
type ResolverFilter interface {
	Update(snapshot PolicySnapshot)
	Clear()
}

// Composite programs nftables first, then the resolver.
type Composite struct {
	packets  PacketFilter
	resolver ResolverFilter
}

// NewComposite builds a Filter over the packet and DNS backends.
func NewComposite(packets PacketFilter, resolver ResolverFilter) Filter {
	return &Composite{packets: packets, resolver: resolver}
}

func (c *Composite) Apply(snapshot PolicySnapshot) error {
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	if err := c.packets.Apply(snapshot); err != nil {
		return fmt.Errorf("apply packet filter: %w", err)
	}
	c.resolver.Update(snapshot)
	return nil
}

func (c *Composite) Clear() error {
	err := c.packets.Clear()
	c.resolver.Clear()
	if err != nil {
		return fmt.Errorf("clear packet filter: %w", err)
	}
	return nil
}

// validateSnapshot rejects snapshots no backend should enforce.
func validateSnapshot(snapshot PolicySnapshot) error {
	switch snapshot.PolicyType {
	case propagation.PolicyTypeWhitelist, propagation.PolicyTypeBlacklist:
		return nil
	}
	return fmt.Errorf("unsupported policy type %q", snapshot.PolicyType)
}
