// Package discovery watches /tmp/gmux-meta for session metadata files
// written by gmux-run, and syncs them into the session store.
package discovery

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gmuxapp/gmux/services/gmuxd/internal/store"
)

const MetaDir = "/tmp/gmux-meta"

type metaFile struct {
	Version    int      `json:"version"`
	SessionID  string   `json:"session_id"`
	AbducoName string   `json:"abduco_name"`
	Kind       string   `json:"kind"`
	Command    []string `json:"command"`
	Cwd        string   `json:"cwd"`
	State      string   `json:"state"`
	CreatedAt  float64  `json:"created_at"`
	UpdatedAt  float64  `json:"updated_at"`
	Pid        int      `json:"pid,omitempty"`
	ExitCode   *int     `json:"exit_code,omitempty"`
	Error      string   `json:"error,omitempty"`
}

func toSession(m metaFile) store.Session {
	title := ""
	if len(m.Command) > 0 {
		title = m.Command[0]
	}
	return store.Session{
		SessionID:  m.SessionID,
		AbducoName: m.AbducoName,
		Title:      title,
		Kind:       m.Kind,
		State:      m.State,
		UpdatedAt:  m.UpdatedAt,
	}
}

// Watch polls the metadata directory and syncs discovered sessions into the store.
// It runs until the stop channel is closed.
func Watch(sessions *store.Store, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			scan(sessions)
		}
	}
}

func scan(sessions *store.Store) {
	entries, err := os.ReadDir(MetaDir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("discovery: read dir: %v", err)
		}
		return
	}

	seen := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(MetaDir, entry.Name()))
		if err != nil {
			continue
		}

		var meta metaFile
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		if meta.SessionID == "" || meta.Version != 1 {
			continue
		}

		seen[meta.SessionID] = true

		existing, exists := sessions.Get(meta.SessionID)
		sess := toSession(meta)

		if !exists {
			sessions.Upsert(sess)
		} else if existing.State != sess.State || existing.UpdatedAt < sess.UpdatedAt {
			sessions.Upsert(sess)
		}
	}
}
