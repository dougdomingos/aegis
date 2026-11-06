// Package identity detects the network identity of the Sentinel host.
package identity

import (
	"fmt"
	"net"
)

// Identity describes how the Sentinel presents itself to the Orchestrator.
type Identity struct {
	// IP is the address that identifies the machine to the Orchestrator.
	IP string

	// MAC is the hardware address of the interface that owns the IP.
	MAC string
}

// Detector resolves the network identity of the local host. Its function
// fields are injection points used by tests; the zero-value-less design keeps
// every access path stubbed in isolation.
type Detector struct {
	// interfaces lists the network interfaces of the host.
	interfaces func() ([]net.Interface, error)

	// interfaceAddrs lists the addresses assigned to a single interface.
	interfaceAddrs func(net.Interface) ([]net.Addr, error)

	// primaryIP resolves the address the host uses for outbound traffic.
	primaryIP func() (net.IP, error)
}

// NewDetector creates a Detector using the host's real network state.
func NewDetector() *Detector {
	return &Detector{
		interfaces:     net.Interfaces,
		interfaceAddrs: defaultInterfaceAddrs,
		primaryIP:      defaultPrimaryIP,
	}
}

// Detect resolves the local network identity. The interface owning the
// primary outbound address wins; otherwise the first interface that is up,
// not loopback and holds an IPv4 address is used.
func (d *Detector) Detect() (Identity, error) {
	list, err := d.interfaces()
	if err != nil {
		return Identity{}, fmt.Errorf("failed to list network interfaces: %w", err)
	}

	if identity, ok := d.detectByPrimaryIP(list); ok {
		return identity, nil
	}

	return d.detectByFallback(list)
}

// detectByPrimaryIP returns the identity of the interface owning the primary
// outbound address, if there is one.
func (d *Detector) detectByPrimaryIP(list []net.Interface) (Identity, bool) {
	primary, err := d.primaryIP()
	if err != nil || primary == nil {
		return Identity{}, false
	}

	for _, iface := range list {
		addrs, err := d.interfaceAddrs(iface)
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if !ownsIPv4(addr, primary) {
				continue
			}

			identity, ok := buildIdentity(iface, addrs)
			if !ok {
				continue
			}

			return identity, true
		}
	}

	return Identity{}, false
}

// detectByFallback returns the identity of the first interface that is up,
// not loopback and holds an IPv4 address.
func (d *Detector) detectByFallback(list []net.Interface) (Identity, error) {
	for _, iface := range list {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := d.interfaceAddrs(iface)
		if err != nil {
			continue
		}

		if identity, ok := buildIdentity(iface, addrs); ok {
			return identity, nil
		}
	}

	return Identity{}, fmt.Errorf("no usable IPv4 address found")
}

// buildIdentity renders the IPv4 address and hardware address of an
// interface, reporting whether the interface holds a usable IPv4 address.
func buildIdentity(iface net.Interface, addrs []net.Addr) (Identity, bool) {
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ipv4 := ipnet.IP.To4()
		if ipv4 == nil {
			continue
		}

		return Identity{IP: ipv4.String(), MAC: iface.HardwareAddr.String()}, true
	}

	return Identity{}, false
}

// ownsIPv4 reports whether the provided address is an IPv4 network that owns
// the provided IP.
func ownsIPv4(addr net.Addr, ip net.IP) bool {
	ipnet, ok := addr.(*net.IPNet)
	if !ok {
		return false
	}

	return ipnet.IP.To4() != nil && ipnet.IP.Equal(ip)
}

// defaultInterfaceAddrs lists the addresses assigned to the provided
// interface through the operating system.
func defaultInterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

// defaultPrimaryIP resolves the address the host uses for outbound traffic by
// opening a UDP connection that does not send any packet.
func defaultPrimaryIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve the primary outbound address: %w", err)
	}
	defer conn.Close()

	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("primary outbound address is not a UDP address")
	}

	return local.IP, nil
}
