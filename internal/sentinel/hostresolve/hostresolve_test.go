package hostresolve_test

import (
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/sentinel/hostresolve"
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

// discardLogger returns a logger that drops every record.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
