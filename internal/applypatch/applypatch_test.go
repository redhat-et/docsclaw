package applypatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatchCleanApply(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,3 +1,3 @@",
		" line1",
		"-line2",
		"+line2modified",
		" line3",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "Patched "+path {
		t.Fatalf("expected success message %q, got %q", "Patched "+path, result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "line1\nline2modified\nline3\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestApplyPatchFailedHunkReturnsError(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "file.txt")

	original := "line1\nline2\nline3\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,3 +1,3 @@",
		" line1",
		"-wrongline",
		"+replacement",
		" line3",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if !result.Error {
		t.Fatal("expected error for failed hunk")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != original {
		t.Fatalf("file should be unchanged, expected %q, got %q", original, string(data))
	}
}

func TestApplyPatchWorkspaceEscapeBlocked(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "..", "escaped.txt")

	patch := strings.Join([]string{
		"--- a/escaped.txt",
		"+++ b/escaped.txt",
		"@@ -1,1 +1,1 @@",
		"-old",
		"+new",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if !result.Error {
		t.Fatal("expected error for path outside workspace")
	}
	if !strings.Contains(result.Output, "workspace") {
		t.Fatalf("expected workspace error, got %q", result.Output)
	}
}

func TestApplyPatchMissingFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "missing.txt")

	patch := strings.Join([]string{
		"--- a/missing.txt",
		"+++ b/missing.txt",
		"@@ -1,1 +1,1 @@",
		"-old",
		"+new",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if !result.Error {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(result.Output, "does not exist") {
		t.Fatalf("expected missing file error, got %q", result.Output)
	}
}

func TestApplyPatchPreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "script.sh")

	if err := os.WriteFile(path, []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/script.sh",
		"+++ b/script.sh",
		"@@ -1,2 +1,2 @@",
		" #!/bin/sh",
		"-echo hello",
		"+echo world",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("expected mode 0755, got %04o", info.Mode().Perm())
	}
}

func TestApplyPatchMultiHunk(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "file.txt")

	original := "line1\nline2\nline3\nline4\nline5\nline6\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,2 +1,2 @@",
		" line1",
		"-line2",
		"+line2modified",
		"@@ -5,2 +5,2 @@",
		" line5",
		"-line6",
		"+line6modified",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "line1\nline2modified\nline3\nline4\nline5\nline6modified\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestApplyPatchSymlinkWorkspaceEscapeBlocked(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	outsideFile := filepath.Join(dir, "..", "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("secret\n"), 0644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}

	linkPath := filepath.Join(dir, "link.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/link.txt",
		"+++ b/link.txt",
		"@@ -1,1 +1,1 @@",
		"-secret",
		"+modified",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  linkPath,
		"patch": patch,
	})
	if !result.Error {
		t.Fatal("expected error for symlink escaping workspace")
	}
	if !strings.Contains(result.Output, "workspace") {
		t.Fatalf("expected workspace error, got %q", result.Output)
	}

	data, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatalf("failed to read outside file: %v", err)
	}
	if string(data) != "secret\n" {
		t.Fatalf("outside file should be unchanged, got %q", string(data))
	}
}

func TestApplyPatchNoNewlineAtEndOfFile(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(path, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,2 +1,2 @@",
		" line1",
		"-line2",
		"+line2modified",
		"\\ No newline at end of file",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "line1\nline2modified"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestApplyPatchRejectsOutOfOrderHunks(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "file.txt")

	original := "line1\nline2\nline3\nline4\nline5\nline6\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -5,2 +5,2 @@",
		" line5",
		"-line6",
		"+line6modified",
		"@@ -1,2 +1,2 @@",
		" line1",
		"-line2",
		"+line2modified",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if !result.Error {
		t.Fatal("expected error for out-of-order hunks")
	}
	if !strings.Contains(result.Output, "out of order or overlaps a previous hunk") {
		t.Fatalf("expected out of order error, got %q", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != original {
		t.Fatalf("file should be unchanged, expected %q, got %q", original, string(data))
	}
}

func TestApplyPatchBlankContextLine(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "file.txt")

	original := "line1\n\nline3\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,3 +1,3 @@",
		" line1",
		" ",
		"-line3",
		"+line3modified",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "line1\n\nline3modified\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestApplyPatchOldSideOnlyNoNewlineMarker(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "file.txt")

	// Original file has no trailing newline.
	original := "line1\nline2"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Replace the last line; the no-newline marker follows the removed line, so
	// it applies only to the old side. The added line should keep a trailing newline.
	patch := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,2 +1,2 @@",
		" line1",
		"-line2",
		"\\ No newline at end of file",
		"+line2modified",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "line1\nline2modified\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestApplyPatchRejectsMalformedHunkHeader(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	path := filepath.Join(dir, "file.txt")

	original := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -0,5 +0,5 @@",
		" line1",
		"-line2",
		"+line2modified",
		" line3",
		" line4",
		" line5",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"patch": patch,
	})
	if !result.Error {
		t.Fatal("expected error for malformed hunk header")
	}
	if !strings.Contains(result.Output, "invalid range") {
		t.Fatalf("expected invalid range error, got %q", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != original {
		t.Fatalf("file should be unchanged, expected %q, got %q", original, string(data))
	}
}

func TestApplyPatchRelativePathResolvedAgainstWorkspace(t *testing.T) {
	dir := t.TempDir()
	tool := NewApplyPatchTool(dir)
	relPath := "subdir/file.txt"
	absPath := filepath.Join(dir, relPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(absPath, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	patch := strings.Join([]string{
		"--- a/subdir/file.txt",
		"+++ b/subdir/file.txt",
		"@@ -1,3 +1,3 @@",
		" line1",
		"-line2",
		"+line2modified",
		" line3",
		"",
	}, "\n")

	result := tool.Execute(context.Background(), map[string]any{
		"path":  relPath,
		"patch": patch,
	})
	if result.Error {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "Patched "+absPath {
		t.Fatalf("expected success message %q, got %q", "Patched "+absPath, result.Output)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	expected := "line1\nline2modified\nline3\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}
