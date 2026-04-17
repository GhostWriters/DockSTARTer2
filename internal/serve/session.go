package serve

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"DockSTARTer2/internal/paths"
)

// SessionManager tracks the active session state and manages lock files.
// Only one primary (read-write) session is allowed at a time.
// Write CLI commands (--add, --env-set, etc.) check the session lock before
// proceeding and are rejected if a session is active. Read commands (--list,
// etc.) are always allowed through unconditionally.
type SessionManager struct {
	mu            sync.Mutex
	primaryActive bool

	sessionLockPath    string // $STATE/session.lock        — active TUI session (PID\nCLIENT_IP)
	serverPIDPath      string // $STATE/server.pid          — SSH server running (PID\nPORT)
	disconnectReqPath  string // $STATE/disconnect.request  — graceful disconnect signal
}

// SessionInfo holds the details read from a session lock file.
type SessionInfo struct {
	PID      int
	ClientIP string // empty if not available
}

// ServerInfo holds the details read from a server PID file.
type ServerInfo struct {
	PID  int
	Port int
}

// NewSessionManager creates a SessionManager and cleans up any stale lock
// files left by a previous crashed process.
func NewSessionManager() *SessionManager {
	stateDir := paths.GetStateDir()
	m := &SessionManager{
		sessionLockPath:   filepath.Join(stateDir, "session.lock"),
		serverPIDPath:     filepath.Join(stateDir, "server.pid"),
		disconnectReqPath: filepath.Join(stateDir, "disconnect.request"),
	}
	m.cleanStaleLocks()
	return m
}

// IsPrimaryActive reports whether a primary session is currently running.
func (m *SessionManager) IsPrimaryActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.primaryActive
}

// AcquirePrimary marks a primary session as active and writes the session
// lock file including the client IP address.
// Returns an error if a session is already active.
func (m *SessionManager) AcquirePrimary(clientIP string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.primaryActive {
		return fmt.Errorf("a session is already active")
	}
	m.primaryActive = true
	return writeInfoFile(m.sessionLockPath, os.Getpid(), clientIP)
}

// ReleasePrimary clears the primary session flag and removes the lock file.
func (m *SessionManager) ReleasePrimary() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.primaryActive = false
	_ = os.Remove(m.sessionLockPath)
}

// AcquireServer writes the server PID file with the listening port.
// Called when the SSH server starts successfully.
func (m *SessionManager) AcquireServer(port int) error {
	return writeInfoFile(m.serverPIDPath, os.Getpid(), strconv.Itoa(port))
}

// ReleaseServer removes the server PID file.
// Called when the SSH server shuts down.
func (m *SessionManager) ReleaseServer() {
	_ = os.Remove(m.serverPIDPath)
}

// SessionLockPID returns the PID stored in the session lock file, or 0 if
// the file does not exist or cannot be parsed.
func (m *SessionManager) SessionLockPID() int {
	info, _ := readInfoFile(m.sessionLockPath)
	return info.pid
}

// ReadSessionInfo returns the session info from the lock file.
// Returns zero-value SessionInfo if no session is active.
func (m *SessionManager) ReadSessionInfo() SessionInfo {
	info, extra := readInfoFile(m.sessionLockPath)
	return SessionInfo{PID: info.pid, ClientIP: extra}
}

// ReadServerInfo returns the server info from the PID file.
// Returns zero-value ServerInfo if the server is not running.
func (m *SessionManager) ReadServerInfo() ServerInfo {
	info, extra := readInfoFile(m.serverPIDPath)
	port, _ := strconv.Atoi(extra)
	return ServerInfo{PID: info.pid, Port: port}
}

// ForceRelease removes the session lock file regardless of state.
// Used by --disconnect to evict a crashed or hung session.
func (m *SessionManager) ForceRelease() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.primaryActive = false
	_ = os.Remove(m.sessionLockPath)
}

// cleanStaleLocks removes lock files left by processes that no longer exist.
func (m *SessionManager) cleanStaleLocks() {
	for _, p := range []string{m.sessionLockPath, m.serverPIDPath} {
		info, _ := readInfoFile(p)
		if info.pid != 0 && !processExists(info.pid) {
			_ = os.Remove(p)
		}
	}
}

// infoFile is the internal representation of a two-line lock file.
type infoFile struct {
	pid int
}

// writeInfoFile writes a two-line file: PID on line 1, extra on line 2.
func writeInfoFile(path string, pid int, extra string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	content := strconv.Itoa(pid) + "\n" + extra + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}

// RequestDisconnect writes the graceful disconnect request file.
// The active SSH session handler watches for this and closes cleanly.
func (m *SessionManager) RequestDisconnect() error {
	return os.WriteFile(m.disconnectReqPath, []byte{}, 0644)
}

// ClearDisconnectRequest removes the disconnect request file.
// Called by the SSH handler after it has acted on the request.
func (m *SessionManager) ClearDisconnectRequest() {
	_ = os.Remove(m.disconnectReqPath)
}

// IsDisconnectRequested reports whether a graceful disconnect has been requested.
func (m *SessionManager) IsDisconnectRequested() bool {
	_, err := os.Stat(m.disconnectReqPath)
	return err == nil
}

// readInfoFile parses a two-line lock file. Returns pid and extra string.
func readInfoFile(path string) (infoFile, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return infoFile{}, ""
	}
	lines := strings.SplitN(strings.TrimRight(string(data), "\n"), "\n", 2)
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return infoFile{}, ""
	}
	extra := ""
	if len(lines) > 1 {
		extra = strings.TrimSpace(lines[1])
	}
	return infoFile{pid: pid}, extra
}
