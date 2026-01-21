package logger

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lmittmann/tint"
)

// Helper to resolve message from any type to string
func resolveMsg(msg any) string {
	switch v := msg.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, "\n")
	case []any:
		var parts []string
		for _, item := range v {
			parts = append(parts, resolveMsg(item))
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprint(v)
	}
}

// Internal helper to split multi-line messages (legacy for auto-timestamp)
func log(ctx context.Context, level slog.Level, msg any, args ...any) {
	logAt(ctx, time.Now(), level, msg, args...)
}

// Internal helper to log with a specific timestamp
func logAt(ctx context.Context, t time.Time, level slog.Level, msg any, args ...any) {
	h := slog.Default().Handler()
	if !h.Enabled(ctx, level) {
		return
	}

	msgStr := resolveMsg(msg)
	// If it's a string (or resolved from a slice), we might need to format it with args.
	// We check if args are present and msgStr has format specifiers.
	if len(args) > 0 && strings.Contains(msgStr, "%") {
		msgStr = fmt.Sprintf(msgStr, args...)
		args = nil // Reset args as they are now consumed
	}
	msgStr = console.Parse(msgStr)

	// Ensure the message resets colors at the end (for single line case)
	// For multi-line, we'll append to each line below.
	if !strings.Contains(msgStr, "\n") {
		r := slog.NewRecord(t, level, msgStr+console.CodeReset, 0)
		r.Add(args...)
		_ = h.Handle(ctx, r)
		return
	}

	lines := strings.Split(msgStr, "\n")
	for i, line := range lines {
		// Append reset to every line to prevent color bleed to next timestamp
		r := slog.NewRecord(t, level, line+console.CodeReset, 0)
		if i == 0 {
			r.Add(args...)
		}
		_ = h.Handle(ctx, r)
	}
}

// Custom log levels to match DockSTARTer
const (
	LevelTrace  = slog.Level(-8)
	LevelDebug  = slog.LevelDebug
	LevelInfo   = slog.Level(-2)
	LevelNotice = slog.LevelInfo
	LevelWarn   = slog.LevelWarn
	LevelError  = slog.LevelError
	LevelFatal  = slog.Level(12)
)

// LevelVar allows dynamic changing of the log level
var LevelVar = new(slog.LevelVar)
var FileLevelVar = new(slog.LevelVar)

func init() {
	LevelVar.Set(LevelNotice)
	FileLevelVar.Set(LevelInfo) // Default file to Info (-v behavior)
}

func SetLevel(level slog.Level) {
	LevelVar.Set(level)
	// File level should be at least Info, or lower if Debug is requested
	if level < LevelInfo {
		FileLevelVar.Set(level)
	} else {
		FileLevelVar.Set(LevelInfo)
	}
}

func NewLogger() *slog.Logger {
	wStderr := os.Stderr

	// Check if output is a terminal (TTY)
	stat, _ := wStderr.Stat()
	isTTY := (stat.Mode() & os.ModeCharDevice) != 0

	// 1. Configure Console Handler (Colors if TTY)
	var (
		ansiReset  string
		ansiBlue   string
		ansiGreen  string
		ansiYellow string
		ansiRed    string
		ansiRedBg  string
	)

	if isTTY {
		ansiReset = console.CodeReset
		ansiBlue = console.CodeBlue
		ansiGreen = console.CodeGreen // Notice
		ansiYellow = console.CodeYellow
		ansiRed = console.CodeRed
		ansiRedBg = console.CodeRedBg + console.CodeWhite
	}

	replaceAttrConsole := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.LevelKey {
			level := a.Value.Any().(slog.Level)
			switch level {
			case LevelTrace:
				a.Value = slog.StringValue(ansiBlue + "[TRACE ]" + ansiReset + "  ")
			case LevelDebug:
				a.Value = slog.StringValue(ansiBlue + "[DEBUG ]" + ansiReset + "  ")
			case LevelInfo:
				a.Value = slog.StringValue(ansiBlue + "[INFO  ]" + ansiReset + "  ")
			case LevelNotice:
				a.Value = slog.StringValue(ansiGreen + "[NOTICE]" + ansiReset + "  ")
			case LevelWarn:
				a.Value = slog.StringValue(ansiYellow + "[WARN  ]" + ansiReset + "  ")
			case LevelError:
				a.Value = slog.StringValue(ansiRed + "[ERROR ]" + ansiReset + "  ")
			case LevelFatal:
				a.Value = slog.StringValue(ansiRedBg + "[FATAL ]" + ansiReset + "  ")
			default:
				a.Value = slog.StringValue("[" + level.String() + "]")
			}
		}
		return a
	}

	consoleOpts := &tint.Options{
		Level:       LevelVar,
		TimeFormat:  "2006-01-02 15:04:05",
		NoColor:     !isTTY,
		ReplaceAttr: replaceAttrConsole,
	}
	consoleHandler := tint.NewHandler(wStderr, consoleOpts)

	// 2. Configure File Handler (No Color)
	logFilePath := "dockstarter.log"
	// Ensure clean start
	_ = os.Remove(logFilePath)
	// Attempt to open file
	wFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
	}

	handlers := []slog.Handler{consoleHandler}

	if wFile != nil {
		replaceAttrFile := func(groups []string, a slog.Attr) slog.Attr {
			// Strip ANSI codes from message if possible, or just Ensure we don't ADD them for levels
			if a.Key == slog.LevelKey {
				level := a.Value.Any().(slog.Level)
				// Clean level strings
				switch level {
				case LevelTrace:
					a.Value = slog.StringValue("[TRACE ]  ")
				case LevelDebug:
					a.Value = slog.StringValue("[DEBUG ]  ")
				case LevelInfo:
					a.Value = slog.StringValue("[INFO  ]  ")
				case LevelNotice:
					a.Value = slog.StringValue("[NOTICE]  ")
				case LevelWarn:
					a.Value = slog.StringValue("[WARN  ]  ")
				case LevelError:
					a.Value = slog.StringValue("[ERROR ]  ")
				case LevelFatal:
					a.Value = slog.StringValue("[FATAL ]  ")
				default:
					a.Value = slog.StringValue("[" + level.String() + "]")
				}
			}
			if a.Key == slog.MessageKey {
				// Strip basic color codes from the message string itself?
				// console.Strip() function might be useful if available,
				// but for now we rely on tint NoColor=true to handle attributes key/values,
				// and manually strip if we built the message with colors.
				// Our 'logAt' helper calls console.Parse(msg).
				// We might need to write a 'Strip' helper in console package later.
				// For now, let's trust tint.Options{NoColor: true}.
				// NOTE: tint's NoColor affects its own formatting, not the message content string.
			}
			return a
		}

		fileOpts := &tint.Options{
			Level:       FileLevelVar,
			TimeFormat:  "2006-01-02 15:04:05",
			NoColor:     true, // Important
			ReplaceAttr: replaceAttrFile,
		}
		fileHandler := tint.NewHandler(wFile, fileOpts)
		handlers = append(handlers, fileHandler)
	}

	return slog.New(&FanoutHandler{handlers: handlers})
}

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

// Global helpers for custom levels that don't satisfy standard slog methods
func Trace(ctx context.Context, msg any, args ...any) {
	log(ctx, LevelTrace, msg, args...)
}

func Debug(ctx context.Context, msg any, args ...any) {
	log(ctx, LevelDebug, msg, args...)
}

func Info(ctx context.Context, msg any, args ...any) {
	log(ctx, LevelInfo, msg, args...)
}

func Notice(ctx context.Context, msg any, args ...any) {
	log(ctx, LevelNotice, msg, args...)
}

func Warn(ctx context.Context, msg any, args ...any) {
	log(ctx, LevelWarn, msg, args...)
}

func Error(ctx context.Context, msg any, args ...any) {
	log(ctx, LevelError, msg, args...)
}

func getSystemInfo() []string {
	var info []string

	// App Info
	info = append(info, fmt.Sprintf("[_ApplicationName_]%s[-] [[_Version_]%s[-]]", version.ApplicationName, version.Version))
	info = append(info, fmt.Sprintf("[_ApplicationName_]DockSTARTer-Templates[-] [[_Version_]%s[-]]", paths.GetTemplatesVersion()))
	info = append(info, "")

	// Process Info
	executable, _ := os.Executable()
	info = append(info, fmt.Sprintf("Currently running as: %s (PID %d)", executable, os.Getpid()))
	info = append(info, "")

	// System Info
	info = append(info, fmt.Sprintf("ARCH:             %s", runtime.GOARCH))
	info = append(info, fmt.Sprintf("OS:               %s", runtime.GOOS))

	// Script/Binary Path
	base := filepath.Base(executable)
	dir := filepath.Dir(executable)
	info = append(info, fmt.Sprintf("SCRIPTPATH:       %s", dir))
	info = append(info, fmt.Sprintf("SCRIPTNAME:       %s", base))
	info = append(info, "")

	// User Info
	currentUser, err := user.Current()
	if err == nil {
		info = append(info, fmt.Sprintf("DETECTED_PUID:    %s", currentUser.Uid))
		info = append(info, fmt.Sprintf("DETECTED_UNAME:   %s", currentUser.Username))
		info = append(info, fmt.Sprintf("DETECTED_GID:     %s", currentUser.Gid))
		info = append(info, fmt.Sprintf("DETECTED_HOMEDIR: %s", currentUser.HomeDir))
	} else {
		info = append(info, fmt.Sprintf("User Info Error: %v", err))
	}

	return info
}

// Fatal logs a message at FatalLevel and exits
func Fatal(ctx context.Context, msg any, args ...any) {
	// Capture time once for all lines
	now := time.Now()

	// 1. Gather Stack Frames
	pc := make([]uintptr, 32)
	n := runtime.Callers(1, pc) // Skip only runtime.Callers, include Fatal
	frames := runtime.CallersFrames(pc[:n])

	// 2. Prepare Log Components

	// A. System Info
	var infoLines []string
	rawInfo := getSystemInfo()
	for _, i := range rawInfo {
		if i != "" {
			infoLines = append(infoLines, "  "+i /* console.Parse handled by logAt */)
		} else {
			infoLines = append(infoLines, "")
		}
	}

	// B. Stack Trace
	var allFrames []runtime.Frame
	for {
		frame, more := frames.Next()
		allFrames = append(allFrames, frame)
		if !more {
			break
		}
	}

	var traceLines []string
	// Calculate required padding width
	maxIndex := len(allFrames) - 1
	width := len(fmt.Sprintf("%d", maxIndex)) // e.g. "9" -> 1, "99" -> 2

	wd, _ := os.Getwd()

	// Iterate in reverse: Main (Last) -> Fatal (First)
	indent := "" // Reset indent
	for i := len(allFrames) - 1; i >= 0; i-- {
		frame := allFrames[i]

		// Try to make path relative to CWD
		if wd != "" {
			if rel, err := filepath.Rel(wd, frame.File); err == nil {
				// Check if it's actually a subpath (doesn't start with ..)
				if !strings.HasPrefix(rel, "..") && !strings.HasPrefix(rel, string(filepath.Separator)) {
					frame.File = "./" + filepath.ToSlash(rel)
				}
			}
		}

		suffix := ""
		arrowIndent := indent
		if i < len(allFrames)-1 {
			suffix = "└>"
			if len(indent) >= 2 {
				arrowIndent = indent[:len(indent)-2]
			}
		}

		// Create format string dynamically to pad frame number
		// Note: We leave the tags in the string. logAt -> console.Parse will replace them.
		fmtStr := fmt.Sprintf("%%s%%%dd[-]%%s %%s%%s%%s%%s:%%s%%d[-] (%%s%%s[-])", width)

		line := fmt.Sprintf(
			fmtStr,
			"[_TraceFrameNumber_]", i,
			":",
			arrowIndent,
			"[_TraceFrameLines_]"+suffix+"[-]",
			"[_TraceSourceFile_]", frame.File,
			"[_TraceLineNumber_]", frame.Line,
			"[_TraceFunction_]", filepath.Base(frame.Function),
		)

		traceLines = append(traceLines, "  "+line)

		// Prepare for next frame
		indent += "  "
	}

	// 3. Assemble Final Output
	// This provides a visual representation of the final log block structure
	output := []any{
		"[_TraceHeader_]### BEGIN SYSTEM INFORMATION AND STACK TRACE ###",
		infoLines,
		"", // Separator
		traceLines,
		"[_TraceFooter_]### END SYSTEM INFORMATION AND STACK TRACE ###",
		"",
		msg,
		"",
		"[_FatalFooter_]Please let the dev know of this error.",
		"[_FatalFooter_]It has been written to [-]'[_File_]FATAL_LOG[-]'[_FatalFooter_], and appended to [-]'[_File_]APPLICATION_LOG[-]'[_FatalFooter_].",
	}

	// 4. Log Everything
	logAt(ctx, now, LevelFatal, output, args...)

	panic(FatalError{})
}

// FatalNoTrace logs a message at FatalLevel without stack trace and exits
func FatalNoTrace(ctx context.Context, msg any, args ...any) {
	output := []any{
		msg,
		"",
		"[_FatalFooter_]Please let the dev know of this error.",
	}
	logAt(ctx, time.Now(), LevelFatal, output, args...)
	panic(FatalError{})
}

// FatalError is a special error used to panic from Fatal logger calls
// This allows the main run loop to recover and perform cleanup before exiting
type FatalError struct{}
