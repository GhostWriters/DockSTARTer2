//go:build windows

package serve

import "os"

func signalProcess(p *os.Process) error {
	// Windows doesn't support SIGTERM; Kill is the only reliable option.
	return p.Kill()
}
