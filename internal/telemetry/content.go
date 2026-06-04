package telemetry

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const maxEventBytes = 10240 // 10KB truncation limit

// AddMessageEvent records an LLM message as a span event.
// Content is truncated at 10KB to avoid overwhelming the collector.
func AddMessageEvent(span trace.Span, eventName string, role string, content string) {
	if !span.IsRecording() {
		return
	}
	span.AddEvent(eventName, trace.WithAttributes(
		attribute.String("message.role", role),
		attribute.String("message.content", truncate(content, maxEventBytes)),
	))
}

// AddToolArgsEvent records tool call arguments as a span event.
func AddToolArgsEvent(span trace.Span, toolName string, args map[string]any) {
	if !span.IsRecording() {
		return
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		argsJSON = []byte("{}")
	}
	span.AddEvent("tool.args", trace.WithAttributes(
		attribute.String("tool.name", toolName),
		attribute.String("tool.args", truncate(string(argsJSON), maxEventBytes)),
	))
}

// AddToolResultEvent records tool execution output as a span event.
func AddToolResultEvent(span trace.Span, toolName string, output string, isError bool) {
	if !span.IsRecording() {
		return
	}
	span.AddEvent("tool.result", trace.WithAttributes(
		attribute.String("tool.name", toolName),
		attribute.String("tool.output", truncate(output, maxEventBytes)),
		attribute.Bool("tool.is_error", isError),
	))
}

// truncate cuts s to at most maxBytes, respecting UTF-8 boundaries.
// The suffix is included in the byte budget so the result never exceeds maxBytes.
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	const suffix = "...[truncated]"
	if maxBytes <= len(suffix) {
		return suffix[:maxBytes]
	}
	truncated := s[:maxBytes-len(suffix)]
	for !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return strings.TrimRight(truncated, "\x00") + suffix
}
