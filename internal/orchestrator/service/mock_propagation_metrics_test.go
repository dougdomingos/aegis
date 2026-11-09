package service_test

import (
	"sync"
	"time"
)

// MetricCall records one RecordPropagation invocation.
type MetricCall struct {
	Group   string
	Status  string
	Latency time.Duration
}

// MockPropagationMetrics provides a thread-safe recorder implementing
// service.PropagationMetrics, since the propagation worker calls it from its
// own goroutine.
type MockPropagationMetrics struct {
	mu    sync.Mutex
	calls []MetricCall
}

// RecordPropagation stores the call for later inspection.
func (mock *MockPropagationMetrics) RecordPropagation(group, status string, latency time.Duration) {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	mock.calls = append(mock.calls, MetricCall{Group: group, Status: status, Latency: latency})
}

// Calls returns a copy of the recorded calls.
func (mock *MockPropagationMetrics) Calls() []MetricCall {
	mock.mu.Lock()
	defer mock.mu.Unlock()

	return append([]MetricCall(nil), mock.calls...)
}
