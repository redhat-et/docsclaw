package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

type URLSource struct {
	Client *http.Client
}

func (s *URLSource) Pull(ctx context.Context, ref string, _ PullOptions) (*Skill, error) {
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", ref, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %q: %w", ref, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %q: status %d", ref, resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", ref, err)
	}

	name := skillNameFromURL(ref)

	return &Skill{Name: name, Content: content}, nil
}

// skillNameFromURL extracts a skill name from a URL path.
// For ".../skill-name/SKILL.md" returns "skill-name".
// For ".../SKILL.md" returns "SKILL".
// For other paths returns the last path segment.
func skillNameFromURL(rawURL string) string {
	p := path.Base(rawURL)

	if strings.EqualFold(p, "SKILL.md") {
		dir := path.Dir(rawURL)
		parent := path.Base(dir)
		if parent != "" && parent != "." && parent != "/" &&
			!strings.Contains(parent, ".") {
			return parent
		}
		return strings.TrimSuffix(p, path.Ext(p))
	}

	return strings.TrimSuffix(p, path.Ext(p))
}
