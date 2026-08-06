//go:build !windows

package system

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// checkPermissions reports whether root's tree already has the ownership
// and/or permission bits SetPermissions/TakeOwnership would apply, letting
// the caller skip whichever of chown/chmod isn't needed (or skip both).
// Stat comparisons need no elevated privileges, so this never requires
// sudo -- only fixing a mismatch does.
//
// Non-recursive mode (TakeOwnership) checks just root's own ownership and
// mode. Recursive mode (SetPermissions) walks the whole tree, stopping once
// both needsChown and needsChmod are known true. Worst case (both needed)
// costs a little extra walk time before the same full recursive fix that
// would run anyway; common case (already correct) skips both sudo calls;
// partial mismatch now skips the unneeded operation instead of always
// running both together.
//
// Target permissions mirror "chmod -R a=,a+rX,u+w,g+w": directories 0775
// (rwxrwxr-x), files 0664 (rw-rw-r--). Symlinks are skipped entirely during
// a recursive check, matching chmod/chown -R's convention.
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
