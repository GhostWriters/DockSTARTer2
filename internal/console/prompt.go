package console

import (
	"context"
	"os"
	"strings"

	"golang.org/x/term"
)

// Printer is a function compatible with logger.Notice
type Printer func(ctx context.Context, msg string, args ...any)

// QuestionPrompt prompts the user with a Yes/No question.
// It returns true if the user answers Yes, false otherwise.
// defaultValue determines the default action if the user just presses Enter ("Y"=Yes, "N"=No, ""=Require Input).
// forceYes if true, immediately returns true without prompting (useful for -y flag).
func QuestionPrompt(ctx context.Context, printer Printer, question string, defaultValue string, forceYes bool) bool {
	if forceYes {
		return true
	}

	// Prepare prompt string
	ynPrompt := "[YN]"
	if strings.EqualFold(defaultValue, "y") {
		ynPrompt = "[Yn]"
	} else if strings.EqualFold(defaultValue, "n") {
		ynPrompt = "[yN]"
	}

	// Print the question (parsing semantic colors)
	printer(ctx, question)
	printer(ctx, ynPrompt)

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
		printer(ctx, "Answered: [_Yes_]Yes[-]")
	} else {
		printer(ctx, "Answered: [_No_]No[-]")
	}

	return answer
}
