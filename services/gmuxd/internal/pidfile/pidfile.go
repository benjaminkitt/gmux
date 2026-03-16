// Package pidfile manages a PID file for gmuxd, enabling clean restarts.
//
// On startup, if an existing PID file points to a live gmuxd process,
// we signal it to shut down and wait for the port to become available.
// This replaces dev-kill.sh and makes `watchexec -r` restarts seamless.
package pidfile

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Acquire writes the current process's PID to the pidfile.
// If an existing process is found, it is signalled to shut down first.
func Acquire(stateDir string) (cleanup func(), err error) {
	path := filepath.Join(stateDir, "gmuxd.pid")

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("pidfile: mkdir: %w", err)
	}

	// Check for existing process.
	if data, err := os.ReadFile(path); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
			if isAlive(pid) {
				log.Printf("pidfile: existing gmuxd at pid %d — sending SIGTERM", pid)
				syscall.Kill(pid, syscall.SIGTERM)
				if !waitForExit(pid, 3*time.Second) {
					log.Printf("pidfile: pid %d did not exit, sending SIGKILL", pid)
					syscall.Kill(pid, syscall.SIGKILL)
					waitForExit(pid, 1*time.Second)
				}
			}
		}
	}

	// Write our PID.
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return nil, fmt.Errorf("pidfile: write: %w", err)
	}

	cleanup = func() {
		os.Remove(path)
	}
	return cleanup, nil
}

// isAlive checks if a process exists (signal 0).
func isAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// waitForExit polls until the process is gone or timeout.
func waitForExit(pid int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			return false
		case <-tick.C:
			if !isAlive(pid) {
				return true
			}
		}
	}
}
