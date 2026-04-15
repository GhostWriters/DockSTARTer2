package console

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// IsRemoteSession returns true if the current session appears to be over SSH.
func IsRemoteSession() bool {
	return os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != ""
}

// OpenURL opens the specified URL in the system's default browser.
// This is used for interactive TUI hyperlink handling.
func OpenURL(ctx context.Context, url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// rundll32 url.dll,FileProtocolHandler is more reliable than 'start' in non-standard shells
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	default: // linux, bsd, etc.
		if IsRemoteSession() && os.Getenv("DISPLAY") == "" {
			return fmt.Errorf("remote session detected: use Ctrl+Click (or Cmd+Click) to open the link in your local terminal")
		}
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	}

	// Start and return immediately, don't wait for the browser process to exit
	return cmd.Start()
}
