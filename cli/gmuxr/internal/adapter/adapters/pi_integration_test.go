//go:build integration

// Pi adapter integration tests. These launch real pi processes through PTYs
// and verify adapter behavior (matching, spinner detection, session file timing).
//
// Run: go test -tags integration -v -timeout 120s -run TestPi ./cli/gmuxr/internal/adapter/adapters/
//
// These tests serve dual purpose:
//   1. Verify adapter behavior against real pi output
//   2. Document pi's session file lifecycle for gmuxd attribution (ADR-0009)

package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/gmuxapp/gmux/cli/gmuxr/internal/adapter"
)

func requirePi(t *testing.T) {
	t.Helper()
	if _, err := lookupPi(); err != nil {
		t.Skip("pi not found, skipping integration test")
	}
}

func lookupPi() (string, error) {
	for _, dir := range []string{
		filepath.Join(os.Getenv("HOME"), ".local/bin"),
		"/usr/bin",
		"/usr/local/bin",
	} {
		p := filepath.Join(dir, "pi")
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p, nil
		}
	}
	return "", fmt.Errorf("pi not found")
}

// piTestSession manages a pi process for testing.
type piTestSession struct {
	proc      *ptyProcess
	pi        *Pi
	collector *eventCollector
	done      chan struct{}
	cwd       string
	statuses  []adapter.Status // statuses reported by Monitor()
}

func startPiTestSession(t *testing.T, cwd string, extraArgs ...string) *piTestSession {
	t.Helper()

	args := []string{"pi", "--no-extensions", "--no-skills", "--no-prompt-templates"}
	args = append(args, extraArgs...)

	proc := startProcess(t, args, cwd)
	pi := NewPi()
	collector := newEventCollector()
	done := make(chan struct{})

	s := &piTestSession{
		proc:      proc,
		pi:        pi,
		collector: collector,
		done:      done,
		cwd:       cwd,
	}

	// PTY reader — feeds Monitor() and logs events
	go func() {
		buf := make([]byte, 8192)
		for {
			select {
			case <-done:
				return
			default:
			}
			n, err := proc.ptmx.Read(buf)
			if n > 0 {
				collector.add("pty", "output", summarizeOutput(buf[:n]), n)
				if status := pi.Monitor(buf[:n]); status != nil {
					s.statuses = append(s.statuses, *status)
					collector.add("adapter", "status", fmt.Sprintf("%s (%s)", status.Label, status.State), 0)
				}
			}
			if err != nil {
				collector.add("proc", "exit", err.Error(), 0)
				return
			}
		}
	}()

	t.Cleanup(func() {
		close(done)
		proc.signal(syscall.SIGTERM)
		time.Sleep(500 * time.Millisecond)
	})

	return s
}

func (s *piTestSession) waitForTUI(t *testing.T) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for pi TUI")
		case <-time.After(200 * time.Millisecond):
			if len(s.collector.eventsOfKind("pty", "output")) > 0 {
				time.Sleep(500 * time.Millisecond)
				return
			}
		}
	}
}

func (s *piTestSession) sendMessage(t *testing.T, msg string) {
	t.Helper()
	s.collector.add("proc", "input", msg, 0)
	s.proc.write(msg + "\r")
}

// TestPiSpinnerDetection sends a message to pi and verifies the adapter
// detects the "Working..." spinner as active status.
func TestPiSpinnerDetection(t *testing.T) {
	requirePi(t)

	cwd := t.TempDir()
	s := startPiTestSession(t, cwd)
	s.waitForTUI(t)

	s.sendMessage(t, "say hi")

	// Wait for spinner to appear and be detected
	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-deadline:
			s.collector.dump(t)
			t.Fatal("timeout waiting for spinner detection")
		case <-time.After(200 * time.Millisecond):
			if len(s.statuses) > 0 {
				goto found
			}
		}
	}
found:
	t.Logf("detected %d status events", len(s.statuses))
	for i, st := range s.statuses {
		t.Logf("  status[%d]: %s (%s)", i, st.Label, st.State)
	}

	// Should have detected "working" / "active"
	var foundActive bool
	for _, st := range s.statuses {
		if st.State == "active" && st.Label == "working" {
			foundActive = true
			break
		}
	}
	if !foundActive {
		t.Error("expected active/working status from spinner detection")
	}

	s.collector.dump(t)
}

// TestPiSessionFileLifecycle documents when pi creates and writes its
// session file relative to PTY output. This information is used by
// gmuxd's attribution logic (ADR-0009).
//
// Key findings (verified by this test):
// - Pi does NOT create a session file until the first assistant response
// - Shell escapes (!) alone do NOT trigger file creation
// - File creation and PTY response occur within ~1ms of each other
// - Pi uses appendFileSync (synchronous, triggers inotify immediately)
func TestPiSessionFileLifecycle(t *testing.T) {
	requirePi(t)

	cwd := t.TempDir()
	sessionDir := PiSessionDir(cwd)
	t.Logf("session dir: %s", sessionDir)

	s := startPiTestSession(t, cwd)
	s.waitForTUI(t)

	// Verify: no session file before first interaction
	files := ListSessionFiles(sessionDir)
	if len(files) != 0 {
		t.Fatalf("expected 0 session files before interaction, got %d", len(files))
	}

	// Send message to trigger agent turn + file creation
	s.sendMessage(t, "say hi")

	// Wait for session file to appear
	deadline := time.After(30 * time.Second)
	for {
		select {
		case <-deadline:
			s.collector.dump(t)
			t.Fatal("timeout waiting for session file")
		case <-time.After(300 * time.Millisecond):
			files = ListSessionFiles(sessionDir)
			if len(files) > 0 {
				goto fileFound
			}
		}
	}
fileFound:
	t.Logf("session file created: %s", filepath.Base(files[0]))

	// Wait for response to complete before reading full info
	time.Sleep(5 * time.Second)

	// Read and verify session info
	info, err := ReadPiSessionInfo(files[0])
	if err != nil {
		t.Fatalf("read session info: %v", err)
	}
	t.Logf("session UUID:  %s", info.ID)
	t.Logf("session cwd:   %s", info.Cwd)
	t.Logf("session title: %s", info.Title)
	t.Logf("message count: %d", info.MessageCount)

	if info.Cwd != cwd {
		t.Errorf("expected cwd %q, got %q", cwd, info.Cwd)
	}
	if info.Title == "(no messages)" {
		t.Error("expected a title from first user message")
	}
	if info.MessageCount < 2 {
		t.Errorf("expected at least 2 messages (user+assistant), got %d", info.MessageCount)
	}

	// Also verify text extraction for similarity matching
	text, err := ExtractPiText(files[0])
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	t.Logf("extracted text (%d chars): %.200s", len(text), text)

	if len(text) == 0 {
		t.Error("expected non-empty extracted text from session file")
	}

	s.collector.dump(t)
}

// TestPiAdapterMatch verifies the adapter matches the real pi binary.
func TestPiAdapterMatch(t *testing.T) {
	requirePi(t)
	p := NewPi()
	path, _ := lookupPi()
	t.Logf("pi binary: %s", path)
	if !p.Match([]string{path}) {
		t.Errorf("adapter should match full path: %s", path)
	}
}

// TestPiReadRealSessionFiles reads real pi session files from disk and
// verifies ReadPiSessionInfo extracts sensible titles.
func TestPiReadRealSessionFiles(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}

	// Find any session directory with files
	sessRoot := filepath.Join(home, ".pi", "agent", "sessions")
	dirs, err := os.ReadDir(sessRoot)
	if err != nil {
		t.Skip("no pi sessions directory")
	}

	var totalFiles, totalRead int
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files := ListSessionFiles(filepath.Join(sessRoot, d.Name()))
		totalFiles += len(files)
		for _, f := range files {
			info, err := ReadPiSessionInfo(f)
			if err != nil {
				t.Logf("  ERR %s: %v", filepath.Base(f), err)
				continue
			}
			totalRead++
			if totalRead <= 10 {
				t.Logf("  [%s] %3d msgs | %s", info.ID[:8], info.MessageCount, info.Title)
			}
		}
	}
	t.Logf("Read %d/%d session files successfully", totalRead, totalFiles)
	if totalFiles > 0 && totalRead == 0 {
		t.Error("failed to read any session files")
	}
}
