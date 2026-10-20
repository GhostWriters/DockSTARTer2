package config

import (
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path so that a crash or a concurrent save
// never leaves it half written: data goes to a uniquely named temporary file
// in the same folder, is flushed to disk, then renamed over path in one step.
// Each call gets its own temporary file, so instances saving at once can't
// write into the same one. A symlinked path, even one whose target doesn't
// exist yet, writes through to the target, so the link stays a link. An
// existing file's mode, and on Linux and macOS its owner, carry over to the
// new one; a new file is 0600. A file without owner write permission is
// refused rather than replaced.
func writeFileAtomic(path string, data []byte) error {
	target := linkTarget(path)
	mode := os.FileMode(0600)
	existing, statErr := os.Stat(target)
	if statErr == nil {
		mode = existing.Mode().Perm()
		if mode&0200 == 0 {
			return &os.PathError{Op: "write", Path: target, Err: os.ErrPermission}
		}
	}

	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	// Mode and owner go through the open file, not its path, so nothing
	// swapped in at that path can be changed instead.
	err = writeAndSync(tmp, data, mode, existing)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return err
	}
	renamed = true
	syncDir(dir)
	return nil
}

// writeAndSync writes data to f, gives it mode and existing's owner (nil
// for a new file), then flushes it to disk.
func writeAndSync(f *os.File, data []byte, mode os.FileMode, existing os.FileInfo) error {
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chmod(mode); err != nil {
		return err
	}
	if existing != nil {
		matchOwner(f, existing)
	}
	return f.Sync()
}

// linkTarget follows path through any chain of symlinks to the file it names,
// which may not exist yet. filepath.EvalSymlinks can't, since it fails on a
// link to a missing file. A loop gives up after 40 links, leaving the path
// as far as it got.
func linkTarget(path string) string {
	for range 40 {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return path
		}
		link, err := os.Readlink(path)
		if err != nil {
			return path
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(path), link)
		}
		path = link
	}
	return path
}
