//go:build windows

package serve

import (
	"os"
	"os/exec"
	"syscall"
)

// SpawnDaemon re-execs the current binary with --server-daemon, detached from
// the console via DETACHED_PROCESS so it continues running after the parent
// exits. Stdout and stderr inherit the parent's so startup log lines are
// visible.
func SpawnDaemon(execPath string, extraArgs []string) (*os.Process, error) {
	args := append([]string{"--server-daemon"}, extraArgs...)
	cmd := exec.Command(execPath, args...)
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}
