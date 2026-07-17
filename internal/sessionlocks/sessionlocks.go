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
	"github.com/google/uuid"
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
	mu                  sync.Mutex
	editActive          bool
	editFlock           *flock.Flock
	procFlock           *flock.Flock // shared flock held on our own .toml for the process lifetime
	restartUnsafeFlock  *flock.Flock // shared flock held on our .restartunsafe file while unsafe

	editLockPath      string // $STATE/locks/edit.lock
	serverPIDPath     string // $STATE/locks/server.pid — kept for legacy cleanup only
	procsDir          string // $STATE/locks/procs/    — one file per running TUI/daemon instance
	versionsDir       string // $STATE/locks/versions/ — one file per executable path
	sessionsDir       string // $STATE/locks/sessions/ — one file per active SSH/web connection
	disconnectReqPath string // $STATE/disconnect.request
	stopReqPath       string // $STATE/stop.request

	localOwner      string // tracks which part of the current process holds the lock (e.g. "Menu", "Console")
	localSource     string // "menu", "cli", or "console" — informational only, recorded in the lock file
	localSessionKey string // identifies the actual session/process holding the lock, for re-entry checks (see heldByLocked) -- a server-daemon process serves many sessions that all use the same lockSource, so lockSource alone can't tell them apart
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

// ServerInfo holds details about a running server daemon instance.
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

// sessionRecord is the TOML structure stored in each session file.
type sessionRecord struct {
	ClientIP  string `toml:"client_ip"`
	ConnType  string `toml:"conn_type"`
	Terminal  string `toml:"terminal"`
	ServerPID int    `toml:"server_pid"`
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

// HoldEditLockLocal reports whether this process holds the edit lock,
// regardless of which of its sessions acquired it. Correct for
// restart-safety checks; use HoldEditLockAs to check a specific session.
func (m *SessionManager) HoldEditLockLocal() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.editActive
}

// HoldEditLockAs reports whether the given session holds the edit lock via
// plain navigation (lockSource "menu"). Use this (not HoldEditLockLocal)
// to decide whether the lock reads as "held by others" for a session's
// destructive menu items: a different session's lock always counts as
// others, and so does the owning session's own lock if it's for an
// actively-running command ("console", "cli", "menu:<action>") rather than
// navigation -- an in-progress operation should still block other
// destructive items in the same session.
func (m *SessionManager) HoldEditLockAs(sessionKey string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.heldByLocked(sessionKey) && m.localSource == "menu"
}

// heldByLocked is the single definition of "does session sessionKey hold the
// edit lock", shared by AcquireEditLock's re-entry check and HoldEditLockAs
// so the two can't drift out of sync. Caller must already hold m.mu.
func (m *SessionManager) heldByLocked(sessionKey string) bool {
	return m.editActive && m.localSessionKey == sessionKey
}

// sessionKey identifies the actual caller (session or standalone process),
// not merely which subsystem it entered through -- see localSessionKey.
func (m *SessionManager) AcquireEditLock(clientIP, connType, lockSource, transport, sessionKey string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.editActive {
		return m.heldByLocked(sessionKey)
	}

	for i := 0; i < 3; i++ {
		if m.editFlock == nil {
			m.editFlock = flock.New(m.editLockPath)
		}

		locked, err := m.editFlock.TryLock()
		if err == nil && locked {
			m.editActive = true
			m.localOwner = connType
			m.localSource = lockSource
			m.localSessionKey = sessionKey
			terminal := DetectTerminal()
			if clientIP != "" && clientIP != "local" {
				for _, cs := range m.ListConnectedSessions() {
					if cs.ClientIP == clientIP {
						terminal = cs.Terminal
						break
					}
				}
			}
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
	m.localSource = ""
	m.localSessionKey = ""
	if m.editFlock != nil {
		_ = m.editFlock.Unlock()
	}
	_ = os.Remove(m.editLockPath)
}

// AcquireServer marks the current process as a server daemon in its proc toml,
// recording the SSH and web ports. Multiple server instances are supported.
func (m *SessionManager) AcquireServer(sshPort, webPort int) error {
	path := m.procPath(os.Getpid())
	r, ok := readProcRecord(path)
	if !ok {
		return fmt.Errorf("proc record not found for PID %d", os.Getpid())
	}
	r.IsServer = true
	r.SSHPort = sshPort
	r.WebPort = webPort
	return writeProcRecord(path, r)
}

// ReleaseServer clears the server flag from the current process's proc toml.
// The proc file itself is removed by UnregisterProc on process exit.
func (m *SessionManager) ReleaseServer() {
	path := m.procPath(os.Getpid())
	r, ok := readProcRecord(path)
	if !ok {
		return
	}
	r.IsServer = false
	r.SSHPort = 0
	r.WebPort = 0
	_ = writeProcRecord(path, r)
	// Also remove legacy server.pid if present from an older build.
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
	IsServer  bool   `toml:"is_server,omitempty"`
	SSHPort   int    `toml:"ssh_port,omitempty"`
	WebPort   int    `toml:"web_port,omitempty"`
}

func (m *SessionManager) procPath(pid int) string {
	return filepath.Join(m.procsDir, strconv.Itoa(pid)+".toml")
}

// isProcFileStale returns true if no live process holds a shared flock on the
// proc toml file at path. An exclusive TryLock succeeds only when no shared
// locks are held, meaning the owning process has exited. Files written by older
// builds that never acquired a flock are also treated as stale and removed —
// they will be re-registered on the next startup of that instance.
func isProcFileStale(path string, _ procRecord) bool {
	f := flock.New(path)
	locked, err := f.TryLock()
	if err != nil {
		return false // can't tell; assume live
	}
	if !locked {
		return false // shared lock held — process is alive
	}
	_ = f.Unlock()
	return true
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
		if parts := strings.Fields(sshConn); len(parts) >= 2 {
			sshClient = parts[0] + ":" + parts[1]
		} else if len(parts) >= 1 {
			sshClient = parts[0]
		}
	}
	terminal := DetectTerminal()
	path := m.procPath(os.Getpid())
	_ = writeProcRecord(path, procRecord{
		PID:       os.Getpid(),
		ExePath:   exePath,
		Version:   currentVersion,
		Args:      args,
		SSHClient: sshClient,
		Terminal:  terminal,
	})
	// Hold a shared flock on our own .toml for the process lifetime.
	// Other instances use TryLock (exclusive) to detect dead processes without PID reuse false positives.
	f := flock.New(path)
	if _, err := f.TryRLock(); err == nil {
		m.procFlock = f
	}
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
	if m.procFlock != nil {
		_ = m.procFlock.Unlock()
		m.procFlock = nil
	}
	pid := os.Getpid()
	_ = os.Remove(m.procPath(pid))
	_ = os.Remove(filepath.Join(m.procsDir, strconv.Itoa(pid)+".restartunsafe"))
}

// MarkRestartUnsafe acquires a shared flock on this process's .restartunsafe
// file, holding it until MarkRestartSafe is called or the process exits.
// SelfRestartUnsafe uses TryLock to detect a live holder without PID parsing.
func (m *SessionManager) MarkRestartUnsafe() {
	_ = os.MkdirAll(m.procsDir, 0755)
	path := filepath.Join(m.procsDir, strconv.Itoa(os.Getpid())+".restartunsafe")
	_ = os.WriteFile(path, []byte{}, 0644)
	f := flock.New(path)
	if _, err := f.TryRLock(); err == nil {
		m.restartUnsafeFlock = f
	}
}

// MarkRestartSafe releases the restart-unsafe flock and removes the marker file.
func (m *SessionManager) MarkRestartSafe() {
	if m.restartUnsafeFlock != nil {
		_ = m.restartUnsafeFlock.Unlock()
		m.restartUnsafeFlock = nil
	}
	_ = os.Remove(filepath.Join(m.procsDir, strconv.Itoa(os.Getpid())+".restartunsafe"))
}

// SelfRestartUnsafe reports whether this process currently holds its own
// restart-unsafe marker (see MarkRestartUnsafe). Deliberately scoped to this
// process only, not every live process system-wide: a restart only replaces
// the calling process's own image (syscall.Exec), so it can never disrupt a
// session belonging to a different process (a different --server-daemon
// instance on another port, or an unrelated local invocation) -- the only
// resource genuinely shared across processes is the edit lock itself
// (editLockPath), tracked separately from these per-PID markers. For a
// --server-daemon, every connected session shares this one process's PID, so
// this still correctly aggregates "is any session I'm hosting unsafe."
func (m *SessionManager) SelfRestartUnsafe() bool {
	path := filepath.Join(m.procsDir, strconv.Itoa(os.Getpid())+".restartunsafe")
	f := flock.New(path)
	locked, err := f.TryLock()
	if err != nil {
		return true // can't tell; assume unsafe
	}
	if !locked {
		return true // shared flock held (by ourselves) — unsafe
	}
	_ = f.Unlock()
	_ = os.Remove(path)
	return false
}

// ConnectedSession holds information about an active SSH or web connection.
type ConnectedSession struct {
	ID        string // unique session identifier (filename)
	ClientIP  string
	ConnType  string // "SSH" or "Web"
	Terminal  string // terminal or browser identifier
	ServerPID int    // PID of the server process that owns this session
}

// RegisterSession writes a session file for an active incoming connection.
// Returns the session ID to pass to UnregisterSession on disconnect. This ID
// also becomes the session's edit-lock identity (see AcquireEditLock's
// re-entry check), so it must be collision-proof -- a random UUID rather than
// a PID+timestamp composite, which could theoretically collide if two
// connections register at the same clock tick.
// terminal is a human-readable identifier: for SSH it's e.g. "WezTerm/xterm-256color",
// for web it's a simplified browser name from the User-Agent.
func (m *SessionManager) RegisterSession(clientIP, connType, terminal string) string {
	_ = os.MkdirAll(m.sessionsDir, 0755)
	id := uuid.NewString()
	path := filepath.Join(m.sessionsDir, id)
	_ = writeTomlFile(path, sessionRecord{
		ClientIP:  clientIP,
		ConnType:  connType,
		Terminal:  terminal,
		ServerPID: os.Getpid(),
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
	// Build a set of running server PIDs for fast lookup.
	runningServers := make(map[int]bool)
	for _, s := range m.ListServerInfos() {
		runningServers[s.PID] = true
	}
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
		var r sessionRecord
		if err := toml.Unmarshal(data, &r); err != nil || r.ClientIP == "" {
			_ = os.Remove(path)
			continue
		}
		// Remove sessions whose server process is no longer running.
		if !runningServers[r.ServerPID] {
			_ = os.Remove(path)
			continue
		}
		sessions = append(sessions, ConnectedSession{
			ID:        e.Name(),
			ConnType:  r.ConnType,
			ClientIP:  r.ClientIP,
			Terminal:  r.Terminal,
			ServerPID: r.ServerPID,
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
	IsServer  bool
	SSHPort   int
	WebPort   int
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
		if !ok || r.PID == 0 || isProcFileStale(path, r) {
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

// SeedInstalledVersion writes currentVersion to the installed-version file
// only if no file exists yet. Never overwrites an existing value even if it
// differs from currentVersion -- a differing value may be a legitimate "a
// newer version was installed" signal from another process this one hasn't
// picked up yet, and stomping it would erase that signal for other sessions
// watching the same file.
func (m *SessionManager) SeedInstalledVersion(exePath, currentVersion string) {
	if m.ReadInstalledVersion(exePath) != "" {
		return
	}
	_ = m.WriteInstalledVersion(exePath, currentVersion)
}

// UpdateEditLockConnType rewrites the conn_type field in the active edit lock file.
// Used to reflect the current activity (e.g. switching from menu to var editor and back).
func (m *SessionManager) UpdateEditLockConnType(connType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.editActive {
		return
	}
	r := m.readEditLockRecord()
	if r.PID == 0 {
		return
	}
	r.ConnType = connType
	m.localOwner = connType
	_ = writeTomlFile(m.editLockPath, r)
}

func (m *SessionManager) readEditLockRecord() editLockRecord {
	var r editLockRecord
	data, err := os.ReadFile(m.editLockPath)
	if err != nil {
		return r
	}
	_ = toml.Unmarshal(data, &r)
	return r
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

// ReadServerInfo returns info for the first live server daemon found in the
// proc directory. Returns zero ServerInfo if none is running.
// For multiple servers, use ListServerInfos.
func (m *SessionManager) ReadServerInfo() ServerInfo {
	servers := m.ListServerInfos()
	if len(servers) == 0 {
		return ServerInfo{}
	}
	return servers[0]
}

// ListServerInfos returns ServerInfo for all live server daemon instances.
func (m *SessionManager) ListServerInfos() []ServerInfo {
	entries, err := os.ReadDir(m.procsDir)
	if err != nil {
		return nil
	}
	var result []ServerInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		path := filepath.Join(m.procsDir, e.Name())
		r, ok := readProcRecord(path)
		if !ok || !r.IsServer {
			continue
		}
		if isProcFileStale(path, r) {
			_ = os.Remove(path)
			continue
		}
		result = append(result, ServerInfo{
			PID:     r.PID,
			Port:    r.SSHPort,
			WebPort: r.WebPort,
		})
	}
	return result
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
	m.localSource = ""
	m.localSessionKey = ""
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

	// Remove legacy server.pid if present from an older build.
	_ = os.Remove(m.serverPIDPath)

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

func (m *SessionManager) sessionDisconnectPath(id string) string {
	return filepath.Join(m.sessionsDir, "disconnect."+id)
}

func (m *SessionManager) RequestSessionDisconnect(id string) error {
	_ = os.MkdirAll(m.sessionsDir, 0755)
	return os.WriteFile(m.sessionDisconnectPath(id), []byte{}, 0644)
}

func (m *SessionManager) ClearSessionDisconnectRequest(id string) {
	_ = os.Remove(m.sessionDisconnectPath(id))
}

func (m *SessionManager) IsSessionDisconnectRequested(id string) bool {
	_, err := os.Stat(m.sessionDisconnectPath(id))
	return err == nil
}

// RequestStop writes the broadcast stop-request file, picked up by all daemons.
func (m *SessionManager) RequestStop() error {
	return os.WriteFile(m.stopReqPath, []byte{}, 0644)
}

// RequestStopPID writes a PID-specific stop-request file, picked up only by
// the daemon with that PID.
func (m *SessionManager) RequestStopPID(pid int) error {
	path := strings.TrimSuffix(m.stopReqPath, ".request") + "." + strconv.Itoa(pid) + ".request"
	return os.WriteFile(path, []byte{}, 0644)
}

func (m *SessionManager) ClearStopRequest() {
	_ = os.Remove(m.stopReqPath)
}

// IsStopRequested returns true if either the broadcast or this process's
// PID-specific stop-request file exists.
func (m *SessionManager) IsStopRequested() bool {
	if _, err := os.Stat(m.stopReqPath); err == nil {
		return true
	}
	pidPath := strings.TrimSuffix(m.stopReqPath, ".request") + "." + strconv.Itoa(os.Getpid()) + ".request"
	if _, err := os.Stat(pidPath); err == nil {
		_ = os.Remove(pidPath)
		return true
	}
	return false
}

// SessionLabel returns the display label for a session transport type.
func SessionLabel(transport string) string {
	switch transport {
	case "local", "ssh":
		return "Terminal session"
	case "ssh-server":
		return "SSH Server session"
	case "web":
		return "Web Server session"
	default:
		return "Session"
	}
}

// EditLockDetail returns the lock detail lines and an optional disconnect hint
// for a SessionInfo. The detail lines describe who holds the lock and what they
// are doing. The hint is non-empty only for ssh-server and web sessions.
// Use EditLockLines when you want all lines together (e.g. logger calls).
//
// Format of detail lines:
//
//	Edit lock:
//	    <session>
//	    <action>
func EditLockDetail(info SessionInfo) (lines []string, hint string) {
	if info.Session == "" {
		return nil, ""
	}
	conn := info.ConnType
	if conn == "" {
		conn = "unknown"
	}
	label := SessionLabel(info.Transport)
	session := info.FormatSession()
	var action string
	switch info.LockSource {
	case "cli":
		action = fmt.Sprintf("Running CLI command '{{|RunningCommand|}}%s{{[-]}}'.", conn)
	case "console":
		action = fmt.Sprintf("Running console command '{{|RunningCommand|}}%s{{[-]}}'.", conn)
	default:
		action = fmt.Sprintf("In the '{{|MenuPage|}}%s{{[-]}}' menu.", conn)
	}
	lines = []string{
		"{{|Warn|}}Edit lock:{{[-]}}",
		fmt.Sprintf("    %s %s", label, session),
		"    " + action,
	}
	if info.Transport == "ssh-server" || info.Transport == "web" {
		hint = "Use '{{|UserCommand|}}ds2 --disconnect{{[-]}}' to force-release the lock."
	}
	return lines, hint
}

// EditLockLines returns all lock detail lines as a flat slice suitable for
// passing to logger.Warn or logger.Error. closing is appended before the
// disconnect hint when non-empty (e.g. "Cannot run '-e' while the
// configuration is being edited.").
func EditLockLines(info SessionInfo, closing string) []string {
	lines, hint := EditLockDetail(info)
	if closing != "" || hint != "" {
		lines = append(lines, "")
	}
	if closing != "" {
		lines = append(lines, closing)
	}
	if hint != "" {
		lines = append(lines, hint)
	}
	return lines
}
