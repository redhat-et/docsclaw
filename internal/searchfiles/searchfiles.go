package searchfiles

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/redhat-et/docsclaw/internal/workspace"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

const maxOutput = 50000

// searchFilesTool searches file contents with ripgrep.
type searchFilesTool struct {
	workspaceDir string
}

// NewSearchFilesTool creates a new search files tool scoped to the given workspace.
func NewSearchFilesTool(workspaceDir string) tools.Tool {
	return &searchFilesTool{workspaceDir: workspaceDir}
}

func (t *searchFilesTool) Name() string { return "search_files" }
func (t *searchFilesTool) Description() string {
	return "Search for a regular expression inside files within a directory. " +
		"Relative paths are resolved against the workspace directory. " +
		"Returns matches as file:line:match."
}
func (t *searchFilesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to search within the workspace. Relative paths are resolved against the workspace directory.",
			},
			"regex": map[string]any{
				"type":        "string",
				"description": "Regular expression to search for",
			},
			"file_pattern": map[string]any{
				"type":        "string",
				"description": "Optional glob pattern to filter files (e.g. '*.go')",
			},
		},
		"required": []string{"path", "regex"},
	}
}

// rgEvent represents the subset of ripgrep's --json output we care about.
type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

func (t *searchFilesTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return tools.Errorf("path is required")
	}

	regex, ok := args["regex"].(string)
	if !ok || regex == "" {
		return tools.Errorf("regex is required")
	}

	var filePattern string
	if v, ok := args["file_pattern"].(string); ok {
		filePattern = v
	}

	searchPath, err := workspace.ResolveWorkspacePath(path, t.workspaceDir)
	if err != nil {
		return tools.Errorf("%s", err)
	}

	if _, err := regexp.Compile(regex); err != nil {
		return tools.Errorf("invalid regex: %s", err)
	}

	// search_files shells out to ripgrep; make sure it is available at runtime.
	if _, err := exec.LookPath("rg"); err != nil {
		return tools.Errorf("ripgrep (rg) is not installed; search_files requires it")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rgArgs := []string{"--no-config", "--json"}
	if filePattern != "" {
		rgArgs = append(rgArgs, "-g", filePattern)
	}
	rgArgs = append(rgArgs, "-e", regex, "--", searchPath)

	cmd := exec.CommandContext(ctx, "rg", rgArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, pipeErr := cmd.StdoutPipe()
	if pipeErr != nil {
		return tools.Errorf("search failed: %s", pipeErr)
	}

	if startErr := cmd.Start(); startErr != nil {
		return tools.Errorf("search failed: %s", startErr)
	}

	const maxScanTokenSize = 4 * 1024 * 1024
	scannerBuf := make([]byte, 0, 64*1024)

	var result strings.Builder
	truncated := false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(scannerBuf, maxScanTokenSize)
	for scanner.Scan() {
		var ev rgEvent
		if json.Unmarshal(scanner.Bytes(), &ev) != nil {
			continue
		}
		if ev.Type != "match" {
			continue
		}
		line := strings.TrimSpace(ev.Data.Lines.Text)
		match := fmt.Sprintf("%s:%d:%s", ev.Data.Path.Text, ev.Data.LineNumber, line)

		sep := ""
		if result.Len() > 0 {
			sep = "\n"
		}

		if result.Len()+len(sep)+len(match) > maxOutput {
			allowed := maxOutput - result.Len() - len(sep)
			if allowed > 0 {
				result.WriteString(sep)
				result.WriteString(match[:allowed])
			}
			truncated = true
			cancel()
			break
		}

		result.WriteString(sep)
		result.WriteString(match)
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return tools.Errorf("search failed: reading rg output: %s", scanErr)
	}

	runErr := cmd.Wait()
	_ = stdout.Close()

	if runErr != nil && !truncated {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 && result.Len() == 0 {
			return tools.OK("No matches found.")
		}
		errMsg := runErr.Error()
		if stderr.Len() > 0 {
			errMsg = fmt.Sprintf("%s: %s", errMsg, strings.TrimSpace(stderr.String()))
		}
		return tools.Errorf("search failed: %s", errMsg)
	}

	if result.Len() == 0 {
		return tools.OK("No matches found.")
	}

	output := result.String()
	if truncated {
		output = strings.ToValidUTF8(output, "") + "\n...(truncated)"
	}

	return tools.OK(output)
}
