package telemetry

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// InitTracer sets up an OTLP exporter for distributed tracing.
// Returns a shutdown function to flush spans on exit.
// If otlpEndpoint is empty, tracing is disabled and a no-op shutdown is returned.
func InitTracer(ctx context.Context, serviceName, otlpEndpoint string) (func(context.Context) error, error) {
	if otlpEndpoint == "" {
		// Tracing disabled; set up propagation only so trace context headers
		// are still forwarded between services.
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(otlpEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// InitMeter sets up a Prometheus exporter for metrics.
// Returns an http.Handler for the /metrics endpoint.
func InitMeter(serviceName string) (http.Handler, error) {
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	// Build a dedicated Prometheus registry so that the default process/go
	// collectors are excluded and we only expose OpenTelemetry metrics.
	reg := promclient.NewRegistry()

	promExporter, err := prometheus.New(prometheus.WithRegisterer(reg))
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(promExporter),
		sdkmetric.WithResource(res),
	)

	otel.SetMeterProvider(mp)

	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), nil
}
