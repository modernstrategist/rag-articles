package runtime

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/example/aea/internal/collector"
	"github.com/example/aea/internal/config"
	"github.com/example/aea/internal/sender"
	"github.com/example/aea/internal/telemetry"
)

// Runtime coordinates collectors, telemetry queue, and sending loop.
type Runtime struct {
	collectors []collector.Collector
	queue      *telemetry.Queue
	sender     sender.Sender
	cfg        config.EffectiveConfig
	batchSize  int
	batchWait  time.Duration
}

func NewRuntime(c []collector.Collector, q *telemetry.Queue, s sender.Sender, cfg config.EffectiveConfig) *Runtime {
	batchSize := cfg.Local.BatchSize
	if batchSize <= 0 {
		batchSize = 64
	}
	batchWait := time.Duration(cfg.Local.BatchIntervalMS) * time.Millisecond
	if batchWait <= 0 {
		batchWait = 2 * time.Second
	}

	return &Runtime{
		collectors: c,
		queue:      q,
		sender:     s,
		cfg:        cfg,
		batchSize:  batchSize,
		batchWait:  batchWait,
	}
}

// Run starts collector scheduling and the send loop until the context is canceled.
func (r *Runtime) Run(ctx context.Context) error {
	var wg sync.WaitGroup

	for _, c := range r.collectors {
		wg.Add(1)
		go func(col collector.Collector) {
			defer wg.Done()
			r.runCollector(ctx, col)
		}(c)
	}

	sendErrs := make(chan error, 1)
	go func() {
		sendErrs <- r.runSender(ctx)
	}()

	select {
	case <-ctx.Done():
		wg.Wait()
		return ctx.Err()
	case err := <-sendErrs:
		return err
	}
}

func (r *Runtime) runCollector(ctx context.Context, c collector.Collector) {
	interval := r.cfg.ResolveWindow(c.Name(), c.Interval())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			payload, err := c.Collect(ctx)
			if err != nil {
				if errors.Is(err, collector.ErrNoData) {
					continue
				}
				log.Printf("collector %s failed: %v", c.Name(), err)
				continue
			}
			if err := r.queue.Enqueue(ctx, payload); err != nil {
				log.Printf("queue drop (%s): %v", c.Name(), err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *Runtime) runSender(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			batch, err := r.queue.DequeueBatch(ctx, r.batchSize, r.batchWait)
			if err != nil {
				if err == ctx.Err() {
					return err
				}
				log.Printf("dequeue error: %v", err)
				continue
			}
			if len(batch) == 0 {
				continue
			}
			if err := r.sender.Send(ctx, batch); err != nil {
				log.Printf("send failed: %v", err)
			}
		}
	}
}
