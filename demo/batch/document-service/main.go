package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Document struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Sensitivity        string `json:"sensitivity"`
	RequiredDepartment string `json:"required_department"`
	Content            string `json:"content"`
}

func loadDocuments(dir string) (map[string]Document, error) {
	docs := make(map[string]Document)

	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", dir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no JSON files found in %s", dir)
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var batch []Document
		if err := json.Unmarshal(data, &batch); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		for _, d := range batch {
			docs[d.ID] = d
		}
		slog.Info("loaded documents", "file", filepath.Base(f), "count", len(batch))
	}
	return docs, nil
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dataDir := flag.String("data-dir", "data", "directory containing JSON document files")
	flag.Parse()

	docs, err := loadDocuments(*dataDir)
	if err != nil {
		slog.Error("failed to load documents", "error", err)
		os.Exit(1)
	}
	slog.Info("document-service ready", "documents", len(docs), "addr", *addr)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"healthy"}`)
	})

	mux.HandleFunc("GET /documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		doc, ok := docs[id]
		if !ok {
			slog.Warn("document not found", "id", id)
			http.Error(w, fmt.Sprintf(`{"error":"document %q not found"}`, id), http.StatusNotFound)
			return
		}
		slog.Info("serving document", "id", id, "title", doc.Title)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"document": doc})
	})

	mux.HandleFunc("GET /documents", func(w http.ResponseWriter, r *http.Request) {
		dept := r.URL.Query().Get("department")
		var list []map[string]string
		for _, d := range docs {
			if dept != "" && !strings.EqualFold(d.RequiredDepartment, dept) {
				continue
			}
			list = append(list, map[string]string{
				"id":    d.ID,
				"title": d.Title,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"documents": list})
	})

	if err := http.ListenAndServe(*addr, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
