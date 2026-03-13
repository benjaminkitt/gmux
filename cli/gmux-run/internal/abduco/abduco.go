package abduco

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SocketDir returns the abduco socket directory for the current user.
func SocketDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".abduco")
}

// SocketPath returns the full socket path for a given abduco session name.
// abduco appends @hostname to the socket file.
func SocketPath(name string) string {
	hostname, _ := os.Hostname()
	return filepath.Join(SocketDir(), name+"@"+hostname)
}

// SessionAlive checks if an abduco session socket exists.
func SessionAlive(name string) bool {
	_, err := os.Stat(SocketPath(name))
	return err == nil
}

// Create launches a new detached abduco session.
// Returns the PID of the abduco server process.
func Create(name string, command []string, cwd string, extraEnv []string) (int, error) {
	abducoPath, err := exec.LookPath("abduco")
	if err != nil {
		return 0, fmt.Errorf("abduco not found: %w", err)
	}

	args := append([]string{"-n", name}, command...)
	cmd := exec.Command(abducoPath, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), extraEnv...)

	// abduco -n forks a server and the client returns immediately.
	// We need to capture the server PID from the process list after.
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("abduco create failed: %w", err)
	}

	// Give the server a moment to start
	time.Sleep(50 * time.Millisecond)

	pid := findAbducoPid(name)
	return pid, nil
}

// WaitForExit polls until the abduco session socket disappears.
func WaitForExit(name string, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if !SessionAlive(name) {
				return
			}
		}
	}
}

// findAbducoPid tries to find the abduco server PID.
// On Linux, search /proc. Otherwise return 0 (unknown).
func findAbducoPid(name string) int {
	if runtime.GOOS != "linux" {
		return 0
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		cmd := string(cmdline)
		if strings.Contains(cmd, "abduco") && strings.Contains(cmd, name) {
			var pid int
			if _, err := fmt.Sscanf(entry.Name(), "%d", &pid); err == nil {
				return pid
			}
		}
	}
	return 0
}
