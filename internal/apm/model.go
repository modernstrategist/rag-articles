package apm

import "time"

// RequestMetric represents a single request observation produced by instrumentation.
type RequestMetric struct {
	ApplicationID string        `json:"application_id"`
	InstanceID    string        `json:"instance_id"`
	Endpoint      string        `json:"endpoint"`
	Method        string        `json:"method"`
	StatusCode    int           `json:"status_code"`
	Duration      time.Duration `json:"duration"`
	Timestamp     time.Time     `json:"timestamp"`
}

// LatencyQuantiles captures latency summaries in milliseconds.
type LatencyQuantiles struct {
	P50 float64 `json:"p50_ms"`
	P90 float64 `json:"p90_ms"`
	P99 float64 `json:"p99_ms"`
}

// EndpointMetrics aggregates metrics for a particular endpoint within a time bucket.
type EndpointMetrics struct {
	ApplicationID string           `json:"application_id"`
	InstanceID    string           `json:"instance_id"`
	Endpoint      string           `json:"endpoint"`
	Method        string           `json:"method"`
	WindowStartMs int64            `json:"window_start_ms"`
	WindowEndMs   int64            `json:"window_end_ms"`
	Throughput    int              `json:"throughput"`
	ErrorRate     float64          `json:"error_rate"`
	Latency       LatencyQuantiles `json:"latency"`
}

// MetricsEnvelope groups aggregated metrics into a payload sent to the collector.
type MetricsEnvelope struct {
	Kind              string            `json:"kind"`
	GeneratedAtMs     int64             `json:"generated_at_ms"`
	ResolutionSeconds int64             `json:"resolution_seconds"`
	Metrics           []EndpointMetrics `json:"metrics"`
}
