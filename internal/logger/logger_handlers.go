package logger

import (
	"DockSTARTer2/internal/console"
	"context"
	"fmt"
	"io"
	"log/slog"
)

// TUIHandler redirects logs to a writer in the context or a global channel.
type TUIHandler struct {
	level  slog.Leveler
	global bool
}

func (h *TUIHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *TUIHandler) Handle(ctx context.Context, r slog.Record) error {
	// Format line consistently (TIME [LEVEL] \t MESSAGE)
	timeStr := r.Time.Format("2006-01-02 15:04:05")
	levelStr := FormatLevel(r.Level)

	// Wrap level in semantic tag for color
	var levelTag string
	switch r.Level {
	case LevelTrace:
		levelTag = "{{|Trace|}}"
	case LevelDebug:
		levelTag = "{{|Debug|}}"
	case LevelInfo:
		levelTag = "{{|Info|}}"
	case LevelNotice:
		levelTag = "{{|Notice|}}"
	case LevelWarn:
		levelTag = "{{|Warn|}}"
	case LevelError:
		levelTag = "{{|Error|}}"
	case LevelFatal:
		levelTag = "{{|Fatal|}}"
	default:
		levelTag = "{{[-]}}"
	}

	timeLevel := fmt.Sprintf("%s %s[%s]{{[-]}} ", timeStr, levelTag, levelStr)
	tuiMsg := timeLevel + r.Message

	if h.global {
		// Send to global log channel for TUI log panel
		select {
		case logLineCh <- tuiMsg:
		default:
			// Drop if full to prevent blocking
		}
	} else {
		// Output to specific TUI writer if present in context (LOCAL command output)
		// We use a goroutine to ensure that logging never blocks the execution of the task.
		// This prevents deadlocks if the TUI is busy or its message queue is full.
		if w, ok := ctx.Value(console.TUIWriterKey).(io.Writer); ok {
			go func() {
				fmt.Fprintln(w, tuiMsg)
			}()
		}
	}

	return nil
}

func (h *TUIHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *TUIHandler) WithGroup(name string) slog.Handler       { return h }

// FanoutHandler broadcasts records to multiple handlers
type FanoutHandler struct {
	handlers []slog.Handler
}

func (h *FanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *FanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (h *FanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &FanoutHandler{handlers: newHandlers}
}

func (h *FanoutHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &FanoutHandler{handlers: newHandlers}
}

// TagProcessorHandler processes custom tags and ANSI codes before passing to the base handler
type TagProcessorHandler struct {
	base slog.Handler
	mode string // "ansi", "strip", or "tui"
}

func (h *TagProcessorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *TagProcessorHandler) Handle(ctx context.Context, r slog.Record) error {
	// Suppress console output in TUI mode (ansi mode is for console)
	// EXCEPT for LevelFatal, which must always be visible.
	if h.mode == "ansi" && TUIMode && r.Level < LevelFatal {
		return nil
	}

	// Resolve message (it contains raw tags)
	msg := r.Message

	// Process based on mode
	switch h.mode {
	case "ansi":
		msg = console.ToConsoleANSI(msg)
	case "strip":
		msg = console.Strip(msg)
	}

	// Create new record with processed message
	newR := slog.NewRecord(r.Time, r.Level, msg, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		newR.AddAttrs(a)
		return true
	})

	return h.base.Handle(ctx, newR)
}

func (h *TagProcessorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TagProcessorHandler{base: h.base.WithAttrs(attrs), mode: h.mode}
}

func (h *TagProcessorHandler) WithGroup(name string) slog.Handler {
	return &TagProcessorHandler{base: h.base.WithGroup(name), mode: h.mode}
}
