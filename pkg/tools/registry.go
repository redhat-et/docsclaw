package tools

import (
	"sort"
	"sync"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

type Registry struct {
	mu            sync.RWMutex
	tools         map[string]Tool
	alwaysAllowed map[string]bool
	allowedFilter map[string]bool // nil means all allowed; an empty map allows none
}

func NewRegistry(allowedTools []string) *Registry {
	r := &Registry{
		tools:         make(map[string]Tool),
		alwaysAllowed: make(map[string]bool),
	}
	// A nil slice means "no filter" (allow all); an explicit empty slice means
	// "allow none". Callers that build a registry from config must pass an
	// empty, non-nil slice for an omitted allowlist so tools are denied.
	if allowedTools != nil {
		r.allowedFilter = make(map[string]bool, len(allowedTools))
		for _, name := range allowedTools {
			r.allowedFilter[name] = true
		}
	}
	return r
}

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
	// A plain registration is allowlist-governed even if an earlier
	// always-allowed entry (e.g. an MCP tool) used the same name.
	delete(r.alwaysAllowed, t.Name())
}

func (r *Registry) RegisterAlwaysAllowed(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
	r.alwaysAllowed[t.Name()] = true
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, exists := r.tools[name]
	if !exists {
		return nil, false
	}
	if r.allowedFilter != nil && !r.allowedFilter[name] && !r.alwaysAllowed[name] {
		return nil, false
	}
	return t, true
}

func (r *Registry) Definitions() []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var defs []llm.ToolDefinition
	for name, t := range r.tools {
		if r.allowedFilter != nil && !r.allowedFilter[name] && !r.alwaysAllowed[name] {
			continue
		}
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})
	return defs
}
