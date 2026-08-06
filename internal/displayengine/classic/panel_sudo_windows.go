//go:build windows

package classic

import (
	"context"
	"io"
)

// runSudoWithPassword has no Windows equivalent -- sudo isn't part of this
// platform's flow (Windows support is test-only). Falls back to a plain
// piped run so the build stays cross-platform.
func runSudoWithPassword(ctx context.Context, cmdStr, pass string, w io.Writer) error {
	return runShellCmd(ctx, cmdStr, w, pass)
}
