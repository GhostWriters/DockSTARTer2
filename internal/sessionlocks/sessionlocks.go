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
	"github.com/pelletier/go-toml/v2"
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
	PID        int
	ClientIP   string
	ConnType   string
	LockSource string
	Session    string
	Transport  string
	Terminal   string
}

// ServerInfo holds the details read from a server PID file.
type ServerInfo struct {
	PID     int
	Port    int
	WebPort int
}

// editLockRecord is the TOML structure stored in edit.lock.
type editLockRecord struct {
	PID        int    `toml:"pid"`
	ClientIP   string `toml:"client_ip"`
	ConnType   string `toml:"conn_type"`
	LockSource string `toml:"lock_source"`
	Session    string `toml:"session"`
	Transport  string `toml:"transport"`
	Terminal   string `toml:"terminal"`
}

// serverRecord is the TOML structure stored in server.pid.
type serverRecord struct {
	PID     int `toml:"pid"`
	Port    int `toml:"port"`
	WebPort int `toml:"web_port"`
}

// sessionRecord is the TOML structure stored in each session file.
type sessionRecord struct {
	ClientIP string `toml:"client_ip"`
	ConnType string `toml:"conn_type"`
	Terminal string `toml:"terminal"`
}

func writeTomlFile(path string, v any) (retErr error) {
	defer func() {
		if rec := recover(); rec != nil {
			retErr = fmt.Errorf("panic writing toml file: %v", rec)
		}
	}()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := toml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
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

func (m *SessionManager) AcquireEditLock(clientIP, connType, lockSource, transport string) bool {
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
			terminal := DetectTerminal()
			session := clientIP
			if session == "" || session == "local" {
				session = "local"
				if sshConn := os.Getenv("SSH_CONNECTION"); sshConn != "" {
					if parts := strings.Fields(sshConn); len(parts) >= 2 {
						session = parts[0] + ":" + parts[1]
					} else if len(parts) >= 1 {
						session = parts[0]
					}
				}
			}
			_ = writeTomlFile(m.editLockPath, editLockRecord{
				PID:        os.Getpid(),
				ClientIP:   clientIP,
				ConnType:   connType,
				LockSource: lockSource,
				Session:    session,
				Transport:  transport,
				Terminal:   terminal,
			})
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
	return writeTomlFile(m.serverPIDPath, serverRecord{
		PID:     os.Getpid(),
		Port:    sshPort,
		WebPort: webPort,
	})
}

func (m *SessionManager) ReleaseServer() {
	_ = os.Remove(m.serverPIDPath)
}

// procRecord is the TOML structure stored in each proc file.
type procRecord struct {
	PID       int    `toml:"pid"`
	ExePath   string `toml:"exe"`
	Version   string `toml:"version"`
	Args      string `toml:"args"`
	SSHClient string `toml:"ssh_client"`
	ConnInfo  string `toml:"conn_info"`
	Terminal  string `toml:"terminal"`
}

func (m *SessionManager) procPath(pid int) string {
	return filepath.Join(m.procsDir, strconv.Itoa(pid)+".toml")
}

func writeProcRecord(path string, r procRecord) (retErr error) {
	defer func() {
		if rec := recover(); rec != nil {
			retErr = fmt.Errorf("panic writing proc record: %v", rec)
		}
	}()
	data, err := toml.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func readProcRecord(path string) (procRecord, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return procRecord{}, false
	}
	var r procRecord
	if err := toml.Unmarshal(data, &r); err != nil {
		// Corrupt file — remove it so it doesn't persist.
		_ = os.Remove(path)
		return procRecord{}, false
	}
	return r, true
}

// RegisterProc writes a TOML registration file for the current process under procs/.
func (m *SessionManager) RegisterProc(exePath, currentVersion string) {
	_ = os.MkdirAll(m.procsDir, 0755)
	args := strings.Join(os.Args[1:], " ")
	sshClient := ""
	if sshConn := os.Getenv("SSH_CONNECTION"); sshConn != "" {
		if parts := strings.Fields(sshConn); len(parts) >= 1 {
			sshClient = parts[0]
		}
	}
	terminal := DetectTerminal()
	_ = writeProcRecord(m.procPath(os.Getpid()), procRecord{
		PID:       os.Getpid(),
		ExePath:   exePath,
		Version:   currentVersion,
		Args:      args,
		SSHClient: sshClient,
		Terminal:  terminal,
	})
}

// UpdateProcConnInfo updates the ConnInfo field in the current process's proc file.
func (m *SessionManager) UpdateProcConnInfo(connInfo string) {
	path := m.procPath(os.Getpid())
	r, ok := readProcRecord(path)
	if !ok {
		return
	}
	r.ConnInfo = connInfo
	_ = writeProcRecord(path, r)
}

// UnregisterProc removes the current process's registration file.
func (m *SessionManager) UnregisterProc() {
	pid := os.Getpid()
	_ = os.Remove(m.procPath(pid))
	_ = os.Remove(filepath.Join(m.procsDir, strconv.Itoa(pid)+".restartunsafe"))
}

// MarkRestartUnsafe creates a marker file indicating this process is currently
// in a state where it is unsafe to restart (e.g. editing).
func (m *SessionManager) MarkRestartUnsafe() {
	_ = os.MkdirAll(m.procsDir, 0755)
	_ = os.WriteFile(filepath.Join(m.procsDir, strconv.Itoa(os.Getpid())+".restartunsafe"), []byte{}, 0644)
}

// MarkRestartSafe removes the restart-unsafe marker for this process.
func (m *SessionManager) MarkRestartSafe() {
	_ = os.Remove(filepath.Join(m.procsDir, strconv.Itoa(os.Getpid())+".restartunsafe"))
}

// AnyRestartUnsafe returns true if any live registered process has marked
// itself as unsafe to restart.
func (m *SessionManager) AnyRestartUnsafe() bool {
	entries, err := os.ReadDir(m.procsDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".restartunsafe") {
			continue
		}
		// Derive PID from filename — strip the .restartunsafe suffix.
		pidStr := strings.TrimSuffix(e.Name(), ".restartunsafe")
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			_ = os.Remove(filepath.Join(m.procsDir, e.Name()))
			continue
		}
		if !ProcessExists(pid) {
			_ = os.Remove(filepath.Join(m.procsDir, e.Name()))
			continue
		}
		return true
	}
	return false
}

// ConnectedSession holds information about an active SSH or web connection.
type ConnectedSession struct {
	ID       string // unique session identifier (filename)
	ClientIP string
	ConnType string // "SSH" or "Web"
	Terminal string // terminal or browser identifier
}

// RegisterSession writes a session file for an active incoming connection.
// Returns the session ID to pass to UnregisterSession on disconnect.
// terminal is a human-readable identifier: for SSH it's e.g. "WezTerm/xterm-256color",
// for web it's a simplified browser name from the User-Agent.
func (m *SessionManager) RegisterSession(clientIP, connType, terminal string) string {
	_ = os.MkdirAll(m.sessionsDir, 0755)
	id := fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano())
	path := filepath.Join(m.sessionsDir, id)
	_ = writeTomlFile(path, sessionRecord{
		ClientIP: clientIP,
		ConnType: connType,
		Terminal: terminal,
	})
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
		if serverInfo.PID == 0 || !ProcessExists(serverInfo.PID) {
			_ = os.Remove(path)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var r sessionRecord
		if err := toml.Unmarshal(data, &r); err != nil || r.ClientIP == "" {
			_ = os.Remove(path)
			continue
		}
		sessions = append(sessions, ConnectedSession{
			ID:       e.Name(),
			ConnType: r.ConnType,
			ClientIP: r.ClientIP,
			Terminal: r.Terminal,
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
	Terminal  string // terminal identifier e.g. "WezTerm/xterm-256color"
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		path := filepath.Join(m.procsDir, e.Name())
		r, ok := readProcRecord(path)
		if !ok || r.PID == 0 || !ProcessExists(r.PID) {
			_ = os.Remove(path)
			continue
		}
		if r.PID == self {
			continue
		}
		infos = append(infos, ProcInfo(r))
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

// FormatSession returns a tag-annotated session string matching the instance-list style:
// {{|IPAddress|}}ip:port{{[-]}} ({{|RunningCommand|}}terminal{{[-]}})
// If terminal is empty, the parens are omitted.
func (info SessionInfo) FormatSession() string {
	s := "{{|IPAddress|}}" + info.Session + "{{[-]}}"
	if info.Terminal != "" {
		s += " ({{|RunningCommand|}}" + info.Terminal + "{{[-]}}"
		s += ")"
	}
	return s
}

func (m *SessionManager) ReadEditInfo() SessionInfo {
	// Robust read with retries to handle Windows file-sharing races where fsnotify triggers
	// before the file is fully written or the handle released.
	var r editLockRecord
	for i := 0; i < 5; i++ {
		data, err := os.ReadFile(m.editLockPath)
		if err == nil {
			if toml.Unmarshal(data, &r) == nil && r.PID != 0 {
				break
			}
			// Bad or old-format file — remove it on last retry.
			if i == 4 {
				_ = os.Remove(m.editLockPath)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return SessionInfo(r)
}

func (m *SessionManager) ReadServerInfo() ServerInfo {
	data, err := os.ReadFile(m.serverPIDPath)
	if err != nil {
		return ServerInfo{}
	}
	var r serverRecord
	if err := toml.Unmarshal(data, &r); err != nil {
		// Corrupt file — remove it so a fresh server start can write a clean one.
		_ = os.Remove(m.serverPIDPath)
		return ServerInfo{}
	}
	return ServerInfo(r)
}

// EditLockPID returns the PID of the process currently holding the edit lock.
func (m *SessionManager) EditLockPID() int {
	return m.ReadEditInfo().PID
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
		if info := m.ReadEditInfo(); info.PID != 0 && !ProcessExists(info.PID) {
			_ = os.Remove(m.editLockPath)
		}
	}

	if si := m.ReadServerInfo(); si.PID != 0 && !ProcessExists(si.PID) {
		_ = os.Remove(m.serverPIDPath)
	}

	// Clean stale proc registration files (dead processes).
	if entries, err := os.ReadDir(m.procsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
				continue
			}
			path := filepath.Join(m.procsDir, e.Name())
			r, ok := readProcRecord(path)
			if !ok || r.PID == 0 || !ProcessExists(r.PID) {
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
