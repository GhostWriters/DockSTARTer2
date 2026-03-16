// Package selection provides shared text-selection utilities for single-line and
// multi-line inputs: a multi-click tracker and a word-boundary helper.
package selection

import (
	"time"
	"unicode"
)

const MultiClickWindow = 400 * time.Millisecond

// ClickTracker detects multi-click sequences (double-click, triple-click, etc.)
// based on position proximity and timing.
type ClickTracker struct {
	lastTime time.Time
	lastCol  int
	Count    int
}

// Track records a click at the given logical column and returns the click count
// (1 = single, 2 = double, 3 = triple, …). Resets to 1 when the column changes
// or when the time since the last click exceeds MultiClickWindow.
func (t *ClickTracker) Track(col int) int {
	now := time.Now()
	if t.Count > 0 && col == t.lastCol && now.Sub(t.lastTime) <= MultiClickWindow {
		t.Count++
	} else {
		t.Count = 1
	}
	t.lastTime = now
	t.lastCol = col
	return t.Count
}

// Reset clears the click sequence.
func (t *ClickTracker) Reset() {
	t.Count = 0
}

// WordBoundsAt returns the [start, end) of the word at col in line, using
// standard whitespace boundaries. Returns (col, col) if col is on whitespace.
func WordBoundsAt(line []rune, col int) (start, end int) {
	n := len(line)
	if n == 0 || col >= n {
		return col, col
	}
	if unicode.IsSpace(line[col]) {
		return col, col
	}
	start = col
	for start > 0 && !unicode.IsSpace(line[start-1]) {
		start--
	}
	end = col
	for end < n && !unicode.IsSpace(line[end]) {
		end++
	}
	return start, end
}
