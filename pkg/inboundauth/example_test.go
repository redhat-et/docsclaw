package inboundauth_test

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/inboundauth"
)

// ExampleMiddleware wraps a mux so every request must carry a verified token.
func ExampleMiddleware() {
	mux := http.NewServeMux()
	mux.HandleFunc("/a2a", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := inboundauth.ClaimsFrom(r.Context())
		if !ok {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprintf(w, "hello %s", claims.Subject)
	})

	handler, err := inboundauth.Middleware(inboundauth.Config{
		Issuer:         "https://issuer.example",
		JWKSURL:        "https://issuer.example/.well-known/jwks.json",
		Audience:       "engineering-analyst",
		RequiredScopes: []string{"engineering"},
		ExemptPaths:    []string{"/health"},
	}, mux)
	if err != nil {
		log.Fatal(err)
	}
	_ = handler

	// Output:
}

// TestNoInternalImports keeps pkg/inboundauth reusable outside this repository:
// an A2A adapter for other agents will import it as a library.
func TestNoInternalImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	const internalPrefix = "github.com/redhat-et/docsclaw/internal/"
	for _, imp := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(imp, internalPrefix) {
			t.Errorf("pkg/inboundauth depends on %s", imp)
		}
	}
}
