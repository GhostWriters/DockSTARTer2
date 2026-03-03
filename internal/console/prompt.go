package console

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strings"

	"golang.org/x/term"
)

// ErrUserAborted is returned when the user aborts an action
var ErrUserAborted = errors.New("user aborted")

// AbortHandler is a function that can be registered to handle CTRL-C aborts globally.
var AbortHandler func(ctx context.Context)

// Printer is a function compatible with logger.Notice
type Printer func(ctx context.Context, msg any, args ...any)

// TUIConfirm is a function that can be registered by the tui package
// to allow QuestionPrompt to show a graphical dialog.
var TUIConfirm func(title, question string, defaultYes bool) bool

// TUIPrompt is a function that can be registered by the tui package
// to allow TextPrompt to show a graphical text input dialog.
var TUIPrompt func(title, question string, sensitive bool) (string, error)

// TUIShutdown is a function that can be registered by the tui package
// to allow the application to cleanly exit the TUI before re-execution.
var TUIShutdown func()

// GlobalYes is set to true when the -y/--yes flag is passed to the application.
// QuestionPrompt prompts the user with a Yes/No question.
// It returns true if the user answers Yes, false otherwise.
// defaultValue determines the default action if the user just presses Enter ("Y"=Yes, "N"=No, ""=Require Input).
// forceYes if true, immediately returns true without prompting (useful for -y flag).
func QuestionPrompt(ctx context.Context, printer Printer, title, question string, defaultValue string, forceYes bool) (bool, error) {
	if forceYes || GlobalYes {
		return true, nil
	}

	// Format text for semantic colors
	questionStr := Sprintf(question)
	if title == "" {
		title = "Confirmation"
	}
	title = Sprintf(title)

	// Check if we should use TUI for this prompt
	if TUIConfirm != nil {
		defaultYes := strings.EqualFold(defaultValue, "y")
		answer := TUIConfirm(title, questionStr, defaultYes)
		if answer {
			printer(ctx, "%s", Sprintf("Answered: {{|Yes|}}Yes{{[-]}}"))
		} else {
			printer(ctx, "%s", Sprintf("Answered: {{|No|}}No{{[-]}}"))
		}
		return answer, nil
	}

	// Prepare prompt string
	ynPrompt := "[YN]"
	if strings.EqualFold(defaultValue, "y") {
		ynPrompt = "[Yn]"
	} else if strings.EqualFold(defaultValue, "n") {
		ynPrompt = "[yN]"
	}

	// Print the question (parsing semantic colors)
	printer(ctx, "%s", questionStr)
	printer(ctx, "%s", ynPrompt)

	// Switch to raw mode to read a single character
	fd := int(os.Stdin.Fd())
	var oldState *term.State
	if term.IsTerminal(fd) {
		var err error
		oldState, err = term.MakeRaw(fd)
		if err == nil {
			defer term.Restore(fd, oldState)
		}
	}

	b := make([]byte, 1)
	answer := false
	answered := false

	for !answered {
		_, err := os.Stdin.Read(b)
		if err != nil {
			// If read fails, use default if available, else defalt to No (safe)
			if strings.EqualFold(defaultValue, "y") {
				answer = true
			} else {
				answer = false
			}
			answered = true
			break
		}

		input := string(b[0])

		// Handle Ctrl-C (0x03)
		if input == "\x03" {
			if oldState != nil {
				_ = term.Restore(fd, oldState)
			}
			if AbortHandler != nil {
				AbortHandler(ctx)
			}
			return false, ErrUserAborted
		}

		// Handle Enter key (CR or LF)
		if input == "\r" || input == "\n" {
			if strings.EqualFold(defaultValue, "y") {
				answer = true
				answered = true
				break
			} else if strings.EqualFold(defaultValue, "n") {
				answer = false
				answered = true
				break
			}
			// If no default (defaultValue == ""), ignore Enter
			continue
		}

		lower := strings.ToLower(input)
		if lower == "y" {
			answer = true
			answered = true
			break
		}
		if lower == "n" {
			answer = false
			answered = true
			break
		}
		// Ignore other keys
	}

	// Restore terminal before printing log messages
	if oldState != nil {
		_ = term.Restore(fd, oldState)
	}

	if answer {
		printer(ctx, "Answered: {{|Yes|}}Yes{{[-]}}")
	} else {
		printer(ctx, "Answered: {{|No|}}No{{[-]}}")
	}

	return answer, nil
}

// TextPrompt prompts the user for string input.
// If sensitive is true, it attempts to mask the input in standard terminal.
func TextPrompt(ctx context.Context, printer Printer, title, question string, sensitive bool) (string, error) {
	// Format text for semantic colors
	questionStr := Sprintf(question)
	if title == "" {
		title = "Input Required"
	}
	title = Sprintf(title)

	if TUIPrompt != nil {
		return TUIPrompt(title, questionStr, sensitive)
	}

	if title != "" {
		printer(ctx, "%s", title)
	}
	printer(ctx, "%s: ", questionStr)

	if sensitive {
		passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		printer(ctx, "") // Print a newline after reading password
		if err != nil {
			return "", err
		}
		return string(passwordBytes), nil
	}

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
