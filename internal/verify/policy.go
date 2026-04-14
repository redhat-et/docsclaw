// Package verify provides signature verification for OCI skill artifacts.
package verify

// Mode controls verification behavior.
type Mode string

const (
	ModeEnforce Mode = "enforce"
	ModeWarn    Mode = "warn"
	ModeSkip    Mode = "skip"
)

// Policy defines trust rules for signature verification.
type Policy struct {
	Mode      Mode
	PublicKey string // path to cosign public key file
}
