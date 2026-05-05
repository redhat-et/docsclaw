package openaiapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/redhat-et/docsclaw/pkg/llm"
	"github.com/redhat-et/docsclaw/pkg/tools"
)

// Handler serves the OpenAI-compatible Chat Completions API.
type Handler struct {
	Provider     llm.Provider
	SystemPrompt string
	Registry     *tools.Registry
	LoopConfig   tools.LoopConfig
	AgentCard    *a2a.AgentCard
	AgentName    string
}

// ChatCompletion handles POST /v1/chat/completions.
func (h *Handler) ChatCompletion(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "invalid_request_error",
				"payload_too_large", "Request body too large (max 1MB).")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request_error",
			"invalid_json", "Failed to parse request body: "+err.Error())
		return
	}

	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request_error",
			"invalid_messages", "Messages must not be empty.")
		return
	}

	msgs, systemPrompt := ConvertMessages(req.Messages, h.SystemPrompt)

	id := GenerateID()
	model := "docsclaw"
	if h.Provider != nil {
		model = h.Provider.Model()
	}

	if req.Stream {
		h.streamCompletion(w, r, id, model, systemPrompt, msgs)
		return
	}

	content, usage, err := h.complete(r.Context(), systemPrompt, msgs)
	if err != nil {
		slog.Error("LLM completion failed", "error", err)
		writeError(w, http.StatusBadGateway, "server_error",
			"llm_error", "LLM error: "+err.Error())
		return
	}

	resp := BuildResponse(id, model, content, usage)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// streamCompletion handles streaming responses. Phase 1 uses real
// provider streaming; phase 2 runs the tool loop then simulates.
func (h *Handler) streamCompletion(w http.ResponseWriter,
	r *http.Request, id, model, systemPrompt string, msgs []llm.Message) {

	if h.Provider == nil {
		StreamResponse(w, id, model, "LLM provider not configured.")
		return
	}

	allMsgs := append([]llm.Message{{
		Role:    "system",
		Content: systemPrompt,
	}}, msgs...)

	// Phase 2: tool loop + simulated streaming
	if h.Registry != nil && len(h.Registry.Definitions()) > 0 {
		content, err := tools.RunToolLoop(r.Context(),
			h.Provider, allMsgs, h.Registry, h.LoopConfig)
		if err != nil {
			slog.Error("tool loop failed", "error", err)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			StreamError(w, "LLM error: "+err.Error())
			return
		}
		StreamResponse(w, id, model, content)
		return
	}

	// Phase 1: real streaming from provider
	onEvent, _ := StreamFromProvider(w, id, model)
	_, err := h.Provider.StreamWithTools(r.Context(), allMsgs, nil, onEvent)
	if err != nil {
		slog.Error("streaming failed", "error", err)
		StreamError(w, "LLM error: "+err.Error())
	}
}

// Models handles GET /v1/models.
func (h *Handler) Models(w http.ResponseWriter, _ *http.Request) {
	model := "docsclaw"
	description := fmt.Sprintf("Agent %q", h.AgentName)
	if h.Provider != nil {
		description += fmt.Sprintf(" backed by %s", h.Provider.Model())
	}

	list := ModelList{
		Object: "list",
		Data: []ModelObject{
			{
				ID:          model,
				Object:      "model",
				Created:     time.Now().Unix(),
				OwnedBy:     "docsclaw",
				Description: description,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// Skills handles GET /v1/skills.
func (h *Handler) Skills(w http.ResponseWriter, _ *http.Request) {
	skills := make([]SkillObject, 0)

	if h.AgentCard != nil {
		for _, s := range h.AgentCard.Skills {
			skills = append(skills, SkillObject{
				ID:          s.ID,
				Name:        s.Name,
				Description: s.Description,
			})
		}
	}

	list := SkillList{Skills: skills}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// complete runs the LLM completion, using the tool loop in phase 2
// or a simple single-shot call in phase 1.
func (h *Handler) complete(ctx context.Context, systemPrompt string,
	msgs []llm.Message) (string, llm.Usage, error) {

	if h.Provider == nil {
		return "LLM provider not configured.", llm.Usage{}, nil
	}

	allMsgs := append([]llm.Message{{
		Role:    "system",
		Content: systemPrompt,
	}}, msgs...)

	// Phase 2: agentic tool-use loop.
	if h.Registry != nil && len(h.Registry.Definitions()) > 0 {
		content, err := tools.RunToolLoop(ctx, h.Provider, allMsgs,
			h.Registry, h.LoopConfig)
		if err != nil {
			return "", llm.Usage{}, err
		}
		return content, llm.Usage{}, nil
	}

	// Phase 1: pass full history via CompleteWithTools (no tools).
	resp, err := h.Provider.CompleteWithTools(ctx, allMsgs, nil)
	if err != nil {
		return "", llm.Usage{}, err
	}
	return resp.Content, resp.Usage, nil
}

// writeError sends an OpenAI-compatible JSON error response.
func writeError(w http.ResponseWriter, status int, errType, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{Message: msg, Type: errType, Code: code},
	})
}
