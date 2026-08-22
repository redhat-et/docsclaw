package agentcontext

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	// DefaultMaxPerFileChars limits how much of a single workspace file is
	// injected into the prompt.
	DefaultMaxPerFileChars = 20_000
	// DefaultMaxTotalChars limits the total workspace context size.
	DefaultMaxTotalChars = 60_000
)

// Loader reads workspace Markdown files into a project-context block.
type Loader struct {
	MaxPerFileChars int
	MaxTotalChars   int
}

// NewLoader creates a Loader with default limits.
func NewLoader() *Loader {
	return &Loader{
		MaxPerFileChars: DefaultMaxPerFileChars,
		MaxTotalChars:   DefaultMaxTotalChars,
	}
}

// Load reads the files declared by profile in order and returns a Markdown
// block prefixed with "## Project Context". Missing files are silently
// skipped. Files are trimmed, truncated per-file and in total, and joined
// with sub-headers derived from the file names.
func (l *Loader) Load(workspaceDir string, profile Profile) string {
	var loaded []string
	var sections []string
	totalChars := 0

	for _, name := range profile.Files {
		if totalChars >= l.MaxTotalChars {
			break
		}

		data, err := os.ReadFile(filepath.Join(workspaceDir, name))
		if err != nil {
			if !os.IsNotExist(err) {
				slog.Warn("failed to read workspace file", "file", name, "error", err)
			}
			continue
		}

		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}

		runeCount := utf8.RuneCountInString(content)
		if runeCount > l.MaxPerFileChars {
			slog.Warn("workspace file truncated",
				"file", name,
				"original_chars", runeCount,
				"limit", l.MaxPerFileChars)
			content = truncateRunes(content, l.MaxPerFileChars)
			runeCount = l.MaxPerFileChars
		}

		remaining := l.MaxTotalChars - totalChars
		if runeCount > remaining {
			slog.Warn("workspace context truncated at total limit",
				"file", name,
				"used_chars", remaining,
				"total_limit", l.MaxTotalChars)
			content = truncateRunes(content, remaining)
			runeCount = remaining
		}

		totalChars += runeCount
		header := strings.TrimSuffix(name, ".md")
		sections = append(sections, fmt.Sprintf("### %s\n%s", header, content))
		loaded = append(loaded, name)
	}

	if len(sections) == 0 {
		return ""
	}

	slog.Info("loaded workspace context",
		"profile", profile.Name,
		"files", loaded,
		"total_chars", totalChars)

	return "\n\n## Project Context\n\n" + strings.Join(sections, "\n\n")
}

// truncateRunes returns s truncated to at most n runes without splitting
// multi-byte characters.
func truncateRunes(s string, n int) string {
	runes := 0
	for i := range s {
		if runes >= n {
			return s[:i]
		}
		runes++
	}
	return s
}
