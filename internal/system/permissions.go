package system

import (
	dsexec "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"
)

// applyPermissionFix performs the actual ownership/permission changes for
// SetPermissions/TakeOwnership: natively in-process (see fixPermissions)
// when this process holds the privileges each requested operation needs,
// otherwise by re-executing DS2 via sudo into the hidden
// --internal-fix-permissions helper, which runs the exact same native fix
// as root. Either way the chown/chmod binaries are no longer involved and
// the whole fix is a single tree walk; the only difference between the
// branches is which process the walk runs in.
//
// Native eligibility, judged per operation:
//   - chown requires CAP_CHOWN (changing a file's owner is privileged
//     regardless of who currently owns it -- see "sudo setcap
//     cap_chown,cap_fowner+ep <binary>" for the opt-in that grants it).
//   - chmod requires owning the files, or CAP_FOWNER. Ownership is judged
//     against puid/pgid: checkPermissions guarantees everything is already
//     owned by puid:pgid when doChown is false, and when doChown is true
//     the chown runs first within the same walk, so in both cases the
//     process's own UID/GID matching puid:pgid is sufficient.
//
// A plain "sudo ds2" never reaches here still elevated (see CheckNotRoot),
// so the invoking user normally IS puid:pgid and chmod-only fixes stay
// native even with no capabilities granted; the UID/GID comparison is a
// failsafe for the rare divergence case (e.g. a stale SUDO_UID/SUDO_GID env
// var inherited from an unrelated earlier "sudo -u otheruser" shell). A
// genuine root login is allowed to run DS2 (CheckNotRoot doesn't reject
// it) and trivially satisfies this check regardless, since root always
// matches whatever puid:pgid it's comparing against.
func applyPermissionFix(ctx context.Context, path string, puid, pgid int, doChown, doChmod, recursive bool) error {
	nativeChown := !doChown || hasCapChown()
	nativeChmod := !doChmod || hasCapFowner() || (os.Getuid() == puid && os.Getgid() == pgid)
	if nativeChown && nativeChmod {
		logger.Debug(ctx, "Fixing ownership/permissions natively (no sudo needed).")
		return fixPermissions(path, puid, pgid, doChown, doChmod, recursive)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot locate own executable for elevated re-exec: %w", err)
	}
	logger.Debug(ctx, "Re-executing via sudo to fix ownership/permissions.")
	cmd, err := dsexec.SudoCommand(ctx, exe,
		InternalFixPermissionsArg,
		strconv.Itoa(puid), strconv.Itoa(pgid),
		bit(doChown), bit(doChmod), bit(recursive),
		path)
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}

func bit(b bool) string {
	if b {
		return "1"
	}
	return "0"
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
	needsChown, needsChmod := checkPermissions(path, puid, pgid, true)
	if !needsChown && !needsChmod {
		return
	}

	// 3. Take Ownership and/or Set Permissions -- only whichever is needed.
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(path, home) {
		logger.Warn(ctx, "Setting permissions for '"+console.FormatFolderPath(path)+"' outside of '"+console.FormatFolderPath(home)+"' may be unsafe.")
	} else {
		logger.Info(ctx, "Setting permissions for '"+console.FormatFolderPath(path)+"'")
	}

	if needsChown {
		logger.Info(ctx, "Taking ownership of '"+console.FormatFolderPath(path)+"' for user '{{|User|}}%d{{[-]}}' and group '{{|User|}}%d{{[-]}}'", puid, pgid)
	}
	if needsChmod {
		logger.Info(ctx, "Setting file and folder permissions in '"+console.FormatFolderPath(path)+"'")
	}

	if err := applyPermissionFix(ctx, path, puid, pgid, needsChown, needsChmod, true); err != nil {
		logger.FatalWithStack(ctx, []string{
			"Failed to set ownership/permissions of folder '" + console.FormatFolderPath(path) + "'.",
			"Error: %v",
		}, err)
	}
}

// TakeOwnership mimics the non-recursive chown used in some bash scripts,
// and also corrects path's own permission bits (non-recursively) if they
// don't match DS2's target mode -- each checked (and skipped independently
// when already correct) via checkPermissions, same as SetPermissions.
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
	needsChown, needsChmod := checkPermissions(path, puid, pgid, false)
	if !needsChown && !needsChmod {
		return
	}

	if needsChown {
		logger.Info(ctx, "Taking ownership of '"+console.FormatFolderPath(path)+"' (non-recursive).")
	}
	if needsChmod {
		logger.Info(ctx, "Setting permissions of '"+console.FormatFolderPath(path)+"' (non-recursive).")
	}

	if err := applyPermissionFix(ctx, path, puid, pgid, needsChown, needsChmod, false); err != nil {
		logger.FatalWithStack(ctx, []string{
			"Failed to set ownership/permissions of folder '" + console.FormatFolderPath(path) + "'.",
			"Error: %v",
		}, err)
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
