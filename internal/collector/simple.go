package collector

import (
	"context"
	"time"
)

// FuncCollector is a lightweight collector backed by a function.
type FuncCollector struct {
	name     string
	interval int64
	fn       func(ctx context.Context) (Payload, error)
}

func NewFuncCollector(name string, intervalMS int64, fn func(ctx context.Context) (Payload, error)) FuncCollector {
	return FuncCollector{name: name, interval: intervalMS, fn: fn}
}

func (f FuncCollector) Name() string    { return f.name }
func (f FuncCollector) Interval() int64 { return f.interval }

func (f FuncCollector) Collect(ctx context.Context) (Payload, error) {
	payload, err := f.fn(ctx)
	if err != nil {
		return Payload{}, err
	}
	if payload.Timestamp == 0 {
		payload.Timestamp = time.Now().UnixMilli()
	}
	if payload.Name == "" {
		payload.Name = f.name
	}
	return payload, nil
}
