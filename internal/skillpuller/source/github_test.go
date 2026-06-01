package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGithubRawURL(t *testing.T) {
	base := "https://raw.githubusercontent.com"
	tests := []struct {
		ref     string
		version string
		wantURL string
		wantErr bool
	}{
		{
			ref:     "org/repo/path/to/my-skill",
			version: "",
			wantURL: "https://raw.githubusercontent.com/org/repo/main/path/to/my-skill/SKILL.md",
		},
		{
			ref:     "org/repo/path/to/my-skill/SKILL.md",
			version: "v1.2.0",
			wantURL: "https://raw.githubusercontent.com/org/repo/v1.2.0/path/to/my-skill/SKILL.md",
		},
		{
			ref:     "org/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		gotURL, _, err := githubRawURL(tt.ref, tt.version, base)
		if tt.wantErr {
			if err == nil {
				t.Errorf("githubRawURL(%q, %q) expected error", tt.ref, tt.version)
			}
			continue
		}
		if err != nil {
			t.Errorf("githubRawURL(%q, %q) unexpected error: %v", tt.ref, tt.version, err)
			continue
		}
		if gotURL != tt.wantURL {
			t.Errorf("githubRawURL(%q, %q) = %q, want %q", tt.ref, tt.version, gotURL, tt.wantURL)
		}
	}
}

func TestGitHubSource_Pull(t *testing.T) {
	content := "---\nname: test-skill\n---\nGitHub skill."

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/org/repo/main/skills/my-skill/SKILL.md"
		if r.URL.Path != want {
			t.Errorf("request path = %q, want %q", r.URL.Path, want)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	src := &GitHubSource{Client: srv.Client(), BaseURL: srv.URL, AllowPrivate: true}
	skill, err := src.Pull(
		context.Background(),
		"org/repo/skills/my-skill",
		PullOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skill.Name != "my-skill" {
		t.Errorf("name = %q, want %q", skill.Name, "my-skill")
	}
	if string(skill.Content) != content {
		t.Errorf("content = %q, want %q", string(skill.Content), content)
	}
}

func TestGitHubSource_Pull_WithVersion(t *testing.T) {
	content := "---\nname: versioned\n---\nVersioned skill."

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/org/repo/v1.2.0/skills/versioned/SKILL.md"
		if r.URL.Path != want {
			t.Errorf("request path = %q, want %q", r.URL.Path, want)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	src := &GitHubSource{Client: srv.Client(), BaseURL: srv.URL, AllowPrivate: true}
	skill, err := src.Pull(
		context.Background(),
		"org/repo/skills/versioned",
		PullOptions{Version: "v1.2.0"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skill.Name != "versioned" {
		t.Errorf("name = %q, want %q", skill.Name, "versioned")
	}
}

func TestSkillNameFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"path/to/my-skill/SKILL.md", "my-skill"},
		{"SKILL.md", "SKILL"},
		{"skills/summarizer/SKILL.md", "summarizer"},
	}

	for _, tt := range tests {
		got := skillNameFromPath(tt.path)
		if got != tt.want {
			t.Errorf("skillNameFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
