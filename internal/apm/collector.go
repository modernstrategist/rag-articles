package apm

import (
	"context"
	"encoding/json"
	"time"

	"github.com/example/aea/internal/collector"
)

// Collector flushes aggregated APM metrics into the telemetry pipeline.
type Collector struct {
	agg      *Aggregator
	interval time.Duration
}

// NewCollector wires an Aggregator into the collector framework.
func NewCollector(agg *Aggregator, interval time.Duration) *Collector {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Collector{agg: agg, interval: interval}
}

func (c *Collector) Name() string {
	return "apm_metrics"
}

func (c *Collector) Interval() int64 {
	return c.interval.Milliseconds()
}

func (c *Collector) Collect(ctx context.Context) (collector.Payload, error) {
	now := time.Now()
	metrics := c.agg.Flush(now)
	if len(metrics) == 0 {
		return collector.Payload{}, collector.ErrNoData
	}

	envelope := MetricsEnvelope{
		Kind:              "apm_metrics",
		GeneratedAtMs:     now.UnixMilli(),
		ResolutionSeconds: int64(c.agg.Window().Seconds()),
		Metrics:           metrics,
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return collector.Payload{}, err
	}

	return collector.Payload{
		Name:      c.Name(),
		Timestamp: now.UnixMilli(),
		Data:      data,
	}, nil
}
