package classic

import (
	"os"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
)

// sensitivePaths returns the absolute paths of files DS2 itself manages
// that must never be readable/writable through a "!"/"!!" shell command,
// regardless of the argv[0] whitelist: the SSH server's host private key
// (impersonation risk), the configured authorized_keys file (remote-access
// grant risk), and dockstarter2.toml itself (stores the auth password as a
// bcrypt hash -- an offline-cracking risk if leaked). The locks directory
// (edit.lock, server.pid, per-session/process tracking -- see underDir's
// caller) and the disconnect/stop request files (see
// isControlRequestFile's caller) are handled separately since they're
// either a whole directory tree or have a dynamic PID-suffixed filename,
// not a fixed list of paths. Mirrors the same default-resolution logic
// serve.go uses for the host key, rather than reimplementing it
// independently.
func sensitivePaths() []string {
	cfg := config.LoadAppConfig()

	hostKeyPath := cfg.Server.HostKey
	if hostKeyPath == "" {
		hostKeyPath = filepath.Join(paths.GetStateDir(), "server_host_key")
	}

	sensitive := []string{hostKeyPath, paths.GetConfigFilePath()}
	if cfg.Server.Auth.AuthKeysFile != "" {
		sensitive = append(sensitive, cfg.Server.Auth.AuthKeysFile)
	}

	for i, p := range sensitive {
		if abs, err := filepath.Abs(p); err == nil {
			sensitive[i] = filepath.Clean(abs)
		}
	}
	return sensitive
}

// findSensitivePathArg reports whether any element of argv (not just
// argv[0] -- a sensitive path can appear anywhere as an argument) refers to
// one of sensitivePaths, returning that argument or "" if none match.
// Matching resolves each argument to an absolute path first, since the
// executed command would resolve it the same way, and falls back to
// os.SameFile when both sides stat successfully so a symlink or
// differently-spelled-but-identical path can't dodge a string comparison.
//
// Only applies when console.RequiresRemoteSudoGate() is true, same as the
// argv[0] whitelist (findBlockedArgvWord): a local session or a real
// external SSH shell already has unrestricted access to these files outside
// DS2, so blocking them here would add no security, only friction.
//
// sensitivePaths entries are matched exactly, not by ancestor directory:
// "rm -r" on a *containing* folder (~/.config, ~/.local, ~) isn't caught,
// deliberately. Chasing ancestors has no natural stopping point short of
// blocking the whole filesystem, since every sensitive file is transitively
// "under" the home directory and "/" too -- that would block unrelated
// operations on shared folders just because DS2's own file happens to live
// somewhere underneath. A directory-level delete/replace taking DS2's files
// down as collateral damage is the same accepted residual risk as giving
// rm/cp/mv to a gated session at all, not something specific to these
// paths. locksDir and .ssh are the exception: DS2 owns locksDir exclusively
// and .ssh is always credential-only, so blocking those whole trees has a
// natural, narrow boundary that doesn't generalize into "block everything."
func findSensitivePathArg(argv []string) string {
	if !console.RequiresRemoteSudoGate() {
		return ""
	}
	sensitive := sensitivePaths()
	locksDir := filepath.Clean(paths.GetLocksDir())
	stateDir := filepath.Clean(paths.GetStateDir())

	for _, arg := range argv {
		abs, err := filepath.Abs(arg)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)

		if inSSHDir(abs) || underDir(abs, locksDir) || isControlRequestFile(abs, stateDir) {
			return arg
		}

		argInfo, argErr := os.Stat(abs)
		for _, s := range sensitive {
			if abs == s {
				return arg
			}
			if argErr != nil {
				continue
			}
			if sInfo, err := os.Stat(s); err == nil && os.SameFile(argInfo, sInfo) {
				return arg
			}
		}
	}
	return ""
}

// styleBlockedPathArg returns raw (unresolved) semstyle tag markup styling
// arg as a file or folder reference (matching console.FormatFilePath's File/
// Folder tag convention), deliberately without a hyperlink: arg was just
// rejected specifically because the console won't act on it, so it must not
// also become a clickable way to open it.
func styleBlockedPathArg(arg string) string {
	if info, err := os.Stat(arg); err == nil && info.IsDir() {
		return console.FormatFolderName(arg, "")
	}
	return console.FormatFileName(arg, "")
}

// underDir reports whether path (already Clean and absolute) is dir itself
// or a descendant of it (also already Clean and absolute).
func underDir(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// isControlRequestFile reports whether abs is one of the state directory's
// file-based control signals: disconnect.request (force-disconnects every
// SSH/web session -- sessionlocks.IsDisconnectRequested) or stop.request /
// stop.<pid>.request (shuts down the whole DS2 server daemon, the same
// mechanism as "ds2 --server stop" -- sessionlocks.IsStopRequested). Their
// mere existence triggers the action, so a whitelisted "touch" would be a
// denial-of-service against every other connected user or the server
// itself. stop's PID-specific variant has a dynamic filename, so this
// matches by pattern (name in stateDir directly) rather than a fixed path.
func isControlRequestFile(abs, stateDir string) bool {
	if filepath.Dir(abs) != stateDir {
		return false
	}
	base := filepath.Base(abs)
	if base == "disconnect.request" || base == "stop.request" {
		return true
	}
	return strings.HasPrefix(base, "stop.") && strings.HasSuffix(base, ".request")
}

// inSSHDir reports whether abs (an already-Clean, absolute path) has ".ssh"
// as a path component anywhere in it. Covers the invoking user's ~/.ssh,
// root's /root/.ssh (relevant since "!!" runs as root), and any other
// user's .ssh referenced by absolute path -- ssh key filenames vary
// (id_rsa, id_ed25519, custom names), so this blocks the directory rather
// than trying to enumerate every possible key filename.
func inSSHDir(abs string) bool {
	for _, part := range strings.Split(abs, string(filepath.Separator)) {
		if part == ".ssh" {
			return true
		}
	}
	return false
}
