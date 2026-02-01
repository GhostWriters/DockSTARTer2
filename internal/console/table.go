package console

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// PrintTable prints a table with the given headers and data.
// data should be a flat list of strings, length must be a multiple of len(headers).
// useLineChars determines if Unicode box drawing characters are used.
func PrintTable(headers []string, data []string, useLineChars bool) {
	cols := len(headers)
	if cols == 0 {
		return
	}

	// 1. Calculate Column Widths
	colWidths := make([]int, cols)

	// Check headers
	for i, h := range headers {
		l := utf8.RuneCountInString(Strip(h))
		if l > colWidths[i] {
			colWidths[i] = l
		}
	}

	// Check data
	for i, d := range data {
		col := i % cols
		l := utf8.RuneCountInString(Strip(d))
		if l > colWidths[col] {
			colWidths[col] = l
		}
	}

	// 2. Define Character Set
	var charSet map[string]string
	if useLineChars {
		charSet = map[string]string{
			"TopLeft":     "┌",
			"TopRight":    "┐",
			"BottomLeft":  "└",
			"BottomRight": "┘",
			"Horizontal":  "─",
			"Vertical":    "│",
			"Cross":       "┼",
			"TLeft":       "├",
			"TRight":      "┤",
			"TTop":        "┬",
			"TBottom":     "┴",
		}
	} else {
		charSet = map[string]string{
			"TopLeft":     "+",
			"TopRight":    "+",
			"BottomLeft":  "+",
			"BottomRight": "+",
			"Horizontal":  "-",
			"Vertical":    "|",
			"Cross":       "+",
			"TLeft":       "|",
			"TRight":      "|",
			"TTop":        "-",
			"TBottom":     "-",
		}
	}

	// 3. Construct Borders
	var topBorder, middleBorder, bottomBorder strings.Builder

	topBorder.WriteString(charSet["TopLeft"])
	middleBorder.WriteString(charSet["TLeft"])
	bottomBorder.WriteString(charSet["BottomLeft"])

	for i := 0; i < cols; i++ {
		width := colWidths[i]
		// Padding is +2 (one space on each side)
		dashCount := width + 2
		dashes := strings.Repeat(charSet["Horizontal"], dashCount)

		topBorder.WriteString(dashes)
		middleBorder.WriteString(dashes)
		bottomBorder.WriteString(dashes)

		if i < cols-1 {
			topBorder.WriteString(charSet["TTop"])
			middleBorder.WriteString(charSet["Cross"])
			bottomBorder.WriteString(charSet["TBottom"])
		} else {
			topBorder.WriteString(charSet["TopRight"])
			middleBorder.WriteString(charSet["TRight"])
			bottomBorder.WriteString(charSet["BottomRight"])
		}
	}

	// 4. Print Table

	// Top Border
	// We use ToANSI (Parse) directly here since border chars might be colored if we supported it,
	// but currently they are plain. However, for consistency we can print them directly if they don't have tags.
	// But `ToANSI` is safe.
	fmt.Println(ToANSI(topBorder.String()))

	// Define helper for row printing
	printRow := func(rowItems []string) {
		var rowBuilder strings.Builder
		rowBuilder.WriteString(charSet["Vertical"])
		for i, item := range rowItems {
			visibleLen := utf8.RuneCountInString(Strip(item))
			padding := colWidths[i] - visibleLen
			padStr := strings.Repeat(" ", padding)

			// Format: " item   |"
			rowBuilder.WriteString(" ")
			rowBuilder.WriteString(item)
			rowBuilder.WriteString(padStr)
			rowBuilder.WriteString(" ")
			rowBuilder.WriteString(charSet["Vertical"])
		}
		// Here `item` might contain tags, so we definitely want ToANSI for the final string
		fmt.Println(ToANSI(rowBuilder.String()))
	}

	// Headers
	printRow(headers)

	// Middle Border
	fmt.Println(ToANSI(middleBorder.String()))

	// Data
	for i := 0; i < len(data); i += cols {
		end := i + cols
		if end > len(data) {
			end = len(data)
		}
		// If incomplete row (shouldn't happen if caller respects valid data len), fill with empty
		rowSlice := data[i:end]
		if len(rowSlice) < cols {
			filled := make([]string, cols)
			copy(filled, rowSlice)
			rowSlice = filled
		}
		printRow(rowSlice)
	}

	// Bottom Border
	fmt.Println(ToANSI(bottomBorder.String()))
}
