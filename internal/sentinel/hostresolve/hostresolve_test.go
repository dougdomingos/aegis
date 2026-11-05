package hostresolve_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"slices"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/sentinel/hostresolve"
	"github.com/miekg/dns"
)

func TestIPs_IPLiteralHosts_ReturnsDeduplicatedAddresses(t *testing.T) {
	// Arrange
	logger := discardLogger()
	hosts := []string{"10.0.0.1", "10.0.0.1", "", "10.0.0.2"}

	// Act
	got := hostresolve.IPs(hosts, "", time.Second, logger)

	// Assert
	want := []string{"10.0.0.1", "10.0.0.2"}
	if !slices.Equal(got, want) {
		t.Fatalf("IPs() = %v, want %v", got, want)
	}
}

func TestIPs_UnresolvableHost_SkipsHost(t *testing.T) {
	// Arrange
	logger := discardLogger()
	hosts := []string{"nonexistent.invalid", "10.0.0.3"}

	// Act: the DNS server address refuses connections, so the lookup fails fast.
	got := hostresolve.IPs(hosts, "127.0.0.1:1", 500*time.Millisecond, logger)

	// Assert
	want := []string{"10.0.0.3"}
	if !slices.Equal(got, want) {
		t.Fatalf("IPs() = %v, want %v", got, want)
	}
}

func TestResolver_WithUpstream_ResolvesThroughUpstream(t *testing.T) {
	// Arrange
	upstream := startFakeDNS(t, "orchestrator.test.", "10.9.8.7")
	resolver := hostresolve.Resolver(upstream)

	// Act
	got, err := resolver.LookupHost(context.Background(), "orchestrator.test")

	// Assert
	if err != nil {
		t.Fatalf("LookupHost() error = %v, want nil", err)
	}

	if !slices.Equal(got, []string{"10.9.8.7"}) {
		t.Fatalf("LookupHost() = %v, want [10.9.8.7]", got)
	}
}

func TestResolver_WithoutUpstream_ReturnsSystemResolver(t *testing.T) {
	// Act
	got := hostresolve.Resolver("")

	// Assert
	if got != net.DefaultResolver {
		t.Errorf("Resolver(\"\") = %v, want the system resolver", got)
	}
}

// startFakeDNS serves a UDP DNS server on a loopback port that answers A
// queries for name with ip, and returns its address.
func startFakeDNS(t *testing.T, name, ip string) string {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for fake DNS: %v", err)
	}

	server := &dns.Server{PacketConn: conn, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(req)

		if req.Question[0].Name == name && req.Question[0].Qtype == dns.TypeA {
			reply.Answer = append(reply.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(ip),
			})
		}

		_ = w.WriteMsg(reply)
	})}

	go func() { _ = server.ActivateAndServe() }()

	t.Cleanup(func() { _ = server.Shutdown() })

	return conn.LocalAddr().String()
}

// discardLogger returns a logger that drops every record.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
