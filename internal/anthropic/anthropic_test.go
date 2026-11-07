package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

func TestProvider_AcceptsPlaceholderKey(t *testing.T) {
	const key = "openshell-placeholder-xyz"

	var gotKey, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant",`+
			`"model":"claude-test","content":[{"type":"text","text":"hello"}],`+
			`"stop_reason":"end_turn","stop_sequence":null,`+
			`"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()

	provider, err := NewAnthropicProvider(llm.Config{
		APIKey: key,
		Model:  "claude-test",
	})
	if err != nil {
		t.Fatalf("constructor rejected placeholder key: %v", err)
	}
	provider.client = anthropic.NewClient(
		option.WithAPIKey(key),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)

	text, err := provider.Complete(context.Background(), "system", "hi")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if text != "hello" {
		t.Fatalf("text = %q, want %q", text, "hello")
	}
	if gotKey != key {
		t.Fatalf("x-api-key = %q, want %q", gotKey, key)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", gotPath)
	}
}
