package service

import "time"

// PropagationMetrics decouples the propagation service from the metrics
// backend: recording is best effort, in memory, and never fails a propagation.
type PropagationMetrics interface {

	// RecordPropagation counts one package propagation outcome for the group.
	// The status is one of "acked", "nacked" or "failed"; the latency is the
	// time between sending and acknowledging, meaningful only for "acked".
	RecordPropagation(group, status string, latency time.Duration)
}
