package apm

import (
	"sort"
	"sync"
	"time"
)

// Aggregator maintains rolling per-endpoint metrics and produces 1-minute windows.
type Aggregator struct {
	mu      sync.Mutex
	window  time.Duration
	buckets map[int64]map[string]*endpointAgg
}

type endpointAgg struct {
	ApplicationID string
	InstanceID    string
	Endpoint      string
	Method        string
	Throughput    int
	ErrorCount    int
	LatenciesMs   []float64
}

var defaultAgg *Aggregator
var defaultOnce sync.Once

// DefaultAggregator returns the singleton aggregator used by the agent runtime and instrumentation helpers.
func DefaultAggregator() *Aggregator {
	defaultOnce.Do(func() {
		defaultAgg = NewAggregator(time.Minute)
	})
	return defaultAgg
}

// NewAggregator constructs an Aggregator with the provided window size.
func NewAggregator(window time.Duration) *Aggregator {
	if window <= 0 {
		window = time.Minute
	}
	return &Aggregator{
		window:  window,
		buckets: make(map[int64]map[string]*endpointAgg),
	}
}

// Window returns the aggregation window size.
func (a *Aggregator) Window() time.Duration {
	return a.window
}

// Record ingests a single request metric observation.
func (a *Aggregator) Record(m RequestMetric) {
	bucketStart := m.Timestamp.Truncate(a.window).UnixMilli()
	key := m.ApplicationID + "|" + m.InstanceID + "|" + m.Method + "|" + m.Endpoint

	a.mu.Lock()
	defer a.mu.Unlock()

	entryMap, ok := a.buckets[bucketStart]
	if !ok {
		entryMap = make(map[string]*endpointAgg)
		a.buckets[bucketStart] = entryMap
	}

	agg, ok := entryMap[key]
	if !ok {
		agg = &endpointAgg{
			ApplicationID: m.ApplicationID,
			InstanceID:    m.InstanceID,
			Endpoint:      m.Endpoint,
			Method:        m.Method,
		}
		entryMap[key] = agg
	}

	agg.Throughput++
	if m.StatusCode >= 500 {
		agg.ErrorCount++
	}
	agg.LatenciesMs = append(agg.LatenciesMs, float64(m.Duration.Microseconds())/1000.0)
}

// Flush returns aggregated metrics for complete windows prior to the provided timestamp.
func (a *Aggregator) Flush(now time.Time) []EndpointMetrics {
	cutoff := now.Truncate(a.window).Add(-a.window).UnixMilli()

	a.mu.Lock()
	defer a.mu.Unlock()

	var results []EndpointMetrics
	for start, entries := range a.buckets {
		if start > cutoff {
			continue
		}

		for _, agg := range entries {
			metrics := EndpointMetrics{
				ApplicationID: agg.ApplicationID,
				InstanceID:    agg.InstanceID,
				Endpoint:      agg.Endpoint,
				Method:        agg.Method,
				WindowStartMs: start,
				WindowEndMs:   start + a.window.Milliseconds(),
				Throughput:    agg.Throughput,
			}
			if agg.Throughput > 0 {
				metrics.ErrorRate = float64(agg.ErrorCount) / float64(agg.Throughput)
			}
			metrics.Latency = computeQuantiles(agg.LatenciesMs)
			results = append(results, metrics)
		}

		delete(a.buckets, start)
	}

	return results
}

func computeQuantiles(latencies []float64) LatencyQuantiles {
	if len(latencies) == 0 {
		return LatencyQuantiles{}
	}
	sorted := append([]float64(nil), latencies...)
	sort.Float64s(sorted)

	return LatencyQuantiles{
		P50: percentile(sorted, 0.50),
		P90: percentile(sorted, 0.90),
		P99: percentile(sorted, 0.99),
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}

	idx := p * float64(len(sorted)-1)
	lower := int(idx)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[lower]
	}
	weight := idx - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}
