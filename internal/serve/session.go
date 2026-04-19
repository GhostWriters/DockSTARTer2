package serve

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"DockSTARTer2/internal/paths"
	"github.com/gofrs/flock"
)

// SessionManager tracks the active session state and manages lock files.
// Only one primary (read-write) session is allowed at a time.
// Write CLI commands (--add, --env-set, etc.) check the session lock before
// proceeding and are rejected if a session is active. Read commands (--list,
// etc.) are always allowed through unconditionally.
type SessionManager struct {
	mu            sync.Mutex
	primaryActive bool
	primaryFlock  *flock.Flock

	sessionLockPath    string // $STATE/session.lock        — active TUI session (PID\nCLIENT_IP\nCONN_TYPE)
	serverPIDPath      string // $STATE/server.pid          — SSH server running (PID\nSSH_PORT\nWEB_PORT)
	disconnectReqPath  string // $STATE/disconnect.request  — graceful disconnect signal
	stopReqPath        string // $STATE/stop.request        — graceful server stop signal
}

// SessionInfo holds the details read from a session lock file.
type SessionInfo struct {
	PID      int
	ClientIP string // empty if not available
	ConnType string // "ssh" or "web"
}

// ServerInfo holds the details read from a server PID file.
type ServerInfo struct {
	PID     int
	Port    int // SSH port
	WebPort int // web port (0 if web server not running)
}

// NewSessionManager creates a SessionManager and cleans up any stale lock
// files left by a previous crashed process.
func NewSessionManager() *SessionManager {
	locksDir := paths.GetLocksDir()
	stateDir := paths.GetStateDir()
	m := &SessionManager{
		sessionLockPath:   filepath.Join(locksDir, "session.lock"),
		serverPIDPath:     filepath.Join(locksDir, "server.pid"),
		disconnectReqPath: filepath.Join(stateDir, "disconnect.request"),
		stopReqPath:       filepath.Join(stateDir, "stop.request"),
	}
	m.cleanStaleLocks()
	return m
}

// IsPrimaryActive reports whether a primary session is currently running.
func (m *SessionManager) IsPrimaryActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. If we are the primary session (process-local), we know it's active.
	if m.primaryActive {
		return true
	}

	// 2. Otherwise, check the lock file on disk using a non-blocking flock.
	f := flock.New(m.sessionLockPath)
	locked, err := f.TryLock()
	if err != nil {
		// If we can't try-lock, assume it's locked by someone else.
		return true
	}
	if locked {
		// We were able to get the lock, so NO ONE else has it.
		_ = f.Unlock()
		return false
	}
	return true
}

// AcquirePrimary marks a primary session as active and writes the session
// lock file including the client IP and connection type ("ssh" or "web").
// Returns an error if a session is already active.
func (m *SessionManager) AcquirePrimary(clientIP, connType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.primaryActive {
		return fmt.Errorf("a session is already active")
	}

	if m.primaryFlock == nil {
		m.primaryFlock = flock.New(m.sessionLockPath)
	}

	locked, err := m.primaryFlock.TryLock()
	if err != nil {
		// If we get a permission error, the file might be a stale lock owned by another user (e.g. root).
		// Attempt to remove it and try one more time.
		if os.IsPermission(err) {
			_ = os.Remove(m.sessionLockPath)
			locked, err = m.primaryFlock.TryLock()
		}
		if err != nil {
			return fmt.Errorf("failed to acquire session lock: %v", err)
		}
	}
	if !locked {
		return fmt.Errorf("a session is already active (locked by another process)")
	}

	m.primaryActive = true
	return writeInfoFile(m.sessionLockPath, os.Getpid(), clientIP, connType)
}

// ReleasePrimary clears the primary session flag and removes the lock file.
func (m *SessionManager) ReleasePrimary() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.primaryActive = false
	if m.primaryFlock != nil {
		_ = m.primaryFlock.Unlock()
	}
	_ = os.Remove(m.sessionLockPath)
}

// AcquireServer writes the server PID file with the SSH and web listening ports.
// Called when the SSH server starts successfully.
func (m *SessionManager) AcquireServer(sshPort, webPort int) error {
	return writeInfoFile(m.serverPIDPath, os.Getpid(), strconv.Itoa(sshPort), strconv.Itoa(webPort))
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
	info, fields := readInfoFile(m.sessionLockPath)
	si := SessionInfo{PID: info.pid}
	if len(fields) > 0 {
		si.ClientIP = fields[0]
	}
	if len(fields) > 1 {
		si.ConnType = fields[1]
	}
	return si
}

// ReadServerInfo returns the server info from the PID file.
// Returns zero-value ServerInfo if the server is not running.
func (m *SessionManager) ReadServerInfo() ServerInfo {
	info, fields := readInfoFile(m.serverPIDPath)
	si := ServerInfo{PID: info.pid}
	if len(fields) > 0 {
		si.Port, _ = strconv.Atoi(fields[0])
	}
	if len(fields) > 1 {
		si.WebPort, _ = strconv.Atoi(fields[1])
	}
	return si
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
	// 1. Session lock: Use flock to check if any process actually holds it.
	f := flock.New(m.sessionLockPath)
	locked, err := f.TryLock()
	if err == nil && locked {
		// We got the lock, so any existing file content is stale.
		_ = f.Unlock()
		_ = os.Remove(m.sessionLockPath)
	}

	// 2. Server PID lock: Use PID existence check (daemon doesn't use flock yet).
	info, _ := readInfoFile(m.serverPIDPath)
	if info.pid != 0 && !ProcessExists(info.pid) {
		_ = os.Remove(m.serverPIDPath)
	}
}

// infoFile is the internal representation of a two-line lock file.
type infoFile struct {
	pid int
}

// writeInfoFile writes a file: PID on line 1, each extra field on subsequent lines.
func writeInfoFile(path string, pid int, extras ...string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	content := strconv.Itoa(pid) + "\n" + strings.Join(extras, "\n") + "\n"
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

// RequestStop writes the graceful stop request file.
// The server watcher goroutine will close active sessions then cancel the server context.
func (m *SessionManager) RequestStop() error {
	return os.WriteFile(m.stopReqPath, []byte{}, 0644)
}

// ClearStopRequest removes the stop request file.
func (m *SessionManager) ClearStopRequest() {
	_ = os.Remove(m.stopReqPath)
}

// IsStopRequested reports whether a graceful stop has been requested.
func (m *SessionManager) IsStopRequested() bool {
	_, err := os.Stat(m.stopReqPath)
	return err == nil
}

// readInfoFile parses a lock file. Returns pid and all extra fields as a slice.
func readInfoFile(path string) (infoFile, []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return infoFile{}, nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return infoFile{}, nil
	}
	var extras []string
	for _, l := range lines[1:] {
		extras = append(extras, strings.TrimSpace(l))
	}
	return infoFile{pid: pid}, extras
}
