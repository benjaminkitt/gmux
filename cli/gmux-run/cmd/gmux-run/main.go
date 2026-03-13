package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gmuxapp/gmux/cli/gmux-run/internal/metadata"
	"github.com/gmuxapp/gmux/cli/gmux-run/internal/naming"
	"github.com/gmuxapp/gmux/cli/gmux-run/internal/ptyserver"
)

func main() {
	log.SetPrefix("gmux-run: ")
	log.SetFlags(0)

	kind := flag.String("adapter", "pi", "session adapter kind (pi, generic, opencode)")
	title := flag.String("title", "", "optional session title")
	cwd := flag.String("cwd", "", "working directory (default: current)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		if *kind == "pi" {
			args = []string{"pi"}
		} else {
			log.Fatal("no command specified")
		}
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
	sockPath := filepath.Join("/tmp/gmux-sessions", sessionID+".sock")

	// Socket existence check for sequential naming (kept for metadata naming)
	sessionName := *kind + ":" + filepath.Base(workDir)

	sessionTitle := *title
	if sessionTitle == "" {
		sessionTitle = strings.Join(args, " ")
	}

	// Write initial metadata
	meta := metadata.New(sessionID, sessionName, *kind, workDir, args)
	meta.SocketPath = sockPath
	if err := meta.Write(); err != nil {
		log.Fatalf("failed to write metadata: %v", err)
	}

	fmt.Printf("session: %s\n", sessionID)
	fmt.Printf("command: %s\n", strings.Join(args, " "))

	// Start PTY server
	srv, err := ptyserver.New(ptyserver.Config{
		Command:    args,
		Cwd:        workDir,
		Env: []string{
			"GMUX_SESSION_ID=" + sessionID,
		},
		SocketPath: sockPath,
	})
	if err != nil {
		meta.SetError(fmt.Sprintf("failed to start: %v", err))
		log.Fatalf("failed to start: %v", err)
	}

	meta.SetRunning(srv.Pid())
	fmt.Printf("pid:     %d\n", srv.Pid())
	fmt.Printf("socket:  %s\n", srv.SocketPath())
	fmt.Println("serving...")

	// Handle signals — clean shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-srv.Done():
		// Child exited
	case sig := <-sigCh:
		fmt.Printf("\nreceived %v, shutting down...\n", sig)
		srv.Shutdown()
	}

	exitCode := srv.ExitCode()
	meta.SetExited(exitCode)
	fmt.Printf("exited:  %d\n", exitCode)
	os.Exit(exitCode)
}
