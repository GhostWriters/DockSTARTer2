//go:build windows

package system

import "errors"

// checkPermissions is never actually consulted on Windows -- SetPermissions
// and TakeOwnership both return before reaching it (see the runtime.GOOS
// checks at their tops) -- but the symbol must exist for this platform to
// compile.
func checkPermissions(root string, puid, pgid int, recursive bool) (needsChown, needsChmod bool) {
	return false, false
}

// fixPermissions is likewise unreachable on Windows (SetPermissions/
// TakeOwnership return early, and DS2 never sudo-re-execs itself here);
// the symbol exists so RunInternalFixPermissions compiles on this platform.
func fixPermissions(root string, puid, pgid int, doChown, doChmod, recursive bool) error {
	return errors.New("fixing ownership/permissions is not supported on windows")
}
