//go:build !windows

package system

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// permissionsMatch reports whether path already has the ownership and
// permission bits SetPermissions/TakeOwnership would otherwise apply,
// letting the caller skip the sudo chown/chmod entirely when nothing needs
// to change. Stat comparisons need no elevated privileges, so this check
// never requires sudo -- only fixing a mismatch does.
//
// Non-recursive mode (used by TakeOwnership) checks just path's own
// ownership and mode. Recursive mode (used by SetPermissions) walks the
// whole tree checking every entry, stopping at the first mismatch found --
// since anything to fix falls through to the same full recursive
// "sudo chown -R"/"sudo chmod -R" pass that would otherwise always run, the
// worst case (something needs fixing) costs only a little extra walk time
// before the same work already being done, while the common case (already
// correct) skips both sudo calls entirely.
//
// Target permissions mirror "chmod -R a=,a+rX,u+w,g+w": directories end up
// 0775 (rwxrwxr-x), regular files 0664 (rw-rw-r--). Symlinks are skipped
// entirely during a recursive check, matching chmod/chown -R's convention
// of leaving the link itself alone during recursion.
func permissionsMatch(root string, puid, pgid int, recursive bool) bool {
	if !recursive {
		info, err := os.Lstat(root)
		if err != nil {
			return false
		}
		return fileMatchesTarget(info, puid, pgid)
	}

	match := true
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Can't verify this entry (e.g. permission denied) -- assume a
			// fix is needed rather than silently skipping it.
			match = false
			return filepath.SkipAll
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			match = false
			return filepath.SkipAll
		}
		if !fileMatchesTarget(info, puid, pgid) {
			match = false
			return filepath.SkipAll
		}
		return nil
	})
	return match
}

// fileMatchesTarget reports whether info's owner and mode already match
// DS2's target for its type: 0775 for directories, 0664 for regular files
// (owned by puid:pgid).
func fileMatchesTarget(info os.FileInfo, puid, pgid int) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	if int(stat.Uid) != puid || int(stat.Gid) != pgid {
		return false
	}
	wantMode := os.FileMode(0664)
	if info.IsDir() {
		wantMode = 0775
	}
	return info.Mode().Perm() == wantMode
}
