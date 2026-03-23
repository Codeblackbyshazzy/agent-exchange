package telemetry

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMiddleware wraps an http.Handler with OpenTelemetry tracing.
// It creates a span for each request with attributes: method, path, status_code.
func HTTPMiddleware(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		tracer := otel.Tracer(serviceName)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from incoming request headers
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPMethodKey.String(r.Method),
					semconv.HTTPTargetKey.String(r.URL.Path),
				),
			)
			defer span.End()

			// Wrap the ResponseWriter to capture the status code
			sw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Serve with the traced context
			next.ServeHTTP(sw, r.WithContext(ctx))

			// Set status code attribute after the handler has run
			span.SetAttributes(attribute.Int("http.status_code", sw.statusCode))

			// Propagate trace context to outgoing response headers
			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(w.Header()))
		})
	}
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

func (sw *statusWriter) Write(b []byte) (int, error) {
	if !sw.wroteHeader {
		sw.wroteHeader = true
	}
	return sw.ResponseWriter.Write(b)
}
