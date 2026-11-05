package dnsserver

// NormalizeUpstreamForTest exposes normalizeUpstream to the external test
// package.
func NormalizeUpstreamForTest(upstream string) string {
	return normalizeUpstream(upstream)
}
