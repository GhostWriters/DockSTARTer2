//go:build !windows

package system

import (
	dsexec "DockSTARTer2/internal/exec"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// fixPermissions applies ownership (to puid:pgid) and/or DS2's target modes
// (0775 directories / 0664 regular files, mirroring
// "chmod -R a=,a+rX,u+w,g+w") to root natively via syscalls -- the chown and
// chmod binaries are not involved, so behavior is identical across
// coreutils implementations (GNU, BSD, uutils) and both operations happen
// in ONE tree walk instead of the two that "chown -R" + "chmod -R" cost.
// Entries already correct are left untouched (no metadata churn), symlinks
// are skipped entirely (same convention as checkPermissions and
// chown/chmod -R recursion), and directories are fixed before their
// contents are read, so a directory whose old mode blocked traversal
// becomes readable as the walk descends into it.
//
// Fails fast with the first real error; os.Lchown/os.Chmod errors are
// *os.PathError values carrying the operation and path, unlike the old
// shelled-out chown/chmod whose stderr was discarded.
func fixPermissions(root string, puid, pgid int, doChown, doChmod, recursive bool) error {
	if !recursive {
		info, err := os.Lstat(root)
		if err != nil {
			return err
		}
		return fixEntry(root, info, puid, pgid, doChown, doChmod)
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fixEntry(p, info, puid, pgid, doChown, doChmod)
	})
}

// fixEntry corrects a single entry's ownership and/or mode, skipping
// whichever is already correct. Chown runs before chmod: if ownership was
// wrong, the subsequent chmod then operates on a file the target user owns.
func fixEntry(p string, info os.FileInfo, puid, pgid int, doChown, doChmod bool) error {
	if doChown && !ownerMatches(info, puid, pgid) {
		if err := os.Lchown(p, puid, pgid); err != nil {
			return err
		}
	}
	if doChmod && !modeMatches(info) {
		want := os.FileMode(0664)
		if info.IsDir() {
			want = 0775
		}
		if err := os.Chmod(p, want); err != nil {
			return err
		}
	}
	return nil
}

// FixOwnerMode makes path owned by uid:gid with exactly the given mode,
// natively wherever the process's own privileges allow it, falling back to
// sudo chown/chmod only for whichever piece it can't do natively.
//
// Unlike applyPermissionFix (which enforces the PUID/PGID + 0664/0775
// appdata convention via the --internal-fix-permissions re-exec helper),
// this takes an explicit owner and mode: it exists for self-update, which
// needs to restore an executable's ownership/mode (0755, not 0664) after a
// sudo mv into a system directory, matching the destination directory's
// owner rather than the app's configured puid:pgid.
func FixOwnerMode(ctx context.Context, path string, uid, gid int, mode os.FileMode) error {
	needChown := true
	needChmod := true
	if info, err := os.Lstat(path); err == nil {
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			needChown = int(st.Uid) != uid || int(st.Gid) != gid
		}
		needChmod = info.Mode().Perm() != mode.Perm()
	}

	nativeChown := !needChown || hasCapChown()
	// Native chmod needs either CAP_FOWNER, or the file to already be owned
	// by this process's own uid -- true after a native chown above only if
	// uid matches our own; after a sudo chown it generally isn't.
	nativeChmod := !needChmod || hasCapFowner() || (nativeChown && os.Getuid() == uid)

	if needChown {
		if nativeChown {
			if err := os.Lchown(path, uid, gid); err != nil {
				return fmt.Errorf("failed to chown: %w", err)
			}
		} else if err := sudoChown(ctx, path, uid, gid); err != nil {
			return err
		}
	}
	if needChmod {
		if nativeChmod {
			if err := os.Chmod(path, mode); err != nil {
				return fmt.Errorf("failed to chmod: %w", err)
			}
		} else if err := sudoChmod(ctx, path, mode); err != nil {
			return err
		}
	}
	return nil
}

func sudoChown(ctx context.Context, path string, uid, gid int) error {
	cmd, err := dsexec.SudoCommand(ctx, "chown", fmt.Sprintf("%d:%d", uid, gid), path)
	if err != nil {
		return err
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sudo chown failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func sudoChmod(ctx context.Context, path string, mode os.FileMode) error {
	cmd, err := dsexec.SudoCommand(ctx, "chmod", fmt.Sprintf("%o", mode.Perm()), path)
	if err != nil {
		return err
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sudo chmod failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
