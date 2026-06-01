package skillpuller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/redhat-et/docsclaw/internal/skillpuller/source"
)

type Server struct {
	SkillsDir string
	Port      int
	log       *slog.Logger
	sources   map[string]source.Source
}

func NewServer(skillsDir string, port int) *Server {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	return &Server{
		SkillsDir: skillsDir,
		Port:      port,
		log:       slog.Default(),
		sources: map[string]source.Source{
			"url":    &source.URLSource{Client: httpClient},
			"github": &source.GitHubSource{Client: httpClient},
		},
	}
}

type pullRequest struct {
	Source  string `json:"source"`
	Ref     string `json:"ref"`
	Version string `json:"version,omitempty"`
}

type pullResponse struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Status string `json:"status"`
	Path   string `json:"path"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type listResponse struct {
	Skills []string `json:"skills"`
}

func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	var req pullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	if req.Source == "" || req.Ref == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "source and ref are required"})
		return
	}

	src, ok := s.sources[req.Source]
	if !ok {
		supported := make([]string, 0, len(s.sources))
		for k := range s.sources {
			supported = append(supported, k)
		}
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("unknown source %q, supported: %s", req.Source, strings.Join(supported, ", ")),
		})
		return
	}

	skill, err := src.Pull(r.Context(), req.Ref, source.PullOptions{Version: req.Version})
	if err != nil {
		s.log.Error("pull failed", "source", req.Source, "ref", req.Ref, "error", err)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
		return
	}

	skillDir := filepath.Join(s.SkillsDir, skill.Name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		s.log.Error("create skill dir", "path", skillDir, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create skill directory"})
		return
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, skill.Content, 0o644); err != nil {
		s.log.Error("write skill", "path", skillPath, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to write skill file"})
		return
	}

	s.log.Info("skill pulled", "name", skill.Name, "source", req.Source, "ref", req.Ref)

	writeJSON(w, http.StatusOK, pullResponse{
		Name:   skill.Name,
		Source: req.Source,
		Status: "ok",
		Path:   skill.Name + "/SKILL.md",
	})
}

func (s *Server) handleList(w http.ResponseWriter, _ *http.Request) {
	var skills []string

	entries, err := os.ReadDir(s.SkillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, listResponse{Skills: []string{}})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to read skills directory"})
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(s.SkillsDir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); err == nil {
			skills = append(skills, entry.Name())
		}
	}

	if skills == nil {
		skills = []string{}
	}

	writeJSON(w, http.StatusOK, listResponse{Skills: skills})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, "ok")
}

func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /skills/pull", s.handlePull)
	mux.HandleFunc("GET /skills/list", s.handleList)
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	addr := fmt.Sprintf(":%d", s.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	done := make(chan bool)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		s.log.Info("shutting down skill-puller...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			s.log.Error("shutdown error", "error", err)
		}
		close(done)
	}()

	s.log.Info("skill-puller starting", "addr", addr, "skills_dir", s.SkillsDir)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	<-done
	s.log.Info("skill-puller stopped")
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
