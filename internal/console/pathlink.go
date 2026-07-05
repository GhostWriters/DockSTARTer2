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
// reference -- unquoted, since call sites are inconsistent about whether
// they quote a path in the surrounding message text; add quotes yourself if
// the original message had them. Suitable for building a
// logger.Notice/Info/etc. message string, which gets resolved later by each
// destination's own renderer (console/file/TUI viewport). Each path segment
// (each directory component, plus the filename) gets its own hyperlink to
// that segment's own path -- so a folder segment can be clicked to open that
// folder without needing to click the final file. Separators between
// segments are colored to match but never wrapped in a hyperlink, so only
// whole segments are clickable. The final segment is tagged {{|File|}};
// every segment before it is tagged {{|Folder|}}, and its target URL gets a
// trailing "/" (see ensureTrailingSlash) so it can only ever resolve to a
// directory, never execute a same-named file. Unless the active session
// is connected through DS2's own SSH or web server from a different machine
// (see blocksHyperlink), each tag carries an explicit file:// URL for its
// own cumulative path so it renders as a clickable hyperlink wherever it's
// eventually shown; otherwise the tags carry no URL, since DS2 knows for
// certain the file only exists on a machine other than the one rendering
// the output.
//
// This must NOT call displayengine.HyperlinkPath/HyperlinkText -- those
// render immediately using the CURRENT rendering context, which would bake
// in the wrong styling (e.g. TUI colors leaking into a file-logged line that
// should have all tags stripped) instead of deferring to whichever handler
// processes this message later.
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
// a display name (e.g. a conceptual/short label like ".env" rather than the
// actual on-disk name) that should link to path -- for cases where the
// message wants to show a friendlier or more concise label than the real
// full path, but a real path is still known and worth linking to (e.g. a
// temp file standing in for ".env" during a write). If no real path is
// available at all, pass an empty path: the name is still styled, just
// without a hyperlink -- linking to "" would otherwise point at a
// nonexistent path in the process's working/root directory. When a real
// full path IS the thing to display, call FormatFilePath directly instead.
func FormatFileName(name, path string) string {
	return formatNamedTag("File", name, path)
}

// FormatFolderName is FormatFileName's {{|Folder|}}-styled counterpart.
func FormatFolderName(name, path string) string {
	return formatNamedTag("Folder", name, path)
}

func formatNamedTag(tag, name, path string) string {
	if path == "" || blocksHyperlink() {
		return "{{|" + tag + "|}}" + name + "{{[-]}}"
	}
	if tag == "Folder" {
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
