package strutil

import "strings"

// WordWrapToSlice wraps text greedily by building lines up to the specified goal width.
// This perfectly replicates the behavior of Bash: `fold -s -w Width | strip_trailing_spaces`
// in DockSTARTer's `misc_functions.sh`.
// `fold -s` wraps AFTER the space. So a line with 74 chars + 1 space = 75 chars matches the width,
// and breaks. Then the bash script strips the trailing space, resulting in a 74-char line.
func WordWrapToSlice(text string, goal int) []string {
	if text == "" {
		return []string{}
	}

	var lines []string
	wordList := strings.Fields(text)

	if len(wordList) == 0 {
		return []string{}
	}

	currentLine := ""

	for _, word := range wordList {
		if currentLine == "" {
			currentLine = word
			continue
		}

		// Calculate the length IF we added a space and the word.
		// `fold -s` conceptually measures: len(currentLine) + len(space) + len(word).
		// If that exceeds the goal, the space becomes the break point.
		if len(currentLine)+1+len(word) > goal {
			// In `fold -s`, the space would be kept at the end of currentLine, 
			// and then stripped by `strip_trailing_spaces`. So we just push currentLine.
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			currentLine += " " + word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

// WordWrap is a convenience wrapper around WordWrapToSlice that returns the wrapped
// lines joined by newline characters (\n) as a single string.
func WordWrap(text string, goal int) string {
	lines := WordWrapToSlice(text, goal)
	return strings.Join(lines, "\n")
}
