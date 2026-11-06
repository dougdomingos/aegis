// Package dnsserver embeds a CoreDNS resolver that enforces the DOMAIN rules
// of the current policy: under WHITELIST only listed domains (and their
// subdomains) are forwarded upstream, under BLACKLIST listed domains are
// refused and everything else is forwarded.
package dnsserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"

	coredns "github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/forward"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"

	"dougdomingos.com/aegis/internal/sentinel/filtering"
)

// defaultDNSPort is appended to upstream addresses given without a port.
const defaultDNSPort = "53"

// ErrNoUpstream is returned by Start when no upstream resolver is configured.
var ErrNoUpstream = errors.New("no upstream DNS servers configured")

// Server runs the embedded CoreDNS resolver enforcing DOMAIN rules.
type Server struct {
	// listenAddress is the host:port served over UDP and TCP.
	listenAddress string

	// upstream are the resolvers allowed queries are forwarded to.
	upstream []string

	// plugin is the Aegis filter holding the current rule set.
	plugin *filterPlugin
}

// NewServer returns a Server listening on listenAddress and forwarding allowed
// queries to upstream. onResolve, when non-nil, receives every A/AAAA answer
// IP of an allowed WHITELIST resolution; a failed callback is logged and the
// client is still answered. The server starts with no rules, forwarding every
// query.
func NewServer(listenAddress string, upstream []string, onResolve func(ip string) error) *Server {
	filter := &filterPlugin{onResolve: onResolve, logger: slog.Default()}
	filter.rules.Store(&ruleSet{})

	return &Server{
		listenAddress: listenAddress,
		upstream:      upstream,
		plugin:        filter,
	}
}

// Start serves DNS on the listen address over UDP and TCP and blocks until ctx
// is cancelled, returning nil, or until serving fails. It must be called once.
func (s *Server) Start(ctx context.Context) error {
	if len(s.upstream) == 0 {
		return ErrNoUpstream
	}

	forwarder := forward.New()
	for _, upstream := range s.upstream {
		forwarder.SetProxy(proxy.NewProxy(forwarder.Name(), normalizeUpstream(upstream), transport.DNS))
	}
	defer func() { _ = forwarder.OnShutdown() }()
	s.plugin.next = forwarder

	server, err := coredns.NewServer(transport.DNS+"://"+s.listenAddress, []*coredns.Config{{
		Zone: ".",
		Plugin: []plugin.Plugin{
			func(plugin.Handler) plugin.Handler { return s.plugin },
		},
	}})
	if err != nil {
		return fmt.Errorf("failed to build DNS server: %w", err)
	}

	packetConn, err := net.ListenPacket("udp", s.listenAddress)
	if err != nil {
		return fmt.Errorf("failed to listen on udp %s: %w", s.listenAddress, err)
	}
	listener, err := net.Listen("tcp", s.listenAddress)
	if err != nil {
		_ = packetConn.Close()
		return fmt.Errorf("failed to listen on tcp %s: %w", s.listenAddress, err)
	}

	served := make(chan error, 2)
	go func() { served <- server.ServePacket(packetConn) }()
	go func() { served <- server.Serve(listener) }()

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-served:
	}

	// Stop shuts down the transports that are already serving; closing the
	// sockets also releases one whose serve loop had not started yet.
	_ = server.Stop()
	_ = packetConn.Close()
	_ = listener.Close()

	if serveErr != nil {
		return fmt.Errorf("DNS server stopped: %w", serveErr)
	}
	return nil
}

// Update atomically replaces the enforced rules with the DOMAIN rules of
// snapshot; queries in flight finish under the rule set they started with.
func (s *Server) Update(snapshot filtering.PolicySnapshot) {
	s.plugin.rules.Store(compileRuleSet(snapshot))
}

// Clear atomically drops every rule, so all queries are forwarded and no
// resolved addresses are reported.
func (s *Server) Clear() {
	s.plugin.rules.Store(&ruleSet{})
}

// normalizeUpstream returns upstream as host:port, appending the default DNS
// port when none is given.
func normalizeUpstream(upstream string) string {
	upstream = strings.TrimSpace(upstream)
	if _, _, err := net.SplitHostPort(upstream); err == nil {
		return upstream
	}
	return net.JoinHostPort(strings.Trim(upstream, "[]"), defaultDNSPort)
}
