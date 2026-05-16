package rag

import "testing"

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{Backend: "weaviate", URL: "http://localhost:8080", Collection: "Docs"}
	cfg.ApplyDefaults()

	if cfg.TextField != "content" {
		t.Errorf("TextField = %q, want %q", cfg.TextField, "content")
	}
	if cfg.DefaultLimit != 5 {
		t.Errorf("DefaultLimit = %d, want 5", cfg.DefaultLimit)
	}
	if cfg.MaxLimit != 20 {
		t.Errorf("MaxLimit = %d, want 20", cfg.MaxLimit)
	}
}

func TestApplyDefaultsPreservesExplicit(t *testing.T) {
	cfg := &Config{
		Backend:      "weaviate",
		URL:          "http://localhost:8080",
		Collection:   "Docs",
		TextField:    "body",
		DefaultLimit: 10,
		MaxLimit:     50,
	}
	cfg.ApplyDefaults()

	if cfg.TextField != "body" {
		t.Errorf("TextField = %q, want %q", cfg.TextField, "body")
	}
	if cfg.DefaultLimit != 10 {
		t.Errorf("DefaultLimit = %d, want 10", cfg.DefaultLimit)
	}
	if cfg.MaxLimit != 50 {
		t.Errorf("MaxLimit = %d, want 50", cfg.MaxLimit)
	}
}

func TestNewClientWeaviate(t *testing.T) {
	cfg := &Config{
		Backend:    "weaviate",
		URL:        "http://localhost:8080",
		Collection: "Docs",
		TextField:  "content",
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClientUnsupported(t *testing.T) {
	cfg := &Config{Backend: "milvus"}
	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported backend")
	}
}

func TestNewClientEmptyBackend(t *testing.T) {
	cfg := &Config{URL: "http://localhost:8080", Collection: "Docs"}
	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("expected error for empty backend")
	}
}
