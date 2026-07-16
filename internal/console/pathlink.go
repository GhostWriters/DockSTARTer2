package console

import (
	"path/filepath"
	"strings"
	"sync/atomic"

	"DockSTARTer2/internal/strutil"
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

// blocksHyperlink reports whether the active session should suppress
// file:// hyperlinks: true only when connected through DS2's own SSH or web
// server AND the client doesn't appear to be the same machine DS2 is
// running on (see isSameMachineClient) -- a client on the same machine can
// still resolve a file:// link even though the session went through DS2's
// own server (e.g. a browser at http://localhost:PORT).
func blocksHyperlink() bool {
	return viaOwnServer.Load() && !isSameMachineClient()
}

// FormatFilePath returns raw (unresolved) semstyle tag markup for a file
// reference -- unquoted (add quotes yourself if the surrounding message
// needs them). Suitable for a logger.Notice/Info/etc. message string,
// resolved later by each destination's own renderer (console/file/TUI
// viewport). Each path segment (directory component, plus filename) gets
// its own hyperlink to that segment's own path, so a folder segment opens
// that folder without needing the final file; separators are colored to
// match but never wrapped in a hyperlink. The final segment is tagged
// {{|File|}}; earlier segments are {{|Folder|}} with a trailing "/" on
// their target URL (see ensureTrailingSlash) so they can only resolve to a
// directory, never execute a same-named file. Tags carry an explicit
// file:// URL unless the session is connected through DS2's own SSH/web
// server from a different machine (see blocksHyperlink), in which case DS2
// knows the file can't exist on the rendering machine and omits the URL.
//
// Must NOT call displayengine.HyperlinkPath/HyperlinkText -- those render
// immediately using the current rendering context (e.g. baking in TUI
// colors that should be stripped for a file-logged line) instead of
// deferring to whichever handler processes this message later.
func FormatFilePath(path string) string {
	return formatPathSegments(path, true)
}

// FormatFolderPath is FormatFilePath's counterpart for referencing a
// directory rather than a single file -- every segment, including the last,
// is tagged {{|Folder|}}.
func FormatFolderPath(path string) string {
	return formatPathSegments(path, false)
}

// FormatFileName returns raw (unresolved), unquoted semstyle tag markup for
// a display name (e.g. a short label like ".env" rather than the actual
// on-disk name) that should link to path. Pass an empty path if no real
// path is known: the name is still styled, just without a hyperlink, since
// linking to "" would otherwise point at the process's working/root
// directory. Call FormatFilePath directly when the full path is the thing
// to display.
func FormatFileName(name, path string) string {
	return FormatFile("File", path, name)
}

// FormatFolderName is FormatFileName's {{|Folder|}}-styled counterpart.
func FormatFolderName(name, path string) string {
	return FormatFolder("Folder", path, name)
}

// FormatFile returns raw (unresolved) semstyle tag markup for path, styled
// with tag (the caller's choice -- lets a path be hyperlinked under any
// semantic style, not just "File") and hyperlinked to path itself when the
// active session permits it. Displays path verbatim as the visible text
// unless a different label is given via the optional name -- most callers
// have nothing shorter to show than the real path, so this avoids having to
// pass path twice. Never forces a trailing slash, since path is understood
// to reference a single file, not a directory.
func FormatFile(tag, path string, name ...string) string {
	return formatPathTag(tag, pathLabel(path, name), path, false)
}

// FormatFolder is FormatFile's directory counterpart: same tag flexibility
// and optional-name default, but always forces a trailing slash on the
// hyperlink target (via ensureTrailingSlash) regardless of what tag is used,
// so it can only ever resolve to a directory -- this is keyed off the
// caller's explicit choice of FormatFile vs FormatFolder, not by
// string-matching tag, so it stays correct even when tag isn't literally
// "Folder".
func FormatFolder(tag, path string, name ...string) string {
	return formatPathTag(tag, pathLabel(path, name), path, true)
}

// pathLabel returns name[0] if given and non-empty, else falls back to path
// itself -- the default-argument pattern for FormatFile/FormatFolder's
// optional display label.
func pathLabel(path string, name []string) string {
	if len(name) > 0 && name[0] != "" {
		return name[0]
	}
	return path
}

func formatPathTag(tag, name, path string, isFolder bool) string {
	if path == "" || blocksHyperlink() {
		return "{{|" + tag + "|}}" + name + "{{[-]}}"
	}
	if isFolder {
		path = ensureTrailingSlash(path)
	}
	return "{{|" + tag + "::::" + strutil.FileURL(path) + "|}}" + name + "{{[-]}}"
}

// ensureTrailingSlash appends "/" if not already present. Used for folder
// targets: a trailing slash forces path resolution to require a directory
// (POSIX open/execve fail with ENOTDIR otherwise), which rules out a
// same-named executable being run instead of the folder being opened.
func ensureTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "\\") {
		return path
	}
	return path + "/"
}

func formatPathSegments(path string, lastIsFile bool) string {
	// DS2 runs natively on both Windows and Linux hosts, so path may use "\"
	// or "/" as its separator; normalize to "/" (via the stdlib, rather than
	// hand-rolling OS-separator detection) so segment-splitting is uniform.
	// The separator actually displayed uses the OS-native character (via
	// filepath.Separator) so a Windows path still reads with "\" rather than
	// switching to "/".
	segments := strings.Split(filepath.ToSlash(path), "/")
	blocked := blocksHyperlink()
	sep := string(filepath.Separator)

	lastIdx := -1
	for i, s := range segments {
		if s != "" {
			lastIdx = i
		}
	}

	var b strings.Builder
	for i, seg := range segments {
		tag := "Folder"
		if lastIsFile && i == lastIdx {
			tag = "File"
		}
		if i > 0 {
			// The separator is styled to match the segment it precedes so
			// the path reads as one continuous colored span, but it's never
			// wrapped in a hyperlink -- only whole segments are clickable.
			b.WriteString("{{|" + tag + "|}}" + sep + "{{[-]}}")
		}
		if seg == "" {
			continue
		}
		if blocked {
			b.WriteString("{{|" + tag + "|}}" + seg + "{{[-]}}")
		} else {
			cumulative := strings.Join(segments[:i+1], "/")
			if tag == "Folder" {
				cumulative = ensureTrailingSlash(cumulative)
			}
			b.WriteString("{{|" + tag + "::::" + strutil.FileURL(cumulative) + "|}}" + seg + "{{[-]}}")
		}
	}
	return b.String()
}
