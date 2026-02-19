// Package strutil provides additional string manipulation functions.
package strutil

import "strings"

// Repeat returns a string consisting of count copies of s.
// Unlike strings.Repeat, it returns an empty string if count is negative.
func Repeat(s string, count int) string {
	if count <= 0 {
		return ""
	}
	return strings.Repeat(s, count)
}
