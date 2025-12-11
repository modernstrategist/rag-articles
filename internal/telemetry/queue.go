package telemetry

import (
	"context"
	"errors"
	"time"

	"github.com/example/aea/internal/collector"
)

var ErrQueueFull = errors.New("telemetry queue backpressure: capacity reached")

// Queue is a non-blocking channel-based queue with bounded capacity.
type Queue struct {
	buf chan collector.Payload
}

func NewQueue(size int) *Queue {
	return &Queue{buf: make(chan collector.Payload, size)}
}

// Enqueue inserts a payload or returns an error when backpressure is hit.
func (q *Queue) Enqueue(ctx context.Context, p collector.Payload) error {
	select {
	case q.buf <- p:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrQueueFull
	}
}

// DequeueBatch retrieves up to batchSize items, waiting up to timeout for at least one.
func (q *Queue) DequeueBatch(ctx context.Context, batchSize int, timeout time.Duration) ([]collector.Payload, error) {
	batch := make([]collector.Payload, 0, batchSize)

	// Initial wait for first item or timeout.
	select {
	case item := <-q.buf:
		batch = append(batch, item)
	case <-time.After(timeout):
		return batch, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	for len(batch) < batchSize {
		select {
		case item := <-q.buf:
			batch = append(batch, item)
		default:
			return batch, nil
		}
	}

	return batch, nil
}

// Length returns the current buffer length.
func (q *Queue) Length() int {
	return len(q.buf)
}
