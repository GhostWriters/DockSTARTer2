//go:build !windows

package config

import (
	"os"
	"syscall"
)

// matchOwner gives path the owner and group of existing, so a save made with
// sudo leaves a user's config owned by that user. Failure is ignored: without
// root, a save of a file someone else owns leaves the new file the caller's.
func matchOwner(path string, existing os.FileInfo) {
	if st, ok := existing.Sys().(*syscall.Stat_t); ok {
		_ = os.Lchown(path, int(st.Uid), int(st.Gid))
	}
}

// syncDir flushes dir's entry for a just-renamed file to disk, so the
// rename survives a crash. Best effort.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
