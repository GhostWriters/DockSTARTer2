package sessionlocks

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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
	procsDir          string // $STATE/locks/procs/    — one file per running TUI/daemon instance
	versionsDir       string // $STATE/locks/versions/ — one file per executable path
	sessionsDir       string // $STATE/locks/sessions/ — one file per active SSH/web connection
	disconnectReqPath string // $STATE/disconnect.request
	stopReqPath       string // $STATE/stop.request

	localOwner string // tracks which part of the current process holds the lock (e.g. "Menu", "Console")
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
		procsDir:          filepath.Join(locksDir, "procs"),
		versionsDir:       filepath.Join(locksDir, "versions"),
		sessionsDir:       filepath.Join(locksDir, "sessions"),
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
		return m.localOwner == connType
	}

	for i := 0; i < 3; i++ {
		if m.editFlock == nil {
			m.editFlock = flock.New(m.editLockPath)
		}

		locked, err := m.editFlock.TryLock()
		if err == nil && locked {
			m.editActive = true
			m.localOwner = connType
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
	if !m.editActive {
		return
	}
	m.editActive = false
	m.localOwner = ""
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

// RegisterProc writes a registration file for the current process under procs/.
// Stores PID, resolved exe path, running version, command-line args, and
// SSH client info (if connected via SSH). Call at startup; pair with UnregisterProc.
func (m *SessionManager) RegisterProc(exePath, currentVersion string) {
	_ = os.MkdirAll(m.procsDir, 0755)
	path := filepath.Join(m.procsDir, strconv.Itoa(os.Getpid()))
	args := strings.Join(os.Args[1:], " ")
	// Capture SSH client IP if this process was started over an SSH session.
	sshClient := ""
	if sshConn := os.Getenv("SSH_CONNECTION"); sshConn != "" {
		// SSH_CONNECTION is "clientIP clientPort serverIP serverPort"
		if parts := strings.Fields(sshConn); len(parts) >= 1 {
			sshClient = parts[0]
		}
	}
	_ = writeInfoFile(path, os.Getpid(), exePath, currentVersion, args, sshClient)
}

// UpdateProcConnInfo rewrites the current process's registration file with
// additional connection info (e.g. SSH/web server ports once the server starts).
func (m *SessionManager) UpdateProcConnInfo(connInfo string) {
	path := filepath.Join(m.procsDir, strconv.Itoa(os.Getpid()))
	pid, fields := readInfoFile(path)
	if pid == 0 {
		return
	}
	// fields: [exePath, version, args, sshClient]
	// Replace or append connInfo as fields[4]
	for len(fields) < 5 {
		fields = append(fields, "")
	}
	fields[4] = connInfo
	_ = writeInfoFile(path, pid, fields...)
}

// UnregisterProc removes the current process's registration file.
func (m *SessionManager) UnregisterProc() {
	_ = os.Remove(filepath.Join(m.procsDir, strconv.Itoa(os.Getpid())))
}

// ConnectedSession holds information about an active SSH or web connection.
type ConnectedSession struct {
	ID       string // unique session identifier (filename)
	ClientIP string
	ConnType string // "SSH" or "Web"
}

// RegisterSession writes a session file for an active incoming connection.
// Returns the session ID to pass to UnregisterSession on disconnect.
func (m *SessionManager) RegisterSession(clientIP, connType string) string {
	_ = os.MkdirAll(m.sessionsDir, 0755)
	// Use PID + nanosecond timestamp for a unique session ID.
	id := fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano())
	path := filepath.Join(m.sessionsDir, id)
	_ = os.WriteFile(path, []byte(connType+"\n"+clientIP+"\n"), 0644)
	return id
}

// UnregisterSession removes the session file for a disconnecting client.
func (m *SessionManager) UnregisterSession(id string) {
	_ = os.Remove(filepath.Join(m.sessionsDir, id))
}

// ListConnectedSessions returns all active connected sessions.
// Stale files (whose server process is dead) are removed.
func (m *SessionManager) ListConnectedSessions() []ConnectedSession {
	entries, err := os.ReadDir(m.sessionsDir)
	if err != nil {
		return nil
	}
	serverInfo := m.ReadServerInfo()
	var sessions []ConnectedSession
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(m.sessionsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// Check if the server that owns this session is still alive.
		if serverInfo.PID == 0 || !ProcessExists(serverInfo.PID) {
			_ = os.Remove(path)
			continue
		}
		lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
		if len(lines) < 2 {
			_ = os.Remove(path)
			continue
		}
		sessions = append(sessions, ConnectedSession{
			ID:       e.Name(),
			ConnType: lines[0],
			ClientIP: lines[1],
		})
	}
	return sessions
}

// ProcInfo holds information about a registered running instance.
type ProcInfo struct {
	PID       int
	ExePath   string
	Version   string
	Args      string
	SSHClient string // client IP if this process was started over SSH, else ""
	ConnInfo  string // additional connection info (e.g. "SSH:40022 Web:40080" for server)
}

// ListProcInfos returns info for all live registered processes, excluding the
// caller. Stale files (dead processes) are removed.
func (m *SessionManager) ListProcInfos() []ProcInfo {
	entries, err := os.ReadDir(m.procsDir)
	if err != nil {
		return nil
	}
	self := os.Getpid()
	var infos []ProcInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(m.procsDir, e.Name())
		pid, fields := readInfoFile(path)
		if pid == 0 || !ProcessExists(pid) {
			_ = os.Remove(path)
			continue
		}
		if pid == self {
			continue
		}
		info := ProcInfo{PID: pid}
		if len(fields) > 0 {
			info.ExePath = fields[0]
		}
		if len(fields) > 1 {
			info.Version = fields[1]
		}
		if len(fields) > 2 {
			info.Args = fields[2]
		}
		if len(fields) > 3 {
			info.SSHClient = fields[3]
		}
		if len(fields) > 4 {
			info.ConnInfo = fields[4]
		}
		infos = append(infos, info)
	}
	return infos
}

// exeVersionPath returns the path of the installed-version file for the given
// resolved executable path. The filename is a short hex hash of the exe path
// so it is filesystem-safe regardless of the original path content.
func (m *SessionManager) exeVersionPath(exePath string) string {
	sum := sha256.Sum256([]byte(exePath))
	return filepath.Join(m.versionsDir, fmt.Sprintf("%x.version", sum[:8]))
}

// WriteInstalledVersion records the version that was just installed for the
// given executable. Any running instance of that binary will detect the change
// on its next poll and set RestartPending.
func (m *SessionManager) WriteInstalledVersion(exePath, newVersion string) error {
	if err := os.MkdirAll(m.versionsDir, 0755); err != nil {
		return err
	}
	path := m.exeVersionPath(exePath)
	return os.WriteFile(path, []byte(newVersion+"\n"), 0644)
}

// ReadInstalledVersion returns the version recorded in the installed-version
// file for the given executable, or "" if no file exists.
func (m *SessionManager) ReadInstalledVersion(exePath string) string {
	_ = os.MkdirAll(m.versionsDir, 0755)
	path := m.exeVersionPath(exePath)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SeedInstalledVersion writes the current running version to the installed-
// version file if it is missing or out of date. Called at every startup so the
// file is always present for the watcher to compare against.
func (m *SessionManager) SeedInstalledVersion(exePath, currentVersion string) {
	if m.ReadInstalledVersion(exePath) == currentVersion {
		return
	}
	_ = m.WriteInstalledVersion(exePath, currentVersion)
}

func (m *SessionManager) ReadEditInfo() SessionInfo {
	// Robust read with retries to handle Windows file-sharing races where fsnotify triggers
	// before the file is fully written or the handle released.
	var pid int
	var fields []string
	for i := 0; i < 5; i++ {
		pid, fields = readInfoFile(m.editLockPath)
		if pid != 0 && len(fields) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

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
	m.localOwner = ""
	if m.editFlock != nil {
		_ = m.editFlock.Unlock()
	}
	_ = os.Remove(m.editLockPath)
}

func (m *SessionManager) cleanStaleLocks() {
	// Clean stale edit lock. Two checks:
	// 1. If TryLock succeeds, no process holds it — remove it.
	// 2. If the PID in the lock file is dead, force-remove it regardless
	//    of flock state (handles PID reuse and race conditions on re-exec).
	f := flock.New(m.editLockPath)
	locked, err := f.TryLock()
	if err == nil && locked {
		_ = f.Unlock()
		_ = os.Remove(m.editLockPath)
	} else {
		pid, _ := readInfoFile(m.editLockPath)
		if pid != 0 && !ProcessExists(pid) {
			_ = os.Remove(m.editLockPath)
		}
	}

	pid, _ := readInfoFile(m.serverPIDPath)
	if pid != 0 && !ProcessExists(pid) {
		_ = os.Remove(m.serverPIDPath)
	}

	// Clean stale proc registration files (dead processes).
	if entries, err := os.ReadDir(m.procsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(m.procsDir, e.Name())
			pid, _ := readInfoFile(path)
			if pid == 0 || !ProcessExists(pid) {
				_ = os.Remove(path)
			}
		}
	}

	// Version files in versionsDir are intentionally persistent — no cleanup needed.
}

// ResolvedExePath returns the real path of the current executable with symlinks resolved.
func ResolvedExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}
	return real
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
