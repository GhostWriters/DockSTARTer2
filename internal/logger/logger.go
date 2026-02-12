package logger

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	charmlog "github.com/charmbracelet/log"
	lipglossv1 "github.com/charmbracelet/lipgloss" // v1 for charmbracelet/log compatibility
)

const TUIWriterKey = "tui_writer"

// WithTUIWriter returns a new context with a TUI writer attached.
func WithTUIWriter(ctx context.Context, w io.Writer) context.Context {
	return context.WithValue(ctx, TUIWriterKey, w)
}

// logLineCh carries TUI-formatted log lines to the log panel.
var logLineCh = make(chan string, 200)

// SubscribeLogLines returns a read-only channel that receives every log line
// formatted for TUI display (same format written to TUI writer).
func SubscribeLogLines() <-chan string {
	return logLineCh
}

// logFilePath is set during NewLogger so the TUI can pre-load it.
var logFilePath string

// GetLogFilePath returns the path to the current log file (empty if not yet initialised).
func GetLogFilePath() string {
	return logFilePath
}

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

	lines := strings.Split(msgStr, "\n")
	for i, line := range lines {
		// 1. Build TUI-formatted line for both the context writer and the log panel channel
		{
			timeStr := t.Format("2006-01-02 15:04:05")
			levelStr := ""
			switch level {
			case LevelTrace:
				levelStr = "TRACE "
			case LevelDebug:
				levelStr = "DEBUG "
			case LevelInfo:
				levelStr = "INFO  "
			case LevelNotice:
				levelStr = "NOTICE"
			case LevelWarn:
				levelStr = "WARN  "
			case LevelError:
				levelStr = "ERROR "
			case LevelFatal:
				levelStr = "FATAL "
			default:
				levelStr = level.String()
			}
			// Match standard console format: TIME [LEVEL] \t MESSAGE (space-tab after level)
			timeLevel := fmt.Sprintf("%s [%s] \t", timeStr, levelStr)

			// Send to context TUI writer using expanded (but not yet ANSI) tags
			if w, ok := ctx.Value(TUIWriterKey).(io.Writer); ok {
				tuiMsg := timeLevel + console.ForTUI(line)
				fmt.Fprintln(w, tuiMsg)
			}

			// Send to log panel channel with full ANSI codes so the viewport can render them
			// (non-blocking — drop if full)
			select {
			case logLineCh <- timeLevel + console.ToANSI(line):
			default:
			}
		}

		// 2. Output to standard slog handlers (stderr, file)
		// We pass the RAW line (with tags) to slog.Record.
		// The TagProcessorHandler will handle ANSI/Strip per handler.
		r := slog.NewRecord(t, level, line, 0)
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

// Package-level logger instances kept for dynamic level updates via SetLevel.
var consoleLogger *charmlog.Logger
var fileLogger *charmlog.Logger

func init() {
	LevelVar.Set(LevelNotice)
	FileLevelVar.Set(LevelInfo) // Default file to Info (-v behavior)
}

func SetLevel(level slog.Level) {
	LevelVar.Set(level)
	if consoleLogger != nil {
		consoleLogger.SetLevel(charmlog.Level(level))
	}
	// File level should be at least Info, or lower if Debug is requested
	fileLevel := LevelInfo
	if level < LevelInfo {
		fileLevel = level
	}
	FileLevelVar.Set(fileLevel)
	if fileLogger != nil {
		fileLogger.SetLevel(charmlog.Level(fileLevel))
	}
}

// buildConsoleStyles returns level styles using lipgloss colors.
// Colors are auto-stripped by charmbracelet/log when the output is not a TTY.
// NOTE: Uses lipgloss v1 for compatibility with charmbracelet/log until it updates to v2.
func buildConsoleStyles() *charmlog.Styles {
	st := charmlog.DefaultStyles()

	blue := lipglossv1.NewStyle().Foreground(lipglossv1.Color("4"))
	green := lipglossv1.NewStyle().Foreground(lipglossv1.Color("2"))
	yellow := lipglossv1.NewStyle().Foreground(lipglossv1.Color("3"))
	red := lipglossv1.NewStyle().Foreground(lipglossv1.Color("1"))
	fatal := lipglossv1.NewStyle().Foreground(lipglossv1.Color("7")).Background(lipglossv1.Color("1"))

	st.Levels[charmlog.Level(LevelTrace)] = blue.SetString("[TRACE ]")
	st.Levels[charmlog.Level(LevelDebug)] = blue.SetString("[DEBUG ]")
	st.Levels[charmlog.Level(LevelInfo)] = blue.SetString("[INFO  ]")
	st.Levels[charmlog.Level(LevelNotice)] = green.SetString("[NOTICE]")
	st.Levels[charmlog.Level(LevelWarn)] = yellow.SetString("[WARN  ]")
	st.Levels[charmlog.Level(LevelError)] = red.SetString("[ERROR ]")
	st.Levels[charmlog.Level(LevelFatal)] = fatal.SetString("[FATAL ]")

	return st
}

func NewLogger() *slog.Logger {
	wStderr := os.Stderr

	// 1. Configure Console Handler using charmbracelet/log.
	// Color support is auto-detected from the output writer (TTY vs non-TTY).
	consoleLogger = charmlog.NewWithOptions(wStderr, charmlog.Options{
		Level:           charmlog.Level(LevelVar.Level()),
		TimeFormat:      "2006-01-02 15:04:05",
		ReportTimestamp: true,
	})
	consoleLogger.SetStyles(buildConsoleStyles())
	consoleHandler := &TagProcessorHandler{base: consoleLogger, mode: "ansi"}

	// 2. Configure File Handler (No Color)
	stateDir := paths.GetStateDir()
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create state directory: %v\n", err)
	}

	appName := strings.ToLower(version.ApplicationName)
	logFilePath = filepath.Join(stateDir, appName+".log")

	// Open file in Append mode
	wFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
	}

	handlers := []slog.Handler{consoleHandler}

	if wFile != nil {
		fmt.Fprintln(wFile, version.ApplicationName+" Log")

		// File handler: charmbracelet/log auto-strips colors for non-TTY writers.
		fileLogger = charmlog.NewWithOptions(wFile, charmlog.Options{
			Level:           charmlog.Level(FileLevelVar.Level()),
			TimeFormat:      "2006-01-02 15:04:05",
			ReportTimestamp: true,
		})
		fileLogger.SetStyles(buildConsoleStyles())
		fileHandler := &TagProcessorHandler{base: fileLogger, mode: "strip"}
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

// TagProcessorHandler processes custom tags and ANSI codes before passing to the base handler
type TagProcessorHandler struct {
	base slog.Handler
	mode string // "ansi", "strip", or "tui"
}

func (h *TagProcessorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *TagProcessorHandler) Handle(ctx context.Context, r slog.Record) error {
	// 1. Resolve message (it contains raw tags)
	msg := r.Message

	// 2. Process based on mode
	switch h.mode {
	case "ansi":
		msg = console.ToANSI(msg)
	case "strip":
		msg = console.Strip(msg)
	}

	// 3. Create new record with processed message
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

// Display prints a message without any timestamps or log level metadata.
// It still redirects to the TUI if a TUI writer is present in the context.
// This output is NOT written to the log file.
func Display(ctx context.Context, msg any, args ...any) {
	msgStr := resolveMsg(msg)
	if len(args) > 0 && strings.Contains(msgStr, "%") {
		msgStr = fmt.Sprintf(msgStr, args...)
	}

	lines := strings.Split(msgStr, "\n")
	for _, line := range lines {
		// 1. Output to TUI if writer is in context
		if w, ok := ctx.Value(TUIWriterKey).(io.Writer); ok {
			// Use ForTUI to keep styles while removing ANSI
			// NOTE: Do NOT escape - ForTUI output is already in proper format
			fmt.Fprintln(w, console.ForTUI(line))
		}

		// 2. Output directly to terminal (stdout)
		// IMPORTANT: Always use ToANSI for stdout to get ANSI colors, regardless of TUI mode
		fmt.Println(console.ToANSI(line) + console.CodeReset)
	}
}

func getSystemInfo() []string {
	var info []string

	// App Info
	info = append(info, fmt.Sprintf("{{_ApplicationName_}}%s{{|-|}} [{{_Version_}}%s{{|-|}}]", version.ApplicationName, version.Version))
	info = append(info, fmt.Sprintf("{{_ApplicationName_}}DockSTARTer-Templates{{|-|}} [{{_Version_}}%s{{|-|}}]", paths.GetTemplatesVersion()))
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
func FatalWithStack(ctx context.Context, msg any, args ...any) {
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
			infoLines = append(infoLines, "\t"+i /* console.ToANSI handled by logAt */)
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
		// Note: We leave the tags in the string. logAt -> console.ToANSI will replace them.
		fmtStr := fmt.Sprintf("%%s%%%dd{{|-|}}%%s %%s%%s%%s%%s:%%s%%d{{|-|}} (%%s%%s{{|-|}})", width)

		line := fmt.Sprintf(
			fmtStr,
			"{{_TraceFrameNumber_}}", i,
			":",
			arrowIndent,
			"{{_TraceFrameLines_}}"+suffix+"{{|-|}}",
			"{{_TraceSourceFile_}}", frame.File,
			"{{_TraceLineNumber_}}", frame.Line,
			"{{_TraceFunction_}}", filepath.Base(frame.Function),
		)

		traceLines = append(traceLines, "\t"+line)

		// Prepare for next frame
		indent += "  "
	}

	// 3. Assemble Final Output
	// This provides a visual representation of the final log block structure
	output := []any{
		"{{_TraceHeader_}}### BEGIN SYSTEM INFORMATION AND STACK TRACE ###",
		infoLines,
		"", // Separator
		traceLines,
		"{{_TraceFooter_}}### END SYSTEM INFORMATION AND STACK TRACE ###",
		"",
		msg,
		"",
		"{{_FatalFooter_}}Please let the dev know of this error.",
		"{{_FatalFooter_}}It has been written to {{|-|}}'{{_File_}}" + filepath.Join(paths.GetStateDir(), strings.ToLower(version.ApplicationName)+".fatal.log") + "{{|-|}}'{{_FatalFooter_}},",
		"and appended to {{|-|}}'{{_File_}}" + filepath.Join(paths.GetStateDir(), strings.ToLower(version.ApplicationName)+".log") + "{{|-|}}'{{_FatalFooter_}}.",
	}

	// 4. Log Everything
	logAt(ctx, now, LevelFatal, output, args...)

	// 5. Write to fatal log file
	writeFatalLog(ctx, now, output, args...)

	panic(FatalError{})
}

// FatalNoTrace logs a message at FatalLevel without stack trace and exits
func Fatal(ctx context.Context, msg any, args ...any) {
	output := []any{
		msg,
		"",
		"{{_FatalFooter_}}Please let the dev know of this error.",
	}
	now := time.Now()
	logAt(ctx, now, LevelFatal, output, args...)

	// Write to fatal log file
	writeFatalLog(ctx, now, output, args...)

	panic(FatalError{})
}

// writeFatalLog writes the resolved message to a separate fatal log file
func writeFatalLog(ctx context.Context, t time.Time, msg any, args ...any) {
	stateDir := paths.GetStateDir()
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return
	}

	appName := strings.ToLower(version.ApplicationName)
	fatalLogPath := filepath.Join(stateDir, appName+".fatal.log")

	// Explicitly truncate the file to ensure we only have the latest fatal error
	f, err := os.OpenFile(fatalLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create fatal log file: %v\n", err)
		return
	}
	defer f.Close()

	msgStr := resolveMsg(msg)
	if len(args) > 0 && strings.Contains(msgStr, "%") {
		msgStr = fmt.Sprintf(msgStr, args...)
	}

	lines := strings.Split(msgStr, "\n")
	for _, line := range lines {
		// Just write plain text
		// We'll leave the tags in for now as they are semantic and might be useful
		// until a StripTags function is available.
		fmt.Fprintln(f, line)
	}
}

// FatalError is a special error used to panic from Fatal logger calls
// This allows the main run loop to recover and perform cleanup before exiting
type FatalError struct{}

// Cleanup performs final logging tasks, such as truncating the log file.
func Cleanup() {
	stateDir := paths.GetStateDir()
	appName := strings.ToLower(version.ApplicationName)
	logFilePath := filepath.Join(stateDir, appName+".log")
	truncateLogFile(logFilePath, 1000)
}

// truncateLogFile keeps the last N lines of the file at path
func truncateLogFile(path string, limit int) {
	content, err := os.ReadFile(path)
	if err != nil {
		return // File likely doesn't exist, which is fine
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) <= limit {
		return // No need to truncate
	}

	// Keep last limit lines
	// Note: Split often results in a trailing empty string if file ends with newline
	// We should be careful.
	// If the file ends with \n, the last element is empty.
	// Let's just take the last limit.

	start := len(lines) - limit
	keptLines := lines[start:]

	output := strings.Join(keptLines, "\n")

	// Rewrite file
	if err := os.WriteFile(path, []byte(output), 0666); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to rotate log file: %v\n", err)
	}
}
