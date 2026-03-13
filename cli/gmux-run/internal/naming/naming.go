package naming

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
)

// AbducoName generates a sequential abduco session name.
// Format: <kind>:<project>:<N>
// Caller must provide a function to check if a name is taken (socket exists).
func AbducoName(kind, cwd string, taken func(string) bool) string {
	project := filepath.Base(cwd)
	project = strings.ReplaceAll(project, ":", "-")
	project = strings.ReplaceAll(project, "/", "-")

	for n := 1; n <= 100; n++ {
		name := fmt.Sprintf("%s:%s:%d", kind, project, n)
		if !taken(name) {
			return name
		}
	}

	// Fallback to random if 100 sequential names exhausted
	return fmt.Sprintf("%s:%s:%s", kind, project, shortID())
}

// SessionID generates a unique session identifier.
func SessionID() string {
	return "sess-" + shortID()
}

func shortID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
