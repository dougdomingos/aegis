//go:build e2e

package e2e

import (
	"fmt"
	"testing"
)

func TestSecretScan_SampleOutput_WithSecrets_FailsAssertions(t *testing.T) {
	tests := map[string]struct {
		secret string
	}{
		"valid lab key":  {secret: validLabKeyHex},
		"wrong lab key":  {secret: wrongLabKeyHex},
		"session secret": {secret: sessionSecret},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			output := fmt.Sprintf("time=... level=INFO msg=operation arg=%s", tc.secret)
			if hits := secretHits(output); len(hits) != 1 {
				t.Errorf("secretHits() on output containing %s = %d hits, want 1", name, len(hits))
			}
		})
	}

	clean := "time=... level=INFO msg=machine authenticated machine=lcc1-001"
	if hits := secretHits(clean); len(hits) != 0 {
		t.Errorf("secretHits() on clean output = %d hits, want 0", len(hits))
	}
}
