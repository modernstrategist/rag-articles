//go:build otel

package apm

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// SpanExporter converts finished spans into APM request metrics for the aggregator.
type SpanExporter struct {
	agg         *Aggregator
	application string
	instance    string
}

// NewSpanExporter creates an OpenTelemetry span exporter backed by the aggregator.
func NewSpanExporter(appID, instanceID string, agg *Aggregator) *SpanExporter {
	if agg == nil {
		agg = DefaultAggregator()
	}
	return &SpanExporter{agg: agg, application: appID, instance: instanceID}
}

// ExportSpans is invoked by the OTEL SDK when spans complete.
func (e *SpanExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	for _, span := range spans {
		attrs := span.Attributes()
		endpoint := attributeValue(attrs, semconv.HTTPRouteKey, semconv.HTTPTargetKey)
		method := attributeValue(attrs, semconv.HTTPRequestMethodKey)
		status := int(attributeInt(attrs, semconv.HTTPStatusCodeKey))
		if status == 0 {
			status = httpStatusFromSpan(span)
		}

		e.agg.Record(RequestMetric{
			ApplicationID: e.application,
			InstanceID:    e.instance,
			Endpoint:      endpoint,
			Method:        method,
			StatusCode:    status,
			Duration:      span.EndTime().Sub(span.StartTime()),
			Timestamp:     span.EndTime(),
		})
	}
	return nil
}

// Shutdown is part of the SpanExporter interface.
func (e *SpanExporter) Shutdown(ctx context.Context) error { return nil }

func attributeValue(attrs []attribute.KeyValue, keys ...attribute.Key) string {
	for _, key := range keys {
		for _, kv := range attrs {
			if kv.Key == key {
				return kv.Value.AsString()
			}
		}
	}
	return ""
}

func attributeInt(attrs []attribute.KeyValue, key attribute.Key) int64 {
	for _, kv := range attrs {
		if kv.Key == key {
			return kv.Value.AsInt64()
		}
	}
	return 0
}

func httpStatusFromSpan(span trace.ReadOnlySpan) int {
	code := span.Status().Code
	if code == codes.Error {
		return 500
	}
	return 200
}

// BatchSpanProcessor configures a span processor that forwards completed spans to the aggregator.
func (e *SpanExporter) BatchSpanProcessor() trace.SpanProcessor {
	return trace.NewBatchSpanProcessor(e, trace.WithMaxExportBatchSize(256), trace.WithBatchTimeout(2*time.Second))
}
