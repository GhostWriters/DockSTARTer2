package appenv

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/system"
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// walkUserAppFolders calls fn(baseAppNameUpper, folderPath) for every leaf
// app-template folder under paths.GetUserAppsDir(). A folder is a leaf iff
// it contains a "<lowercase-dirname>.yml" file (the compose snippet every
// real app template has). A folder is organizational (recursed into, not a
// leaf) only if it isn't a leaf and its name ends in ".d" -- otherwise
// there'd be no way to tell "a folder of apps" from "an app folder missing
// its .yml" just by looking at it, so anything that's neither is skipped
// entirely rather than guessed at.
//
// A dot-prefixed folder (leaf or organizational, e.g. ".TEMPLATE",
// ".hiddenapp", ".media.d") is filtered out up front, before either check
// runs -- prepending "." disables an app or a whole organizational group
// without deleting it.
//
// Each directory level is processed in two phases: every bare leaf folder
// first (alphabetically), then every ".d" folder (alphabetically),
// recursing into each with the same two-phase rule -- so a bare app folder
// always wins a name collision against anything sitting inside a ".d"
// folder, regardless of alphabetical position between the two. Numbering a
// ".d" folder (e.g. "01.mygroup.d") only affects its priority relative to
// other ".d" folders at that same level, not against bare folders.
//
// Every check before the leaf check itself is a plain string comparison on
// the entry's name -- dot-prefix, ".d" suffix, and IsAppNameValid (a dot
// anywhere else in the name, or any other disallowed character, fails it
// too) -- so the os.Stat call that actually confirms a folder is a leaf
// only ever runs for a name that could plausibly be a real app to begin
// with, not for every directory entry.
func walkUserAppFolders(fn func(name, dir string) error) error {
	return scanUserAppFolderLevel(paths.GetUserAppsDir(), fn)
}

// scanUserAppFolderLevel implements one level of walkUserAppFolders' two-phase
// scan; see its doc comment for the ordering rules.
func scanUserAppFolderLevel(dir string, fn func(name, path string) error) error {
	entries, err := os.ReadDir(dir) // sorted by filename
	if err != nil {
		return nil
	}

	var orgDirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue // disabled app or organizational group
		}
		if strings.HasSuffix(name, ".d") {
			orgDirs = append(orgDirs, filepath.Join(dir, name))
			continue
		}
		upperName := strings.ToUpper(name)
		if !IsAppNameValid(upperName) {
			continue // can't be a real app name -- skip without touching disk
		}
		path := filepath.Join(dir, name)
		if _, statErr := os.Stat(filepath.Join(path, strings.ToLower(name)+".yml")); statErr == nil {
			if err := fn(upperName, path); err != nil {
				return err
			}
		}
	}

	for _, path := range orgDirs {
		if err := scanUserAppFolderLevel(path, fn); err != nil {
			return err
		}
	}
	return nil
}

// WithDotDSuffix appends ".d" to each path segment of rel that doesn't
// already have it, so a caller-supplied organizational path (e.g.
// "media/streaming") becomes "media.d/streaming.d" -- matching the
// convention walkUserAppFolders enforces for recognizing organizational
// folders. A dot-prefixed segment (e.g. ".hidden") still gets the suffix
// appended (-> ".hidden.d"), and walkUserAppFolders treats the result as a
// disabled organizational group (dot-prefix wins over the ".d" suffix
// check there) -- so a dot-prefixed destination stays hidden regardless.
// rel must be slash-separated; "" (root, no subfolder) passes through
// unchanged.
func WithDotDSuffix(rel string) string {
	if rel == "" {
		return ""
	}
	segments := strings.Split(filepath.ToSlash(rel), "/")
	for i, seg := range segments {
		if seg != "" && !strings.HasSuffix(seg, ".d") {
			segments[i] = seg + ".d"
		}
	}
	return filepath.Join(segments...)
}

// resolveAppTemplateFolder returns the template folder for a base app name,
// preferring a user-supplied override under paths.GetUserAppsDir() over the
// bundled DockSTARTer-Templates repo copy when both exist (isUser reports
// which one). Falls back to the repo path unconditionally when no user
// folder matches (callers do their own os.Stat to determine whether that
// fallback actually exists). No logging here -- called on every
// IsAppBuiltIn check, far too hot a path for that; AppInstanceFile logs
// when it actually reads a resolved user override.
func resolveAppTemplateFolder(base string) (dir string, isUser bool) {
	if found, ok := findUserAppFolder(base); ok {
		return found, true
	}
	return filepath.Join(paths.GetTemplatesDir(), constants.TemplatesDirName, strings.ToLower(base)), false
}

// IsUserTemplate reports whether appName currently resolves via a user app
// template override, as opposed to the bundled DockSTARTer-Templates repo
// copy or not being a recognized app template at all. Unlike IsAppBuiltIn
// (which answers "is there a template folder at all, from either source")
// this distinguishes the source -- e.g. for UI styling that wants to mark
// a user-overridden app differently from a genuine repo-bundled one.
// findUserAppFolder only reports a match after confirming the folder
// exists, so no separate os.Stat is needed here the way IsRepoTemplate
// needs one for its own (unconditional) fallback path.
func IsUserTemplate(appName string) bool {
	_, isUser := resolveAppTemplateFolder(strings.ToLower(AppNameToBaseAppName(appName)))
	return isUser
}

// IsRepoTemplate reports whether appName currently resolves via the
// bundled DockSTARTer-Templates repo copy (not a user override). See
// IsUserTemplate; together IsUserTemplate(appName) || IsRepoTemplate(appName)
// is equivalent to IsAppBuiltIn(appName).
func IsRepoTemplate(appName string) bool {
	dir, isUser := resolveAppTemplateFolder(strings.ToLower(AppNameToBaseAppName(appName)))
	if isUser {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// TemplateFolder returns the resolved template folder for appName (base or
// instance-qualified, e.g. "radarr" or "radarr__4k" -- instances don't have
// their own template, so this always resolves against the base app): a
// user app template override if one exists, otherwise the bundled
// DockSTARTer-Templates repo copy. The single entry point every call site
// that just wants "the folder", not the user/repo distinction itself,
// should use, rather than hardcoding the repo path directly or calling
// resolveAppTemplateFolder and discarding its bool.
func TemplateFolder(appName string) string {
	base := strings.ToLower(AppNameToBaseAppName(appName))
	dir, _ := resolveAppTemplateFolder(base)
	return dir
}

// TemplateAppVarFileSuffixes returns an AppInstanceFile-style fileSuffix
// pattern (base app name replaced with "*", matching TemplateFile's own
// convention) for every .env.app.* file baseApp's template folder defines --
// the plain file plus, for a multi-service app, any per-service
// (".env.app.<base>___service") or shared/virtual (".env.app.<base>-suffix")
// files alongside it. Passing each pattern through AppInstanceFile resolves
// and instance-substitutes it the same way the single plain-file case
// already worked, since TemplateFile only ever substitutes "*" with the
// base app name, never the instance -- instance qualification happens via
// AppInstanceFile's own instanceFile naming, unaffected by the service
// suffix riding along after the "*".
func TemplateAppVarFileSuffixes(baseApp string) []string {
	templateFolder := TemplateFolder(baseApp)
	entries, err := os.ReadDir(templateFolder)
	if err != nil {
		return nil
	}

	baseLower := strings.ToLower(baseApp)
	baseUpper := strings.ToUpper(baseApp)
	var patterns []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), constants.AppEnvFileNamePrefix) {
			continue
		}
		nameSuffix := strings.TrimPrefix(entry.Name(), constants.AppEnvFileNamePrefix)
		if stripServiceSuffix(strings.ToUpper(nameSuffix)) != baseUpper {
			continue
		}
		pattern := constants.AppEnvFileNamePrefix + strings.Replace(nameSuffix, baseLower, "*", 1)
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	return patterns
}

// TemplateFile returns the resolved path to a specific file within
// appName's template folder, with "*" in fileSuffix replaced by the base
// app name -- e.g. TemplateFile("plex", "*.yml") -> ".../plex/plex.yml"
// -- matching AppInstanceFile's own fileSuffix convention. Pass a literal
// filename (no "*") for a fixed name like "README.md".
func TemplateFile(appName, fileSuffix string) string {
	base := strings.ToLower(AppNameToBaseAppName(appName))
	return filepath.Join(TemplateFolder(appName), strings.ReplaceAll(fileSuffix, "*", base))
}

// findUserAppFolder looks for a specific, already-known app name under
// paths.GetUserAppsDir(), applying the same two-phase precedence as
// walkUserAppFolders (a bare folder always wins over anything inside a
// ".d" folder; among competing ".d" folders, alphabetically-first wins) --
// but since the target name is known up front, unlike walkUserAppFolders
// (used only for a real listing, where every name is unknown) this doesn't
// enumerate or validate every directory entry. It just probes directly for
// "<dir>/<lowerBase>" at each level ("does this folder exist?"), only
// falling back to os.ReadDir to discover ".d" folders to check next when
// that direct probe misses.
//
// Existence of the folder itself is the signal, not its "<lowerBase>.yml"
// file specifically -- an override is per-folder, all-or-nothing (see
// HandleAppTemplateNew), so a folder claimed for this app but still
// missing files (e.g. a draft in progress) is still *the* location for it,
// not "not found." AppInstanceFile already handles an individual missing
// file gracefully, and IsAppRunnable checks for the compose file
// specifically where that distinction actually matters.
func findUserAppFolder(base string) (string, bool) {
	return findUserAppFolderIn(paths.GetUserAppsDir(), strings.ToLower(base))
}

func findUserAppFolderIn(dir, lowerBase string) (string, bool) {
	candidate := filepath.Join(dir, lowerBase)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, true
	}

	entries, err := os.ReadDir(dir) // sorted by filename
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".d") {
			continue
		}
		if found, ok := findUserAppFolderIn(filepath.Join(dir, name), lowerBase); ok {
			return found, true
		}
	}
	return "", false
}

// SyncUserAppTemplateReference keeps the user apps folder's .TEMPLATE
// reference copy in sync with the templates repo's own .apps/.TEMPLATE --
// writing/updating only files that are missing or stale (byte comparison),
// never touching a file the user has renamed or moved out of .TEMPLATE.
// A no-op if the templates repo hasn't been cloned yet. Unlike the embedded
// theme template, this source only exists once EnsureTemplates has run, so
// callers must re-invoke this after every clone/update, not just once at
// startup.
func SyncUserAppTemplateReference(ctx context.Context) {
	srcRoot := filepath.Join(paths.GetTemplatesDir(), constants.TemplatesDirName, ".TEMPLATE")
	if _, err := os.Stat(srcRoot); err != nil {
		return
	}
	dstRoot := filepath.Join(paths.GetUserAppsDir(), ".TEMPLATE")

	wroteAny := false
	_ = filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(srcRoot, path)
		if relErr != nil {
			return nil
		}
		dst := filepath.Join(dstRoot, rel)
		srcData, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		existing, existErr := os.ReadFile(dst)
		if existErr == nil && bytes.Equal(existing, srcData) {
			return nil
		}
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0700); mkErr != nil {
			return nil
		}
		if writeErr := os.WriteFile(dst, srcData, 0600); writeErr == nil {
			system.SetPermissions(ctx, dst)
			wroteAny = true
		}
		return nil
	})

	if wroteAny {
		logger.Notice(ctx, "App template reference updated in '"+console.FormatFolderPath(dstRoot)+"'.")
	}
}
