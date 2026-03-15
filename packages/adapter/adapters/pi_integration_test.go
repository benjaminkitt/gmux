//go:build integration

// Pi adapter integration tests. These launch real pi processes through PTYs
// and verify adapter behavior (matching, spinner detection, session file timing).
//
// Run: go test -tags integration -v -timeout 120s -run TestPi ./packages/adapter/adapters/
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

	"github.com/gmuxapp/gmux/packages/adapter"
	"github.com/gmuxapp/gmux/packages/adapter/adapters/testutil"
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
	proc      *testutil.PTYProcess
	pi        *Pi
	collector *testutil.EventCollector
	done      chan struct{}
	cwd       string
	statuses  []adapter.Status
}

func startPiTestSession(t *testing.T, cwd string, extraArgs ...string) *piTestSession {
	t.Helper()

	args := []string{"pi", "--no-extensions", "--no-skills", "--no-prompt-templates"}
	args = append(args, extraArgs...)

	proc := testutil.StartProcess(t, args, cwd)
	pi := NewPi()
	collector := testutil.NewEventCollector()
	done := make(chan struct{})

	s := &piTestSession{
		proc:      proc,
		pi:        pi,
		collector: collector,
		done:      done,
		cwd:       cwd,
	}

	go func() {
		buf := make([]byte, 8192)
		for {
			select {
			case <-done:
				return
			default:
			}
			n, err := proc.Ptmx.Read(buf)
			if n > 0 {
				collector.Add("pty", "output", testutil.SummarizeOutput(buf[:n]), n)
				if status := pi.Monitor(buf[:n]); status != nil {
					s.statuses = append(s.statuses, *status)
					collector.Add("adapter", "status", fmt.Sprintf("%s (%s)", status.Label, status.State), 0)
				}
			}
			if err != nil {
				collector.Add("proc", "exit", err.Error(), 0)
				return
			}
		}
	}()

	t.Cleanup(func() {
		close(done)
		proc.Signal(syscall.SIGTERM)
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
			if len(s.collector.EventsOfKind("pty", "output")) > 0 {
				time.Sleep(500 * time.Millisecond)
				return
			}
		}
	}
}

func (s *piTestSession) sendMessage(t *testing.T, msg string) {
	t.Helper()
	s.collector.Add("proc", "input", msg, 0)
	s.proc.Write(msg + "\r")
}

func TestPiSpinnerDetection(t *testing.T) {
	requirePi(t)

	cwd := t.TempDir()
	s := startPiTestSession(t, cwd)
	s.waitForTUI(t)
	s.sendMessage(t, "say hi")

	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-deadline:
			s.collector.Dump(t)
			t.Fatal("timeout waiting for spinner detection")
		case <-time.After(200 * time.Millisecond):
			if len(s.statuses) > 0 {
				goto found
			}
		}
	}
found:
	var foundActive bool
	for _, st := range s.statuses {
		if st.State == "active" && st.Label == "working" {
			foundActive = true
		}
	}
	if !foundActive {
		t.Error("expected active/working status from spinner detection")
	}
	s.collector.Dump(t)
}

func TestPiSessionFileLifecycle(t *testing.T) {
	requirePi(t)

	cwd := t.TempDir()
	sessionDir := NewPi().SessionDir(cwd)

	s := startPiTestSession(t, cwd)
	s.waitForTUI(t)

	if files := ListSessionFiles(sessionDir); len(files) != 0 {
		t.Fatalf("expected 0 session files before interaction, got %d", len(files))
	}

	s.sendMessage(t, "say hi")

	var files []string
	deadline := time.After(30 * time.Second)
	for {
		select {
		case <-deadline:
			s.collector.Dump(t)
			t.Fatal("timeout waiting for session file")
		case <-time.After(300 * time.Millisecond):
			files = ListSessionFiles(sessionDir)
			if len(files) > 0 {
				goto fileFound
			}
		}
	}
fileFound:
	time.Sleep(5 * time.Second)

	info, err := NewPi().ParseSessionFile(files[0])
	if err != nil {
		t.Fatalf("parse session file: %v", err)
	}
	t.Logf("UUID: %s  title: %s  msgs: %d", info.ID, info.Title, info.MessageCount)

	if info.Cwd != cwd {
		t.Errorf("expected cwd %q, got %q", cwd, info.Cwd)
	}
	if info.Title == "(new)" {
		t.Error("expected a title from first user message")
	}
	if info.MessageCount < 2 {
		t.Errorf("expected ≥2 messages, got %d", info.MessageCount)
	}

	text, err := ExtractPiText(files[0])
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(text) == 0 {
		t.Error("expected non-empty extracted text")
	}

	s.collector.Dump(t)
}

func TestPiAdapterMatch(t *testing.T) {
	requirePi(t)
	path, _ := lookupPi()
	if !NewPi().Match([]string{path}) {
		t.Errorf("should match full path: %s", path)
	}
}

func TestPiReadRealSessionFiles(t *testing.T) {
	home, _ := os.UserHomeDir()
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
			info, err := NewPi().ParseSessionFile(f)
			if err != nil {
				continue
			}
			totalRead++
			if totalRead <= 10 {
				t.Logf("  [%s] %3d msgs | %s", info.ID[:8], info.MessageCount, info.Title)
			}
		}
	}
	t.Logf("Read %d/%d session files", totalRead, totalFiles)
	if totalFiles > 0 && totalRead == 0 {
		t.Error("failed to read any session files")
	}
}
