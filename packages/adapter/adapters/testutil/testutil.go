//go:build integration

// Package testutil provides PTY helpers and event collection for adapter
// integration tests. Gated behind the "integration" build tag.
package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Event is a timestamped event for logging.
type Event struct {
	Time   time.Time
	Source string // "pty", "fs", "proc", "adapter"
	Kind   string
	Detail string
	Size   int
}

func (e Event) String() string {
	if e.Size > 0 {
		return fmt.Sprintf("[%-7s %-12s] (%d bytes) %s", e.Source, e.Kind, e.Size, e.Detail)
	}
	return fmt.Sprintf("[%-7s %-12s] %s", e.Source, e.Kind, e.Detail)
}

// EventCollector collects timestamped events from multiple goroutines.
type EventCollector struct {
	mu     sync.Mutex
	events []Event
	start  time.Time
}

func NewEventCollector() *EventCollector {
	return &EventCollector{start: time.Now()}
}

func (c *EventCollector) Add(source, kind, detail string, size int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, Event{
		Time:   time.Now(),
		Source: source,
		Kind:   kind,
		Detail: detail,
		Size:   size,
	})
}

func (c *EventCollector) Snapshot() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]Event, len(c.events))
	copy(cp, c.events)
	return cp
}

func (c *EventCollector) Dump(t *testing.T) {
	t.Helper()
	events := c.Snapshot()
	t.Logf("--- %d events ---", len(events))
	for _, ev := range events {
		rel := ev.Time.Sub(c.start).Truncate(time.Millisecond)
		t.Logf("  [%12s] %s", rel, ev)
	}
}

func (c *EventCollector) EventsOfKind(source, kind string) []Event {
	var result []Event
	for _, ev := range c.Snapshot() {
		if ev.Source == source && ev.Kind == kind {
			result = append(result, ev)
		}
	}
	return result
}

// --- PTY process ---

type PTYProcess struct {
	Ptmx *os.File
	PID  int
}

func StartProcess(t *testing.T, args []string, cwd string) *PTYProcess {
	t.Helper()
	binary, err := exec.LookPath(args[0])
	if err != nil {
		t.Skipf("%s not found in PATH, skipping", args[0])
	}

	ptmx, pts, err := openPTY()
	if err != nil {
		t.Fatalf("openpty: %v", err)
	}

	if ws, err := getWinSize(os.Stdout.Fd()); err == nil {
		setWinSize(ptmx.Fd(), ws)
	} else {
		setWinSize(ptmx.Fd(), &winSize{Rows: 24, Cols: 80})
	}

	env := os.Environ()
	env = append(env, "TERM=xterm-256color")

	attr := &syscall.ProcAttr{
		Dir:   cwd,
		Env:   env,
		Files: []uintptr{pts.Fd(), pts.Fd(), pts.Fd()},
		Sys: &syscall.SysProcAttr{
			Setsid:  true,
			Setctty: true,
			Ctty:    0,
		},
	}

	pid, _, err := syscall.StartProcess(binary, args, attr)
	pts.Close()
	if err != nil {
		ptmx.Close()
		t.Fatalf("start process: %v", err)
	}

	return &PTYProcess{Ptmx: ptmx, PID: pid}
}

func (p *PTYProcess) Write(data string) {
	p.Ptmx.Write([]byte(data))
}

func (p *PTYProcess) Signal(sig syscall.Signal) {
	syscall.Kill(-p.PID, sig)
}

// --- Output helpers ---

func SummarizeOutput(data []byte) string {
	s := StripANSI(string(data))
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func StripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) {
			switch s[i+1] {
			case '[':
				j := i + 2
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
				if j < len(s) {
					j++
				}
				i = j
			case ']':
				j := i + 2
				for j < len(s) {
					if s[j] == 0x07 || (s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\') {
						if s[j] == 0x07 {
							j++
						} else {
							j += 2
						}
						break
					}
					j++
				}
				i = j
			default:
				i += 2
			}
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

// --- Low-level PTY ---

func openPTY() (ptmx *os.File, pts *os.File, err error) {
	p, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	ptmx = os.NewFile(uintptr(p), "/dev/ptmx")

	unlock := 0
	if err := unix.IoctlSetPointerInt(p, unix.TIOCSPTLCK, unlock); err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("unlock pty: %w", err)
	}

	sn, err := unix.IoctlGetInt(p, unix.TIOCGPTN)
	if err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("get pty number: %w", err)
	}

	sname := fmt.Sprintf("/dev/pts/%d", sn)
	s, err := unix.Open(sname, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		ptmx.Close()
		return nil, nil, fmt.Errorf("open slave %s: %w", sname, err)
	}
	pts = os.NewFile(uintptr(s), sname)

	return ptmx, pts, nil
}

type winSize struct {
	Rows, Cols, XPixel, YPixel uint16
}

func getWinSize(fd uintptr) (*winSize, error) {
	ws := &winSize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))
	if errno != 0 {
		return nil, errno
	}
	return ws, nil
}

func setWinSize(fd uintptr, ws *winSize) {
	syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(ws)))
}
