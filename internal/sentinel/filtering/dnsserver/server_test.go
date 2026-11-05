package dnsserver_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/sentinel/filtering"
	"dougdomingos.com/aegis/internal/sentinel/filtering/dnsserver"
)

// stubAnswerIP is the address the stub upstream answers every A query with.
const stubAnswerIP = "192.0.2.10"

func TestServer_Update_WithWhitelist_ResolvesListedDomainAndRefusesOthers(t *testing.T) {
	// Arrange
	server, address := arrangeServerTest(t, nil)
	server.Update(snapshotFor(propagation.PolicyTypeWhitelist, "example.com"))

	// Act
	listed := query(t, address, "example.com")
	other := query(t, address, "other.org")

	// Assert
	assertResolved(t, listed, "example.com")
	assertRefused(t, other, "other.org")
}

func TestServer_Update_WithWhitelist_MatchesSubdomainsOfListedDomain(t *testing.T) {
	// Arrange
	server, address := arrangeServerTest(t, nil)
	server.Update(snapshotFor(propagation.PolicyTypeWhitelist, "example.com"))

	cases := map[string]struct {
		name     string
		resolved bool
	}{
		"direct subdomain":  {name: "www.example.com", resolved: true},
		"nested subdomain":  {name: "a.b.example.com", resolved: true},
		"mixed case":        {name: "WWW.Example.COM", resolved: true},
		"suffix lookalike":  {name: "badexample.com", resolved: false},
		"listed as a label": {name: "example.com.evil.org", resolved: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Act
			response := query(t, address, tc.name)

			// Assert
			if tc.resolved {
				assertResolved(t, response, tc.name)
			} else {
				assertRefused(t, response, tc.name)
			}
		})
	}
}

func TestServer_Update_WithWhitelist_ReportsResolvedIPsToCallback(t *testing.T) {
	// Arrange
	recorder := &resolveRecorder{}
	server, address := arrangeServerTest(t, recorder.record)
	server.Update(snapshotFor(propagation.PolicyTypeWhitelist, "example.com"))

	// Act
	query(t, address, "www.example.com")
	query(t, address, "other.org")

	// Assert
	reported := recorder.snapshot()
	if len(reported) != 1 || reported[0] != stubAnswerIP {
		t.Errorf("onResolve received %v, want exactly [%s]", reported, stubAnswerIP)
	}
}

func TestServer_Update_WithFailingCallback_StillAnswersClient(t *testing.T) {
	// Arrange
	recorder := &resolveRecorder{err: errors.New("set missing")}
	server, address := arrangeServerTest(t, recorder.record)
	server.Update(snapshotFor(propagation.PolicyTypeWhitelist, "example.com"))

	// Act
	response := query(t, address, "example.com")

	// Assert
	assertResolved(t, response, "example.com")
	if reported := recorder.snapshot(); len(reported) != 1 {
		t.Errorf("onResolve called %d times, want 1", len(reported))
	}
}

func TestServer_Update_WithBlacklist_RefusesListedDomainAndForwardsOthers(t *testing.T) {
	// Arrange
	server, address := arrangeServerTest(t, nil)
	server.Update(snapshotFor(propagation.PolicyTypeBlacklist, "example.com"))

	// Act
	listed := query(t, address, "example.com")
	subdomain := query(t, address, "www.example.com")
	other := query(t, address, "other.org")

	// Assert
	assertRefused(t, listed, "example.com")
	assertRefused(t, subdomain, "www.example.com")
	assertResolved(t, other, "other.org")
}

func TestServer_Update_WithBlacklist_NeverReportsResolvedIPs(t *testing.T) {
	// Arrange
	recorder := &resolveRecorder{}
	server, address := arrangeServerTest(t, recorder.record)
	server.Update(snapshotFor(propagation.PolicyTypeBlacklist, "example.com"))

	// Act
	response := query(t, address, "other.org")
	query(t, address, "example.com")

	// Assert
	assertResolved(t, response, "other.org")
	if reported := recorder.snapshot(); len(reported) != 0 {
		t.Errorf("onResolve received %v under BLACKLIST, want no calls", reported)
	}
}

func TestServer_Clear_ResolvesNothingAfterwards(t *testing.T) {
	// Arrange
	recorder := &resolveRecorder{}
	server, address := arrangeServerTest(t, recorder.record)
	server.Update(snapshotFor(propagation.PolicyTypeWhitelist, "example.com"))

	// Act
	server.Clear()
	previouslyRefused := query(t, address, "other.org")
	previouslyListed := query(t, address, "example.com")

	// Assert
	assertResolved(t, previouslyRefused, "other.org")
	assertResolved(t, previouslyListed, "example.com")
	if reported := recorder.snapshot(); len(reported) != 0 {
		t.Errorf("onResolve received %v after Clear, want no calls", reported)
	}
}

func TestServer_Update_WithConcurrentQueries_NeverServesTornState(t *testing.T) {
	// Arrange
	// Under both rulesets "allowed.test" resolves and "denied.test" is refused.
	// A torn state (one ruleset's policy type with the other's domains) flips
	// one of those answers.
	server, address := arrangeServerTest(t, nil)
	whitelist := snapshotFor(propagation.PolicyTypeWhitelist, "allowed.test")
	blacklist := snapshotFor(propagation.PolicyTypeBlacklist, "denied.test")
	server.Update(whitelist)

	const workers = 8
	const queriesPerWorker = 40

	stop := make(chan struct{})
	updaterDone := make(chan struct{})
	go func() {
		defer close(updaterDone)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if i%2 == 0 {
				server.Update(blacklist)
			} else {
				server.Update(whitelist)
			}
		}
	}()

	// Act
	var wg sync.WaitGroup
	failures := make(chan string, workers*queriesPerWorker*2)
	for range workers {
		wg.Go(func() {
			for range queriesPerWorker {
				if response := exchange(address, "allowed.test"); !isResolved(response) {
					failures <- "allowed.test was not resolved"
				}
				if response := exchange(address, "denied.test"); !isRefused(response) {
					failures <- "denied.test was not refused"
				}
			}
		})
	}
	wg.Wait()
	close(stop)
	<-updaterDone
	close(failures)

	// Assert
	for failure := range failures {
		t.Errorf("inconsistent response under concurrent updates: %s", failure)
	}
}

func TestServer_Start_WithoutUpstream_ReturnsError(t *testing.T) {
	// Arrange
	server := dnsserver.NewServer(freeLoopbackAddress(t), nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Act
	err := server.Start(ctx)

	// Assert
	if !errors.Is(err, dnsserver.ErrNoUpstream) {
		t.Errorf("Start() error = %v, want %v", err, dnsserver.ErrNoUpstream)
	}
}

func TestNormalizeUpstream_WithVariousForms_AppendsDefaultPort(t *testing.T) {
	cases := map[string]struct {
		input string
		want  string
	}{
		"ipv4 without port": {input: "192.0.2.1", want: "192.0.2.1:53"},
		"ipv4 with port":    {input: "192.0.2.1:5353", want: "192.0.2.1:5353"},
		"ipv6 without port": {input: "2001:db8::1", want: "[2001:db8::1]:53"},
		"ipv6 with port":    {input: "[2001:db8::1]:5353", want: "[2001:db8::1]:5353"},
		"surrounding space": {input: " 192.0.2.1 ", want: "192.0.2.1:53"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Act
			got := dnsserver.NormalizeUpstreamForTest(tc.input)

			// Assert
			if got != tc.want {
				t.Errorf("normalizeUpstream(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// resolveRecorder is an onResolve callback that records every reported IP and
// returns a configurable error.
type resolveRecorder struct {
	mu  sync.Mutex
	ips []string
	err error
}

// record stores ip and returns the recorder's configured error.
func (r *resolveRecorder) record(ip string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ips = append(r.ips, ip)
	return r.err
}

// snapshot returns a copy of the IPs recorded so far.
func (r *resolveRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ips...)
}

// arrangeServerTest starts a stub upstream and an Aegis server forwarding to
// it, both on ephemeral loopback ports, and stops them when the test ends. It
// returns the server and its listen address.
func arrangeServerTest(t *testing.T, onResolve func(ip string) error) (*dnsserver.Server, string) {
	t.Helper()

	upstream := startStubUpstream(t)
	address := freeLoopbackAddress(t)
	server := dnsserver.NewServer(address, []string{upstream}, onResolve)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Start() returned %v after cancellation, want nil", err)
			}
		case <-time.After(10 * time.Second):
			t.Errorf("Start() did not return after cancellation")
		}
	})

	waitUntilServing(t, address, done)
	return server, address
}

// startStubUpstream runs a UDP DNS server on an ephemeral loopback port that
// answers every A query with stubAnswerIP, and returns its address.
func startStubUpstream(t *testing.T) string {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for stub upstream: %v", err)
	}

	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		response := new(dns.Msg)
		response.SetReply(r)
		for _, question := range r.Question {
			if question.Qtype != dns.TypeA {
				continue
			}
			response.Answer = append(response.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(stubAnswerIP),
			})
		}
		_ = w.WriteMsg(response)
	})

	stub := &dns.Server{PacketConn: conn, Handler: handler}
	go func() { _ = stub.ActivateAndServe() }()
	t.Cleanup(func() { _ = stub.Shutdown() })

	return conn.LocalAddr().String()
}

// freeLoopbackAddress reserves and releases an ephemeral loopback UDP port so
// the server can bind UDP and TCP on the same address.
func freeLoopbackAddress(t *testing.T) string {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a loopback port: %v", err)
	}
	address := conn.LocalAddr().String()
	_ = conn.Close()
	return address
}

// waitUntilServing polls address until the server answers a query, failing
// the test if Start exits early or the server never comes up.
func waitUntilServing(t *testing.T, address string, done <-chan error) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("Start() returned early: %v", err)
		default:
		}
		if exchange(address, "readiness.check") != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s never started answering", address)
}

// snapshotFor builds a snapshot of policyType with one DOMAIN rule per domain.
func snapshotFor(policyType string, domains ...string) filtering.PolicySnapshot {
	snapshot := filtering.PolicySnapshot{PolicyType: policyType}
	for _, domain := range domains {
		snapshot.Rules = append(snapshot.Rules, propagation.Rule{Type: "DOMAIN", Value: domain})
	}
	return snapshot
}

// query sends an A query for name to address and fails the test when no
// response arrives.
func query(t *testing.T, address, name string) *dns.Msg {
	t.Helper()

	response := exchange(address, name)
	if response == nil {
		t.Fatalf("no response for %s from %s", name, address)
	}
	return response
}

// exchange sends an A query for name to address, returning nil on failure.
func exchange(address, name string) *dns.Msg {
	message := new(dns.Msg)
	message.SetQuestion(dns.Fqdn(name), dns.TypeA)

	client := &dns.Client{Net: "udp", Timeout: 2 * time.Second}
	response, _, err := client.Exchange(message, address)
	if err != nil {
		return nil
	}
	return response
}

// isResolved reports whether response succeeded with the stub's answer IP.
func isResolved(response *dns.Msg) bool {
	if response == nil || response.Rcode != dns.RcodeSuccess {
		return false
	}
	for _, answer := range response.Answer {
		if record, ok := answer.(*dns.A); ok && record.A.String() == stubAnswerIP {
			return true
		}
	}
	return false
}

// isRefused reports whether response carries the REFUSED rcode.
func isRefused(response *dns.Msg) bool {
	return response != nil && response.Rcode == dns.RcodeRefused
}

// assertResolved fails the test unless response resolved name to the stub IP.
func assertResolved(t *testing.T, response *dns.Msg, name string) {
	t.Helper()

	if !isResolved(response) {
		t.Errorf("%s: rcode %s, answers %v; want NOERROR with %s",
			name, dns.RcodeToString[response.Rcode], response.Answer, stubAnswerIP)
	}
}

// assertRefused fails the test unless response refused name.
func assertRefused(t *testing.T, response *dns.Msg, name string) {
	t.Helper()

	if !isRefused(response) {
		t.Errorf("%s: rcode %s, want REFUSED", name, dns.RcodeToString[response.Rcode])
	}
}
