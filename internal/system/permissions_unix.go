//go:build !windows

package system

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// permissionsMatch reports whether path's entire tree already has the
// ownership and permission bits SetPermissions would otherwise apply,
// letting the caller skip the recursive sudo chown/chmod entirely when
// nothing needs to change. Stat comparisons need no elevated privileges, so
// this check never requires sudo -- only fixing a mismatch does. It stops at
// the first mismatch found rather than walking the whole tree, since
// anything to fix falls through to the same full "sudo chown -R"/
// "sudo chmod -R" pass that would otherwise always run -- so the worst case
// (something needs fixing) costs only a little extra walk time before the
// same work already being done, while the common case (already correct)
// skips both sudo calls entirely.
//
// Target permissions mirror "chmod -R a=,a+rX,u+w,g+w": directories end up
// 0775 (rwxrwxr-x), regular files 0664 (rw-rw-r--). Symlinks are skipped
// entirely, matching chmod/chown -R's convention of leaving the link itself
// alone during recursion.
func permissionsMatch(root string, puid, pgid int) bool {
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
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			match = false
			return filepath.SkipAll
		}
		if int(stat.Uid) != puid || int(stat.Gid) != pgid {
			match = false
			return filepath.SkipAll
		}
		wantMode := os.FileMode(0664)
		if d.IsDir() {
			wantMode = 0775
		}
		if info.Mode().Perm() != wantMode {
			match = false
			return filepath.SkipAll
		}
		return nil
	})
	return match
}
