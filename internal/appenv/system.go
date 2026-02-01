package appenv

import (
	"DockSTARTer2/internal/logger"
	"context"
	"os/exec"
	"runtime"
	"strings"
)

// GroupId returns the group ID for the given group name.
// Mirrors group_id function in misc_functions.sh.
func GroupId(ctx context.Context, groupName string) string {
	if runtime.GOOS == "linux" {
		// Linux: getent group "${GroupName}" | cut -d: -f3
		cmd := exec.Command("getent", "group", groupName)
		out, err := cmd.Output()
		if err == nil {
			parts := strings.Split(string(out), ":")
			if len(parts) >= 3 {
				return strings.TrimSpace(parts[2])
			}
		}
	} else if runtime.GOOS == "darwin" {
		// MacOS: dscl . -read /Groups/"${GroupName}" PrimaryGroupID | cut -d ' ' -f2
		cmd := exec.Command("dscl", ".", "-read", "/Groups/"+groupName, "PrimaryGroupID")
		out, err := cmd.Output()
		if err == nil {
			parts := strings.Split(string(out), " ")
			if len(parts) >= 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	logger.Warn(ctx, "Unable to get group id of '%s'. Defaulting to 1000.", groupName)
	return "1000"
}
