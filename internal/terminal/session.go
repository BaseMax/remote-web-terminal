package terminal

import (
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// outputBufSize is the maximum bytes kept per session output ring buffer.
	outputBufSize = 256 * 1024 // 256 KB
	// maxChunkSize is how many bytes to return per poll.
	maxChunkSize = 32 * 1024 // 32 KB
)

// ptyConn is the platform-agnostic PTY interface.
// Implementations live in pty_unix.go and pty_windows.go.
type ptyConn interface {
	io.ReadWriteCloser
	resize(cols, rows uint16) error
	// wait blocks until the child process exits.
	wait() error
}

// Session represents a single PTY terminal session.
type Session struct {
	ID       string
	ptmx     ptyConn
	mu       sync.Mutex
	outBuf   []byte
	closed   bool
	lastSeen time.Time
}

// Manager manages all terminal sessions.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewManager returns a new Manager.
func NewManager() *Manager {
	m := &Manager{sessions: make(map[string]*Session)}
	go m.idleReaper()
	return m
}

// Create starts a new PTY session and returns its ID.
func (m *Manager) Create(shell string, shellArgs []string, cols, rows uint16, idleTimeout time.Duration) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()

	ptmx, err := startPTY(shell, shellArgs, cols, rows)
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}

	sess := &Session{
		ID:       id,
		ptmx:     ptmx,
		outBuf:   make([]byte, 0, 4096),
		lastSeen: time.Now(),
	}

	m.sessions[id] = sess

	// Goroutine: read PTY output into ring buffer
	go sess.readLoop()
	// Goroutine: wait for process exit and mark closed
	go func() {
		_ = ptmx.wait()
		sess.mu.Lock()
		sess.closed = true
		sess.mu.Unlock()
		log.Printf("terminal %s: process exited", id)
	}()

	return sess, nil
}

// Get returns the session with the given ID, or false.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if ok {
		s.mu.Lock()
		s.lastSeen = time.Now()
		s.mu.Unlock()
	}
	return s, ok
}

// Close terminates a session.
func (m *Manager) Close(id string) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if ok {
		sess.destroy()
	}
}

// idleReaper periodically kills sessions that haven't had a heartbeat.
func (m *Manager) idleReaper() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		m.mu.Lock()
		for id, sess := range m.sessions {
			sess.mu.Lock()
			idle := time.Since(sess.lastSeen)
			closed := sess.closed
			sess.mu.Unlock()
			if closed || idle > 10*time.Minute {
				delete(m.sessions, id)
				go sess.destroy()
				log.Printf("terminal %s: reaped (idle=%v closed=%v)", id, idle, closed)
			}
		}
		m.mu.Unlock()
	}
}

// readLoop copies PTY output into the session's ring buffer.
func (s *Session) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.outBuf = append(s.outBuf, buf[:n]...)
			// Trim to last outputBufSize bytes to avoid unbounded growth
			if len(s.outBuf) > outputBufSize {
				trim := len(s.outBuf) - outputBufSize
				s.outBuf = s.outBuf[trim:]
			}
			s.mu.Unlock()
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("terminal %s: read error: %v", s.ID, err)
			}
			return
		}
	}
}

// ReadOutput returns up to maxChunkSize bytes from the output buffer
// starting at the given offset. Returns the next offset to use.
func (s *Session) ReadOutput(offset int) (data []byte, nextOffset int, closed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bufLen := len(s.outBuf)

	if offset > bufLen {
		offset = bufLen
	}

	available := bufLen - offset
	if available <= 0 {
		return nil, bufLen, s.closed
	}
	if available > maxChunkSize {
		available = maxChunkSize
	}
	chunk := make([]byte, available)
	copy(chunk, s.outBuf[offset:offset+available])
	return chunk, offset + available, s.closed
}

// Write sends input bytes to the PTY.
func (s *Session) Write(data []byte) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return fmt.Errorf("session closed")
	}
	_, err := s.ptmx.Write(data)
	return err
}

// Resize changes the PTY window size.
func (s *Session) Resize(cols, rows uint16) error {
	return s.ptmx.resize(cols, rows)
}

func (s *Session) destroy() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	_ = s.ptmx.Close()
}
