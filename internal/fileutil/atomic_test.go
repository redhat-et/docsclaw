package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomically(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "target.txt")

	if err := WriteFileAtomically(target, []byte("hello"), 0640); err != nil {
		t.Fatalf("WriteFileAtomically failed: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read target: %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("expected content %q, got %q", "hello", string(content))
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("failed to stat target: %v", err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("expected mode 0640, got %o", info.Mode().Perm())
	}
}

func TestWriteFileAtomicallyOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "target.txt")

	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatalf("failed to write original file: %v", err)
	}

	if err := WriteFileAtomically(target, []byte("updated"), 0644); err != nil {
		t.Fatalf("WriteFileAtomically failed: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read target: %v", err)
	}
	if string(content) != "updated" {
		t.Fatalf("expected content %q, got %q", "updated", string(content))
	}
}
