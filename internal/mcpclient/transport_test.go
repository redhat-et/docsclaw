package mcpclient

import (
	"testing"

	"gopkg.in/yaml.v3"
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

func TestMCPServerConfig_YAMLParsing(t *testing.T) {
	data := []byte(`
- name: weather
  transport: streamable_http
  url: "http://weather-tool:8000/mcp"
- name: localtools
  transport: stdio
  command: python
  args: ["-m", "local_mcp_server"]
  env:
    LOG_LEVEL: debug
`)
	var configs []MCPServerConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		t.Fatalf("YAML parse: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}
	if configs[0].Name != "weather" || configs[0].URL != "http://weather-tool:8000/mcp" {
		t.Fatalf("unexpected first config: %+v", configs[0])
	}
	if configs[1].Command != "python" || len(configs[1].Args) != 2 {
		t.Fatalf("unexpected second config: %+v", configs[1])
	}
	if configs[1].Env["LOG_LEVEL"] != "debug" {
		t.Fatalf("expected env LOG_LEVEL=debug, got %v", configs[1].Env)
	}
}
