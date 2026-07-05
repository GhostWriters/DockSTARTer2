//go:build windows

package system

// permissionsMatch is never actually consulted on Windows -- SetPermissions
// returns before reaching it (see the runtime.GOOS check at its top) -- but
// the symbol must exist for this platform to compile.
func permissionsMatch(root string, puid, pgid int) bool {
	return false
}
