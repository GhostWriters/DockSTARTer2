package system

import (
	"DockSTARTer2/internal/logger"
	"context"
	"os"
	"runtime"
)

// CheckNotRoot exits fatally only for a broken-identity case: a process
// with root effective privilege but a non-root real uid (e.g. a setuid-root
// copy of the binary), which has no legitimate origin in normal use and
// would silently create root-owned files outside the drop/sudo machinery.
//
// A genuine root login (uid 0 all the way through -- someone who works
// entirely as root and never created a standard user) is deliberately
// allowed through untouched: DS2 isn't the place to police that choice, any
// more than docker or systemctl are. "sudo ds2" from a real standard user
// never reaches here as root at all -- boot's demoteSudoPrivileges (run at
// package-init, before main) already dropped it back to the invoking user
// via sudo's SUDO_UID breadcrumbs, so what's left is: true root (allowed)
// or this mismatched-identity case (rejected).
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
