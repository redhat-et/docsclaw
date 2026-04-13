package verify

import (
	"context"
	"fmt"
	"log/slog"
)

// Verify checks the cosign signature of an OCI artifact against the given policy.
func Verify(ctx context.Context, ref string, policy Policy) error {
	switch policy.Mode {
	case ModeSkip:
		slog.Debug("signature verification skipped")
		return nil

	case ModeWarn:
		if policy.PublicKey == "" {
			slog.Warn("public key not provided, skipping signature verification")
			return nil
		}
		if err := verifySignature(ctx, ref, policy.PublicKey); err != nil {
			slog.Warn("signature verification failed", "ref", ref, "error", err)
			return nil
		}
		slog.Info("signature verification successful", "ref", ref)
		return nil

	case ModeEnforce:
		if policy.PublicKey == "" {
			return fmt.Errorf("enforce mode requires a public key")
		}
		if err := verifySignature(ctx, ref, policy.PublicKey); err != nil {
			return err
		}
		slog.Info("signature verification successful", "ref", ref)
		return nil

	default:
		return fmt.Errorf("unknown verification mode: %s", policy.Mode)
	}
}

// verifySignature is a stub that checks the cosign signature of an OCI artifact.
// TODO(#4): Replace with real sigstore-go verification.
func verifySignature(_ context.Context, ref, publicKeyPath string) error {
	return fmt.Errorf("sigstore verification not yet implemented (ref=%s, key=%s)", ref, publicKeyPath)
}
