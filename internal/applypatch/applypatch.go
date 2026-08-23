package applypatch

import (
	"context"
	"fmt"
	"os"
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

	absPath, err := workspace.ResolveWorkspacePath(path, t.workspaceDir)
	if err != nil {
		return tools.Errorf("%s", err)
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

	originalText := string(original)
	originalLines := strings.Split(originalText, "\n")
	originalHadTrailingNewline := strings.HasSuffix(originalText, "\n")
	if originalHadTrailingNewline && len(originalLines) > 0 {
		// Drop the trailing empty element so strings.Join produces the correct text.
		originalLines = originalLines[:len(originalLines)-1]
	}
	newLines, noTrailingNewline, oldNoNewline, err := applyHunks(originalLines, hunks)
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
	if noTrailingNewline {
		output = strings.TrimSuffix(output, "\n")
	} else if originalHadTrailingNewline || oldNoNewline {
		output += "\n"
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
	oldNoNewline   bool
	newNoNewline   bool
}

func parsePatch(patch string) ([]hunk, error) {
	var hunks []hunk
	var current *hunk

	lines := strings.Split(patch, "\n")
	// Ignore the trailing empty element produced when the patch ends with a newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	for _, raw := range lines {
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
			// The marker is emitted when the preceding line (on either or both
			// sides) lacks a trailing newline. Walk backward to find the most
			// recent change line to decide which side the marker applies to. In
			// a remove/add pair, the marker that appears once refers to the
			// removed (old) line unless a second marker follows the added line.
			for i := len(current.lines) - 1; i >= 0; i-- {
				switch current.lines[i].op {
				case diffRemove:
					current.oldNoNewline = true
					goto markerDone
				case diffAdd:
					current.newNoNewline = true
					goto markerDone
				case diffContext:
					current.oldNoNewline = true
					current.newNoNewline = true
					goto markerDone
				}
			}
			// No prior line in the hunk; conservatively apply to both sides.
			current.oldNoNewline = true
			current.newNoNewline = true
		markerDone:
			continue
		}

		if len(line) == 0 {
			// Blank lines inside a hunk are context. Only the trailing element
			// produced by strings.Split after the final newline is skipped here.
			current.lines = append(current.lines, diffLine{op: diffContext, text: ""})
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
		return 0, 0, fmt.Errorf("invalid range: %s", s)
	}
	if start < 0 || count < 0 {
		return 0, 0, fmt.Errorf("negative range: %s", s)
	}
	return start, count, nil
}

func applyHunks(original []string, hunks []hunk) ([]string, bool, bool, error) {
	result := make([]string, 0, len(original))
	origIdx := 0

	for i, h := range hunks {
		insertAt := h.oldStart - 1
		if h.oldStart == 0 {
			insertAt = 0
		}

		if insertAt < origIdx {
			return nil, false, false, fmt.Errorf("hunk %d is out of order or overlaps a previous hunk", i+1)
		}
		if insertAt > len(original) {
			return nil, false, false, fmt.Errorf("hunk %d starts beyond end of file", i+1)
		}

		result = append(result, original[origIdx:insertAt]...)
		origIdx = insertAt

		if origIdx+h.oldCount > len(original) {
			return nil, false, false, fmt.Errorf("hunk %d extends past end of file", i+1)
		}

		oldConsumed := 0
		contextCount := 0
		addCount := 0
		for _, dl := range h.lines {
			switch dl.op {
			case diffContext:
				if origIdx+oldConsumed >= len(original) {
					return nil, false, false, fmt.Errorf("hunk %d context mismatch at line %d", i+1, origIdx+oldConsumed+1)
				}
				if original[origIdx+oldConsumed] != dl.text {
					return nil, false, false, fmt.Errorf("hunk %d context mismatch at line %d", i+1, origIdx+oldConsumed+1)
				}
				result = append(result, dl.text)
				oldConsumed++
				contextCount++
			case diffRemove:
				if origIdx+oldConsumed >= len(original) {
					return nil, false, false, fmt.Errorf("hunk %d removal mismatch at line %d", i+1, origIdx+oldConsumed+1)
				}
				if original[origIdx+oldConsumed] != dl.text {
					return nil, false, false, fmt.Errorf("hunk %d removal mismatch at line %d", i+1, origIdx+oldConsumed+1)
				}
				oldConsumed++
			case diffAdd:
				result = append(result, dl.text)
				addCount++
			}
		}

		if oldConsumed != h.oldCount {
			return nil, false, false, fmt.Errorf("hunk %d expected %d old lines, got %d", i+1, h.oldCount, oldConsumed)
		}
		if contextCount+addCount != h.newCount {
			return nil, false, false, fmt.Errorf("hunk %d expected %d new lines, got %d", i+1, h.newCount, contextCount+addCount)
		}

		origIdx += h.oldCount
	}

	result = append(result, original[origIdx:]...)
	lastHunk := len(hunks) - 1
	if lastHunk < 0 {
		return result, false, false, nil
	}
	return result, hunks[lastHunk].newNoNewline, hunks[lastHunk].oldNoNewline, nil
}
