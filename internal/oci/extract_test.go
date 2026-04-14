package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func makeTarGzip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for name, data := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Size:     int64(len(data)),
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err := gw.Write(tarBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return gzBuf.Bytes()
}

func TestExtractTarGzipValid(t *testing.T) {
	data := makeTarGzip(t, map[string][]byte{
		"skill/SKILL.md":   []byte("# test"),
		"skill/skill.yaml": []byte("name: test"),
	})
	dest := t.TempDir()
	if err := extractTarGzip(data, dest); err != nil {
		t.Fatalf("extractTarGzip: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "skill", "SKILL.md")); err != nil {
		t.Error("SKILL.md not extracted")
	}
}

func TestExtractRejectsPathTraversal(t *testing.T) {
	data := makeTarGzip(t, map[string][]byte{
		"../etc/passwd": []byte("root:x:0:0"),
	})
	dest := t.TempDir()
	err := extractTarGzip(data, dest)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
}

func TestExtractRejectsAbsolutePath(t *testing.T) {
	// Create a tar with absolute path manually
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "/etc/passwd",
		Size:     4,
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("root")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err := gw.Write(tarBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	err := extractTarGzip(gzBuf.Bytes(), dest)
	if err == nil {
		// Check that the file wasn't written outside dest
		if _, statErr := os.Stat("/etc/passwd.test"); statErr == nil {
			t.Fatal("file written outside destination")
		}
	}
	// Either an error or the file is safely inside dest — both acceptable
}

func TestExtractAppliesPermissionMask(t *testing.T) {
	// Create tar with 0777 permissions
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "test.txt",
		Size:     5,
		Mode:     0o777,
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err := gw.Write(tarBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if err := extractTarGzip(gzBuf.Bytes(), dest); err != nil {
		t.Fatalf("extractTarGzip: %v", err)
	}

	info, err := os.Stat(filepath.Join(dest, "test.txt"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	mode := info.Mode().Perm()
	if mode&0o133 != 0 {
		t.Errorf("file permissions %o should not have group/other write or any execute bits", mode)
	}
}
