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

// TestNoRawTransportsOrDialers keeps every outbound client on the stock
// transport. A raw http.Transport literal silently drops Proxy and a custom
// dialer bypasses the supervisor's namespace redirect.
func TestNoRawTransportsOrDialers(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	seen := map[string]bool{}

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
		return nil
	})
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
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
