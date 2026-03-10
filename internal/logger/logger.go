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
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/muesli/termenv"
)

// TUIMode suppresses direct console output (stdout/stderr) when active.
var TUIMode bool

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
	if len(args) > 0 && strings.Contains(msgStr, "%") {
		msgStr = fmt.Sprintf(msgStr, args...)
		args = nil
	}

	lines := strings.Split(msgStr, "\n")
	for i, line := range lines {
		r := slog.NewRecord(t, level, line, 0)
		if i == 0 {
			r.Add(args...)
		}
		_ = h.Handle(ctx, r)
	}
}

// FormatLevel returns a consistent 6-character string for log levels.
func FormatLevel(level slog.Level) string {
	switch level {
	case LevelTrace:
		return "TRACE "
	case LevelDebug:
		return "DEBUG "
	case LevelInfo:
		return "INFO  "
	case LevelNotice:
		return "NOTICE"
	case LevelWarn:
		return "WARN  "
	case LevelError:
		return "ERROR "
	case LevelFatal:
		return "FATAL "
	default:
		return level.String()
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

	// Register global abort handler for console prompts
	console.AbortHandler = func(ctx context.Context) {
		Error(ctx, "User aborted via CTRL-C")
	}
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

// SetColorProfile forces the color profile for the console logger.
func SetColorProfile(profile termenv.Profile) {
	if consoleLogger != nil {
		consoleLogger.SetColorProfile(colorprofile.Profile(profile))
	}
}

// SetConsoleOutput redirects the console logger output to the provided writer.
// It returns a function that restores the original output (os.Stderr).
// This is useful for capturing logger output in TUI components.
func SetConsoleOutput(w io.Writer) func() {
	if consoleLogger == nil {
		return func() {}
	}

	consoleLogger.SetOutput(w)

	return func() {
		// Restore to default stderr
		consoleLogger.SetOutput(os.Stderr)
	}
}

// buildConsoleStyles returns level styles using lipgloss colors.
// Colors are auto-stripped by charmbracelet/log when the output is not a TTY.
func buildConsoleStyles() *charmlog.Styles {
	st := charmlog.DefaultStyles()
	st.Timestamp = lipgloss.NewStyle().Faint(true)

	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	fatal := lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Background(lipgloss.Color("1"))

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

	// Configure Console Handler using charmbracelet/log.
	// Color support is auto-detected from the output writer (TTY vs non-TTY).
	consoleLogger = charmlog.NewWithOptions(wStderr, charmlog.Options{
		Level:           charmlog.Level(LevelVar.Level()),
		TimeFormat:      "2006-01-02 15:04:05",
		ReportTimestamp: true,
	})
	consoleLogger.SetStyles(buildConsoleStyles())
	consoleHandler := &TagProcessorHandler{base: consoleLogger, mode: "ansi"}

	// Configure File Handler (No Color)
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

	handlers := []slog.Handler{
		consoleHandler,
		&TUIHandler{level: LevelDebug, global: true},
		&TUIHandler{level: LevelVar, global: false},
	}

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
		// Output to TUI if writer is in context
		if w, ok := ctx.Value(console.TUIWriterKey).(io.Writer); ok {
			// Use ForTUI to keep styles while removing ANSI
			fmt.Fprintln(w, console.ForTUI(line))
		}

		// Output directly to terminal (stdout)
		// IMPORTANT: Always use ToANSI for stdout to get ANSI colors, regardless of TUI mode
		// Suppress based on TUIMode
		if !TUIMode {
			fmt.Println(console.ToANSI(line) + console.CodeReset)
		}
	}
}
