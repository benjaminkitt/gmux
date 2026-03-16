package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gmuxapp/gmux/cli/gmux/internal/binhash"
	"github.com/gmuxapp/gmux/cli/gmux/internal/localterm"
	"github.com/gmuxapp/gmux/cli/gmux/internal/naming"
	"github.com/gmuxapp/gmux/cli/gmux/internal/ptyserver"
	"github.com/gmuxapp/gmux/cli/gmux/internal/session"
	"github.com/gmuxapp/gmux/packages/adapter"
	"github.com/gmuxapp/gmux/packages/adapter/adapters"
)

// version is set at build time via -ldflags "-X main.version=..."
// Falls back to "dev" for local builds.
var version = "dev"

func main() {
	log.SetPrefix("gmux: ")
	log.SetFlags(0)

	// Internal subcommand: gmux adapters → print launcher JSON and exit.
	// Used by gmuxd to discover available adapters.
	if len(os.Args) > 1 && os.Args[1] == "adapters" {
		out, _ := json.Marshal(adapters.AllLaunchers())
		fmt.Println(string(out))
		return
	}

	// Internal flags used by gmuxd when launching sessions.
	// Users don't need these — gmux uses cwd from the current directory.
	title := flag.String("title", "", "")
	cwd := flag.String("cwd", "", "")
	flag.Parse()

	args := flag.Args()

	// No args → open the UI in a browser.
	if len(args) == 0 {
		gmuxdAddr := os.Getenv("GMUXD_ADDR")
		if gmuxdAddr == "" {
			gmuxdAddr = "http://localhost:8790"
		}
		ensureGmuxd(gmuxdAddr)

		// Verify gmuxd is actually reachable before opening browser.
		client := &http.Client{Timeout: 3 * time.Second}
		ready := false
		for range 15 {
			if resp, err := client.Get(gmuxdAddr + "/v1/health"); err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					ready = true
					break
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		if !ready {
			log.Fatalf("gmuxd is not running at %s (check %s/gmuxd.log for errors)", gmuxdAddr, os.TempDir())
		}
		openBrowser(gmuxdAddr)
		return
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
	registry.SetFallback(adapters.DefaultFallback())
	a := registry.Resolve(args)

	// Get adapter-specific env vars
	adapterEnv := a.Env(adapter.EnvContext{
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
		ID:          sessionID,
		Command:     args,
		Cwd:         workDir,
		Kind:        a.Name(),
		SocketPath:  sockPath,
		Title:       sessionTitle,
		TitlePinned: *title != "",
		BinaryHash:  binhash.Self(),
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
		Command:    args,
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
		fmt.Printf("command:  %s\n", strings.Join(args, " "))
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

	// Auto-start gmuxd if not running (one-shot, never retried), then register.
	gmuxdAddr := os.Getenv("GMUXD_ADDR")
	if gmuxdAddr == "" {
		gmuxdAddr = "http://localhost:8790"
	}
	if started := ensureGmuxd(gmuxdAddr); started && interactive {
		fmt.Fprintf(os.Stderr, "gmux UI: %s\n", gmuxdAddr)
	}
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
		//   (but we still catch them on gmux in case raw mode is somehow bypassed)
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

// ensureGmuxd checks if gmuxd is reachable and starts it if not.
// Called once at startup — if gmuxd dies later, we don't restart it.
// Returns true if gmuxd was started by this call.
func ensureGmuxd(gmuxdAddr string) bool {
	// Quick health check — if it's already running, nothing to do.
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(gmuxdAddr + "/v1/health")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			return false
		}
	}

	// Not running — find gmuxd binary: sibling first, then PATH.
	var gmuxdBin string
	if self, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(self), "gmuxd")
		if _, err := os.Stat(sibling); err == nil {
			gmuxdBin = sibling
		}
	}
	if gmuxdBin == "" {
		if p, err := exec.LookPath("gmuxd"); err == nil {
			gmuxdBin = p
		}
	}
	if gmuxdBin == "" {
		log.Printf("warning: gmuxd not found (install it alongside gmux or add it to PATH)")
		return false
	}

	// Log gmuxd output to a file so users can diagnose startup failures.
	logPath := filepath.Join(os.TempDir(), "gmuxd.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		logFile = nil
	}

	cmd := exec.Command(gmuxdBin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		log.Printf("warning: could not start gmuxd: %v", err)
		if logFile != nil {
			logFile.Close()
		}
		return false
	}
	go func() {
		cmd.Wait()
		if logFile != nil {
			logFile.Close()
		}
	}()

	log.Printf("started gmuxd (pid %d), log: %s", cmd.Process.Pid, logPath)
	return true
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

	// Retry a few times — gmux may start before the HTTP server is ready
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

// openBrowser opens the gmux UI. Prefers Chrome/Chromium in --app mode
// for a standalone window; falls back to the default browser.
func openBrowser(url string) {
	// Wait briefly for gmuxd to be ready if we just started it.
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for range 10 {
		if resp, err := client.Get(url + "/v1/health"); err == nil {
			ok := resp.StatusCode == 200
			resp.Body.Close()
			if ok {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Try Chrome/Chromium in --app mode first.
	var browsers []string
	switch runtime.GOOS {
	case "darwin":
		// macOS: use open -a to find Chrome. Run() waits for open to exit
		// so we can detect if the app wasn't found (exit code 1).
		for _, app := range []string{"Google Chrome", "Chromium"} {
			cmd := exec.Command("open", "-a", app, "--args", "--app="+url)
			if err := cmd.Run(); err == nil {
				return
			}
		}
	default:
		browsers = []string{"google-chrome-stable", "google-chrome", "chromium-browser", "chromium"}
	}

	for _, browser := range browsers {
		if p, err := exec.LookPath(browser); err == nil {
			cmd := exec.Command(p, "--app="+url)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			if err := cmd.Start(); err == nil {
				return
			}
		}
	}

	// Fallback: default browser.
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

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
