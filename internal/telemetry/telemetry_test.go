package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestInitDisabled(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("Init with Enabled=false should not error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown should not error: %v", err)
	}
}

func TestInitStdout(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{
		Enabled:        true,
		StdoutExporter: true,
		ServiceVersion: "test",
	})
	if err != nil {
		t.Fatalf("Init with StdoutExporter should not error: %v", err)
	}
	t.Cleanup(func() {
		_ = shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	// Verify the global tracer produces recording spans.
	_, span := otel.Tracer(TracerName).Start(context.Background(), "test-span")
	if !span.IsRecording() {
		t.Error("span should be recording when OTel is enabled")
	}
	span.End()
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		max      int
		wantLen  bool // true = check length is at most max + suffix
		wantFull bool // true = expect no truncation
	}{
		{"short string", "hello", 100, false, true},
		{"exact limit", "hello", 5, false, true},
		{"over limit", "hello world", 5, true, false},
		{"empty", "", 10, false, true},
		{"multibyte UTF-8", "hello 世界!", 8, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.max)
			if tt.wantFull && result != tt.input {
				t.Errorf("expected no truncation, got %q", result)
			}
			if tt.wantLen && len(result) > tt.max+len("...[truncated]") {
				t.Errorf("truncated result too long: %d bytes", len(result))
			}
		})
	}
}

func TestAddMessageEvent(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	ctx, span := otel.Tracer(TracerName).Start(context.Background(), "test")
	AddMessageEvent(span, "llm.input", "user", "What is the weather?")
	span.End()

	_ = tp.ForceFlush(ctx)
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	events := spans[0].Events
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "llm.input" {
		t.Errorf("expected event name 'llm.input', got %q", events[0].Name)
	}
}

func TestAddToolArgsEvent(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	ctx, span := otel.Tracer(TracerName).Start(context.Background(), "test")
	AddToolArgsEvent(span, "web_fetch", map[string]any{"url": "https://example.com"})
	span.End()

	_ = tp.ForceFlush(ctx)
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if len(spans[0].Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(spans[0].Events))
	}
	if spans[0].Events[0].Name != "tool.args" {
		t.Errorf("expected event name 'tool.args', got %q", spans[0].Events[0].Name)
	}
}

func TestAddToolResultEvent(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	ctx, span := otel.Tracer(TracerName).Start(context.Background(), "test")
	AddToolResultEvent(span, "exec", "exit code 0", false)
	span.End()

	_ = tp.ForceFlush(ctx)
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if len(spans[0].Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(spans[0].Events))
	}
	if spans[0].Events[0].Name != "tool.result" {
		t.Errorf("expected event name 'tool.result', got %q", spans[0].Events[0].Name)
	}
}
