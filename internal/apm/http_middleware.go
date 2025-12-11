package apm

import (
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// HTTPMiddleware captures request timing and status information for net/http handlers.
func HTTPMiddleware(appID, instanceID string, agg *Aggregator) func(http.Handler) http.Handler {
	if agg == nil {
		agg = DefaultAggregator()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(sr, r)
			duration := time.Since(start)

			agg.Record(RequestMetric{
				ApplicationID: appID,
				InstanceID:    instanceID,
				Endpoint:      r.URL.Path,
				Method:        r.Method,
				StatusCode:    sr.status,
				Duration:      duration,
				Timestamp:     time.Now(),
			})
		})
	}
}
