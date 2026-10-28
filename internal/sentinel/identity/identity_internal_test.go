package identity

import (
	"net"
	"testing"
)

func TestDetector_Detect_WithDefaultRouteAvailable_ReturnsPrimaryAddress(t *testing.T) {
	addrs := map[string]string{
		"docker0": "172.17.0.1",
		"eth0":    "10.0.1.11",
	}

	detector := arrangeDetector(t, addrs)
	detector.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			fakeInterface("docker0", net.FlagUp, "172.17.0.1", "02:42:aa:bb:cc:dd"),
			fakeInterface("eth0", net.FlagUp, "10.0.1.11", "aa:bb:cc:dd:ee:ff"),
		}, nil
	}
	detector.primaryIP = func() (net.IP, error) {
		return net.ParseIP("10.0.1.11"), nil
	}

	identity, err := detector.Detect()
	if err != nil {
		t.Fatalf("detector.Detect() error = %v, want nil", err)
	}

	if identity.IP != "10.0.1.11" {
		t.Errorf("identity.IP = %q, want 10.0.1.11", identity.IP)
	}

	if identity.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("identity.MAC = %q, want aa:bb:cc:dd:ee:ff", identity.MAC)
	}
}

func TestDetector_Detect_WithDefaultRouteUnavailable_FallsBackToFirstUsableInterface(t *testing.T) {
	addrs := map[string]string{
		"lo":   "127.0.0.1",
		"eth0": "10.0.1.11",
		"eth1": "10.0.1.12",
	}

	detector := arrangeDetector(t, addrs)
	detector.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			fakeInterface("lo", net.FlagUp|net.FlagLoopback, "127.0.0.1", ""),
			fakeInterface("eth0", 0, "10.0.1.11", ""),
			fakeInterface("eth1", net.FlagUp, "10.0.1.12", "aa:bb:cc:dd:ee:00"),
		}, nil
	}
	detector.primaryIP = func() (net.IP, error) {
		return nil, net.UnknownNetworkError("stub primaryIP failure")
	}

	identity, err := detector.Detect()
	if err != nil {
		t.Fatalf("detector.Detect() error = %v, want nil", err)
	}

	if identity.IP != "10.0.1.12" {
		t.Errorf("identity.IP = %q, want 10.0.1.12", identity.IP)
	}

	if identity.MAC != "aa:bb:cc:dd:ee:00" {
		t.Errorf("identity.MAC = %q, want aa:bb:cc:dd:ee:00", identity.MAC)
	}
}

func TestDetector_Detect_WithPrimaryIPOutsideLocalInterfaces_FallsBackToInterface(t *testing.T) {
	addrs := map[string]string{
		"eth0": "10.0.1.11",
	}

	detector := arrangeDetector(t, addrs)
	detector.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			fakeInterface("eth0", net.FlagUp, "10.0.1.11", "aa:bb:cc:dd:ee:ff"),
		}, nil
	}
	detector.primaryIP = func() (net.IP, error) {
		return net.ParseIP("192.0.2.7"), nil
	}

	identity, err := detector.Detect()
	if err != nil {
		t.Fatalf("detector.Detect() error = %v, want nil", err)
	}

	if identity.IP != "10.0.1.11" {
		t.Errorf("identity.IP = %q, want 10.0.1.11", identity.IP)
	}

	if identity.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("identity.MAC = %q, want aa:bb:cc:dd:ee:ff", identity.MAC)
	}
}

func TestDetector_Detect_WithNoUsableIPv4_ReturnsError(t *testing.T) {
	cases := map[string]struct {
		interfaces []net.Interface
		addrs      map[string]string
	}{
		"only loopback": {interfaces: []net.Interface{
			fakeInterface("lo", net.FlagUp|net.FlagLoopback, "127.0.0.1", ""),
		}, addrs: map[string]string{"lo": "127.0.0.1"}},
		"only IPv6": {interfaces: []net.Interface{
			fakeInterface("eth0", net.FlagUp, "fd00::1", "aa:bb:cc:dd:ee:ff"),
		}, addrs: map[string]string{"eth0": "fd00::1"}},
		"all interfaces down": {interfaces: []net.Interface{
			fakeInterface("eth0", 0, "10.0.1.11", ""),
			fakeInterface("eth1", 0, "10.0.1.12", ""),
		}, addrs: map[string]string{"eth0": "10.0.1.11", "eth1": "10.0.1.12"}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			detector := arrangeDetector(t, tc.addrs)
			detector.interfaces = func() ([]net.Interface, error) {
				return tc.interfaces, nil
			}
			detector.primaryIP = func() (net.IP, error) {
				return nil, net.UnknownNetworkError("stub primaryIP failure")
			}

			if _, err := detector.Detect(); err == nil {
				t.Errorf("detector.Detect() error = nil, want error")
			}
		})
	}
}

func TestDetector_Detect_WithInterfaceWithoutHardwareAddress_ReturnsEmptyMAC(t *testing.T) {
	addrs := map[string]string{
		"eth0": "10.0.1.11",
	}

	detector := arrangeDetector(t, addrs)
	detector.interfaces = func() ([]net.Interface, error) {
		return []net.Interface{
			fakeInterface("eth0", net.FlagUp, "10.0.1.11", ""),
		}, nil
	}
	detector.primaryIP = func() (net.IP, error) {
		return net.ParseIP("10.0.1.11"), nil
	}

	identity, err := detector.Detect()
	if err != nil {
		t.Fatalf("detector.Detect() error = %v, want nil", err)
	}

	if identity.IP != "10.0.1.11" {
		t.Errorf("identity.IP = %q, want 10.0.1.11", identity.IP)
	}

	if identity.MAC != "" {
		t.Errorf("identity.MAC = %q, want empty", identity.MAC)
	}
}

func TestDetector_Detect_WithInterfaceListingError_ReturnsError(t *testing.T) {
	detector := arrangeDetector(t, map[string]string{})
	detector.interfaces = func() ([]net.Interface, error) {
		return nil, net.UnknownNetworkError("stub interfaces failure")
	}

	if _, err := detector.Detect(); err == nil {
		t.Errorf("detector.Detect() error = nil, want error")
	}
}

// arrangeDetector creates a Detector whose function fields are stubbed to
// resolve addresses from the provided name-based mapping.
func arrangeDetector(t *testing.T, addrs map[string]string) *Detector {
	t.Helper()

	detector := NewDetector()
	detector.interfaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
		ip, ok := addrs[iface.Name]
		if !ok {
			return nil, nil
		}

		return []net.Addr{&net.IPNet{IP: net.ParseIP(ip), Mask: net.CIDRMask(24, 32)}}, nil
	}

	return detector
}

// fakeInterface builds a net.Interface with the provided flags, name, address
// and hardware address. The address is not carried by net.Interface itself:
// net.Interface.Addrs() resolves it through a syscall by index, so tests
// honor the address through the stubbed interfaceAddrs function instead.
func fakeInterface(name string, flags net.Flags, ip string, mac string) net.Interface {
	var hardware net.HardwareAddr
	if mac != "" {
		hardware, _ = net.ParseMAC(mac)
	}

	return net.Interface{Name: name, Flags: flags, HardwareAddr: hardware}
}
