package commands

import (
	"sync"
)

// Registry holds the canonical command set used by both dispatch and
// optional platform registration adapters.
//
// The base set (builtins) is fixed at construction; extras — the slash
// commands contributed by plugins — can be replaced at runtime with
// SetExtras, which is what the agent loop does after every plugin (re)load.
type Registry struct {
	mu     sync.RWMutex
	base   []Definition
	extras []Definition
	defs   []Definition
	index  map[string]int
}

// NewRegistry stores the canonical command set used by both dispatch and
// optional platform registration adapters.
func NewRegistry(defs []Definition) *Registry {
	stored := make([]Definition, len(defs))
	copy(stored, defs)

	registry := &Registry{base: stored}
	registry.rebuild()
	return registry
}

// SetExtras replaces the set of non-builtin definitions. Definitions whose
// name (or alias) collides with an earlier definition are dropped, so base
// definitions always win over plugin commands. Returns the number of extra
// definitions actually registered.
func (r *Registry) SetExtras(defs []Definition) int {
	stored := make([]Definition, len(defs))
	copy(stored, defs)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.extras = stored
	return r.rebuildLocked()
}

// Add appends definitions to the extras set. Definitions whose name is
// already registered are ignored, so calling Add repeatedly (for example on
// every config reload) stays idempotent. Returns the number of definitions
// actually added.
func (r *Registry) Add(defs ...Definition) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	added := 0
	for _, def := range defs {
		key := normalizeCommandName(def.Name)
		if key == "" {
			continue
		}
		if _, exists := r.index[key]; exists {
			continue
		}
		r.extras = append(r.extras, def)
		added++
	}
	if added > 0 {
		r.rebuildLocked()
	}
	return added
}

// Definitions returns all registered command definitions.
// Command availability is global and no longer channel-scoped.
func (r *Registry) Definitions() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Definition, len(r.defs))
	copy(out, r.defs)
	return out
}

// Lookup returns a command definition by normalized command name or alias.
func (r *Registry) Lookup(name string) (Definition, bool) {
	key := normalizeCommandName(name)
	if key == "" {
		return Definition{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	idx, ok := r.index[key]
	if !ok {
		return Definition{}, false
	}
	return r.defs[idx], true
}

// rebuild recomputes defs and the alias index from base + extras.
// Callers must hold the write lock.
func (r *Registry) rebuild() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rebuildLocked()
}

func (r *Registry) rebuildLocked() int {
	defs := make([]Definition, 0, len(r.base)+len(r.extras))
	defs = append(defs, r.base...)

	index := make(map[string]int, (len(r.base)+len(r.extras))*2)
	for i, def := range defs {
		registerCommandName(index, def.Name, i)
		for _, alias := range def.Aliases {
			registerCommandName(index, alias, i)
		}
	}

	registered := 0
	for _, def := range r.extras {
		key := normalizeCommandName(def.Name)
		if key == "" {
			continue
		}
		if _, exists := index[key]; exists {
			continue
		}
		index[key] = len(defs)
		for _, alias := range def.Aliases {
			registerCommandName(index, alias, len(defs))
		}
		defs = append(defs, def)
		registered++
	}

	r.defs = defs
	r.index = index
	return registered
}

func registerCommandName(index map[string]int, name string, defIndex int) {
	key := normalizeCommandName(name)
	if key == "" {
		return
	}
	if _, exists := index[key]; exists {
		return
	}
	index[key] = defIndex
}
