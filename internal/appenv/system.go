package appenv

import (
	"DockSTARTer2/internal/logger"
	"context"
	"os/user"
	"runtime"
)

// GroupId returns the group ID for the given group name.
// Mirrors group_id function in misc_functions.sh.
func GroupId(ctx context.Context, groupName string) string {
	if runtime.GOOS == "windows" {
		return "1000"
	}

	g, err := user.LookupGroup(groupName)
	if err == nil {
		return g.Gid
	}

	logger.Warn(ctx, "Unable to get group id of '%s'. Defaulting to 1000. Error: %v", groupName, err)
	return "1000"
}
