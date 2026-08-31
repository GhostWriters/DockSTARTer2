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
	isDefaultLabel := name == path
	if isFolder {
		path = ensureTrailingSlash(path)
	}
	url := strutil.FileURL(path)
	// ":::N:" (empty fg/bg, "N" flag) marks the tag as location-only for semstyle's
	// hyperlink_mode=auto -- only when name is just path displayed verbatim (the
	// FormatFile/FormatFolder default), not a genuinely different caller-supplied label
	// (FormatFileName/FormatFolderName): a distinct label doesn't reveal the destination
	// the way the bare path does, so auto still has something worth adding there. See
	// locationOnlyFlag.
	if isDefaultLabel {
		return "{{|" + tag + ":::N:" + url + "|}}" + name + "{{[-]}}"
	}
	return "{{|" + tag + "::::" + url + "|}}" + name + "{{[-]}}"
}

// FormatUserFolderPath returns raw semstyle markup for fullPath expressed
// as "user:<relative path>" instead of the raw absolute filesystem path --
// e.g. a user apps folder override at .../user/apps/media.d/plex displays
// as "user:media.d/plex", and baseDir itself displays as bare "user:".
// baseDir (e.g. paths.GetUserAppsDir(), paths.GetThemesDir()) is
// hyperlinked as just the word "user" (the ":" is styled but not part of
// the link, same as the "/" separators below); each remaining path segment
// gets its own hyperlink to its own real cumulative path, exactly like
// FormatFolderPath does for a plain absolute path -- so the whole
// "user:..." reads as one continuous clickable path, not just a label.
// Falls back to plain FormatFolderPath(fullPath) if fullPath isn't
// actually under baseDir.
func FormatUserFolderPath(baseDir, fullPath string) string {
	return formatUserPathSegments(baseDir, fullPath, false)
}

// FormatUserFilePath is FormatUserFolderPath's counterpart for referencing
// a file rather than a directory -- the final segment is tagged
// {{|File|}}, matching FormatFilePath.
func FormatUserFilePath(baseDir, fullPath string) string {
	return formatUserPathSegments(baseDir, fullPath, true)
}

func formatUserPathSegments(baseDir, fullPath string, lastIsFile bool) string {
	rel, err := filepath.Rel(baseDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		if lastIsFile {
			return FormatFilePath(fullPath)
		}
		return FormatFolderPath(fullPath)
	}
	blocked := blocksHyperlink()

	var b strings.Builder
	if blocked {
		b.WriteString("{{|Folder|}}user{{[-]}}")
	} else {
		b.WriteString("{{|Folder:::N:" + strutil.FileURL(ensureTrailingSlash(baseDir)) + "|}}user{{[-]}}")
	}
	// The ":" is styled to match but never wrapped in the hyperlink -- same
	// convention as the "/" separators below, only the segment itself
	// (here, the word "user") is clickable.
	b.WriteString("{{|Folder|}}:{{[-]}}")
	if rel == "." {
		return b.String()
	}

	segments := strings.Split(filepath.ToSlash(rel), "/")
	sep := string(filepath.Separator)
	lastIdx := -1
	for i, s := range segments {
		if s != "" {
			lastIdx = i
		}
	}
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		tag := "Folder"
		if lastIsFile && i == lastIdx {
			tag = "File"
		}
		if i > 0 {
			b.WriteString("{{|" + tag + "|}}" + sep + "{{[-]}}")
		}
		if blocked {
			b.WriteString("{{|" + tag + "|}}" + seg + "{{[-]}}")
		} else {
			cumulative := filepath.Join(baseDir, filepath.Join(segments[:i+1]...))
			if tag == "Folder" {
				cumulative = ensureTrailingSlash(cumulative)
			}
			b.WriteString("{{|" + tag + ":::N:" + strutil.FileURL(cumulative) + "|}}" + seg + "{{[-]}}")
		}
	}
	return b.String()
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
			b.WriteString("{{|" + tag + ":::N:" + strutil.FileURL(cumulative) + "|}}" + seg + "{{[-]}}")
		}
	}
	return b.String()
}
