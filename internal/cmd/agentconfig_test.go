package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAgentConfig_DocumentServices(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		wantErr    string
		wantURL    string
		wantAbsent bool
	}{
		{
			name: "valid map",
			config: `document_services:
  finance: http://document-finance:8080
  hr: https://document-hr:8443
`,
			wantURL: "http://document-finance:8080",
		},
		{
			name: "no document services",
			config: `tools:
  allowed: []
`,
			wantAbsent: true,
		},
		{
			name: "invalid key",
			config: `document_services:
  Eng: http://document-eng:8080
`,
			wantErr: "Eng",
		},
		{
			name: "non-http scheme",
			config: `document_services:
  finance: ftp://document-finance:8080
`,
			wantErr: "finance",
		},
		{
			name: "relative url",
			config: `document_services:
  finance: /documents
`,
			wantErr: "finance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "agent-config.yaml"), []byte(tt.config), 0o600); err != nil {
				t.Fatal(err)
			}

			cfg, err := loadAgentConfig(dir)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantAbsent {
				if len(cfg.DocumentServices) != 0 {
					t.Fatalf("DocumentServices = %v, want empty", cfg.DocumentServices)
				}
				return
			}
			if got := cfg.DocumentServices["finance"]; got != tt.wantURL {
				t.Fatalf("finance URL = %q, want %q", got, tt.wantURL)
			}
		})
	}
}
