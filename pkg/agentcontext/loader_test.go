package agentcontext

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

func TestLoaderAllFiles(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "openclaw-workspace")
	loader := NewLoader()
	profile := DefaultProfiles[ProfileOpenClaw]

	result := loader.Load(dir, profile)

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

func TestLoaderPartialFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "SOUL.md"), "Be direct.")
	writeTestFile(t, filepath.Join(dir, "USER.md"), "Pavel, OCTO team")

	loader := NewLoader()
	profile := DefaultProfiles[ProfileOpenClaw]
	result := loader.Load(dir, profile)

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

func TestLoaderNoFiles(t *testing.T) {
	dir := t.TempDir()

	loader := NewLoader()
	result := loader.Load(dir, DefaultProfiles[ProfileOpenClaw])

	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
	}
}

func TestLoaderPerFileTruncation(t *testing.T) {
	dir := t.TempDir()

	largeContent := strings.Repeat("a", 25_000)
	writeTestFile(t, filepath.Join(dir, "AGENTS.md"), largeContent)

	loader := NewLoader()
	result := loader.Load(dir, DefaultProfiles[ProfileOpenClaw])

	contentStart := strings.Index(result, "### AGENTS\n") + len("### AGENTS\n")
	content := result[contentStart:]
	if len(content) > DefaultMaxPerFileChars {
		t.Fatalf("content should be truncated to %d chars, got %d", DefaultMaxPerFileChars, len(content))
	}
}

func TestLoaderTotalTruncation(t *testing.T) {
	dir := t.TempDir()

	fileContent := strings.Repeat("x", 18_000)
	for _, name := range DefaultProfiles[ProfileOpenClaw].Files {
		writeTestFile(t, filepath.Join(dir, name), fileContent)
	}

	loader := NewLoader()
	result := loader.Load(dir, DefaultProfiles[ProfileOpenClaw])

	totalContent := 0
	for _, name := range DefaultProfiles[ProfileOpenClaw].Files {
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

	if totalContent > DefaultMaxTotalChars {
		t.Fatalf("total content should not exceed %d chars, got %d", DefaultMaxTotalChars, totalContent)
	}
}

func TestLoaderEmptyFiles(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "AGENTS.md"), "")
	writeTestFile(t, filepath.Join(dir, "SOUL.md"), "   \n  ")
	writeTestFile(t, filepath.Join(dir, "USER.md"), "has content")

	loader := NewLoader()
	result := loader.Load(dir, DefaultProfiles[ProfileOpenClaw])

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

func TestLoaderUnicodeTruncation(t *testing.T) {
	dir := t.TempDir()

	// Each character is 3 bytes in UTF-8; 25K runes = 75K bytes.
	// Truncation should preserve whole runes, not split mid-character.
	unicodeContent := strings.Repeat("日", 25_000)
	writeTestFile(t, filepath.Join(dir, "AGENTS.md"), unicodeContent)

	loader := NewLoader()
	result := loader.Load(dir, DefaultProfiles[ProfileOpenClaw])

	contentStart := strings.Index(result, "### AGENTS\n") + len("### AGENTS\n")
	content := result[contentStart:]
	runeCount := utf8.RuneCountInString(content)
	if runeCount > DefaultMaxPerFileChars {
		t.Fatalf("expected at most %d runes, got %d", DefaultMaxPerFileChars, runeCount)
	}
	if !utf8.ValidString(content) {
		t.Fatal("truncated content is not valid UTF-8")
	}
}

func TestLoaderNonexistentDir(t *testing.T) {
	loader := NewLoader()
	result := loader.Load("/nonexistent/path", DefaultProfiles[ProfileOpenClaw])

	if result != "" {
		t.Fatalf("expected empty string for nonexistent dir, got %q", result)
	}
}

func TestResolveProfileDefaults(t *testing.T) {
	p, ok := ResolveProfile("")
	if ok || p.Name != DefaultProfiles[ProfileDocsclaw].Name {
		t.Fatal("empty profile should resolve to docsclaw with ok=false")
	}
	p, ok = ResolveProfile("unknown")
	if ok || p.Name != DefaultProfiles[ProfileDocsclaw].Name {
		t.Fatal("unknown profile should resolve to docsclaw with ok=false")
	}
}

func TestResolveProfileKnown(t *testing.T) {
	for name := range DefaultProfiles {
		p, ok := ResolveProfile(name)
		if !ok {
			t.Fatalf("expected profile %q to be known", name)
		}
		if p.Name != name {
			t.Fatalf("expected profile %q, got %q", name, p.Name)
		}
	}
}

func TestHermesProfileIncludesMemory(t *testing.T) {
	p := DefaultProfiles[ProfileHermes]
	hasMemory := false
	for _, f := range p.Files {
		if f == "MEMORY.md" {
			hasMemory = true
		}
	}
	if !hasMemory {
		t.Fatal("hermes profile should include MEMORY.md")
	}
	for _, f := range p.Files {
		if f == "IDENTITY.md" || f == "TOOLS.md" {
			t.Fatalf("hermes profile should not include %s", f)
		}
	}
}

func TestOpenClawProfileIncludesMemory(t *testing.T) {
	p := DefaultProfiles[ProfileOpenClaw]
	hasMemory := false
	for _, f := range p.Files {
		if f == "MEMORY.md" {
			hasMemory = true
		}
	}
	if !hasMemory {
		t.Fatal("openclaw profile should include MEMORY.md")
	}
}
