package applypatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/redhat-et/docsclaw/internal/fileutil"
	"github.com/redhat-et/docsclaw/internal/workspace"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

type applyPatchTool struct {
	workspaceDir string
}

// NewApplyPatchTool creates a new apply patch tool scoped to the given workspace.
func NewApplyPatchTool(workspaceDir string) tools.Tool {
	return &applyPatchTool{workspaceDir: workspaceDir}
}

func (t *applyPatchTool) Name() string { return "apply_patch" }
func (t *applyPatchTool) Description() string {
	return "Apply a unified diff patch to a file atomically. " +
		"All hunks must apply cleanly; otherwise the file is left unchanged."
}
func (t *applyPatchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to patch within the workspace. Relative paths are resolved against the workspace directory.",
			},
			"patch": map[string]any{
				"type":        "string",
				"description": "Unified diff patch string",
			},
		},
		"required": []string{"path", "patch"},
	}
}

func (t *applyPatchTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return tools.Errorf("path is required")
	}

	patch, ok := args["patch"].(string)
	if !ok || patch == "" {
		return tools.Errorf("patch is required")
	}

	if t.workspaceDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(t.workspaceDir, path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return tools.Errorf("failed to resolve path: %s", err)
	}

	if t.workspaceDir != "" {
		if !workspace.IsInsideWorkspace(absPath, t.workspaceDir) {
			return tools.Errorf("Access denied: path outside workspace")
		}
	}

	hunks, err := parsePatch(patch)
	if err != nil {
		return tools.Errorf("failed to parse patch: %s", err)
	}
	if len(hunks) == 0 {
		return tools.Errorf("no hunks found in patch")
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.Errorf("file does not exist: %s", absPath)
		}
		return tools.Errorf("failed to stat file: %s", err)
	}
	originalMode := info.Mode().Perm()

	original, err := os.ReadFile(absPath)
	if err != nil {
		return tools.Errorf("failed to read file: %s", err)
	}

	originalLines := strings.Split(string(original), "\n")
	newLines, noTrailingNewline, err := applyHunks(originalLines, hunks)
	if err != nil {
		return tools.Errorf("patch failed: %s", err)
	}

	var output string
	if len(newLines) == 1 && newLines[0] == "" {
		// Split of an empty file produces [""]; avoid writing a blank line.
		output = ""
	} else {
		output = strings.Join(newLines, "\n")
	}
	if noTrailingNewline && strings.HasSuffix(output, "\n") {
		output = strings.TrimSuffix(output, "\n")
	}

	if err := fileutil.WriteFileAtomically(absPath, []byte(output), originalMode); err != nil {
		return tools.Errorf("failed to apply patch: %s", err)
	}

	return tools.OK("Patched " + absPath)
}

type diffOp int

const (
	diffContext diffOp = iota
	diffRemove
	diffAdd
)

type diffLine struct {
	op   diffOp
	text string
}

type hunk struct {
	oldStart       int
	oldCount       int
	newStart       int
	newCount       int
	lines          []diffLine
	noNewlineAtEnd bool
}

func parsePatch(patch string) ([]hunk, error) {
	var hunks []hunk
	var current *hunk

	for _, raw := range strings.Split(patch, "\n") {
		line := strings.TrimRight(raw, "\r")

		if strings.HasPrefix(line, "@@") {
			if current != nil {
				hunks = append(hunks, *current)
			}
			newHunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			current = &newHunk
			continue
		}

		if current == nil {
			// Ignore everything before the first hunk header.
			continue
		}

		if line == "\\ No newline at end of file" {
			// The marker is emitted when the last line of a hunk (on either
			// or both sides) lacks a trailing newline. We record it on the
			// hunk and use it for the new file when the hunk contributes the
			// final lines of the output. This is a best-effort interpretation:
			// if the marker refers only to the old side of the last hunk, the
			// output may incorrectly lose its trailing newline.
			current.noNewlineAtEnd = true
			continue
		}

		if len(line) == 0 {
			// Ignore empty lines; a real blank context line is represented as " ".
			continue
		}

		switch line[0] {
		case ' ':
			current.lines = append(current.lines, diffLine{op: diffContext, text: line[1:]})
		case '-':
			current.lines = append(current.lines, diffLine{op: diffRemove, text: line[1:]})
		case '+':
			current.lines = append(current.lines, diffLine{op: diffAdd, text: line[1:]})
		default:
			// Ignore patch metadata such as file headers.
		}
	}

	if current != nil {
		hunks = append(hunks, *current)
	}

	return hunks, nil
}

func parseHunkHeader(line string) (hunk, error) {
	parts := strings.SplitN(line, "@@", 3)
	if len(parts) < 3 {
		return hunk{}, fmt.Errorf("invalid hunk header: %q", line)
	}

	ranges := strings.TrimSpace(parts[1])
	fields := strings.Fields(ranges)
	if len(fields) != 2 {
		return hunk{}, fmt.Errorf("invalid hunk ranges: %q", ranges)
	}

	var h hunk
	var err error
	h.oldStart, h.oldCount, err = parseRange(fields[0])
	if err != nil {
		return hunk{}, err
	}
	h.newStart, h.newCount, err = parseRange(fields[1])
	if err != nil {
		return hunk{}, err
	}
	return h, nil
}

func parseRange(s string) (start, count int, err error) {
	if len(s) < 2 {
		return 0, 0, fmt.Errorf("invalid range: %q", s)
	}
	body := s[1:]
	idx := strings.Index(body, ",")
	var startStr, countStr string
	if idx == -1 {
		startStr = body
		countStr = "1"
	} else {
		startStr = body[:idx]
		countStr = body[idx+1:]
	}
	start, err = strconv.Atoi(startStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range start: %q", startStr)
	}
	count, err = strconv.Atoi(countStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range count: %q", countStr)
	}
	if start == 0 && count != 0 {
		count = 0
	}
	if start < 0 || count < 0 {
		return 0, 0, fmt.Errorf("negative range: %s", s)
	}
	return start, count, nil
}

func applyHunks(original []string, hunks []hunk) ([]string, bool, error) {
	sort.Slice(hunks, func(i, j int) bool {
		return hunks[i].oldStart < hunks[j].oldStart
	})

	result := make([]string, 0, len(original))
	origIdx := 0

	for i, h := range hunks {
		insertAt := h.oldStart - 1
		if h.oldStart == 0 {
			insertAt = 0
		}

		if insertAt < origIdx {
			return nil, false, fmt.Errorf("hunk %d overlaps a previous hunk", i+1)
		}
		if insertAt > len(original) {
			return nil, false, fmt.Errorf("hunk %d starts beyond end of file", i+1)
		}

		result = append(result, original[origIdx:insertAt]...)
		origIdx = insertAt

		if origIdx+h.oldCount > len(original) {
			return nil, false, fmt.Errorf("hunk %d extends past end of file", i+1)
		}

		oldConsumed := 0
		contextCount := 0
		addCount := 0
		for _, dl := range h.lines {
			switch dl.op {
			case diffContext:
				if original[origIdx+oldConsumed] != dl.text {
					return nil, false, fmt.Errorf("hunk %d context mismatch at line %d", i+1, origIdx+oldConsumed+1)
				}
				result = append(result, dl.text)
				oldConsumed++
				contextCount++
			case diffRemove:
				if original[origIdx+oldConsumed] != dl.text {
					return nil, false, fmt.Errorf("hunk %d removal mismatch at line %d", i+1, origIdx+oldConsumed+1)
				}
				oldConsumed++
			case diffAdd:
				result = append(result, dl.text)
				addCount++
			}
		}

		if oldConsumed != h.oldCount {
			return nil, false, fmt.Errorf("hunk %d expected %d old lines, got %d", i+1, h.oldCount, oldConsumed)
		}
		if contextCount+addCount != h.newCount {
			return nil, false, fmt.Errorf("hunk %d expected %d new lines, got %d", i+1, h.newCount, contextCount+addCount)
		}

		origIdx += h.oldCount
	}

	result = append(result, original[origIdx:]...)
	noTrailingNewline := len(hunks) > 0 && hunks[len(hunks)-1].noNewlineAtEnd
	return result, noTrailingNewline, nil
}
