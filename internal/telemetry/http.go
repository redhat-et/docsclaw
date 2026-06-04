package telemetry

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMiddleware wraps an http.Handler to create a root span for each
// request and extract incoming W3C traceparent headers for distributed
// tracing. When OTel is disabled (no-op TracerProvider), this adds
// negligible overhead.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract any incoming trace context (e.g. from an A2A caller).
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		spanName := fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path)
		ctx, span := otel.Tracer(TracerName).Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", r.URL.Path),
			),
		)
		defer span.End()

		sw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(sw, r.WithContext(ctx))

		span.SetAttributes(attribute.Int("http.status_code", sw.statusCode))
	})
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wroteHeader {
		sw.statusCode = code
		sw.wroteHeader = true
	}
	sw.ResponseWriter.WriteHeader(code)
}
