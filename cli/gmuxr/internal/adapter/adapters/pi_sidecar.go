package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PiSessionDir computes pi's session directory for a given cwd.
// Pi encodes: strip leading /, replace remaining / with -, wrap in --.
// /home/mg/dev/gmux → --home-mg-dev-gmux--
func PiSessionDir(cwd string) string {
	home, _ := os.UserHomeDir()
	path := strings.TrimPrefix(cwd, "/")
	encoded := "--" + strings.ReplaceAll(path, "/", "-") + "--"
	return filepath.Join(home, ".pi", "agent", "sessions", encoded)
}

// PiSessionInfo holds metadata extracted from a pi JSONL session file.
// Mirrors pi's own buildSessionInfo() priority: session_info.name if
// present, first user message as fallback title.
type PiSessionInfo struct {
	ID           string    // UUID from session header
	Cwd          string    // working directory
	Created      time.Time // from session header timestamp
	Title        string    // session_info.name or first user message
	MessageCount int       // total message entries
	FilePath     string    // path to the .jsonl file
}

// ReadPiSessionInfo reads a pi JSONL session file and extracts metadata
// for display in the sidebar. Reads just enough lines to get the title
// (name from session_info, or first user message).
func ReadPiSessionInfo(path string) (*PiSessionInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty file")
	}

	// Parse header (first line)
	var header struct {
		Type      string `json:"type"`
		Version   int    `json:"version"`
		ID        string `json:"id"`
		Cwd       string `json:"cwd"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		return nil, err
	}
	if header.Type != "session" {
		return nil, fmt.Errorf("not a session header: type=%s", header.Type)
	}

	created, _ := time.Parse(time.RFC3339Nano, header.Timestamp)

	info := &PiSessionInfo{
		ID:       header.ID,
		Cwd:      header.Cwd,
		Created:  created,
		FilePath: path,
	}

	// Scan entries for name and first user message
	var name string
	var firstUserMsg string

	for _, line := range lines[1:] {
		if line == "" {
			continue
		}

		// Quick type check without full parse
		var peek struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &peek); err != nil {
			continue
		}

		switch peek.Type {
		case "session_info":
			var si struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal([]byte(line), &si); err == nil && si.Name != "" {
				name = strings.TrimSpace(si.Name)
			}

		case "message":
			info.MessageCount++
			if firstUserMsg == "" {
				firstUserMsg = extractFirstUserText(line)
			}
		}
	}

	// Priority: explicit name > first user message > fallback
	switch {
	case name != "":
		info.Title = name
	case firstUserMsg != "":
		info.Title = truncateTitle(firstUserMsg, 80)
	default:
		info.Title = "(no messages)"
	}

	return info, nil
}

// extractFirstUserText parses a message JSONL line and returns the text
// if it's a user message. Returns "" for non-user or non-text messages.
func extractFirstUserText(line string) string {
	var entry struct {
		Message *struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Message == nil {
		return ""
	}
	if entry.Message.Role != "user" {
		return ""
	}

	// Content can be string or array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(entry.Message.Content, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return b.Text
			}
		}
		return ""
	}

	// Try plain string
	var s string
	if err := json.Unmarshal(entry.Message.Content, &s); err == nil {
		return s
	}
	return ""
}

// truncateTitle shortens text to maxLen, breaking at word boundary.
func truncateTitle(s string, maxLen int) string {
	s = strings.Join(strings.Fields(s), " ") // collapse whitespace
	if len(s) <= maxLen {
		return s
	}
	// Find last space before maxLen
	cut := strings.LastIndex(s[:maxLen], " ")
	if cut < maxLen/2 {
		cut = maxLen // no good break point, hard cut
	}
	return s[:cut] + "…"
}

// ExtractPiText reads a pi JSONL session file and extracts conversation
// text suitable for similarity matching. Returns concatenated text content
// from message entries (user, assistant, tool results).
//
// This is the pi-specific text extractor for ADR-0009 content similarity
// matching. It extracts "text" values from content arrays in message
// entries, ignoring JSON structure, keys, and non-text content types.
func ExtractPiText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		// Quick check: does this line contain text content?
		if !strings.Contains(line, `"text"`) {
			continue
		}
		// Parse and extract text values from content arrays
		var entry struct {
			Type    string `json:"type"`
			Message *struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Message == nil {
			continue
		}
		// Content can be a string or array of content blocks
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(entry.Message.Content, &blocks); err != nil {
			// Try as plain string
			var s string
			if err := json.Unmarshal(entry.Message.Content, &s); err == nil {
				out.WriteString(s)
				out.WriteByte(' ')
			}
			continue
		}
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				out.WriteString(b.Text)
				out.WriteByte(' ')
			}
		}
	}
	return out.String(), nil
}

// ListSessionFiles returns all .jsonl files in a directory.
func ListSessionFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}
