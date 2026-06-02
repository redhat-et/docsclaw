package skillpuller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/redhat-et/docsclaw/internal/skillpuller/source"
)

func newTestServer(skillsDir string) *Server {
	srv := NewServer(skillsDir, 0)
	srv.sources["url"] = &source.URLSource{AllowPrivate: true}
	srv.sources["github"] = &source.GitHubSource{AllowPrivate: true}
	return srv
}

func TestHandlePull_URL(t *testing.T) {
	skillContent := "---\nname: fetched-skill\n---\nA test skill."

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(skillContent))
	}))
	defer upstream.Close()

	skillsDir := t.TempDir()
	srv := newTestServer(skillsDir)

	body, _ := json.Marshal(pullRequest{
		Source: "url",
		Ref:    upstream.URL + "/skills/test-skill/SKILL.md",
	})

	req := httptest.NewRequest(http.MethodPost, "/skills/pull", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.handlePull(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp pullResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}

	if resp.Name != "test-skill" {
		t.Errorf("name = %q, want %q", resp.Name, "test-skill")
	}

	got, err := os.ReadFile(filepath.Join(skillsDir, "test-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("skill file not written: %v", err)
	}
	if string(got) != skillContent {
		t.Errorf("file content = %q, want %q", string(got), skillContent)
	}
}

func TestHandlePull_InvalidSource(t *testing.T) {
	srv := NewServer(t.TempDir(), 0)

	body, _ := json.Marshal(pullRequest{Source: "ftp", Ref: "example.com/skill"})
	req := httptest.NewRequest(http.MethodPost, "/skills/pull", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.handlePull(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandlePull_MissingFields(t *testing.T) {
	srv := NewServer(t.TempDir(), 0)

	body, _ := json.Marshal(pullRequest{Source: "url"})
	req := httptest.NewRequest(http.MethodPost, "/skills/pull", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.handlePull(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleList_Empty(t *testing.T) {
	srv := NewServer(t.TempDir(), 0)

	req := httptest.NewRequest(http.MethodGet, "/skills/list", nil)
	w := httptest.NewRecorder()

	srv.handleList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp listResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(resp.Skills) != 0 {
		t.Errorf("skills = %v, want empty", resp.Skills)
	}
}

func TestHandleList_WithSkills(t *testing.T) {
	skillsDir := t.TempDir()

	for _, name := range []string{"skill-a", "skill-b"} {
		dir := filepath.Join(skillsDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(skillsDir, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(skillsDir, 0)

	req := httptest.NewRequest(http.MethodGet, "/skills/list", nil)
	w := httptest.NewRecorder()

	srv.handleList(w, req)

	var resp listResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(resp.Skills) != 2 {
		t.Fatalf("skills count = %d, want 2; got %v", len(resp.Skills), resp.Skills)
	}
	slices.Sort(resp.Skills)
	if resp.Skills[0] != "skill-a" || resp.Skills[1] != "skill-b" {
		t.Errorf("skills = %v, want [skill-a skill-b]", resp.Skills)
	}
}

func TestHandlePull_PathTraversal(t *testing.T) {
	srv := newTestServer(t.TempDir())

	// Inject a skill with a malicious name directly (simulates a
	// source that returns a crafted skill name)
	srv.sources["url"] = &pathTraversalSource{}

	body, _ := json.Marshal(pullRequest{
		Source: "url",
		Ref:    "anything",
	})

	req := httptest.NewRequest(http.MethodPost, "/skills/pull", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.handlePull(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

type pathTraversalSource struct{}

func (p *pathTraversalSource) Pull(_ context.Context, _ string, _ source.PullOptions) (*source.Skill, error) {
	return &source.Skill{Name: "../../etc", Content: []byte("malicious")}, nil
}

func TestHandleHealthz(t *testing.T) {
	srv := NewServer(t.TempDir(), 0)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	srv.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
