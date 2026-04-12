//go:build windows

package console

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

const (
	enableVirtualTerminalProcessing = 0x0004
)

// EnableVirtualTerminalProcessing enables ANSI escape sequence support on Windows.
func EnableVirtualTerminalProcessing() {
	stdout := os.Stdout.Fd()

	var mode uint32
	ret, _, _ := procGetConsoleMode.Call(stdout, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}

	mode |= enableVirtualTerminalProcessing
	procSetConsoleMode.Call(stdout, uintptr(mode))
}
