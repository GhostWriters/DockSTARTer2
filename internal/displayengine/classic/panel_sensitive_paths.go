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
// bcrypt hash -- an offline-cracking risk if leaked). Mirrors the same
// default-resolution logic serve.go uses for the host key, rather than
// reimplementing it independently.
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
func findSensitivePathArg(argv []string) string {
	sensitive := sensitivePaths()

	for _, arg := range argv {
		abs, err := filepath.Abs(arg)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)

		if inSSHDir(abs) {
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
