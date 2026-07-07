package system

import (
	dsexec "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"bytes"
	"context"
	"fmt"
	"os/user"
	"runtime"
	"slices"
	"strings"
)

// OfferDockerGroupFix handles the permission-denied-on-docker-socket case
// when it's specifically caused by the invoking user not being in the
// "docker" group: it explains the cause and offers to fix it (DS1's
// self-heal, but with an explicit prompt -- DS1 could piggyback the fix on
// its Docker install flow, while DS2 doesn't install Docker, so detecting
// exactly this problem IS its consent moment). Returns true when it
// produced the user-facing messaging (offer shown, fixed, declined, or the
// fix failed) so the caller skips the generic permission warning; false
// when the situation doesn't match (no docker group, already a member,
// lookup failure, non-Unix) and the generic warning should run instead.
//
// Group membership changes only apply to NEW login sessions -- the running
// process (and the user's current shell) cannot pick it up, not even via
// re-exec -- so a successful fix tells the user to log out and back in.
func OfferDockerGroupFix(ctx context.Context) bool {
	if runtime.GOOS == "windows" {
		return false
	}
	u, err := user.Current()
	if err != nil {
		return false
	}
	g, err := user.LookupGroup("docker")
	if err != nil {
		// No docker group on this system -- a different problem than
		// missing membership; let the generic message handle it.
		return false
	}
	ids, err := u.GroupIds()
	if err != nil {
		return false
	}
	if slices.Contains(ids, g.Gid) {
		// Already a member; the permission problem is something else.
		return false
	}

	logger.Warn(ctx, "Permission denied connecting to the Docker daemon socket: user '{{|User|}}%s{{[-]}}' is not in the '{{|User|}}docker{{[-]}}' group.", u.Username)
	yes, err := console.QuestionPrompt(ctx, logger.Notice, "Docker Group", fmt.Sprintf("Add user '%s' to the 'docker' group now?", u.Username), "Y", false)
	if err != nil || !yes {
		logger.Notice(ctx, "Not modifying group membership. To fix manually, run '{{|UserCommand|}}%s{{[-]}}' and then log out and back in.", addToGroupCommand(u.Username))
		return true
	}

	if err := addUserToDockerGroup(ctx, u.Username); err != nil {
		logger.Warn(ctx, "Failed to add user to the docker group: %v", err)
		return true
	}
	logger.Notice(ctx, "Added '{{|User|}}%s{{[-]}}' to the '{{|User|}}docker{{[-]}}' group. Log out and back in (or reboot) for it to take effect.", u.Username)
	return true
}

// addToGroupCommand returns the OS-appropriate command line for adding
// userName to the docker group, for display in messages.
func addToGroupCommand(userName string) string {
	if runtime.GOOS == "darwin" {
		return "sudo dseditgroup -o edit -a " + userName + " -t user docker"
	}
	return "sudo usermod -aG docker " + userName
}

// addUserToDockerGroup runs the OS-appropriate group-membership command via
// sudo, mirroring DS1's add_user_to_group (usermod on Linux, dseditgroup on
// macOS).
func addUserToDockerGroup(ctx context.Context, userName string) error {
	var cmdName string
	var args []string
	if runtime.GOOS == "darwin" {
		cmdName = "dseditgroup"
		args = []string{"-o", "edit", "-a", userName, "-t", "user", "docker"}
	} else {
		cmdName = "usermod"
		args = []string{"-aG", "docker", userName}
	}
	cmd, err := dsexec.SudoCommand(ctx, cmdName, args...)
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}
