//go:build !windows

package serve

import (
	"os"
	"syscall"
)

func signalProcess(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
