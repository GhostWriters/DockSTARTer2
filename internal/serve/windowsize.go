package serve

import (
	"context"

	"DockSTARTer2/internal/tui"

	"github.com/charmbracelet/ssh"
)

// makeWindowSizeChan converts wish's window-change channel into a
// tui.WindowSizeEvent channel. The initial PTY size is sent immediately,
// then subsequent resize events are forwarded until ctx is cancelled.
func makeWindowSizeChan(initial ssh.Pty, changes <-chan ssh.Window, ctx context.Context) <-chan tui.WindowSizeEvent {
	ch := make(chan tui.WindowSizeEvent, 8)
	go func() {
		defer close(ch)
		// Send initial size
		ch <- tui.WindowSizeEvent{Width: initial.Window.Width, Height: initial.Window.Height}
		for {
			select {
			case <-ctx.Done():
				return
			case w, ok := <-changes:
				if !ok {
					return
				}
				ch <- tui.WindowSizeEvent{Width: w.Width, Height: w.Height}
			}
		}
	}()
	return ch
}
