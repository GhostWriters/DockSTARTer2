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

// confirmFuncKey is the context key for a session-scoped confirm callback.
type confirmFuncKey struct{}

// WithConfirmFunc attaches a session-scoped Yes/No confirm callback to ctx.
// QuestionPrompt prefers this over the global TUIConfirm var, which is
// shared process-wide and can point at the wrong (or an already-exited)
// session in a server-daemon process serving multiple concurrent sessions.
func WithConfirmFunc(ctx context.Context, fn func(title, question string, defaultYes bool) bool) context.Context {
	return context.WithValue(ctx, confirmFuncKey{}, fn)
}

// ConfirmFuncFromContext returns the session-scoped confirm callback attached
// via WithConfirmFunc, or nil if none is present.
func ConfirmFuncFromContext(ctx context.Context) func(title, question string, defaultYes bool) bool {
	fn, _ := ctx.Value(confirmFuncKey{}).(func(title, question string, defaultYes bool) bool)
	return fn
}

// promptFuncKey is the context key for a session-scoped text-prompt callback.
type promptFuncKey struct{}

// WithPromptFunc attaches a session-scoped text-prompt callback to ctx.
// TextPrompt prefers this over the global TUIPrompt var, for the same reason
// QuestionPrompt prefers WithConfirmFunc over TUIConfirm (see its doc comment).
func WithPromptFunc(ctx context.Context, fn func(title, question string, sensitive bool, initialValue ...string) (string, error)) context.Context {
	return context.WithValue(ctx, promptFuncKey{}, fn)
}

// PromptFuncFromContext returns the session-scoped prompt callback attached
// via WithPromptFunc, or nil if none is present.
func PromptFuncFromContext(ctx context.Context) func(title, question string, sensitive bool, initialValue ...string) (string, error) {
	fn, _ := ctx.Value(promptFuncKey{}).(func(title, question string, sensitive bool, initialValue ...string) (string, error))
	return fn
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

// OutputContentWidthFn is set by the tui package to report the current content width
// (in columns) of the active output viewport — the program box or the console panel.
// compose calls this to size proportional progress bars to the viewport rather than the
// raw terminal width. Returns 0 if no TUI viewport is active (fall back to terminal width).
var OutputContentWidthFn func() int
