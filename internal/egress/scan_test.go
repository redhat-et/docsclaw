package egress

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

var forbiddenDialers = map[string]bool{
	"Dial":           true,
	"DialContext":    true,
	"DialTLSContext": true,
}

// roundTripperAllowlist names files that intentionally wrap an
// http.RoundTripper. They are still checked for raw transports and custom
// dialers, but not for RoundTripper wrappers. Add a file here only with a
// reason: a wrapper anywhere else bypasses internal/egress.
var roundTripperAllowlist = map[string]string{
	"internal/a2acall/a2acall.go": "captures non-2xx peer responses while wrapping the injected egress transport",
}

// TestNoRawTransportsOrDialers keeps every outbound client on the stock
// transport. A raw http.Transport literal silently drops Proxy; a custom dialer
// changes DNS and TLS behavior and diverges from the stock connection pool.
func TestNoRawTransportsOrDialers(t *testing.T) {
	seen := map[string]bool{}
	walkProductionGoFiles(t, func(_ string, file *ast.File, fset *token.FileSet) {
		report := func(pos token.Pos, what string) {
			position := fset.Position(pos)
			key := position.String()
			if seen[key] {
				return
			}
			seen[key] = true
			t.Errorf("%s: %s; use egress.NewTransport()", key, what)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				if isHTTPTransport(node.Type) {
					report(node.Pos(), "raw http.Transport literal")
				}
			case *ast.KeyValueExpr:
				if id, ok := node.Key.(*ast.Ident); ok && forbiddenDialers[id.Name] {
					report(node.Pos(), "custom dialer field "+id.Name)
				}
			case *ast.AssignStmt:
				for _, lhs := range node.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && forbiddenDialers[sel.Sel.Name] {
						report(sel.Pos(), "custom dialer assignment "+sel.Sel.Name)
					}
				}
			}
			return true
		})
	})
}

// TestNoAdHocRoundTrippers closes the gap that let a2a_call bypass the egress
// client by wrapping an http.RoundTripper: the transport scan above only sees
// http.Transport literals. This test flags hand-written RoundTripper
// implementations and Transport assignments whose value does not come from
// egress. Files in roundTripperAllowlist are exempt.
func TestNoAdHocRoundTrippers(t *testing.T) {
	walkProductionGoFiles(t, func(rel string, file *ast.File, fset *token.FileSet) {
		if reason, ok := roundTripperAllowlist[rel]; ok {
			_ = reason
			return
		}
		report := func(pos token.Pos, what string) {
			t.Errorf("%s: %s; use egress.NewClient() or egress.NewTransport(), "+
				"or add the file to roundTripperAllowlist with a reason",
				fset.Position(pos), what)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.Field:
				if isHTTPRoundTripper(node.Type) {
					report(node.Pos(), "http.RoundTripper field")
				}
			case *ast.TypeSpec:
				if isHTTPRoundTripper(node.Type) {
					report(node.Pos(), "http.RoundTripper type alias")
				}
			case *ast.FuncDecl:
				if node.Recv != nil && node.Name.Name == "RoundTrip" {
					report(node.Pos(), "custom http.RoundTripper implementation")
				}
			case *ast.AssignStmt:
				for i, lhs := range node.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "Transport" {
						continue
					}
					if !isEgressTransportValue(assignRHS(node, i)) {
						report(sel.Pos(), "Transport assignment")
					}
				}
			case *ast.KeyValueExpr:
				id, ok := node.Key.(*ast.Ident)
				if ok && id.Name == "Transport" && !isEgressTransportValue(node.Value) {
					report(node.Pos(), "Transport field assignment")
				}
			}
			return true
		})
	})
}

// walkProductionGoFiles calls fn for every non-test Go file outside the
// demo, vendor, and egress trees. It parses each file once with a shared
// FileSet so positions stay valid.
func walkProductionGoFiles(t *testing.T, fn func(rel string, file *ast.File, fset *token.FileSet)) {
	t.Helper()

	root := filepath.Join("..", "..")
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", ".claude", ".superpowers", "vendor", "node_modules", "demo":
				return filepath.SkipDir
			}
			if rel == "internal/egress" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		fn(rel, file, fset)
		return nil
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
}

func assignRHS(node *ast.AssignStmt, i int) ast.Expr {
	if len(node.Rhs) == len(node.Lhs) {
		return node.Rhs[i]
	}
	if len(node.Rhs) == 1 {
		return node.Rhs[0]
	}
	return nil
}

// isEgressTransportValue reports whether expr is a nil transport or a value
// produced by egress.NewTransport/egress.NewClient. A nil Transport means
// http.DefaultTransport, which keeps Proxy.
func isEgressTransportValue(expr ast.Expr) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case *ast.ParenExpr:
		return isEgressTransportValue(e.X)
	case *ast.Ident:
		return e.Name == "nil"
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		return ok && id.Name == "egress" &&
			(sel.Sel.Name == "NewTransport" || sel.Sel.Name == "NewClient")
	}
	return false
}

// isHTTPTransport matches the selector http.Transport. It does not resolve
// import aliases, so a file importing net/http under another name (for
// example stdhttp "net/http") would evade the scan. This repo does not use
// aliased net/http imports; treat this as a guard against the common case,
// not a proof of absence.
func isHTTPTransport(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "http" && sel.Sel.Name == "Transport"
}

// isHTTPRoundTripper matches the selector http.RoundTripper with the same
// alias limitation as isHTTPTransport.
func isHTTPRoundTripper(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "http" && sel.Sel.Name == "RoundTripper"
}
