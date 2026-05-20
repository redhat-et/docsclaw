package cmd

import (
	"testing"

	"github.com/redhat-et/docsclaw/pkg/manifest"
)

func TestResolveSecrets_FlagOverride(t *testing.T) {
	decls := []manifest.SecretDecl{
		{Name: "API_KEY", Required: true},
	}
	flagSecrets := []string{"API_KEY=flag-value"}

	t.Setenv("API_KEY", "env-value")

	resolved, err := resolveSecrets(decls, flagSecrets)
	if err != nil {
		t.Fatalf("resolveSecrets() error: %v", err)
	}

	if resolved["API_KEY"] != "flag-value" {
		t.Errorf("expected flag value, got %q", resolved["API_KEY"])
	}
}

func TestResolveSecrets_EnvVarFallback(t *testing.T) {
	decls := []manifest.SecretDecl{
		{Name: "API_KEY", Required: true},
	}

	t.Setenv("API_KEY", "env-value")

	resolved, err := resolveSecrets(decls, nil)
	if err != nil {
		t.Fatalf("resolveSecrets() error: %v", err)
	}

	if resolved["API_KEY"] != "env-value" {
		t.Errorf("expected env value, got %q", resolved["API_KEY"])
	}
}

func TestResolveSecrets_RequiredMissing(t *testing.T) {
	decls := []manifest.SecretDecl{
		{Name: "DOCSCLAW_TEST_MISSING_KEY", Required: true},
	}

	_, err := resolveSecrets(decls, nil)
	if err == nil {
		t.Fatal("expected error for missing required secret")
	}
}

func TestResolveSecrets_OptionalMissing(t *testing.T) {
	decls := []manifest.SecretDecl{
		{Name: "DOCSCLAW_TEST_MISSING_KEY", Required: false},
	}

	resolved, err := resolveSecrets(decls, nil)
	if err != nil {
		t.Fatalf("resolveSecrets() error: %v", err)
	}

	if _, exists := resolved["DOCSCLAW_TEST_MISSING_KEY"]; exists {
		t.Error("optional secret should not be in resolved map when missing")
	}
}

func TestResolveSecrets_InvalidFlagFormat(t *testing.T) {
	decls := []manifest.SecretDecl{
		{Name: "API_KEY", Required: true},
	}
	flagSecrets := []string{"INVALID_FORMAT"}

	_, err := resolveSecrets(decls, flagSecrets)
	if err == nil {
		t.Fatal("expected error for invalid flag format")
	}
}

func TestResolveSecrets_MultipleSecrets(t *testing.T) {
	decls := []manifest.SecretDecl{
		{Name: "API_KEY", Required: true},
		{Name: "DB_PASSWORD", Required: true},
		{Name: "OPTIONAL_KEY", Required: false},
	}
	flagSecrets := []string{"API_KEY=flag-value"}

	t.Setenv("DB_PASSWORD", "db-pass")

	resolved, err := resolveSecrets(decls, flagSecrets)
	if err != nil {
		t.Fatalf("resolveSecrets() error: %v", err)
	}

	if resolved["API_KEY"] != "flag-value" {
		t.Errorf("API_KEY: expected flag-value, got %q", resolved["API_KEY"])
	}
	if resolved["DB_PASSWORD"] != "db-pass" {
		t.Errorf("DB_PASSWORD: expected db-pass, got %q", resolved["DB_PASSWORD"])
	}
	if _, exists := resolved["OPTIONAL_KEY"]; exists {
		t.Error("optional key should not be in resolved map when missing")
	}
}
