package verify

import (
	"context"
	"testing"
)

func TestVerifySkipMode(t *testing.T) {
	policy := Policy{
		Mode: ModeSkip,
	}
	err := Verify(context.Background(), "ghcr.io/example/skill:latest", policy)
	if err != nil {
		t.Errorf("expected no error in skip mode, got: %v", err)
	}
}

func TestVerifyEnforceNoKey(t *testing.T) {
	policy := Policy{
		Mode:      ModeEnforce,
		PublicKey: "",
	}
	err := Verify(context.Background(), "ghcr.io/example/skill:latest", policy)
	if err == nil {
		t.Error("expected error when enforce mode has no public key")
	}
}
