package adapter

import "os"

// Registry holds adapters in priority order and resolves which one
// handles a given command.
type Registry struct {
	adapters []Adapter
	fallback Adapter
}

// NewRegistry creates a registry with built-in adapters.
// The generic adapter is always the fallback.
func NewRegistry() *Registry {
	return &Registry{
		adapters: nil, // no specific adapters yet; pi, pytest, etc. added later
		fallback: NewGeneric(0),
	}
}

// Register adds an adapter to the registry. Adapters are tried in
// registration order; first match wins. The generic fallback is always last.
func (r *Registry) Register(a Adapter) {
	r.adapters = append(r.adapters, a)
}

// Resolve picks the adapter for the given command.
//
// Priority:
//  1. GMUX_ADAPTER env var — explicit override, bypass matching
//  2. Walk registered adapters, first Match() wins
//  3. Generic fallback
func (r *Registry) Resolve(command []string) Adapter {
	// Tier 1: explicit override
	if name := os.Getenv("GMUX_ADAPTER"); name != "" {
		for _, a := range r.adapters {
			if a.Name() == name {
				return a
			}
		}
		// If explicit name doesn't match any registered adapter,
		// fall through to matching (don't error — might be a typo,
		// better to degrade gracefully)
	}

	// Tier 2: auto-match
	for _, a := range r.adapters {
		if a.Match(command) {
			return a
		}
	}

	// Tier 3: generic fallback
	return r.fallback
}
