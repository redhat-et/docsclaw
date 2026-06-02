package source

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

type GitHubSource struct {
	Client       *http.Client
	BaseURL      string // override for testing; defaults to raw.githubusercontent.com
	AllowPrivate bool   // skip SSRF checks (for testing only)
}

func (s *GitHubSource) Pull(ctx context.Context, ref string, opts PullOptions) (*Skill, error) {
	baseURL := s.BaseURL
	if baseURL == "" {
		baseURL = "https://raw.githubusercontent.com"
	}

	rawURL, skillName, err := githubRawURL(ref, opts.Version, baseURL)
	if err != nil {
		return nil, err
	}

	urlSource := &URLSource{Client: s.Client, AllowPrivate: s.AllowPrivate}
	skill, err := urlSource.Pull(ctx, rawURL, PullOptions{})
	if err != nil {
		return nil, fmt.Errorf("github pull %q: %w", ref, err)
	}

	skill.Name = skillName
	return skill, nil
}

// githubRawURL converts a GitHub ref like "owner/repo/path/to/skill"
// into a raw.githubusercontent.com URL. Returns the URL and the
// skill name (last segment of the path).
//
// If the path does not end with SKILL.md, it is appended automatically.
var safeSegment = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func githubRawURL(ref, version, baseURL string) (string, string, error) {
	parts := strings.SplitN(ref, "/", 3)
	if len(parts) < 3 {
		return "", "", fmt.Errorf("github ref must be owner/repo/path, got %q", ref)
	}

	owner, repo, filePath := parts[0], parts[1], parts[2]

	if !safeSegment.MatchString(owner) {
		return "", "", fmt.Errorf("invalid owner %q", owner)
	}
	if !safeSegment.MatchString(repo) {
		return "", "", fmt.Errorf("invalid repo %q", repo)
	}

	if version == "" {
		version = "main"
	}
	if !safeSegment.MatchString(version) {
		return "", "", fmt.Errorf("invalid version %q", version)
	}

	if !strings.HasSuffix(filePath, "SKILL.md") {
		filePath = filePath + "/SKILL.md"
	}

	// Extract skill name from the directory containing SKILL.md
	skillName := skillNameFromPath(filePath)

	rawURL := fmt.Sprintf("%s/%s/%s/%s/%s",
		baseURL, owner, repo, version, filePath)

	return rawURL, skillName, nil
}

// skillNameFromPath extracts the skill name from a file path.
// For "path/to/my-skill/SKILL.md" returns "my-skill".
func skillNameFromPath(p string) string {
	parts := strings.Split(p, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.EqualFold(parts[i], "SKILL.md") && i > 0 {
			return parts[i-1]
		}
	}
	if len(parts) > 0 {
		return strings.TrimSuffix(parts[len(parts)-1], ".md")
	}
	return "unknown"
}
