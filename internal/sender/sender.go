package sender

import (
	"context"

	"github.com/example/aea/internal/collector"
)

// Sender transports batches of collected payloads to the Collector backend.
type Sender interface {
	Send(ctx context.Context, batch []collector.Payload) error
}
