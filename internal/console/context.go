package console

import (
	"context"
	"io"
)

type contextKey string

// TUIWriterKey is the context key for the TUI writer.
const TUIWriterKey contextKey = "tui_writer"

// PanelWriterKey marks a TUI writer that feeds the console panel scanner.
// When present, the global log handler suppresses logLineCh to avoid
// double-logging (the panel already receives lines via the pipe scanner).
const PanelWriterKey contextKey = "panel_writer"

// WithTUIWriter returns a new context with a TUI writer attached.
func WithTUIWriter(ctx context.Context, w io.Writer) context.Context {
	return context.WithValue(ctx, TUIWriterKey, w)
}

// WithPanelWriter attaches a TUI writer and marks it as a panel writer.
func WithPanelWriter(ctx context.Context, w io.Writer) context.Context {
	ctx = context.WithValue(ctx, TUIWriterKey, w)
	return context.WithValue(ctx, PanelWriterKey, struct{}{})
}

// IsTUI returns true if the context has a TUI writer attached or TUI mode is globally enabled.
func IsTUI(ctx context.Context) bool {
	return ctx.Value(TUIWriterKey) != nil || IsTUIEnabled()
}

// GetTUIWriter returns the TUI writer from the context if it exists.
func GetTUIWriter(ctx context.Context) io.Writer {
	if w, ok := ctx.Value(TUIWriterKey).(io.Writer); ok {
		return w
	}
	return nil
}

// ReplaceOutputLinesFn is set by the tui package to send a replaceOutputMsg to
// the running program. compose calls this to do live in-place updates inside the
// program box without cursor-movement ANSI sequences.
var ReplaceOutputLinesFn func([]string)
