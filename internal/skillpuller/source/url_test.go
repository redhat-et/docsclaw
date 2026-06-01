package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestURLSource_Pull(t *testing.T) {
	content := "---\nname: test-skill\n---\nDo something useful."

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	defer srv.Close()

	src := &URLSource{Client: srv.Client(), AllowPrivate: true}
	skill, err := src.Pull(context.Background(), srv.URL+"/skills/my-skill/SKILL.md", PullOptions{})
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

func TestURLSource_Pull_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	src := &URLSource{Client: srv.Client(), AllowPrivate: true}
	_, err := src.Pull(context.Background(), srv.URL+"/missing/SKILL.md", PullOptions{})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestSkillNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/skills/my-skill/SKILL.md", "my-skill"},
		{"https://example.com/SKILL.md", "SKILL"},
		{"https://example.com/path/to/summarizer/SKILL.md", "summarizer"},
		{"https://example.com/some-file.yaml", "some-file"},
	}

	for _, tt := range tests {
		got := skillNameFromURL(tt.url)
		if got != tt.want {
			t.Errorf("skillNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
