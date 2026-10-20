//go:build windows

package config

import "os"

// matchOwner is a no-op on Windows, where a new file inherits the folder's
// permissions.
func matchOwner(string, os.FileInfo) {}

// syncDir is a no-op on Windows, which doesn't support flushing a folder.
func syncDir(string) {}
