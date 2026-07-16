package logger

import (
	"DockSTARTer2/internal/console"
	"github.com/GhostWriters/semstyle"
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
	// Format line consistently (TIME [LEVEL] \t MESSAGE). Tag names come from
	// timestampTagName/levelTagName (logger.go), also used by buildConsoleStyles.
	timeStr := r.Time.Format("2006-01-02 15:04:05")
	levelStr := FormatLevel(r.Level)

	levelTag := "{{[-]}}"
	if name := levelTagName(r.Level); name != "" {
		levelTag = "{{|" + name + "|}}"
	}

	timeLevel := fmt.Sprintf("{{|%s|}}%s{{[-]}} %s[%s]{{[-]}} ", timestampTagName, timeStr, levelTag, levelStr)
	tuiMsg := timeLevel + r.Message

	if h.global {
		// Skip the broadcast when a panel writer is active — the panel already
		// receives lines via its pipe scanner, so publishLogLine would double-log.
		// Program box writers use TUIWriterKey only (no PanelWriterKey), so they
		// still reach subscribers via publishLogLine.
		if ctx.Value(console.PanelWriterKey) != nil {
			return nil
		}
		publishLogLine(tuiMsg)
	} else {
		if w, ok := ctx.Value(console.TUIWriterKey).(io.Writer); ok {
			if !isSuppressed(ctx, w) {
				fmt.Fprintln(w, tuiMsg)
			}
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
	base          slog.Handler
	mode          string    // "ansi", "strip", or "tui"
	consoleWriter io.Writer // set for "ansi" handlers; used to match suppressWriterKey
}

func (h *TagProcessorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *TagProcessorHandler) Handle(ctx context.Context, r slog.Record) error {
	// Suppress console output in TUI mode or file-only context (ansi mode is for console)
	// EXCEPT for LevelFatal, which must always be visible.
	if h.mode == "ansi" && r.Level < LevelFatal {
		if TUIMode {
			return nil
		}
		// Suppress console when this command has a TUI writer (running inside a program box).
		// Do NOT suppress globally when IsTUIEnabled() — console panel commands have no TUI
		// writer on their context and rely on stdout reaching the panel's pipe scanner.
		if ctx.Value(console.TUIWriterKey) != nil {
			return nil
		}
		if isSuppressed(ctx, h.consoleWriter) || isSuppressed(ctx, currentConsoleWriter) {
			return nil
		}
	}

	// Resolve message (it contains raw tags)
	msg := r.Message

	// Process based on mode
	switch h.mode {
	case "ansi":
		msg = semstyle.ToANSI(msg)
	case "strip":
		msg = semstyle.ToPlain(msg)
	}

	// Create new record with processed message
	newR := slog.NewRecord(r.Time, r.Level, msg, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		newR.AddAttrs(a)
		return true
	})

	if h.mode == "ansi" {
		// Hold termMu across clear+write+show so the spinner goroutine cannot
		// interleave between these three steps.
		console.LockTerminal()
		console.ClearSpinnerLine()
		err := h.base.Handle(ctx, newR)
		console.ShowSpinnerFrame()
		console.UnlockTerminal()
		return err
	}
	return h.base.Handle(ctx, newR)
}

func (h *TagProcessorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TagProcessorHandler{base: h.base.WithAttrs(attrs), mode: h.mode, consoleWriter: h.consoleWriter}
}

func (h *TagProcessorHandler) WithGroup(name string) slog.Handler {
	return &TagProcessorHandler{base: h.base.WithGroup(name), mode: h.mode, consoleWriter: h.consoleWriter}
}
