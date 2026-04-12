package acp

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type SessionInfo struct {
	ThreadKey    string    `json:"thread_key"`
	SessionID    string    `json:"session_id"`
	CreatedAt    time.Time `json:"created_at"`
	LastActive   time.Time `json:"last_active"`
	MessageCount uint64    `json:"message_count"`
	Alive        bool      `json:"alive"`
}

type SessionPool struct {
	connections map[string]*AcpConnection
	mu          sync.RWMutex
	command     string
	args        []string
	workingDir  string
	env         map[string]string
	maxSessions int
}

func (p *SessionPool) WorkingDir() string {
	return p.workingDir
}

func NewSessionPool(command string, args []string, workingDir string, env map[string]string, maxSessions int) *SessionPool {
	return &SessionPool{
		connections: make(map[string]*AcpConnection),
		command:     command,
		args:        args,
		workingDir:  workingDir,
		env:         env,
		maxSessions: maxSessions,
	}
}

func (p *SessionPool) GetOrCreate(threadID string) error {
	// Check if alive connection exists (read lock)
	p.mu.RLock()
	if conn, ok := p.connections[threadID]; ok && conn.Alive() {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	// Need to create or rebuild (write lock)
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if conn, ok := p.connections[threadID]; ok {
		if conn.Alive() {
			return nil
		}
		slog.Warn("stale connection, rebuilding", "thread_id", threadID)
		conn.Kill()
		delete(p.connections, threadID)
	}

	if len(p.connections) >= p.maxSessions {
		// LRU eviction: kill the least recently used session
		var lruKey string
		var lruTime time.Time
		for key, conn := range p.connections {
			if lruTime.IsZero() || conn.LastActive.Before(lruTime) {
				lruKey = key
				lruTime = conn.LastActive
			}
		}
		if lruKey != "" {
			slog.Info("evicting LRU session", "evicted_key", lruKey, "last_active", lruTime)
			p.connections[lruKey].Kill()
			delete(p.connections, lruKey)
		}
	}

	conn, err := SpawnConnection(p.command, p.args, p.workingDir, p.env, threadID)
	if err != nil {
		return err
	}

	if err := conn.Initialize(); err != nil {
		conn.Kill()
		return err
	}

	if _, err := conn.SessionNew(p.workingDir); err != nil {
		conn.Kill()
		return err
	}

	if _, existed := p.connections[threadID]; existed {
		conn.SessionReset = true
	}

	p.connections[threadID] = conn
	return nil
}

// WithConnection provides access to a connection. Caller must have called GetOrCreate first.
func (p *SessionPool) WithConnection(threadID string, fn func(conn *AcpConnection) error) error {
	p.mu.Lock()
	conn, ok := p.connections[threadID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("no connection for thread %s", threadID)
	}
	p.mu.Unlock()
	return fn(conn)
}

func (p *SessionPool) CleanupIdle(ttlSecs int64) {
	cutoff := time.Now().Add(-time.Duration(ttlSecs) * time.Second)

	p.mu.Lock()
	defer p.mu.Unlock()

	var stale []string
	for key, conn := range p.connections {
		if conn.LastActive.Before(cutoff) || !conn.Alive() {
			stale = append(stale, key)
		}
	}

	for _, key := range stale {
		slog.Info("cleaning up idle session", "thread_id", key)
		if conn, ok := p.connections[key]; ok {
			conn.Kill()
		}
		delete(p.connections, key)
	}
}

func (p *SessionPool) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	count := len(p.connections)
	for _, conn := range p.connections {
		conn.Kill()
	}
	p.connections = make(map[string]*AcpConnection)
	slog.Info("pool shutdown complete", "count", count)
}

// ListSessions returns a snapshot of all active sessions.
func (p *SessionPool) ListSessions() []SessionInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	sessions := make([]SessionInfo, 0, len(p.connections))
	for key, conn := range p.connections {
		sessions = append(sessions, SessionInfo{
			ThreadKey:    key,
			SessionID:    conn.SessionID,
			CreatedAt:    conn.CreatedAt,
			LastActive:   conn.LastActive,
			MessageCount: conn.MessageCount.Load(),
			Alive:        conn.Alive(),
		})
	}
	return sessions
}

// GetSessionInfo returns metadata for a specific session.
func (p *SessionPool) GetSessionInfo(threadKey string) (*SessionInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	conn, ok := p.connections[threadKey]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", threadKey)
	}
	return &SessionInfo{
		ThreadKey:    threadKey,
		SessionID:    conn.SessionID,
		CreatedAt:    conn.CreatedAt,
		LastActive:   conn.LastActive,
		MessageCount: conn.MessageCount.Load(),
		Alive:        conn.Alive(),
	}, nil
}

// KillSession terminates a specific session and removes it from the pool.
func (p *SessionPool) KillSession(threadKey string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	conn, ok := p.connections[threadKey]
	if !ok {
		return fmt.Errorf("session not found: %s", threadKey)
	}
	slog.Info("killing session", "thread_key", threadKey)
	conn.Kill()
	delete(p.connections, threadKey)
	return nil
}

// Stats returns pool utilization.
func (p *SessionPool) Stats() (active int, max int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.connections), p.maxSessions
}
