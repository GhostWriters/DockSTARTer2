package console

import (
	"context"
	"io"
)

type contextKey string

// TUIWriterKey is the context key for the TUI writer.
const TUIWriterKey contextKey = "tui_writer"

// WithTUIWriter returns a new context with a TUI writer attached.
func WithTUIWriter(ctx context.Context, w io.Writer) context.Context {
	return context.WithValue(ctx, TUIWriterKey, w)
}

// IsTUI returns true if the context has a TUI writer attached or TUI mode is globally enabled.
func IsTUI(ctx context.Context) bool {
	return ctx.Value(TUIWriterKey) != nil || IsTUIEnabled()
}
