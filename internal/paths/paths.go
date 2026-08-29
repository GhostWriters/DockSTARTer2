package paths

import (
	// The blank boot import is deliberate and load-bearing: boot's init()
	// drops sudo privileges (and corrects HOME/XDG) BEFORE this package --
	// and therefore before every filesystem-touching package that imports
	// it -- can compute a path or write anything from root's environment.
	_ "DockSTARTer2/internal/boot"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/version"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"sync"
	"time"

	"github.com/adrg/xdg"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var (
	// StateHomeOverride allows overriding the state home for tests.
	StateHomeOverride string
	// TemplatesDirOverride allows overriding the templates directory for tests.
	TemplatesDirOverride string
	// ConfigHomeOverride allows overriding the config home for tests.
	ConfigHomeOverride string
	// CacheDirOverride allows overriding the cache directory for tests.
	CacheDirOverride string

	// Version caching
	versionCacheMu sync.RWMutex
	lastTmplVer    string
	lastTmplCheck  time.Time

	appTimestampCacheMu sync.Mutex
)

// appTimestampCacheDir returns the folder holding one touch file per
// (app, template|vars) pair -- same convention as appenv's env_update
// markers (see internal/appenv/update.go's updateFileChanged): the file's
// own mtime *is* the cached value (set via os.Chtimes), so a cache hit is
// just an os.Stat, no read/parse needed. A sibling "_head_hash" file (whose
// *content*, not mtime, is the templates repo's HEAD hash) pins the whole
// folder to the repo state it was computed against; a mismatch means the
// templates repo moved (e.g. `ds2 -u`) and every touch file here is stale.
func appTimestampCacheDir() string {
	return filepath.Join(GetTimestampsDir(), "app_templates")
}

// getCachedAppTimestamps returns the cached timestamps for appKey if the
// on-disk cache is still current for headHash. Also handles invalidating a
// stale cache (wiping the folder) so callers don't need to.
func getCachedAppTimestamps(headHash, appKey string) (templateUpdated, varsUpdated time.Time, ok bool) {
	appTimestampCacheMu.Lock()
	defer appTimestampCacheMu.Unlock()

	dir := appTimestampCacheDir()
	hashFile := filepath.Join(dir, "_head_hash")
	storedHash, err := os.ReadFile(hashFile)
	if err != nil || string(storedHash) != headHash {
		// Stale or never-populated -- wipe so old entries can't leak
		// through once a new one is written for the same appKey.
		_ = os.RemoveAll(dir)
		if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
			return time.Time{}, time.Time{}, false
		}
		_ = os.WriteFile(hashFile, []byte(headHash), 0644)
		return time.Time{}, time.Time{}, false
	}

	templateInfo, tErr := os.Stat(filepath.Join(dir, appKey+".template"))
	varsInfo, vErr := os.Stat(filepath.Join(dir, appKey+".vars"))
	if tErr != nil && vErr != nil {
		return time.Time{}, time.Time{}, false
	}
	if tErr == nil {
		templateUpdated = templateInfo.ModTime()
	}
	if vErr == nil {
		varsUpdated = varsInfo.ModTime()
	}
	return templateUpdated, varsUpdated, true
}

// setCachedAppTimestamps writes a touch file per non-zero timestamp,
// encoding the value as the file's mtime. Assumes the cache folder (and its
// _head_hash marker) is already current -- getCachedAppTimestamps always
// runs first and creates/wipes it as needed.
func setCachedAppTimestamps(appKey string, templateUpdated, varsUpdated time.Time) {
	appTimestampCacheMu.Lock()
	defer appTimestampCacheMu.Unlock()

	dir := appTimestampCacheDir()
	touch := func(name string, when time.Time) {
		if when.IsZero() {
			return
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0644); err != nil {
			return
		}
		_ = os.Chtimes(path, when, when)
	}
	touch(appKey+".template", templateUpdated)
	touch(appKey+".vars", varsUpdated)
}

// GetConfigFilePath returns the absolute path to the dockstarter2.toml file.
// It places it in a subdirectory named after the application (e.g., ~/.config/dockstarter2/dockstarter2.toml).
func GetConfigFilePath() string {
	appName := strings.ToLower(version.ApplicationName)
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", appName, constants.AppConfigFileName)
	}
	if ConfigHomeOverride != "" {
		return filepath.Join(ConfigHomeOverride, appName, constants.AppConfigFileName)
	}
	return filepath.Join(xdg.ConfigHome, appName, constants.AppConfigFileName)
}

// GetTemplatesDir returns the absolute path to the DockSTARTer-Templates repository.
// It uses xdg.StateHome (e.g., %LOCALAPPDATA% on Windows) with a dockstarter subfolder (SHARED WITH BASH).
func GetTemplatesDir() string {
	if TemplatesDirOverride != "" {
		return TemplatesDirOverride
	}
	appName := constants.LegacyApplicationName
	return filepath.Join(xdg.StateHome, appName, "templates", "DockSTARTer-Templates")
}

// GetTemplatesEnvFile returns the absolute path to the global .env template
// at the DockSTARTer-Templates repo root -- fetched/cached the same way as
// every per-app template file, so DS1 and DS2 read the exact same file.
func GetTemplatesEnvFile() string {
	return filepath.Join(GetTemplatesDir(), ".env")
}

// GetLegacyStateDir returns the state folder for the legacy DockSTARTer
// install -- the parent of GetTemplatesDir(), which is shared with that
// install, so it shouldn't be removed while it's still present.
func GetLegacyStateDir() string {
	return filepath.Join(xdg.StateHome, constants.LegacyApplicationName)
}

// GetLegacyConfigDir returns the config folder for the legacy DockSTARTer
// install (holding dockstarter.toml) -- never touched by DS2, only
// referenced for display when reporting what's being left alone.
func GetLegacyConfigDir() string {
	return filepath.Join(xdg.ConfigHome, constants.LegacyApplicationName)
}

// InvalidateTemplatesVersionCache forces the next GetTemplatesVersion call to
// re-read the repository instead of returning a cached value up to 60
// seconds stale. Callers that just changed the templates repo's HEAD (e.g.
// after applying an update) must call this, or the immediately-following
// "updated to X" display can show the pre-update version.
func InvalidateTemplatesVersionCache() {
	versionCacheMu.Lock()
	lastTmplCheck = time.Time{}
	versionCacheMu.Unlock()
}

// GetTemplatesVersion retrieves the current version of the DockSTARTer-Templates repository.
func GetTemplatesVersion() string {
	versionCacheMu.RLock()
	if time.Since(lastTmplCheck) < 60*time.Second {
		v := lastTmplVer
		versionCacheMu.RUnlock()
		return v
	}
	versionCacheMu.RUnlock()

	templatesDir := GetTemplatesDir()

	// Open repository
	r, err := git.PlainOpen(templatesDir)
	if err != nil {
		return "Unknown Version"
	}

	// Get HEAD
	head, err := r.Head()
	if err != nil {
		return "Unknown Version"
	}

	// Get Tag (if any)
	// Iterate valid tags and check if any point to HEAD
	tags, err := r.Tags()
	foundTag := ""
	if err == nil {
		_ = tags.ForEach(func(ref *plumbing.Reference) error {
			if ref.Hash() == head.Hash() {
				// Found a tag for this commit. Use strict short name (e.g. v1.0.0)
				foundTag = ref.Name().Short()
				return fmt.Errorf("found") // Stop iteration
			}
			return nil
		})
	}

	var result string
	if foundTag != "" {
		result = foundTag
	} else {
		// 3. Fallback to format: "BranchName commit shortHash"
		branchName := "HEAD"
		if head.Name().IsBranch() {
			branchName = head.Name().Short()
		}

		// Short hash
		hash := head.Hash().String()
		if len(hash) > 7 {
			hash = hash[:7]
		}
		result = fmt.Sprintf("%s commit %s", branchName, hash)
	}

	versionCacheMu.Lock()
	lastTmplVer = result
	lastTmplCheck = time.Now()
	versionCacheMu.Unlock()

	return result
}

// GetAppTemplateTimestamps returns the most recent commit time (in local
// time) touching anything under an app's templates folder
// (.apps/<baseAppName>/ -- compose.yml, override snippets, icons,
// .migrate files, var files), and separately the most recent commit time
// touching just its var files (.env / .env.app.*). Either return value is
// the zero Time if the templates dir isn't a git checkout, has no matching
// history, or baseAppName has no template folder.
//
// This deliberately avoids go-git's Log(PathFilter:...): that computes a
// full tree diff/patch per commit, which is documented as extremely slow on
// real-sized histories (go-git#137 -- ~30s for one file on a ~3000-commit
// repo; confirmed here too, timed out past 2 minutes on this repo's 3090
// commits). Shelling out to `git` or linking libgit2/gitoxide via cgo would
// both fix the speed but reintroduce an external dependency DS2 doesn't
// otherwise have (it clones via go-git's pure-Go PlainClone, no git binary
// needed). Instead, walk first-parent history ourselves comparing tree
// object hashes directly -- a directory tree's hash already changes if
// anything under it changed, so this needs one cheap hash comparison per
// commit instead of a full diff, and first-parent-only matches this repo's
// GitHub-merge-commit shape (each PR's merge commit's tree already reflects
// whatever that PR changed, compared against mainline before the merge).
func GetAppTemplateTimestamps(baseAppName string) (templateUpdated, varsUpdated time.Time) {
	r, err := git.PlainOpen(GetTemplatesDir())
	if err != nil {
		return
	}
	head, err := r.Head()
	if err != nil {
		return
	}
	headHash := head.Hash().String()
	appKey := strings.ToLower(baseAppName)

	if tmpl, vars, ok := getCachedAppTimestamps(headHash, appKey); ok {
		return tmpl, vars
	}
	defer func() {
		setCachedAppTimestamps(appKey, templateUpdated, varsUpdated)
	}()

	headCommit, err := r.CommitObject(head.Hash())
	if err != nil {
		return
	}

	appDirPath := constants.TemplatesDirName + "/" + appKey

	var foundTemplate, foundVars bool
	commit := headCommit
	prevSnap := appTemplateSnapshot(r, commit, appDirPath)
	if !prevSnap.exists {
		// App has no template folder at HEAD -- nothing to report.
		return
	}
	for {
		var parent *object.Commit
		if commit.NumParents() > 0 {
			parent, err = commit.Parent(0)
		}
		var curSnap appDirSnapshot
		if parent != nil && err == nil {
			curSnap = appTemplateSnapshot(r, parent, appDirPath)
		}
		// curSnap zero value (exists=false) correctly signals "changed" if
		// there's no parent (root commit) or the folder didn't exist yet.
		if !foundTemplate && curSnap.wholeHash != prevSnap.wholeHash {
			templateUpdated = commit.Author.When.Local()
			foundTemplate = true
		}
		if !foundVars && curSnap.varsSig != prevSnap.varsSig {
			varsUpdated = commit.Author.When.Local()
			foundVars = true
		}
		if foundTemplate && foundVars {
			return
		}
		if parent == nil {
			return
		}
		commit, prevSnap = parent, curSnap
	}
}

// appDirSnapshot captures an app's template folder contents at one commit,
// cheaply enough to compare across commits without diffing file contents.
type appDirSnapshot struct {
	exists    bool
	wholeHash plumbing.Hash // the subtree's own hash -- changes if anything under it changes
	varsSig   string        // "name:hash " for just the var-bearing entries, sorted by name
}

// appTemplateSnapshot resolves appDirPath (e.g. ".apps/audiobookshelf") within
// commit's tree. Returns the zero value (exists=false) if the path doesn't
// exist at this commit.
func appTemplateSnapshot(r *git.Repository, commit *object.Commit, appDirPath string) appDirSnapshot {
	tree, err := commit.Tree()
	if err != nil {
		return appDirSnapshot{}
	}
	entry, err := tree.FindEntry(appDirPath)
	if err != nil {
		return appDirSnapshot{}
	}
	subtree, err := r.TreeObject(entry.Hash)
	if err != nil {
		// Path exists but isn't resolvable as a tree -- treat as opaque,
		// still comparable via its own hash for the "whole" signature.
		return appDirSnapshot{exists: true, wholeHash: entry.Hash}
	}

	var names []string
	hashByName := make(map[string]plumbing.Hash, len(subtree.Entries))
	for _, e := range subtree.Entries {
		if e.Name == constants.EnvFileName || strings.HasPrefix(e.Name, constants.AppEnvFileNamePrefix) {
			names = append(names, e.Name)
			hashByName[e.Name] = e.Hash
		}
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, n := range names {
		sb.WriteString(n)
		sb.WriteByte(':')
		sb.WriteString(hashByName[n].String())
		sb.WriteByte(' ')
	}

	return appDirSnapshot{exists: true, wholeHash: entry.Hash, varsSig: sb.String()}
}

// GetCacheDir returns the absolute path to the dockstarter2 cache directory.
func GetCacheDir() string {
	if CacheDirOverride != "" {
		return CacheDirOverride
	}
	appName := strings.ToLower(version.ApplicationName)
	return filepath.Join(xdg.CacheHome, appName)
}

// GetConfigDir returns the absolute path to the dockstarter2 configuration directory.
func GetConfigDir() string {
	return filepath.Dir(GetConfigFilePath())
}

// GetThemesDir returns the absolute path to the user themes directory,
// under the user-content folder (see constants.UserDirName) alongside
// other user-supplied content (see GetUserAppsDir).
func GetThemesDir() string {
	return filepath.Join(GetConfigDir(), constants.UserDirName, constants.ThemesDirName)
}

// GetUserAppsDir returns the absolute path to the user app templates
// directory, under the user-content folder alongside GetThemesDir. Lets a
// user add an app template DS2 doesn't ship, or locally override a
// built-in one, without editing the cloned DockSTARTer-Templates repo
// directly (see internal/appenv/user_templates.go).
func GetUserAppsDir() string {
	return filepath.Join(GetConfigDir(), constants.UserDirName, constants.AppsDirName)
}

// GetLegacyThemesDir returns the pre-user-folder location of the themes
// directory (directly under the config dir, no "user" subfolder) -- kept
// only so callers can detect and migrate an install that predates
// GetThemesDir's move into constants.UserDirName.
func GetLegacyThemesDir() string {
	return filepath.Join(GetConfigDir(), constants.ThemesDirName)
}

// GetStateDir returns the absolute path to the dockstarter2 state directory.
func GetStateDir() string {
	if StateHomeOverride != "" {
		return StateHomeOverride
	}
	appName := strings.ToLower(version.ApplicationName)
	return filepath.Join(xdg.StateHome, appName)
}

// GetLocksDir returns the absolute path to the dockstarter2 locks directory.
func GetLocksDir() string {
	return filepath.Join(GetStateDir(), "locks")
}

// GetInstancesDir returns the absolute path to the dockstarter2 instances directory.
func GetInstancesDir() string {
	return filepath.Join(GetStateDir(), constants.InstancesDirName)
}

// GetTimestampsDir returns the absolute path to the dockstarter2 timestamps directory.
func GetTimestampsDir() string {
	return filepath.Join(GetStateDir(), constants.TimestampsDirName)
}

// GetLocalLockPath returns the path to the local-side operation lock file.
// Written by the local TUI/CLI while a destructive operation is in progress.
func GetLocalLockPath() string {
	return filepath.Join(GetLocksDir(), "local.lock")
}

// GetRemoteLockPath returns the path to the remote-side operation lock file.
// Written by the SSH/web server session while a destructive operation is in progress.
func GetRemoteLockPath() string {
	return filepath.Join(GetLocksDir(), "remote.lock")
}

// GetActiveThemeFile returns the path to the currently-active theme file in the state directory.
// This is the single file Load() reads at runtime; it is written by EnsureThemeExtracted.
func GetActiveThemeFile() string {
	return filepath.Join(GetStateDir(), "theme.ds2theme")
}

// GetInstanceDir returns the absolute path to the folder for a specific app instance.
func GetInstanceDir(appName string) string {
	return filepath.Join(GetInstancesDir(), appName)
}

// GetExecDirectory returns the directory of the currently running executable.
func GetExecDirectory() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// ResetNeeds deletes the timestamps directory, mirroring reset_needs.sh.
func ResetNeeds() error {
	timestampDir := GetTimestampsDir()
	if _, err := os.Stat(timestampDir); err == nil {
		return os.RemoveAll(timestampDir)
	}
	return nil
}

// GetBashScriptFolder attempts to find the installation directory of the legacy Bash version of DockSTARTer.
// It mimics the logic in main.sh by finding the 'ds' command and following symlinks to its source.
func GetBashScriptFolder() string {
	// 1. Try to find the 'ds' command in the system path
	dsPath, err := exec.LookPath("ds")
	if err == nil {
		// Resolve all symlinks to find the canonical path (equivalent to the while loop in main.sh)
		if realPath, err := filepath.EvalSymlinks(dsPath); err == nil {
			return filepath.Dir(realPath)
		}
	}

	// 2. Fallback to default locations (~/.docker then ~/.dockstarter)
	home, err := os.UserHomeDir()
	if err == nil {
		for _, name := range []string{".dockstarter", ".docker"} {
			legacyPath := filepath.Join(home, name)
			if info, err := os.Stat(legacyPath); err == nil && info.IsDir() {
				return legacyPath
			}
		}
	}

	return ""
}

// GetLegacyIniPaths returns a slice of potential paths for legacy .ini configuration files.
func GetLegacyIniPaths() []string {
	var paths []string

	// 1. Check XDG Config Home (Modern nested first, then older root)
	paths = append(paths, filepath.Join(xdg.ConfigHome, "dockstarter", "dockstarter.ini"))
	paths = append(paths, filepath.Join(xdg.ConfigHome, "dockstarter.ini"))

	// 2. Check Legacy Script Folder locations
	if bashFolder := GetBashScriptFolder(); bashFolder != "" {
		paths = append(paths, filepath.Join(bashFolder, "dockstarter.ini"))
		paths = append(paths, filepath.Join(bashFolder, "menu.ini"))
	}

	// Return only paths that actually exist
	var existing []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
		}
	}

	return existing
}

// ResolvePath resolves legacy variables like ${ScriptFolder}, ${HOME}, and ${XDG_CONFIG_HOME}.
func ResolvePath(path string) string {
	bashFolder := GetBashScriptFolder()
	if bashFolder == "" {
		bashFolder = xdg.ConfigHome
	}
	home, _ := os.UserHomeDir()

	r := strings.NewReplacer(
		"${ScriptFolder}", bashFolder,
		"${HOME}", home,
		"${XDG_CONFIG_HOME}", xdg.ConfigHome,
	)
	return r.Replace(path)
}

// DetectComposeFolderResult holds the results of the compose folder detection.
type DetectComposeFolderResult struct {
	LegacyPath    string
	CurrentPath   string
	LegacyExists  bool
	CurrentExists bool
}

// DetectComposeFolder replicates the detection logic from config_create.sh.
func DetectComposeFolder(currentConfiguredPath string) DetectComposeFolderResult {
	res := DetectComposeFolderResult{}

	// 1. Resolve Legacy Path (${ScriptFolder}/compose)
	res.LegacyPath = ResolvePath("${ScriptFolder}/compose")
	if res.LegacyPath != "" {
		if info, err := os.Stat(res.LegacyPath); err == nil && info.IsDir() {
			// A folder exists, now check if it's not empty (matches bash folder_is_empty)
			if entries, err := os.ReadDir(res.LegacyPath); err == nil && len(entries) > 0 {
				res.LegacyExists = true
			}
		}
	}

	// 2. Resolve Current Configured Path (e.g. from TOML)
	res.CurrentPath = ResolvePath(currentConfiguredPath)
	if res.CurrentPath != "" {
		if info, err := os.Stat(res.CurrentPath); err == nil && info.IsDir() {
			if entries, err := os.ReadDir(res.CurrentPath); err == nil && len(entries) > 0 {
				res.CurrentExists = true
			}
		}
	}

	return res
}

// DetectLegacyTemplatesInScriptFolder reports whether app templates are still
// stored inside the DS1 script folder itself, rather than their own separate
// templates location -- the layout used by very old DS1 installs, before
// templates moved into their own repo. Signaled by a ".apps" folder directly
// in scriptFolder or in scriptFolder/compose.
func DetectLegacyTemplatesInScriptFolder(scriptFolder string) bool {
	if scriptFolder == "" {
		return false
	}
	for _, candidate := range []string{
		filepath.Join(scriptFolder, ".apps"),
		filepath.Join(scriptFolder, "compose", ".apps"),
	} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}
