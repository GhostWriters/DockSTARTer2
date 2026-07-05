package system

import (
	"DockSTARTer2/internal/logger"
	"context"
	"os"
	"runtime"
)

// CheckNotRoot exits fatally if DS2 is being run as the root user, or was
// launched via sudo. Mirrors DS1's check_root/check_sudo (main.sh).
//
// DS2 relies on running as an unprivileged user and elevating via sudo only
// for the specific commands that need it (see SetPermissions/TakeOwnership);
// if the whole process ran as root, every file it creates directly -- not
// just the ones deliberately chowned/chmoded afterward -- would be
// root-owned from the moment of creation, breaking that guarantee. It's
// also a plain security concern independent of that: a CLI/TUI managing
// Docker configs has no legitimate need to hold root for its entire process
// lifetime, so running it that way needlessly maximizes the blast radius of
// any bug, for no benefit (root can do nothing here that the surgical sudo
// calls don't already cover).
func CheckNotRoot(ctx context.Context) {
	if runtime.GOOS == "windows" {
		return
	}

	// Is the actual logged-in account root? (mirrors DS1's DETECTED_PUID,
	// which GetIDs already computes the same way: SUDO_UID if set, else the
	// real UID.)
	puid, _ := GetIDs()
	home, _ := os.UserHomeDir()
	if puid == 0 || home == "/root" {
		logger.Fatal(ctx, []string{
			"Running as '{{|User|}}root{{[-]}}' is not supported.",
			"Please run as a standard user.",
		})
	}

	// Is this invocation itself currently elevated? (i.e. launched via
	// "sudo ds2 ..." even though the underlying account isn't root.)
	if os.Geteuid() == 0 {
		logger.Fatal(ctx, []string{
			"Running with '{{|UserCommand|}}sudo{{[-]}}' is not supported.",
			"Commands requiring '{{|UserCommand|}}sudo{{[-]}}' will prompt automatically when required.",
		})
	}
}
