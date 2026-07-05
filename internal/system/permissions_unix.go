//go:build !windows

package system

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// checkPermissions reports whether root's tree already has the ownership
// and/or permission bits SetPermissions/TakeOwnership would otherwise
// apply, letting the caller skip whichever of chown/chmod isn't actually
// needed -- and skip both entirely when nothing needs to change. Stat
// comparisons need no elevated privileges, so this check never requires
// sudo -- only fixing a mismatch does.
//
// Non-recursive mode (used by TakeOwnership) checks just root's own
// ownership and mode. Recursive mode (used by SetPermissions) walks the
// whole tree checking every entry, stopping as soon as both needsChown and
// needsChmod are known true (no further mismatches can add information at
// that point) -- since anything to fix falls through to the same full
// recursive "sudo chown -R"/"sudo chmod -R" pass that would otherwise
// always run, the worst case (both needed) costs only a little extra walk
// time before the same work already being done, while the common case
// (already correct) skips both sudo calls entirely, and a partial mismatch
// (only one dimension wrong) now skips the other operation too instead of
// always running both together.
//
// Target permissions mirror "chmod -R a=,a+rX,u+w,g+w": directories end up
// 0775 (rwxrwxr-x), regular files 0664 (rw-rw-r--). Symlinks are skipped
// entirely during a recursive check, matching chmod/chown -R's convention
// of leaving the link itself alone during recursion.
func checkPermissions(root string, puid, pgid int, recursive bool) (needsChown, needsChmod bool) {
	if !recursive {
		info, err := os.Lstat(root)
		if err != nil {
			return true, true
		}
		return !ownerMatches(info, puid, pgid), !modeMatches(info)
	}

	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Can't verify this entry (e.g. permission denied) -- assume a
			// fix is needed rather than silently skipping it.
			needsChown, needsChmod = true, true
			return filepath.SkipAll
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			needsChown, needsChmod = true, true
			return filepath.SkipAll
		}
		if !needsChown && !ownerMatches(info, puid, pgid) {
			needsChown = true
		}
		if !needsChmod && !modeMatches(info) {
			needsChmod = true
		}
		if needsChown && needsChmod {
			return filepath.SkipAll
		}
		return nil
	})
	return needsChown, needsChmod
}

// ownerMatches reports whether info is already owned by puid:pgid.
func ownerMatches(info os.FileInfo, puid, pgid int) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return int(stat.Uid) == puid && int(stat.Gid) == pgid
}

// modeMatches reports whether info's mode already matches DS2's target for
// its type: 0775 for directories, 0664 for regular files.
func modeMatches(info os.FileInfo) bool {
	want := os.FileMode(0664)
	if info.IsDir() {
		want = 0775
	}
	return info.Mode().Perm() == want
}
