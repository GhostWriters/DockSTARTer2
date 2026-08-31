package console

import (
	"sync/atomic"

	"github.com/GhostWriters/semstyle"
)

// viaOwnServer tracks whether the active session is connected through one of
// DS2's own servers (its SSH server or its web server), as opposed to a
// plain CLI invocation or a TUI/CLI invocation reached via some other means
// (e.g. a real external SSH shell). It defaults to false, which is correct
// for CLI code (CLI commands never run behind DS2's own servers) and for
// the TUI before Start has parsed its connection info. Only the TUI ever
// calls SetViaOwnServer; CLI code never needs to.
var viaOwnServer atomic.Bool

// IsViaOwnServer reports whether the active session is connected through
// DS2's own SSH or web server. Use this (not any general connType check) to
// decide whether a file:// hyperlink is worth emitting: DS2 only knows for
// certain that the rendering terminal/browser is on a different machine
// when it's serving the connection itself. A real external SSH shell
// running the CLI/TUI directly may render file:// links just fine, and DS2
// has no way to know either way, so it isn't treated as ineligible.
func IsViaOwnServer() bool {
	return viaOwnServer.Load()
}

// SetViaOwnServer updates the active session's DS2-own-server status. Called
// by the TUI once it has parsed its connection info; CLI code never calls
// this since it never runs behind DS2's own servers.
func SetViaOwnServer(v bool) {
	viaOwnServer.Store(v)
}

// RequiresRemoteSudoGate reports whether the active session should be
// treated as "remote" for System Console's per-command/enable-time sudo
// re-verification. Use this (not any general connType check) for that
// decision: the gate exists because DS2's own SSH/web server lets multiple
// DS2-authenticated users (distinct pubkeys/passwords, independent of OS
// accounts) share one daemon, so a session's DS2 auth doesn't prove OS sudo
// rights. A real external SSH shell running the CLI/TUI directly never went
// through that multi-tenant auth layer -- the user already needed real OS
// credentials to get that shell -- so it's exempt, the same as a local
// session, even though connType reports "ssh" for it (see parseClientInfo).
func RequiresRemoteSudoGate() bool {
	return viaOwnServer.Load()
}

// blocksHyperlink reports whether the active session should suppress
// file:// hyperlinks: true only when connected through DS2's own SSH or web
// server AND the client doesn't appear to be the same machine DS2 is
// running on (see isSameMachineClient) -- a client on the same machine can
// still resolve a file:// link even though the session went through DS2's
// own server (e.g. a browser at http://localhost:PORT). Wired into
// semstyle.HyperlinkEligibleFunc at init (see profile.go) -- this is the
// one piece of the old hyperlink-path logic that had to stay local, since
// only DS2 knows about its own SSH/web server and same-machine detection.
func blocksHyperlink() bool {
	return viaOwnServer.Load() && !isSameMachineClient()
}

// FormatFilePath, FormatFolderPath, FormatFileName, FormatFolderName,
// FormatFile, FormatFolder, FormatUserFolderPath, and FormatUserFilePath
// delegate to semstyle -- the path-segmenting/tag-building logic itself is
// app-agnostic and now lives there (see semstyle.HyperlinkEligibleFunc,
// wired in profile.go's init to blocksHyperlink above). Kept as DS2-local
// wrappers only so the many existing call sites across the codebase don't
// need to change to call the semstyle package directly.

func FormatFilePath(path string) string {
	return semstyle.FormatFilePath(path)
}

func FormatFolderPath(path string) string {
	return semstyle.FormatFolderPath(path)
}

func FormatFileName(name, path string) string {
	return semstyle.FormatFileName(name, path)
}

func FormatFolderName(name, path string) string {
	return semstyle.FormatFolderName(name, path)
}

func FormatFile(tag, path string, name ...string) string {
	return semstyle.FormatFile(tag, path, name...)
}

func FormatFolder(tag, path string, name ...string) string {
	return semstyle.FormatFolder(tag, path, name...)
}

func FormatUserFolderPath(baseDir, fullPath string) string {
	return semstyle.FormatUserFolderPath(baseDir, fullPath)
}

func FormatUserFilePath(baseDir, fullPath string) string {
	return semstyle.FormatUserFilePath(baseDir, fullPath)
}
