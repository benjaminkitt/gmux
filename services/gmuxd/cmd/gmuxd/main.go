package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gmuxapp/gmux/services/gmuxd/internal/store"
)

func main() {
	sessions := store.NewWithSeeds()
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"ok": true,
			"data": map[string]any{
				"service": "gmuxd",
				"node_id": "node-local",
				"status":  "ready",
			},
		})
	})

	mux.HandleFunc("/v1/capabilities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"ok": true,
			"data": map[string]any{
				"adapters": []string{"pi", "generic"},
				"transport": map[string]any{
					"kind":   "ttyd",
					"replay": false,
				},
			},
		})
	})

	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
			return
		}
		writeJSON(w, map[string]any{"ok": true, "data": sessions.List()})
	})

	mux.HandleFunc("/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		sessionID := parts[2]
		action := ""
		if len(parts) == 4 {
			action = parts[3]
		}

		switch action {
		case "attach":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
				return
			}
			_, ok := sessions.Get(sessionID)
			if !ok {
				writeError(w, http.StatusNotFound, "not_found", "session not found")
				return
			}
			writeJSON(w, map[string]any{
				"ok": true,
				"data": map[string]any{
					"transport": "ttyd",
					"port":      7711,
					"is_new":    true,
				},
			})

		case "kill":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
				return
			}
			if !sessions.Remove(sessionID) {
				writeError(w, http.StatusNotFound, "not_found", "session not found")
				return
			}
			writeJSON(w, map[string]any{"ok": true, "data": map[string]any{}})

		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "bad_request", "method not allowed")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Send current state as upserts
		for _, sess := range sessions.List() {
			s := sess // copy
			sendSSE(w, "session-upsert", store.Event{
				Type:      "session-upsert",
				SessionID: s.SessionID,
				UpdatedAt: s.UpdatedAt,
				Session:   &s,
			})
		}
		flusher.Flush()

		// Stream updates
		ch, cancel := sessions.Subscribe()
		defer cancel()

		notify := r.Context().Done()
		for {
			select {
			case <-notify:
				return
			case ev, open := <-ch:
				if !open {
					return
				}
				sendSSE(w, ev.Type, ev)
				flusher.Flush()
			}
		}
	})

	addr := envOr("GMUXD_ADDR", ":8790")
	log.Printf("gmuxd listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func sendSSE(w http.ResponseWriter, event string, payload any) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, bytes)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": map[string]any{"code": code, "message": message},
	})
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
