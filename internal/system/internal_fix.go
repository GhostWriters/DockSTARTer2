package system

import (
	"DockSTARTer2/internal/constants"
	"fmt"
	"os"
	"strconv"
)

// InternalFixPermissionsArg is the hidden first argument that turns a DS2
// invocation into a bare permission-fixing helper (see
// RunInternalFixPermissions). Never shown in help output -- it exists only
// for DS2 to invoke on itself via sudo. The value lives in constants so
// internal/boot's init-time sudo demotion can recognize (and skip demoting)
// this deliberately-root helper without importing this package.
const InternalFixPermissionsArg = constants.InternalFixPermissionsArg

// RunInternalFixPermissions is the entry point for the hidden elevated
// helper mode:
//
//	sudo ds2 --internal-fix-permissions <puid> <pgid> <chown 0|1> <chmod 0|1> <recursive 0|1> <path>
//
// applyPermissionFix re-execs DS2 via sudo into this mode when the current
// process lacks the privileges to fix ownership/permissions natively; the
// elevated child performs the exact same single-walk native fix, just as
// root. It must be dispatched as the very first thing in main() -- before
// config loading, logging setup, instance detection, and especially
// CheckNotRoot (this child deliberately runs as root) -- and does nothing
// but parse its arguments, run the fix, and report any error on stderr for
// the parent to relay into its own log.
func RunInternalFixPermissions(args []string) int {
	if len(args) != 6 {
		fmt.Fprintln(os.Stderr, "usage:", InternalFixPermissionsArg, "<puid> <pgid> <chown 0|1> <chmod 0|1> <recursive 0|1> <path>")
		return 2
	}
	puid, errU := strconv.Atoi(args[0])
	pgid, errG := strconv.Atoi(args[1])
	doChown, okChown := parseBit(args[2])
	doChmod, okChmod := parseBit(args[3])
	recursive, okRec := parseBit(args[4])
	path := args[5]
	if errU != nil || errG != nil || !okChown || !okChmod || !okRec || path == "" {
		fmt.Fprintln(os.Stderr, "invalid arguments for", InternalFixPermissionsArg)
		return 2
	}
	if err := fixPermissions(path, puid, pgid, doChown, doChmod, recursive); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func parseBit(s string) (value, ok bool) {
	switch s {
	case "0":
		return false, true
	case "1":
		return true, true
	}
	return false, false
}
