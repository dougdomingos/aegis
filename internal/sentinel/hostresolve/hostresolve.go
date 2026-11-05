// Package hostresolve resolves hostnames to IP addresses through a chosen DNS
// server, so the Sentinel can resolve before its own embedded DNS is serving.
package hostresolve

import (
	"context"
	"log/slog"
	"net"
	"slices"
	"time"
)

// IPs returns the deduplicated IP addresses of hosts. When dnsServer is not
// empty (a host or host:port, port 53 by default) lookups go to that server
// through the pure Go resolver; otherwise the system resolver is used. Each
// lookup is bounded by timeout. Hosts that fail to resolve are logged and
// skipped.
func IPs(hosts []string, dnsServer string, timeout time.Duration, logger *slog.Logger) []string {
	resolver := net.DefaultResolver
	if dnsServer != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, withPort(dnsServer))
			},
		}
	}

	var ips []string

	for _, host := range hosts {
		if host == "" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		addrs, err := resolver.LookupHost(ctx, host)

		cancel()

		if err != nil {
			logger.Warn("failed to resolve host", "host", host, "error", err)

			continue
		}

		for _, addr := range addrs {
			if !slices.Contains(ips, addr) {
				ips = append(ips, addr)
			}
		}
	}

	return ips
}

// withPort appends port 53 to a DNS server address that has no port.
func withPort(server string) string {
	if _, _, err := net.SplitHostPort(server); err == nil {
		return server
	}

	return net.JoinHostPort(server, "53")
}
