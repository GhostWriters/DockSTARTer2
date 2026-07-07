//go:build !windows

package system

import (
	"io/fs"
	"os"
	"path/filepath"
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
