package metrics_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dougdomingos.com/aegis/internal/orchestrator/metrics"
)

func TestPropagationMetrics_RecordPropagation_WithAckedStatus_ObservesHistogramAndCounter(t *testing.T) {
	m := metrics.NewPropagationMetrics()

	m.RecordPropagation("LCC1", "acked", 120*time.Millisecond)

	body := scrape(t, m)
	assertContains(t, body, `aegis_propagation_duration_seconds_count{group="LCC1"} 1`)
	assertContains(t, body, `aegis_propagations_total{group="LCC1",status="acked"} 1`)
}

func TestPropagationMetrics_RecordPropagation_WithFailedStatus_IncrementsOnlyCounter(t *testing.T) {
	m := metrics.NewPropagationMetrics()

	m.RecordPropagation("LCC1", "failed", 0)

	body := scrape(t, m)
	assertContains(t, body, `aegis_propagations_total{group="LCC1",status="failed"} 1`)
	if strings.Contains(body, `aegis_propagation_duration_seconds_count{group="LCC1"}`) {
		t.Errorf("expected no histogram observation for a failed propagation, got:\n%s", body)
	}
}

func TestPropagationMetrics_Handler_AfterRecording_ExposesSeriesPerGroup(t *testing.T) {
	m := metrics.NewPropagationMetrics()

	m.RecordPropagation("LCC1", "acked", 50*time.Millisecond)
	m.RecordPropagation("LCC2", "nacked", 0)

	body := scrape(t, m)
	assertContains(t, body, `group="LCC1"`)
	assertContains(t, body, `aegis_propagations_total{group="LCC2",status="nacked"} 1`)
}

// ============================================================================
// Helpers
// ============================================================================

// scrape serves the metrics handler once and returns the response body.
func scrape(t *testing.T, m *metrics.PropagationMetrics) string {
	t.Helper()

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))

	if rec.Code != 200 {
		t.Fatalf("expected status code 200, got %d", rec.Code)
	}

	return rec.Body.String()
}

// assertContains fails the test when the scraped body lacks the fragment.
func assertContains(t *testing.T, body, fragment string) {
	t.Helper()

	if !strings.Contains(body, fragment) {
		t.Errorf("expected body to contain %q, got:\n%s", fragment, body)
	}
}
