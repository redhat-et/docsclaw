package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileStoreLoadMissing(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "MEMORY.md"))

	content, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "" {
		t.Fatalf("expected empty content, got %q", content)
	}
}

func TestFileStoreRememberCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	store := NewFileStore(path)

	if err := store.Remember(context.Background(), "First entry"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if !strings.Contains(string(data), "First entry") {
		t.Fatalf("expected entry in file, got %q", string(data))
	}
}

func TestFileStoreRememberPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	store := NewFileStore(path)

	if err := store.Remember(context.Background(), "Entry"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat memory file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected mode 0600, got %o", info.Mode().Perm())
	}
}

func TestFileStoreRememberTightensPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	// Pre-create the file with overly broad permissions.
	if err := os.WriteFile(path, []byte("existing"), 0644); err != nil {
		t.Fatalf("failed to create memory file: %v", err)
	}

	store := NewFileStore(path)
	if err := store.Remember(context.Background(), "Entry"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat memory file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected mode 0600, got %o", info.Mode().Perm())
	}
}

func TestFileStoreRememberAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	store := NewFileStore(path)

	if err := store.Remember(context.Background(), "First"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.Remember(context.Background(), "Second"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(content, "First") {
		t.Fatal("missing first entry")
	}
	if !strings.Contains(content, "Second") {
		t.Fatal("missing second entry")
	}

	firstIdx := strings.Index(content, "First")
	secondIdx := strings.Index(content, "Second")
	if firstIdx >= secondIdx {
		t.Fatal("entries not appended in order")
	}
}

func TestFileStoreRememberEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	store := NewFileStore(path)

	if err := store.Remember(context.Background(), "   "); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected no file for empty entry")
	}
}

func BenchmarkFileStoreRememberLargeFile(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "MEMORY.md")

	// Seed a 1 MiB memory file.
	large := strings.Repeat("x", 1024*1024)
	if err := os.WriteFile(path, []byte(large), 0600); err != nil {
		b.Fatalf("failed to seed memory file: %v", err)
	}

	store := NewFileStore(path)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.Remember(ctx, "benchmark entry"); err != nil {
			b.Fatalf("remember failed: %v", err)
		}
	}
}

func TestFileStoreLoadExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	if err := os.WriteFile(path, []byte("existing memory"), 0644); err != nil {
		t.Fatalf("failed to write memory file: %v", err)
	}

	store := NewFileStore(path)
	content, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "existing memory" {
		t.Fatalf("expected existing memory, got %q", content)
	}
}
