package adapter

import (
	"sync"
	"time"
)

// Generic is the fallback adapter. It matches all commands and produces
// status based on output activity: "active" when output is flowing,
// "paused" after a silence timeout.
type Generic struct {
	silenceTimeout time.Duration

	mu         sync.Mutex
	lastOutput time.Time
	wasActive  bool
}

// NewGeneric creates a generic adapter with the given silence timeout.
// If timeout is 0, defaults to 10 seconds.
func NewGeneric(silenceTimeout time.Duration) *Generic {
	if silenceTimeout <= 0 {
		silenceTimeout = 10 * time.Second
	}
	return &Generic{silenceTimeout: silenceTimeout}
}

func (g *Generic) Name() string { return "generic" }

func (g *Generic) Match(command []string) bool { return true }

func (g *Generic) Prepare(ctx PrepareContext) ([]string, []string) {
	return ctx.Command, nil
}

func (g *Generic) Monitor(output []byte) *Status {
	now := time.Now()
	g.mu.Lock()
	g.lastOutput = now
	wasActive := g.wasActive
	g.wasActive = true
	g.mu.Unlock()

	if !wasActive {
		return &Status{Label: "running", State: "active"}
	}
	return nil
}

// CheckSilence returns a "paused" status if no output has been received
// for longer than the silence timeout. Called periodically by the runner.
// Returns nil if still active or no output has ever been seen.
func (g *Generic) CheckSilence() *Status {
	g.mu.Lock()
	lastOutput := g.lastOutput
	wasActive := g.wasActive
	g.mu.Unlock()

	if !wasActive {
		return nil
	}

	if time.Since(lastOutput) > g.silenceTimeout {
		g.mu.Lock()
		g.wasActive = false
		g.mu.Unlock()
		return &Status{Label: "idle", State: "paused"}
	}
	return nil
}
