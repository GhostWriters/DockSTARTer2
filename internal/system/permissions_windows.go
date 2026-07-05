//go:build windows

package system

// permissionsMatch is never actually consulted on Windows -- SetPermissions
// and TakeOwnership both return before reaching it (see the runtime.GOOS
// checks at their tops) -- but the symbol must exist for this platform to
// compile.
func permissionsMatch(root string, puid, pgid int, recursive bool) bool {
	return false
}
