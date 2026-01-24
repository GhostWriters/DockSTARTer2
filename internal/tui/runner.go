package tui

import (
	"DockSTARTer2/internal/logger"
	"context"

	"codeberg.org/tslocum/cview"
)

// TUIWriter implements io.Writer to pipe output to a cview.TextView
type TUIWriter struct {
	textView *cview.TextView
	app      *cview.Application
}

func (w *TUIWriter) Write(p []byte) (n int, err error) {
	// We use QueueUpdateDraw to ensure thread safety
	w.app.QueueUpdateDraw(func() {
		w.textView.Write(p)
		w.textView.ScrollToEnd()
	})
	return len(p), nil
}

// RunCommand runs a function in a TUI "ProgramBox" context.
func RunCommand(ctx context.Context, title string, task func(context.Context) error) error {
	// If a TUI writer is already attached to the context, we are already
	// running within a TUI ProgramBox. In this case, just run the task.
	if ctx.Value("tui_writer") != nil {
		return task(ctx)
	}

	standalone := (app == nil)
	if err := Initialize(ctx); err != nil {
		return err
	}
	if standalone {
		defer app.GetScreen().Fini()
	}

	// Create the ProgramBox
	tv, btnOK, frame := NewProgramBox(title, "Running task...")

	panels.AddPanel("programbox", frame, true, true)
	app.SetRoot(rootGrid, true)

	tuiWriter := &TUIWriter{textView: tv, app: app}
	taskCtx := logger.WithTUIWriter(ctx, tuiWriter)

	done := make(chan error, 1)
	go func() {
		err := task(taskCtx)

		app.QueueUpdateDraw(func() {
			btnOK.SetSelectedFunc(func() {
				if standalone {
					app.Stop()
				} else {
					panels.RemovePanel("programbox")
				}
			})
			app.SetFocus(btnOK)
			helpline.SetText(" Task finished. Press OK to continue. ")
		})

		done <- err
	}()

	if standalone {
		return app.Run()
	}

	return nil
}
