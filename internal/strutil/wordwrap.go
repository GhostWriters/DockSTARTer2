package strutil

import "strings"

// WordWrapToSlice wraps text approximating GNU `fmt` line balancing (e.g., `fmt -w 75 -g 75`).
// It builds lines greedily, but if a line is close to the goal (within ~10 chars),
// it looks ahead at the remaining text. If the remaining text is short enough to fit
// on exactly ONE more line, it finds the break that minimizes the combined penalty.
// This perfectly achieves the 68/73 split for typical descriptions (e.g., RustDesk).
func WordWrapToSlice(text string, goal int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{}
	}

	var lines []string

	i := 0
	for i < len(words) {
		lineRestTotal := 0
		for j := i; j < len(words); j++ {
			lineRestTotal += len(words[j])
			if j > i {
				lineRestTotal += 1 // space
			}
		}

		if lineRestTotal <= goal {
			// It all fits on one line!
			lines = append(lines, strings.Join(words[i:], " "))
			break
		}

		bestBreak := i + 1
		bestCost := 999999

		currentLen := 0
		for breakIdx := i + 1; breakIdx <= len(words); breakIdx++ {
			if breakIdx > i + 1 {
				currentLen += 1 // space
			}
			currentLen += len(words[breakIdx-1])

			if currentLen > goal && breakIdx > i+1 {
				break // Cannot exceed goal (greedily). We check breakIdx > i+1 to ensure at least 1 word.
			}

			// Calculate the cost of this break
			cost := (goal - currentLen) * (goal - currentLen)

			// Lookahead for next line
			nextLen := 0
			for j := breakIdx; j < len(words); j++ {
				if j > breakIdx {
					nextLen += 1
				}
				nextLen += len(words[j])
			}

			if nextLen <= goal {
				// Remaining fits on one line! Factor it into the cost to balance them.
				cost += (goal - nextLen) * (goal - nextLen)
			}

			// Tie breaker: prefer longer lines if costs are similar (greedy preference)
			if cost < bestCost || (cost == bestCost && currentLen > len(strings.Join(words[i:bestBreak], " "))) {
				bestCost = cost
				bestBreak = breakIdx
			}
		}

		lines = append(lines, strings.Join(words[i:bestBreak], " "))
		i = bestBreak
	}

	return lines
}

// WordWrap is a convenience wrapper around WordWrapToSlice that returns the wrapped
// lines joined by newline characters (\n) as a single string.
func WordWrap(text string, goal int) string {
	lines := WordWrapToSlice(text, goal)
	return strings.Join(lines, "\n")
}
