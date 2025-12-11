package collector

import (
	"context"
	"errors"
)

// ErrNoData indicates a collector ran successfully but had nothing to emit.
var ErrNoData = errors.New("collector had no data to emit")

// Payload represents a collection result that can be enqueued for transport.
type Payload struct {
	Name      string
	Timestamp int64
	Data      []byte
}

// Collector defines the behavior every Atlas Edge Agent collector must implement.
type Collector interface {
	// Name is a stable identifier for the collector (e.g., "host_metrics").
	Name() string
	// Interval specifies how often the collector should run.
	Interval() int64 // milliseconds to avoid allocations
	// Collect executes the collector logic and returns a Payload ready for transport.
	Collect(ctx context.Context) (Payload, error)
}
