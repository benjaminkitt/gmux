// Package ptyserver allocates a PTY, execs a command, and serves
// a WebSocket endpoint on a Unix socket. Replaces abduco + ttyd.
package ptyserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"nhooyr.io/websocket"
)

// ResizeMsg is the JSON message clients send to resize the terminal.
type ResizeMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// Server holds a PTY and serves WebSocket connections.
type Server struct {
	cmd      *exec.Cmd
	ptmx     *os.File
	sockPath string
	listener net.Listener

	mu      sync.Mutex
	clients map[*wsClient]struct{}

	done chan struct{} // closed when child exits
	err  error        // child exit error
}

type wsClient struct {
	conn     *websocket.Conn
	ctx      context.Context
	cancel   context.CancelFunc
	readonly bool
}

// Config for creating a new PTY server.
type Config struct {
	Command    []string
	Cwd        string
	Env        []string
	SocketPath string
	Cols       uint16
	Rows       uint16
}

// New creates and starts a PTY server.
func New(cfg Config) (*Server, error) {
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("no command specified")
	}
	if cfg.Cols == 0 {
		cfg.Cols = 80
	}
	if cfg.Rows == 0 {
		cfg.Rows = 25
	}

	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Dir = cfg.Cwd
	cmd.Env = append(os.Environ(), cfg.Env...)

	// Start command in a new PTY
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: cfg.Cols,
		Rows: cfg.Rows,
	})
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}

	// Ensure socket dir exists and remove stale socket
	os.MkdirAll("/tmp/gmux-sessions", 0o755)
	os.Remove(cfg.SocketPath)

	listener, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		ptmx.Close()
		cmd.Process.Kill()
		return nil, fmt.Errorf("listen unix: %w", err)
	}

	s := &Server{
		cmd:      cmd,
		ptmx:     ptmx,
		sockPath: cfg.SocketPath,
		listener: listener,
		clients:  make(map[*wsClient]struct{}),
		done:     make(chan struct{}),
	}

	// Read PTY output and broadcast to clients
	go s.readPTY()

	// Wait for child process to exit
	go s.waitChild()

	// Serve HTTP/WebSocket on Unix socket
	go s.serve()

	return s, nil
}

// Pid returns the child process PID.
func (s *Server) Pid() int {
	if s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

// SocketPath returns the Unix socket path.
func (s *Server) SocketPath() string {
	return s.sockPath
}

// Done returns a channel that is closed when the child process exits.
func (s *Server) Done() <-chan struct{} {
	return s.done
}

// ExitCode returns the child process exit code (only valid after Done).
func (s *Server) ExitCode() int {
	if s.err == nil {
		return 0
	}
	if exitErr, ok := s.err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

// Shutdown closes the listener and all connections.
func (s *Server) Shutdown() {
	s.listener.Close()
	s.ptmx.Close()
	os.Remove(s.sockPath)

	s.mu.Lock()
	for c := range s.clients {
		c.cancel()
	}
	s.mu.Unlock()
}

func (s *Server) serve() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWS)

	server := &http.Server{Handler: mux}
	server.Serve(s.listener)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // local Unix socket, no origin check needed
	})
	if err != nil {
		log.Printf("ptyserver: ws accept: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	client := &wsClient{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}

	s.mu.Lock()
	s.clients[client] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, client)
		s.mu.Unlock()
		conn.Close(websocket.StatusNormalClosure, "")
		cancel()
	}()

	// Read from WebSocket, write to PTY
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return // client disconnected
		}

		// Text frames might be resize messages
		if typ == websocket.MessageText {
			var msg ResizeMsg
			if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" {
				s.resize(msg.Cols, msg.Rows)
				continue
			}
		}

		// Write input to PTY
		if _, err := s.ptmx.Write(data); err != nil {
			return
		}
	}
}

func (s *Server) resize(cols, rows uint16) {
	if cols == 0 || rows == 0 {
		return
	}
	pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})

	// Send SIGWINCH to the child process group
	if s.cmd.Process != nil {
		syscall.Kill(-s.cmd.Process.Pid, syscall.SIGWINCH)
	}
}

func (s *Server) readPTY() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.broadcast(buf[:n])
		}
		if err != nil {
			return // PTY closed
		}
	}
}

func (s *Server) broadcast(data []byte) {
	s.mu.Lock()
	clients := make([]*wsClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()

	for _, c := range clients {
		// Binary frames for terminal output
		err := c.conn.Write(c.ctx, websocket.MessageBinary, data)
		if err != nil {
			c.cancel()
		}
	}
}

func (s *Server) waitChild() {
	s.err = s.cmd.Wait()
	close(s.done)

	// Give clients a moment to receive final output, then close
	s.mu.Lock()
	for c := range s.clients {
		c.conn.Close(websocket.StatusNormalClosure, "process exited")
	}
	s.mu.Unlock()
}
