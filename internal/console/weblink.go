package console

import "github.com/GhostWriters/semstyle"

// FormatLink delegates to semstyle.FormatLink -- kept as a DS2-local wrapper
// only so existing call sites don't need to change to call the semstyle
// package directly.
func FormatLink(tag, label, url string) string {
	return semstyle.FormatLink(tag, label, url)
}
