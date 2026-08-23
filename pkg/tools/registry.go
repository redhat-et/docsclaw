package tools

import (
	"fmt"
	"sort"
	"sync"

	"github.com/redhat-et/docsclaw/pkg/llm"
)

type Registry struct {
	mu            sync.RWMutex
	tools         map[string]Tool
	alwaysAllowed map[string]bool
	allowedFilter map[string]bool // nil means all allowed
	aliases       map[string]string
}

func NewRegistry(allowedTools []string) *Registry {
	r := &Registry{
		tools:         make(map[string]Tool),
		alwaysAllowed: make(map[string]bool),
		aliases:       make(map[string]string),
	}
	if len(allowedTools) > 0 {
		r.allowedFilter = make(map[string]bool, len(allowedTools))
		for _, name := range allowedTools {
			r.allowedFilter[name] = true
		}
	}
	return r
}

func (r *Registry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.aliases[t.Name()]; exists {
		return fmt.Errorf("tool name %q conflicts with an existing alias", t.Name())
	}
	r.tools[t.Name()] = t
	return nil
}

func (r *Registry) RegisterAlwaysAllowed(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.aliases[t.Name()]; exists {
		return fmt.Errorf("tool name %q conflicts with an existing alias", t.Name())
	}
	r.tools[t.Name()] = t
	r.alwaysAllowed[t.Name()] = true
	return nil
}

// RegisterAlias maps alias to canonical so that Get(alias) resolves to the
// tool registered under canonical. Aliases are resolved before the allowed
// filter is applied; therefore allowedTools must list the canonical tool
// name, not the alias, for the alias to be usable. RegisterAlias returns an
// error if alias or canonical are empty, or if alias matches a tool name
// already registered in the registry. Register and RegisterAlwaysAllowed
// symmetrically reject a tool whose name matches an existing alias.
func (r *Registry) RegisterAlias(alias, canonical string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if alias == "" {
		return fmt.Errorf("alias cannot be empty")
	}
	if canonical == "" {
		return fmt.Errorf("canonical cannot be empty")
	}
	if existingCanonical, exists := r.aliases[alias]; exists && existingCanonical != canonical {
		return fmt.Errorf("alias %q already maps to %q", alias, existingCanonical)
	}
	if _, exists := r.tools[alias]; exists {
		return fmt.Errorf("alias %q conflicts with an existing tool name", alias)
	}
	r.aliases[alias] = canonical
	return nil
}

func (r *Registry) resolveLocked(name string) string {
	if canonical, ok := r.aliases[name]; ok {
		return canonical
	}
	return name
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	canonical := r.resolveLocked(name)
	t, exists := r.tools[canonical]
	if !exists {
		return nil, false
	}
	if r.allowedFilter != nil && !r.allowedFilter[canonical] && !r.alwaysAllowed[canonical] {
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
