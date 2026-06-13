//go:build !windows

package console

import (
	"os"
	"syscall"
)

func viewportSendSignal(b byte) {
	switch b {
	case 0x03: // Ctrl+C → SIGINT
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	case 0x1C: // Ctrl+\ → SIGQUIT
		_ = syscall.Kill(os.Getpid(), syscall.SIGQUIT)
	}
}
