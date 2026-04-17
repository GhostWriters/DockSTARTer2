//go:build !windows

package serve

import (
	"os"
	"os/exec"
	"syscall"
)


// SpawnDaemon re-execs the current binary with --server-daemon, detached from
// the controlling terminal via a new session (setsid). Stdin is redirected to
// /dev/null; stdout and stderr inherit the parent's so startup log lines are
// visible, then the process runs independently.
func SpawnDaemon(execPath string, extraArgs []string) (*os.Process, error) {
	args := append([]string{"--server-daemon"}, extraArgs...)
	cmd := exec.Command(execPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}
