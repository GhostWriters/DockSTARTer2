//go:build windows

package boot

// demoteSudoPrivileges is a no-op on Windows: there is no sudo, and the
// unix uid/gid model this addresses doesn't exist.
func demoteSudoPrivileges() error { return nil }
