package system

import (
	dsexec "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/logger"
	"context"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"
)

// SetPermissions mimics the bash set_permissions.sh logic exactly.
func SetPermissions(ctx context.Context, path string) {
	if runtime.GOOS == "windows" {
		return
	}

	if path == "" {
		return
	}

	// 1. System path check
	if isSystemPath(path) {
		logger.Error(ctx, "Skipping permissions on '{{|Folder|}}%s{{[-]}}' because it is a system path.", path)
		return
	}

	// 2. Home directory check
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(path, home) {
		logger.Warn(ctx, "Setting permissions for '{{|Folder|}}%s{{[-]}}' outside of '{{|Folder|}}%s{{[-]}}' may be unsafe.", path, home)
	} else {
		logger.Info(ctx, "Setting permissions for '{{|Folder|}}%s{{[-]}}'", path)
	}

	// 3. Take Ownership and Set Permissions
	puid, pgid := GetIDs()
	if puid != 0 && pgid != 0 {
		logger.Info(ctx, "Taking ownership of '{{|Folder|}}%s{{[-]}}' for user '{{|User|}}%d{{[-]}}' and group '{{|User|}}%d{{[-]}}'", path, puid, pgid)
		if cmdChown, err := dsexec.SudoCommand(ctx, "chown", "-R", fmt.Sprintf("%d:%d", puid, pgid), path); err == nil {
			_ = cmdChown.Run()
		}

		logger.Info(ctx, "Setting file and folder permissions in '{{|Folder|}}%s{{[-]}}'", path)
		if cmdChmod, err := dsexec.SudoCommand(ctx, "chmod", "-R", "a=,a+rX,u+w,g+w", path); err == nil {
			_ = cmdChmod.Run()
		}
	}
}

// TakeOwnership mimics the non-recursive chown used in some bash scripts.
func TakeOwnership(ctx context.Context, path string) {
	if runtime.GOOS == "windows" {
		return
	}

	if path == "" {
		return
	}

	puid, pgid := GetIDs()
	if puid != 0 && pgid != 0 {
		logger.Info(ctx, "Taking ownership of '{{|Folder|}}%s{{[-]}}' (non-recursive).", path)
		if cmd, err := dsexec.SudoCommand(ctx, "chown", fmt.Sprintf("%d:%d", puid, pgid), path); err == nil {
			_ = cmd.Run()
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
