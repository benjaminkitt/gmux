package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gmuxapp/gmux/cli/gmux-run/internal/abduco"
	"github.com/gmuxapp/gmux/cli/gmux-run/internal/metadata"
	"github.com/gmuxapp/gmux/cli/gmux-run/internal/naming"
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

	abducoName := naming.AbducoName(*kind, workDir)
	sessionID := naming.SessionID()

	sessionTitle := *title
	if sessionTitle == "" {
		sessionTitle = strings.Join(args, " ")
	}

	// Write initial metadata (starting state)
	meta := metadata.New(sessionID, abducoName, *kind, workDir, args)
	if err := meta.Write(); err != nil {
		log.Fatalf("failed to write metadata: %v", err)
	}

	fmt.Printf("session: %s\n", sessionID)
	fmt.Printf("abduco:  %s\n", abducoName)
	fmt.Printf("command: %s\n", strings.Join(args, " "))

	// Create detached abduco session
	pid, err := abduco.Create(abducoName, args, workDir, []string{
		"GMUX_SESSION_ID=" + sessionID,
		"GMUX_ABDUCO_NAME=" + abducoName,
	})
	if err != nil {
		meta.SetError(fmt.Sprintf("failed to start: %v", err))
		log.Fatalf("failed to create abduco session: %v", err)
	}

	meta.SetRunning(pid)
	fmt.Printf("pid:     %d\n", pid)
	fmt.Printf("socket:  %s\n", abduco.SocketPath(abducoName))
	fmt.Println("monitoring session...")

	// Handle signals — clean shutdown
	stop := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		close(stop)
	}()

	// Monitor: wait for abduco session to disappear
	abduco.WaitForExit(abducoName, 1*time.Second, stop)

	meta.SetExited(0)
	fmt.Println("session ended")
}
