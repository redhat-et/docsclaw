package cmd

import (
	"os"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/manifest"
)

func TestResolveSecrets_FlagOverride(t *testing.T) {
	decls := []manifest.SecretDecl{
		{Name: "API_KEY", Required: true},
	}
	flagSecrets := []string{"API_KEY=flag-value"}

	// Set env var that should be overridden
	os.Setenv("API_KEY", "env-value")
	defer os.Unsetenv("API_KEY")

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

	os.Setenv("API_KEY", "env-value")
	defer os.Unsetenv("API_KEY")

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
		{Name: "API_KEY", Required: true},
	}

	os.Unsetenv("API_KEY")

	_, err := resolveSecrets(decls, nil)
	if err == nil {
		t.Fatal("expected error for missing required secret")
	}

	expectedMsg := "required secret \"API_KEY\" not set"
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("expected error message prefix %q, got %q", expectedMsg, err.Error())
	}
}

func TestResolveSecrets_OptionalMissing(t *testing.T) {
	decls := []manifest.SecretDecl{
		{Name: "API_KEY", Required: false},
	}

	os.Unsetenv("API_KEY")

	resolved, err := resolveSecrets(decls, nil)
	if err != nil {
		t.Fatalf("resolveSecrets() error: %v", err)
	}

	if _, exists := resolved["API_KEY"]; exists {
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

	expectedMsg := "invalid --secret format"
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("expected error message prefix %q, got %q", expectedMsg, err.Error())
	}
}

func TestResolveSecrets_MultipleSecrets(t *testing.T) {
	decls := []manifest.SecretDecl{
		{Name: "API_KEY", Required: true},
		{Name: "DB_PASSWORD", Required: true},
		{Name: "OPTIONAL_KEY", Required: false},
	}
	flagSecrets := []string{"API_KEY=flag-value"}

	os.Setenv("DB_PASSWORD", "db-pass")
	defer os.Unsetenv("DB_PASSWORD")

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
