//go:build ignore

package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "time"

    "github.com/example/aea/internal/apm"
)

// Demonstrates wiring AEA APM instrumentation into a simple net/http API service.
func main() {
    agg := apm.DefaultAggregator()
    appID := "shopping-service"
    instanceID := os.Getenv("HOSTNAME")

    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })
    mux.HandleFunc("/checkout", func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(37 * time.Millisecond)
        w.WriteHeader(http.StatusCreated)
        _, _ = w.Write([]byte("order accepted"))
    })

    handler := apm.HTTPMiddleware(appID, instanceID, agg)(mux)

    srv := &http.Server{
        Addr:    ":8080",
        Handler: handler,
    }

    go func() {
        ticker := time.NewTicker(time.Minute)
        for now := range ticker.C {
            metrics := agg.Flush(now)
            if len(metrics) == 0 {
                continue
            }
            payload := apm.MetricsEnvelope{
                Kind:              "apm_metrics",
                GeneratedAtMs:     now.UnixMilli(),
                ResolutionSeconds: int64(agg.Window().Seconds()),
                Metrics:           metrics,
            }
            log.Printf("payload to send: %+v", payload)
        }
    }()

    log.Printf("listening on %s", srv.Addr)
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("server error: %v", err)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = srv.Shutdown(ctx)
}

// To use OpenTelemetry instead of the custom middleware:
//
//  exporter := apm.NewSpanExporter(appID, instanceID, agg)
//  bsp := exporter.BatchSpanProcessor()
//  tp := trace.NewTracerProvider(trace.WithSpanProcessor(bsp))
//  otel.SetTracerProvider(tp)
//  // instrument your handlers with otelhttp.NewHandler(...)
//
// Spans will be exported to the AEA aggregator without leaving the host.
