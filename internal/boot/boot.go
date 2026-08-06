// Package boot performs process-identity fixups that must happen before
// ANY other DS2 code can read paths or touch the filesystem -- specifically,
// dropping privileges when launched via sudo, so nothing (not even
// package-level initializers like sessionlocks' global manager, which
// creates its lock directories at init time) ever computes a path from
// root's environment or writes into root's home.
//
// It runs from init() rather than main() because Go initializes imported
// packages first: main() is too late to beat another package's init-time
// side effects. internal/paths blank-imports this package, which places it
// below every filesystem-touching package in the dependency graph and
// guarantees it initializes before all of them.
package boot

import (
	"fmt"
	"os"

	"DockSTARTer2/internal/constants"
)

func init() {
	// The hidden elevated helper (DS2 sudo'ing itself to fix ownership)
	// deliberately runs as root and must never be demoted.
	if len(os.Args) >= 2 && os.Args[1] == constants.InternalFixPermissionsArg {
		return
	}
	if err := demoteSudoPrivileges(); err != nil {
		// No logger exists this early; a failed privilege drop must not be
		// survivable (continuing would run as root).
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

var demotionNotice string

// DemotionNotice returns a human-readable description of the privilege drop
// performed at startup ("" when none happened). The drop runs before the
// logger exists, so main logs this once logging is up.
func DemotionNotice() string { return demotionNotice }
