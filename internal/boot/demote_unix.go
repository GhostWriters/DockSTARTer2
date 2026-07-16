//go:build !windows

package boot

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/adrg/xdg"
)

// demoteSudoPrivileges makes an accidental "sudo ds2" behave like a plain
// invocation by the original user, instead of contaminating their config
// tree with root-owned files: when root AND sudo's SUDO_UID/SUDO_GID
// identify the real invoking user, drops privileges back to that user
// (supplementary groups, then gid, then uid -- a dropped uid can no longer
// change the others) and re-points HOME/XDG at their account.
//
// Three cases:
//   - root via sudo (SUDO_UID present, non-root): demote, scrub env, go on.
//   - true root (direct login, or root sudo'ing to root): left untouched;
//     CheckNotRoot rejects it once logging is initialized.
//   - not root: only scrub stale SUDO_* breadcrumbs. Chains like
//     "sudo su <user>" leave SUDO_UID pointing at the original account
//     while the process runs as the account switched TO -- clearing the
//     stale values makes downstream code (GetIDs, ownership targets,
//     capability failsafes) trust the real current user.
func demoteSudoPrivileges() error {
	if os.Geteuid() != 0 && os.Getuid() != 0 {
		clearSudoEnv()
		return nil
	}

	sudoUID := os.Getenv("SUDO_UID")
	if sudoUID == "" || sudoUID == "0" {
		return nil // true root: no unprivileged identity to return to
	}
	uid, err := strconv.Atoi(sudoUID)
	if err != nil || uid <= 0 {
		return fmt.Errorf("cannot drop sudo privileges: invalid SUDO_UID %q", sudoUID)
	}
	u, err := user.LookupId(sudoUID)
	if err != nil {
		return fmt.Errorf("cannot drop sudo privileges: unknown uid %s: %w", sudoUID, err)
	}

	gid := 0
	if s := os.Getenv("SUDO_GID"); s != "" {
		if g, err := strconv.Atoi(s); err == nil {
			gid = g
		}
	}
	if gid <= 0 {
		if g, err := strconv.Atoi(u.Gid); err == nil {
			gid = g
		}
	}
	if gid <= 0 {
		return fmt.Errorf("cannot drop sudo privileges: no valid target gid for uid %d", uid)
	}

	// The target user's real supplementary groups -- without this, the
	// process would keep ROOT's group memberships after the uid/gid drop.
	// This is also what makes docker access after demotion exactly as
	// right as it should be: it works iff the real user is in the docker
	// group.
	var gids []int
	if idStrs, err := u.GroupIds(); err == nil {
		for _, s := range idStrs {
			if g, err := strconv.Atoi(s); err == nil {
				gids = append(gids, g)
			}
		}
	}
	if len(gids) == 0 {
		gids = []int{gid}
	}

	if err := syscall.Setgroups(gids); err != nil {
		return fmt.Errorf("cannot drop sudo privileges: setgroups: %w", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("cannot drop sudo privileges: setgid %d: %w", gid, err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("cannot drop sudo privileges: setuid %d: %w", uid, err)
	}
	if os.Getuid() != uid || os.Geteuid() != uid {
		return fmt.Errorf("privilege drop verification failed: still uid %d / euid %d", os.Getuid(), os.Geteuid())
	}

	// Re-point the environment at the target user: HOME still holds root's
	// at this point, and anything derived from it (config paths, XDG
	// fallbacks) would land in /root otherwise.
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", u.HomeDir)
	_ = os.Setenv("USER", u.Username)
	_ = os.Setenv("LOGNAME", u.Username)
	for _, v := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		if val := os.Getenv(v); val != "" && oldHome != "" && oldHome != u.HomeDir && strings.HasPrefix(val, oldHome) {
			_ = os.Unsetenv(v)
		}
	}
	// Root's runtime dir (/run/user/0) is useless and misleading for the
	// demoted user (e.g. rootless-docker socket discovery).
	if val := os.Getenv("XDG_RUNTIME_DIR"); val != "" && !strings.HasSuffix(val, "/"+sudoUID) {
		_ = os.Unsetenv("XDG_RUNTIME_DIR")
	}
	clearSudoEnv()

	// The xdg library computed its paths when IT initialized (before this
	// package -- boot imports xdg, so xdg inits first); recompute them from
	// the corrected environment.
	xdg.Reload()

	demotionNotice = fmt.Sprintf("Started via '{{|UserCommand|}}sudo{{[-]}}'; dropped privileges back to '{{|User|}}%s{{[-]}}' (uid {{|User|}}%d{{[-]}}). Files created this run stay owned by that user.", u.Username, uid)
	return nil
}

// clearSudoEnv removes sudo's identity breadcrumbs so nothing downstream
// mistakes a past sudo hop for the process's current identity.
func clearSudoEnv() {
	for _, v := range []string{"SUDO_UID", "SUDO_GID", "SUDO_USER", "SUDO_COMMAND", "SUDO_HOME"} {
		_ = os.Unsetenv(v)
	}
}
