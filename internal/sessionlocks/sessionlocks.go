package sessionlocks

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"context"

	"DockSTARTer2/internal/paths"
	"github.com/gofrs/flock"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of the plaintext password for storage
// in dockstarter2.toml. This is shared between the server and TUI via the
// sessionlocks package to avoid circular dependencies.
func HashPassword(plaintext string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Disconnect requests a graceful disconnect of the active editor session.
func (m *SessionManager) Disconnect(ctx context.Context, force bool) error {
	pid := m.EditLockPID()
	if pid == 0 {
		return nil
	}

	if force {
		return m.forceDisconnect(pid)
	}

	if err := m.RequestDisconnect(); err != nil {
		return err
	}

	// Wait up to 10 seconds for the session to release the lock.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		if m.EditLockPID() == 0 {
			return nil
		}
	}

	return nil
}

func (m *SessionManager) forceDisconnect(pid int) error {
	proc, err := os.FindProcess(pid)
	if err == nil {
		_ = signalProcess(proc)
	}
	m.ForceRelease()
	m.ClearDisconnectRequest()
	return nil
}

// SessionManager tracks the active session state and manages lock files.
type SessionManager struct {
	mu         sync.Mutex
	editActive bool
	editFlock  *flock.Flock

	editLockPath      string // $STATE/locks/edit.lock
	serverPIDPath     string // $STATE/locks/server.pid
	disconnectReqPath  string // $STATE/disconnect.request
	stopReqPath        string // $STATE/stop.request
}

// SessionInfo holds the details read from a session lock file.
type SessionInfo struct {
	PID      int
	ClientIP string
	ConnType string
}

// ServerInfo holds the details read from a server PID file.
type ServerInfo struct {
	PID     int
	Port    int
	WebPort int
}

// Sessions is the global session manager instance.
var Sessions = NewSessionManager()

// NewSessionManager creates a SessionManager and cleans up any stale lock files.
func NewSessionManager() *SessionManager {
	locksDir := paths.GetLocksDir()
	stateDir := paths.GetStateDir()

	_ = os.MkdirAll(locksDir, 0755)

	m := &SessionManager{
		editLockPath:      filepath.Join(locksDir, "edit.lock"),
		serverPIDPath:     filepath.Join(locksDir, "server.pid"),
		disconnectReqPath: filepath.Join(stateDir, "disconnect.request"),
		stopReqPath:       filepath.Join(stateDir, "stop.request"),
	}
	m.cleanStaleLocks()
	return m
}

func (m *SessionManager) IsEditLocked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.editActive {
		return true
	}

	f := flock.New(m.editLockPath)
	locked, err := f.TryLock()
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file or directory") {
			return false
		}
		return true
	}
	if locked {
		_ = f.Unlock()
		return false
	}
	return true
}

// HoldEditLockLocal reports whether the current process holds the edit lock.
func (m *SessionManager) HoldEditLockLocal() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.editActive
}

func (m *SessionManager) AcquireEditLock(clientIP, connType string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.editActive {
		return true
	}

	for i := 0; i < 3; i++ {
		if m.editFlock == nil {
			m.editFlock = flock.New(m.editLockPath)
		}

		locked, err := m.editFlock.TryLock()
		if err == nil && locked {
			m.editActive = true
			_ = writeInfoFile(m.editLockPath, os.Getpid(), clientIP, connType)
			return true
		}

		if err != nil && os.IsPermission(err) {
			_ = os.Remove(m.editLockPath)
			m.editFlock = nil
		}

		if i < 2 {
			m.mu.Unlock()
			time.Sleep(150 * time.Millisecond)
			m.mu.Lock()
			if m.editActive {
				return false
			}
		}
	}

	return false
}

func (m *SessionManager) ReleaseEditLock() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.editActive = false
	if m.editFlock != nil {
		_ = m.editFlock.Unlock()
	}
	_ = os.Remove(m.editLockPath)
}

func (m *SessionManager) AcquireServer(sshPort, webPort int) error {
	return writeInfoFile(m.serverPIDPath, os.Getpid(), strconv.Itoa(sshPort), strconv.Itoa(webPort))
}

func (m *SessionManager) ReleaseServer() {
	_ = os.Remove(m.serverPIDPath)
}

func (m *SessionManager) ReadEditInfo() SessionInfo {
	pid, fields := readInfoFile(m.editLockPath)
	si := SessionInfo{PID: pid}
	if len(fields) > 0 {
		si.ClientIP = fields[0]
	}
	if len(fields) > 1 {
		si.ConnType = fields[1]
	}
	return si
}

func (m *SessionManager) ReadServerInfo() ServerInfo {
	pid, fields := readInfoFile(m.serverPIDPath)
	si := ServerInfo{PID: pid}
	if len(fields) > 0 {
		si.Port, _ = strconv.Atoi(fields[0])
	}
	if len(fields) > 1 {
		si.WebPort, _ = strconv.Atoi(fields[1])
	}
	return si
}

// EditLockPID returns the PID of the process currently holding the edit lock.
func (m *SessionManager) EditLockPID() int {
	pid, _ := readInfoFile(m.editLockPath)
	return pid
}

// ForceRelease removes the edit lock file regardless of state.
// Used by --disconnect to evict a session that is stuck in edit mode.
func (m *SessionManager) ForceRelease() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.editActive = false
	if m.editFlock != nil {
		_ = m.editFlock.Unlock()
	}
	_ = os.Remove(m.editLockPath)
}

func (m *SessionManager) cleanStaleLocks() {
	f := flock.New(m.editLockPath)
	locked, err := f.TryLock()
	if err == nil && locked {
		_ = f.Unlock()
		_ = os.Remove(m.editLockPath)
	}

	pid, _ := readInfoFile(m.serverPIDPath)
	if pid != 0 && !ProcessExists(pid) {
		_ = os.Remove(m.serverPIDPath)
	}
}

func writeInfoFile(path string, pid int, extras ...string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	content := strconv.Itoa(pid) + "\n" + strings.Join(extras, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}

func readInfoFile(path string) (int, []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, nil
	}
	var extras []string
	for _, l := range lines[1:] {
		extras = append(extras, strings.TrimSpace(l))
	}
	return pid, extras
}

func (m *SessionManager) RequestDisconnect() error {
	return os.WriteFile(m.disconnectReqPath, []byte{}, 0644)
}

func (m *SessionManager) ClearDisconnectRequest() {
	_ = os.Remove(m.disconnectReqPath)
}

func (m *SessionManager) IsDisconnectRequested() bool {
	_, err := os.Stat(m.disconnectReqPath)
	return err == nil
}

func (m *SessionManager) RequestStop() error {
	return os.WriteFile(m.stopReqPath, []byte{}, 0644)
}

func (m *SessionManager) ClearStopRequest() {
	_ = os.Remove(m.stopReqPath)
}

func (m *SessionManager) IsStopRequested() bool {
	_, err := os.Stat(m.stopReqPath)
	return err == nil
}
