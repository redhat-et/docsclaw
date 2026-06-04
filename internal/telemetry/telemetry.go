package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	TracerName = "github.com/redhat-et/docsclaw"
)

// Config controls OTel initialization.
type Config struct {
	Enabled           bool
	CollectorEndpoint string // OTLP gRPC endpoint (e.g. "localhost:4317")
	StdoutExporter    bool   // write spans as JSON to stdout
	ServiceVersion    string
}

// Init sets up the global TracerProvider and W3C trace-context propagator.
// Returns a shutdown function that flushes pending spans.
// When cfg.Enabled is false, returns a no-op shutdown — the global
// TracerProvider stays as the built-in no-op (zero overhead).
func Init(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }
	if !cfg.Enabled {
		return noop, nil
	}

	hostname, _ := os.Hostname()
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("docsclaw"),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.ServiceInstanceID(hostname),
		),
	)
	if err != nil {
		return noop, fmt.Errorf("creating OTel resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}

	// OTLP gRPC exporter — sends to Jaeger, Tempo, or any OTLP collector.
	// When CollectorEndpoint is empty, otlptracegrpc falls back to the
	// standard OTEL_EXPORTER_OTLP_ENDPOINT env var (default: localhost:4317).
	if cfg.CollectorEndpoint != "" || os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		grpcOpts := []otlptracegrpc.Option{}
		if cfg.CollectorEndpoint != "" {
			grpcOpts = append(grpcOpts, otlptracegrpc.WithEndpoint(cfg.CollectorEndpoint))
		}
		// Default to TLS. Set OTEL_EXPORTER_OTLP_INSECURE=true for
		// plaintext (local dev with a collector on localhost).
		if os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") == "true" {
			grpcOpts = append(grpcOpts, otlptracegrpc.WithInsecure())
		}

		otlpExp, otlpErr := otlptracegrpc.New(ctx, grpcOpts...)
		if otlpErr != nil {
			return noop, fmt.Errorf("creating OTLP exporter: %w", otlpErr)
		}
		opts = append(opts, sdktrace.WithBatcher(otlpExp))
		slog.Info("OTel OTLP exporter configured", "endpoint", cfg.CollectorEndpoint)
	}

	// Stdout exporter — JSON spans to stdout for kubectl logs / log pipelines.
	// Uses SimpleSpanProcessor for immediate output.
	if cfg.StdoutExporter {
		stdoutExp, stdoutErr := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if stdoutErr != nil {
			return noop, fmt.Errorf("creating stdout exporter: %w", stdoutErr)
		}
		opts = append(opts, sdktrace.WithSpanProcessor(
			sdktrace.NewSimpleSpanProcessor(stdoutExp),
		))
		slog.Info("OTel stdout exporter configured")
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)

	// W3C Trace Context propagation for distributed tracing across A2A calls.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("OpenTelemetry tracing initialized",
		"service_version", cfg.ServiceVersion,
		"hostname", hostname)

	return tp.Shutdown, nil
}
