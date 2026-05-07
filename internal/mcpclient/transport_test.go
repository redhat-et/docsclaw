package mcpclient

import (
	"testing"
)

func TestValidateConfig_StreamableHTTP(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "weather",
		Transport: "streamable_http",
		URL:       "http://localhost:8080/mcp",
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid streamable_http config rejected: %v", err)
	}
}

func TestValidateConfig_StreamableHTTP_MissingURL(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "weather",
		Transport: "streamable_http",
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestValidateConfig_Stdio(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "s3",
		Transport: "stdio",
		Command:   "python",
		Args:      []string{"-m", "s3_server"},
		Env:       map[string]string{"AWS_REGION": "us-east-1"},
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid stdio config rejected: %v", err)
	}
}

func TestValidateConfig_Stdio_MissingCommand(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "s3",
		Transport: "stdio",
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestValidateConfig_MissingName(t *testing.T) {
	cfg := MCPServerConfig{
		Transport: "stdio",
		Command:   "python",
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidateConfig_UnknownTransport(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "bad",
		Transport: "grpc",
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for unknown transport")
	}
}
