package system

import (
	"DockSTARTer2/internal/logger"
	"context"
	"os"
	"runtime"
)

// CheckNotRoot exits fatally only for a broken-identity case: a process
// with root effective privilege but a non-root real uid (e.g. a setuid-root
// copy of the binary), which has no legitimate origin in normal use.
//
// A genuine root login is deliberately allowed through untouched -- DS2
// isn't the place to police that choice. "sudo ds2" from a standard user
// never reaches here as root at all: boot's demoteSudoPrivileges (run at
// package-init) already dropped it back via sudo's SUDO_UID breadcrumbs.
func CheckNotRoot(ctx context.Context) {
	if runtime.GOOS == "windows" {
		return
	}

	if os.Geteuid() == 0 && os.Getuid() != 0 {
		logger.Fatal(ctx, []string{
			"Running with mismatched privileges (elevated effective ID, non-root real ID) is not supported.",
			"This usually indicates a setuid-root copy of the binary; run it normally instead.",
		})
	}
}
