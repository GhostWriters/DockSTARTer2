package logger

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

func getSystemInfo() []string {
	var info []string

	// App Info
	info = append(info, fmt.Sprintf("{{|ApplicationName|}}%s{{[-]}} [{{|Version|}}%s{{[-]}}]", version.ApplicationName, version.Version))
	info = append(info, fmt.Sprintf("{{|ApplicationName|}}DockSTARTer-Templates{{[-]}} [{{|Version|}}%s{{[-]}}]", paths.GetTemplatesVersion()))
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
	FatalWithStackSkip(ctx, 1, msg, args...)
}

// FatalWithStackSkip allows specifying how many frames to skip in the stack trace
func FatalWithStackSkip(ctx context.Context, skip int, msg any, args ...any) {
	// Capture time once for all lines
	now := time.Now()

	// Gather Stack Frames
	pc := make([]uintptr, 32)
	n := runtime.Callers(skip+1, pc) // Skip requested frames + this one
	frames := runtime.CallersFrames(pc[:n])

	// Capture raw stack for argument parsing
	rawStack := debug.Stack()
	argMap := parseRawStack(rawStack)

	// Prepare Log Components

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

	// Get goroutine ID (Go doesn't expose this easily, we use a small trick if available or just omit for now)
	// Actually, for START block parity, we'll add a header line after SYSTEM INFO
	infoLines = append(infoLines, "")
	infoLines = append(infoLines, "\t"+"{{|TraceHeader|}}GOROUTINE INFO:{{[-]}}")
	infoLines = append(infoLines, "\t"+"  goroutine unknown [running]") // Simplified for now as Go stdlib hides this

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

		// Calculate instruction offset (+0x...)
		// PC is the program counter. frame.Entry is the start of the function.
		offset := frame.PC - frame.Entry
		offsetStr := fmt.Sprintf("+0x%x", offset)

		// Create format string dynamically to pad frame number
		// Format: "Num: [Indent]Arrow File:Line+Offset (Function)"
		fmtStr := fmt.Sprintf("%%s%%%dd{{[-]}}%%s %%s%%s%%s%%s:%%s%%d%%s{{[-]}} (%%s%%s{{[-]}})", width)

		line := fmt.Sprintf(
			fmtStr,
			"{{|TraceFrameNumber|}}", i,
			":",
			arrowIndent,
			"{{|TraceFrameLines|}}"+suffix+"{{[-]}}",
			"{{|TraceSourceFile|}}", frame.File,
			"{{|TraceLineNumber|}}", frame.Line,
			"{{|TraceOffset|}}"+offsetStr,
			"{{|TraceFunction|}}", filepath.Base(frame.Function),
		)

		traceLines = append(traceLines, "\t"+line)

		// ADD CALL LINE (│ style) to show what this frame CALLED
		// Mirrors Bash: if i > 0, it means this function called the one at i-1
		if i > 0 {
			nextFrame := allFrames[i-1]
			callPrefix := "{{|TraceFrameLines|}}│{{[-]}}"
			// Calculate indent for the call line (FrameNumber + 2 + arrowIndent length)
			callLineIndent := strings.Repeat(" ", width+2+len(indent))

			// Look up arguments for the function being CALLED (nextFrame)
			args := "(...)"
			fileLineKey := fmt.Sprintf("%s:%d", nextFrame.File, nextFrame.Line)
			if a, ok := argMap[fileLineKey]; ok {
				args = a
			}

			callLine := fmt.Sprintf("\t%s%s%s{{|TraceCmd|}}%s{{[-]}}%s",
				callLineIndent,
				callPrefix,
				" ",
				filepath.Base(nextFrame.Function),
				args,
			)
			traceLines = append(traceLines, callLine)
		}

		// Prepare for next frame
		indent += "  "
	}

	// Assemble Final Output
	// This provides a visual representation of the final log block structure
	output := []any{
		"{{|TraceHeader|}}### BEGIN SYSTEM INFORMATION AND STACK TRACE ###",
		infoLines,
		"", // Separator
		traceLines,
		"{{|TraceFooter|}}### END SYSTEM INFORMATION AND STACK TRACE ###",
		"",
		msg,
		"",
		"{{|FatalFooter|}}Please let the dev know of this error.",
		"{{|FatalFooter|}}It has been written to {{[-]}}'{{|File|}}" + filepath.Join(paths.GetStateDir(), strings.ToLower(version.ApplicationName)+".fatal.log") + "{{[-]}}'{{|FatalFooter|}},",
		"and appended to {{[-]}}'{{|File|}}" + filepath.Join(paths.GetStateDir(), strings.ToLower(version.ApplicationName)+".log") + "{{[-]}}'{{|FatalFooter|}}.",
	}

	// Log Everything
	logAt(ctx, now, LevelFatal, output, args...)

	// Write to fatal log file
	writeFatalLog(ctx, now, output, args...)

	// Brief sleep to allow stdout/stderr to flush before exit
	time.Sleep(100 * time.Millisecond)
	os.Exit(1)
}

// FatalNoTrace logs a message at FatalLevel without stack trace and exits
func Fatal(ctx context.Context, msg any, args ...any) {
	output := []any{
		msg,
		"",
		"{{|FatalFooter|}}Please let the dev know of this error.",
	}
	now := time.Now()
	logAt(ctx, now, LevelFatal, output, args...)

	// Write to fatal log file
	writeFatalLog(ctx, now, output, args...)

	// Brief sleep to allow stdout/stderr to flush before exit
	time.Sleep(100 * time.Millisecond)
	os.Exit(1)
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
		// Strip semantic style tags and ANSI codes from the fatal log file.
		fmt.Fprintln(f, console.Strip(line))
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

// parseRawStack parses debug.Stack() output to extract function arguments.
// It maps "file:line" to "(arg1, arg2, ...)"
func parseRawStack(raw []byte) map[string]string {
	argMap := make(map[string]string)
	lines := strings.Split(string(raw), "\n")

	for i := 0; i < len(lines)-1; i++ {
		line := strings.TrimSpace(lines[i])
		// Look for function call line: main.myFunc(0x1, 0x2)
		// Go stack traces always end the function line with arguments in parens
		if strings.Contains(line, "(") && strings.HasSuffix(line, ")") && strings.Contains(line, "(0x") {
			start := strings.LastIndex(line, "(")
			args := line[start:]

			// Next line MUST be the file:line info
			nextLine := strings.TrimSpace(lines[i+1])
			// It starts with a tab in raw output, but we TrimSpace'd it.
			// Format: /path/to/file.go:123 +0x456
			fileLine := nextLine
			if idx := strings.Index(nextLine, " +0x"); idx != -1 {
				fileLine = nextLine[:idx]
			}
			if fileLine != "" {
				argMap[fileLine] = args
			}
			i++ // Skip the fileLine we just processed
		}
	}
	return argMap
}
