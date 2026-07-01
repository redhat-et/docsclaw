package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file %s: %v", path, err)
	}
}

func TestLoadWorkspaceContextAllFiles(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "openclaw-workspace")
	result := loadWorkspaceContext(dir)

	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !strings.Contains(result, "## Project Context") {
		t.Fatal("expected '## Project Context' header")
	}

	expectedHeaders := []string{"### AGENTS", "### SOUL", "### USER", "### IDENTITY", "### TOOLS"}
	for _, h := range expectedHeaders {
		if !strings.Contains(result, h) {
			t.Fatalf("missing section header: %s", h)
		}
	}

	agentsIdx := strings.Index(result, "### AGENTS")
	soulIdx := strings.Index(result, "### SOUL")
	userIdx := strings.Index(result, "### USER")
	identityIdx := strings.Index(result, "### IDENTITY")
	toolsIdx := strings.Index(result, "### TOOLS")

	if agentsIdx >= soulIdx || soulIdx >= userIdx || userIdx >= identityIdx || identityIdx >= toolsIdx {
		t.Fatal("sections not in expected order: AGENTS, SOUL, USER, IDENTITY, TOOLS")
	}
}

func TestLoadWorkspaceContextPartialFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "SOUL.md"), "Be direct.")
	writeTestFile(t, filepath.Join(dir, "USER.md"), "Pavel, OCTO team")

	result := loadWorkspaceContext(dir)

	if !strings.Contains(result, "### SOUL") {
		t.Fatal("expected SOUL section")
	}
	if !strings.Contains(result, "### USER") {
		t.Fatal("expected USER section")
	}
	if strings.Contains(result, "### AGENTS") {
		t.Fatal("should not contain AGENTS section")
	}
	if strings.Contains(result, "### IDENTITY") {
		t.Fatal("should not contain IDENTITY section")
	}
	if strings.Contains(result, "### TOOLS") {
		t.Fatal("should not contain TOOLS section")
	}
}

func TestLoadWorkspaceContextNoFiles(t *testing.T) {
	dir := t.TempDir()

	result := loadWorkspaceContext(dir)

	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
	}
}

func TestLoadWorkspaceContextPerFileTruncation(t *testing.T) {
	dir := t.TempDir()

	largeContent := strings.Repeat("a", 25_000)
	writeTestFile(t, filepath.Join(dir, "AGENTS.md"), largeContent)

	result := loadWorkspaceContext(dir)

	contentStart := strings.Index(result, "### AGENTS\n") + len("### AGENTS\n")
	content := result[contentStart:]
	if len(content) > maxPerFileChars {
		t.Fatalf("content should be truncated to %d chars, got %d", maxPerFileChars, len(content))
	}
}

func TestLoadWorkspaceContextTotalTruncation(t *testing.T) {
	dir := t.TempDir()

	fileContent := strings.Repeat("x", 18_000)
	for _, name := range openClawFiles {
		writeTestFile(t, filepath.Join(dir, name), fileContent)
	}

	result := loadWorkspaceContext(dir)

	totalContent := 0
	for _, name := range openClawFiles {
		header := "### " + strings.TrimSuffix(name, ".md") + "\n"
		idx := strings.Index(result, header)
		if idx < 0 {
			continue
		}
		sectionStart := idx + len(header)
		nextHeader := strings.Index(result[sectionStart:], "\n\n### ")
		var section string
		if nextHeader < 0 {
			section = result[sectionStart:]
		} else {
			section = result[sectionStart : sectionStart+nextHeader]
		}
		totalContent += len(section)
	}

	if totalContent > maxTotalChars {
		t.Fatalf("total content should not exceed %d chars, got %d", maxTotalChars, totalContent)
	}
}

func TestLoadWorkspaceContextEmptyFiles(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "AGENTS.md"), "")
	writeTestFile(t, filepath.Join(dir, "SOUL.md"), "   \n  ")
	writeTestFile(t, filepath.Join(dir, "USER.md"), "has content")

	result := loadWorkspaceContext(dir)

	if strings.Contains(result, "### AGENTS") {
		t.Fatal("should skip empty AGENTS.md")
	}
	if strings.Contains(result, "### SOUL") {
		t.Fatal("should skip whitespace-only SOUL.md")
	}
	if !strings.Contains(result, "### USER") {
		t.Fatal("should include USER.md with content")
	}
}

func TestLoadWorkspaceContextUnicodeTruncation(t *testing.T) {
	dir := t.TempDir()

	// Each character is 3 bytes in UTF-8; 25K runes = 75K bytes.
	// Truncation should preserve whole runes, not split mid-character.
	unicodeContent := strings.Repeat("日", 25_000)
	writeTestFile(t, filepath.Join(dir, "AGENTS.md"), unicodeContent)

	result := loadWorkspaceContext(dir)

	contentStart := strings.Index(result, "### AGENTS\n") + len("### AGENTS\n")
	content := result[contentStart:]
	runeCount := utf8.RuneCountInString(content)
	if runeCount > maxPerFileChars {
		t.Fatalf("expected at most %d runes, got %d", maxPerFileChars, runeCount)
	}
	if !utf8.ValidString(content) {
		t.Fatal("truncated content is not valid UTF-8")
	}
}

func TestLoadWorkspaceContextNonexistentDir(t *testing.T) {
	result := loadWorkspaceContext("/nonexistent/path")

	if result != "" {
		t.Fatalf("expected empty string for nonexistent dir, got %q", result)
	}
}
