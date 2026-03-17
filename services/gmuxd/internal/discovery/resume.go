package discovery

import (
	"strings"
	"sync"
)

// PendingResumes tracks sessions being resumed. When gmux registers,
// we match by command (which contains the unique session file path)
// and merge into the existing store entry.
type PendingResumes struct {
	mu sync.Mutex
	// key: joined command string → store session ID (the file-xxx entry)
	pending map[string]string
}

func NewPendingResumes() *PendingResumes {
	return &PendingResumes{pending: make(map[string]string)}
}

// Add records that a resumable session is being resumed.
func (pr *PendingResumes) Add(command []string, sessionID string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.pending[strings.Join(command, "\x00")] = sessionID
}

// Has checks if a command matches a pending resume without consuming it.
// Used by discovery to avoid creating duplicate sessions for runners
// that haven't called Register() yet.
func (pr *PendingResumes) Has(command []string) bool {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	_, ok := pr.pending[strings.Join(command, "\x00")]
	return ok
}

// Take checks if a newly registered session matches a pending resume.
// Matches by exact command array. Consumes the entry.
func (pr *PendingResumes) Take(command []string) (string, bool) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	key := strings.Join(command, "\x00")
	id, ok := pr.pending[key]
	if ok {
		delete(pr.pending, key)
	}
	return id, ok
}
