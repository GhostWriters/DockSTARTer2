package system

import (
	dsexec "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
)

// chmodCommand builds the command to run for a chmod, skipping sudo when the
// current process's real UID/GID already equal puid:pgid -- chmod only
// requires the effective UID to match the file's current owner (or be
// root), unlike chown, which generally needs CAP_CHOWN/root regardless. DS2
// disallows running as root or via sudo at all (see CheckNotRoot), so the
// invoking user is always supposed to already BE puid:pgid; this check is a
// failsafe for the rare case they diverge (e.g. a stale SUDO_UID/SUDO_GID
// env var inherited from an unrelated earlier "sudo -u otheruser" shell,
// which wouldn't trip CheckNotRoot since that process isn't itself
// currently elevated).
func chmodCommand(ctx context.Context, puid, pgid int, args ...string) (*exec.Cmd, error) {
	if os.Getuid() == puid && os.Getgid() == pgid {
		return exec.CommandContext(ctx, "chmod", args...), nil
	}
	return dsexec.SudoCommand(ctx, "chmod", args...)
}

// SetPermissions mimics the bash set_permissions.sh logic exactly.
func SetPermissions(ctx context.Context, path string) {
	if runtime.GOOS == "windows" {
		return
	}

	if path == "" {
		return
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	// 1. System path check
	if isSystemPath(path) {
		logger.Error(ctx, "Skipping permissions on '"+console.FormatFolderPath(path)+"' because it is a system path.")
		return
	}

	// 2. Check current ownership/permissions first. This is a read-only walk
	// needing no elevated privileges, so it's always safe regardless of
	// location -- only the chown/chmod below is a destructive action worth
	// warning about, so that warning is deferred until we know it'll run.
	logger.Info(ctx, "Checking ownership and permissions of '"+console.FormatFolderPath(path)+"'")

	puid, pgid := GetIDs()
	if puid == 0 || pgid == 0 {
		return
	}
	if permissionsMatch(path, puid, pgid, true) {
		return
	}

	// 3. Take Ownership and Set Permissions
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(path, home) {
		logger.Warn(ctx, "Setting permissions for '"+console.FormatFolderPath(path)+"' outside of '"+console.FormatFolderPath(home)+"' may be unsafe.")
	} else {
		logger.Info(ctx, "Setting permissions for '"+console.FormatFolderPath(path)+"'")
	}

	logger.Info(ctx, "Taking ownership of '"+console.FormatFolderPath(path)+"' for user '{{|User|}}%d{{[-]}}' and group '{{|User|}}%d{{[-]}}'", puid, pgid)
	if cmdChown, err := dsexec.SudoCommand(ctx, "chown", "-R", fmt.Sprintf("%d:%d", puid, pgid), path); err == nil {
		if err := cmdChown.Run(); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to set ownership of folder.",
				"Failing command: {{|FailingCommand|}}sudo chown -R \"%d:%d\" \"%s\"{{[-]}}",
			}, puid, pgid, path)
		}
	}

	logger.Info(ctx, "Setting file and folder permissions in '"+console.FormatFolderPath(path)+"'")
	if cmdChmod, err := chmodCommand(ctx, puid, pgid, "-R", "a=,a+rX,u+w,g+w", path); err == nil {
		if err := cmdChmod.Run(); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to set permissions of folder.",
				"Failing command: {{|FailingCommand|}}sudo chmod -R \"a=,a+rX,u+w,g+w\" \"%s\"{{[-]}}",
			}, path)
		}
	}
}

// TakeOwnership mimics the non-recursive chown used in some bash scripts,
// and also corrects path's own permission bits (non-recursively) if they
// don't match DS2's target mode -- both checked (and skipped when already
// correct) via permissionsMatch, same as SetPermissions.
func TakeOwnership(ctx context.Context, path string) {
	if runtime.GOOS == "windows" {
		return
	}

	if path == "" {
		return
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	logger.Info(ctx, "Checking ownership and permissions of '"+console.FormatFolderPath(path)+"' (non-recursive).")

	puid, pgid := GetIDs()
	if puid == 0 || pgid == 0 {
		return
	}
	if permissionsMatch(path, puid, pgid, false) {
		return
	}

	logger.Info(ctx, "Taking ownership of '"+console.FormatFolderPath(path)+"' (non-recursive).")
	if cmd, err := dsexec.SudoCommand(ctx, "chown", fmt.Sprintf("%d:%d", puid, pgid), path); err == nil {
		if err := cmd.Run(); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to set ownership of folder.",
				"Failing command: {{|FailingCommand|}}sudo chown \"%d:%d\" \"%s\"{{[-]}}",
			}, puid, pgid, path)
		}
	}

	logger.Info(ctx, "Setting permissions of '"+console.FormatFolderPath(path)+"' (non-recursive).")
	if cmd, err := chmodCommand(ctx, puid, pgid, "0775", path); err == nil {
		if err := cmd.Run(); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to set permissions of folder.",
				"Failing command: {{|FailingCommand|}}sudo chmod \"0775\" \"%s\"{{[-]}}",
			}, path)
		}
	}
}

// GetIDs returns the PUID and PGID detected from environment variables (SUDO_UID/SUDO_GID) or os package.
// Mirrors Bash: DETECTED_PUID=${SUDO_UID:-$UID} and DETECTED_PGID=$(id -g "${DETECTED_PUID}")
func GetIDs() (int, int) {
	if runtime.GOOS == "windows" {
		return 1000, 1000
	}

	uid := os.Getuid()
	if sudoUID := os.Getenv("SUDO_UID"); sudoUID != "" {
		if i, err := strconv.Atoi(sudoUID); err == nil {
			uid = i
		}
	}

	gid := os.Getgid()
	// Try to get group ID of the detected UID (which might be the sudo user)
	if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
		if i, err := strconv.Atoi(u.Gid); err == nil {
			gid = i
		}
	}

	// Double check SUDO_GID if id command failed or for extra parity
	if sudoGID := os.Getenv("SUDO_GID"); sudoGID != "" {
		if i, err := strconv.Atoi(sudoGID); err == nil {
			gid = i
		}
	}

	return uid, gid
}

func isSystemPath(path string) bool {
	systemPaths := []string{
		"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/media",
		"/mnt", "/opt", "/proc", "/root", "/sbin", "/srv", "/sys", "/tmp", "/unix",
		"/usr", "/usr/include", "/usr/lib", "/usr/libexec", "/usr/local", "/usr/share",
		"/var", "/var/log", "/var/mail", "/var/spool", "/var/tmp",
	}
	for _, sp := range systemPaths {
		if path == sp {
			return true
		}
	}
	return false
}
