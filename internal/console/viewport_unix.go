//go:build !windows

package console

import (
	"os"
	"syscall"
)

func viewportSendSignal(b byte) {
	switch b {
	case 0x03: // Ctrl+C → SIGINT
		syscall.Kill(os.Getpid(), syscall.SIGINT)
	case 0x1C: // Ctrl+\ → SIGQUIT
		syscall.Kill(os.Getpid(), syscall.SIGQUIT)
	}
}
