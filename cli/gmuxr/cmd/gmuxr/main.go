package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gmuxapp/gmux/cli/gmuxr/internal/adapter"
	"github.com/gmuxapp/gmux/cli/gmuxr/internal/adapter/adapters"
	"github.com/gmuxapp/gmux/cli/gmuxr/internal/localterm"
	"github.com/gmuxapp/gmux/cli/gmuxr/internal/naming"
	"github.com/gmuxapp/gmux/cli/gmuxr/internal/ptyserver"
	"github.com/gmuxapp/gmux/cli/gmuxr/internal/session"
)

const version = "0.1.0"

func main() {
	log.SetPrefix("gmuxr: ")
	log.SetFlags(0)

	// Subcommand: gmuxr adapters → print launcher JSON and exit
	if len(os.Args) > 1 && os.Args[1] == "adapters" {
		// Import adapters package to trigger init() registrations
		out, _ := json.Marshal(adapters.Launchers)
		fmt.Println(string(out))
		return
	}

	title := flag.String("title", "", "optional session title")
	cwd := flag.String("cwd", "", "working directory (default: current)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		args = []string{"pi"}
	}

	workDir := *cwd
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			log.Fatalf("cannot determine cwd: %v", err)
		}
	}

	sessionID := naming.SessionID()
	socketDir := os.Getenv("GMUX_SOCKET_DIR")
	if socketDir == "" {
		socketDir = "/tmp/gmux-sessions"
	}
	sockPath := filepath.Join(socketDir, sessionID+".sock")

	// Resolve adapter — registered adapters first, shell fallback
	registry := adapter.NewRegistry()
	for _, a := range adapters.All {
		registry.Register(a)
	}
	registry.SetFallback(adapters.Fallback())
	a := registry.Resolve(args)

	// Let adapter prepare the command and env
	preparedCmd, adapterEnv := a.Prepare(adapter.PrepareContext{
		Command:    args,
		Cwd:        workDir,
		SessionID:  sessionID,
		SocketPath: sockPath,
	})

	// Build session title — use basename for the command if it's a path
	sessionTitle := *title
	if sessionTitle == "" {
		display := make([]string, len(args))
		copy(display, args)
		if len(display) > 0 && strings.Contains(display[0], "/") {
			display[0] = filepath.Base(display[0])
		}
		sessionTitle = strings.Join(display, " ")
	}

	// Create in-memory session state (replaces metadata files)
	state := session.New(session.Config{
		ID:         sessionID,
		Command:    args,
		Cwd:        workDir,
		Kind:       a.Name(),
		SocketPath: sockPath,
		Title:      sessionTitle,
	})

	// Common env vars — set for every child, per ADR-0005
	env := []string{
		"GMUX=1",
		"GMUX_SOCKET=" + sockPath,
		"GMUX_SESSION_ID=" + sessionID,
		"GMUX_ADAPTER=" + a.Name(),
		"GMUX_VERSION=" + version,
	}
	env = append(env, adapterEnv...)

	interactive := localterm.IsInteractive()

	// Determine initial PTY size — use terminal size if interactive
	ptyCfg := ptyserver.Config{
		Command:    preparedCmd,
		Cwd:        workDir,
		Env:        env,
		SocketPath: sockPath,
		Adapter:    a,
		State:      state,
	}
	if interactive {
		if cols, rows, err := localterm.TerminalSize(); err == nil {
			ptyCfg.Cols = cols
			ptyCfg.Rows = rows
		}
	}

	if !interactive {
		fmt.Printf("session:  %s\n", sessionID)
		fmt.Printf("adapter:  %s\n", a.Name())
		fmt.Printf("command:  %s\n", strings.Join(preparedCmd, " "))
	}

	// Start PTY server
	srv, err := ptyserver.New(ptyCfg)
	if err != nil {
		log.Fatalf("failed to start: %v", err)
	}

	state.SetRunning(srv.Pid())

	if !interactive {
		fmt.Printf("pid:      %d\n", srv.Pid())
		fmt.Printf("socket:   %s\n", srv.SocketPath())
		fmt.Println("serving...")
	}

	// Register with gmuxd (best-effort, non-blocking)
	go registerWithGmuxd(sessionID, sockPath)

	if interactive {
		// Transparent mode: attach local terminal to the PTY
		attach, err := localterm.New(localterm.Config{
			PTYWriter: ptyWriterFunc(func(p []byte) (int, error) {
				return srv.WritePTY(p)
			}),
			ResizeFn: srv.Resize,
		})
		if err != nil {
			log.Fatalf("failed to attach terminal: %v", err)
		}
		srv.SetLocalOutput(attach)

		// In interactive mode:
		// - SIGHUP → detach local terminal, keep session running
		// - SIGINT/SIGTERM are consumed by raw mode and forwarded to child via PTY
		//   (but we still catch them on gmuxr in case raw mode is somehow bypassed)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-srv.Done():
			// Child exited — detach and exit
			attach.Detach()
		case <-attach.Done():
			// Local terminal gone (stdin closed) — session continues headless
			srv.SetLocalOutput(nil)
			// Wait for child to exit (session persists, accessible via web UI)
			<-srv.Done()
		case sig := <-sigCh:
			if sig == syscall.SIGHUP {
				// Terminal closed — detach, keep session alive
				attach.Detach()
				srv.SetLocalOutput(nil)
				// Continue running headless until child exits
				<-srv.Done()
			} else {
				// SIGINT/SIGTERM — clean shutdown
				attach.Detach()
				srv.Shutdown()
			}
		}
	} else {
		// Non-interactive: original behavior
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-srv.Done():
			// Child exited
		case sig := <-sigCh:
			fmt.Printf("\nreceived %v, shutting down...\n", sig)
			srv.Shutdown()
		}
	}

	exitCode := srv.ExitCode()
	state.SetExited(exitCode)

	// Deregister from gmuxd (best-effort)
	deregisterFromGmuxd(sessionID)

	if !interactive {
		fmt.Printf("exited:   %d\n", exitCode)
	}
	os.Exit(exitCode)
}

func registerWithGmuxd(sessionID, socketPath string) {
	gmuxdAddr := os.Getenv("GMUXD_ADDR")
	if gmuxdAddr == "" {
		gmuxdAddr = "http://localhost:8790"
	}

	payload, _ := json.Marshal(map[string]string{
		"session_id":  sessionID,
		"socket_path": socketPath,
	})

	// Retry a few times — gmuxr may start before the HTTP server is ready
	for i := 0; i < 5; i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Post(gmuxdAddr+"/v1/register", "application/json", bytes.NewReader(payload))
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return
		}
	}
}

// ptyWriterFunc is an adapter to use a function as an io.Writer.
type ptyWriterFunc func([]byte) (int, error)

func (f ptyWriterFunc) Write(p []byte) (int, error) { return f(p) }

func deregisterFromGmuxd(sessionID string) {
	gmuxdAddr := os.Getenv("GMUXD_ADDR")
	if gmuxdAddr == "" {
		gmuxdAddr = "http://localhost:8790"
	}

	payload, _ := json.Marshal(map[string]string{"session_id": sessionID})
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(gmuxdAddr+"/v1/deregister", "application/json", bytes.NewReader(payload))
	if err != nil {
		return
	}
	resp.Body.Close()
}
