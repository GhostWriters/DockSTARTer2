//go:build windows

package update

import (
	"context"
	"fmt"
)

func SelfUpdate(ctx context.Context, force bool, yes bool, requestedVersion string, restArgs []string) error {
	return fmt.Errorf("self-update is not supported on Windows")
}

func ReExec(ctx context.Context, exePath string, args []string) error {
	return fmt.Errorf("re-exec is not supported on Windows")
}
