package commands

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
)

// fileWord returns "file" or "files" to match n.
func fileWord(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}

// resolveAppDestSubfolder resolves the optional destination-subfolder
// argument shared by --app-template-extract and --app-template-new. Unlike
// --theme-extract (which reasonably supports extracting to any arbitrary
// filesystem path), an app template destination is always somewhere under
// paths.GetUserAppsDir() -- there's no other place DS2 would ever look for
// it -- so arg must be given as "user:<subfolder>" (or omitted entirely for
// the user apps folder root); a bare non-"user:" arg is rejected rather
// than silently treated as an arbitrary path. Each organizational subfolder
// segment gets ".d"-suffixed (see appenv.WithDotDSuffix / walkUserAppFolders)
// so the extracted/scaffolded app is still recognized as a leaf, not
// swallowed as an ambiguous folder.
func resolveAppDestSubfolder(arg string) (string, error) {
	if arg == "" {
		return paths.GetUserAppsDir(), nil
	}
	sub, ok := strings.CutPrefix(arg, "user:")
	if !ok {
		return "", fmt.Errorf("destination must be given as user:<subfolder>, e.g. user:mynewapps: %s", arg)
	}
	return filepath.Join(paths.GetUserAppsDir(), appenv.WithDotDSuffix(sub)), nil
}

// HandleAppTemplateExtract copies a whole app template folder (compose
// snippet, .env defaults, .meta.toml, etc.) from the DockSTARTer-Templates
// repo to a destination folder -- typically the user apps folder (see
// internal/appenv/user_templates.go), so a template can be drafted and
// tested locally before opening a PR against the templates repo.
func HandleAppTemplateExtract(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) == 0 {
		logger.Error(ctx, "Usage: --app-template-extract <AppName|.TEMPLATE> [user:<subfolder>]")
		return fmt.Errorf("missing app name")
	}
	appName := strings.ToLower(group.Args[0])

	srcDir := filepath.Join(paths.GetTemplatesDir(), constants.TemplatesDirName, appName)
	if info, err := os.Stat(srcDir); err != nil || !info.IsDir() {
		logger.Error(ctx, "App template '{{|App|}}%s{{[-]}}' not found.", appName)
		return fmt.Errorf("app template not found: %s", appName)
	}

	destArg := ""
	if len(group.Args) >= 2 {
		destArg = group.Args[1]
	}
	destBase, err := resolveAppDestSubfolder(destArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	destDir := filepath.Join(destBase, appName)

	if _, statErr := os.Stat(destDir); statErr == nil {
		if !console.GlobalForce {
			logger.Error(ctx, "'"+console.FormatUserFolderPath(paths.GetUserAppsDir(), destDir)+
				"' already exists. Use {{|UserCommand|}}-f{{[-]}}/{{|UserCommand|}}--force{{[-]}} to overwrite it.")
			return fmt.Errorf("destination already exists: %s", destDir)
		}
		// Force wipes the folder first rather than overwriting in place, so
		// a file that existed in an old extraction but no longer exists in
		// the current source doesn't linger behind.
		if err := os.RemoveAll(destDir); err != nil {
			logger.Error(ctx, "Failed to remove existing '"+console.FormatUserFolderPath(paths.GetUserAppsDir(), destDir)+"': %v", err)
			return err
		}
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		logger.Error(ctx, "Failed to create directory '"+console.FormatUserFolderPath(paths.GetUserAppsDir(), destDir)+"': %v", err)
		return err
	}

	copied := 0
	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		dst := filepath.Join(destDir, rel)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0755); mkErr != nil {
			return mkErr
		}
		if writeErr := os.WriteFile(dst, data, 0644); writeErr != nil {
			return writeErr
		}
		copied++
		return nil
	})
	if err != nil {
		logger.Error(ctx, "Failed to copy app template: %v", err)
		return err
	}

	logger.Notice(ctx, "App template '{{|App|}}%s{{[-]}}' extracted to: "+console.FormatUserFolderPath(paths.GetUserAppsDir(), destDir)+" (%d %s)", appName, copied, fileWord(copied))
	if appName == ".template" {
		logger.Notice(ctx, "This is a reference starter template, not one meant to be used as-is. Rename it and edit the copy.")
	} else {
		// destBase is always under the user apps folder (resolveAppDestSubfolder
		// never returns an arbitrary path), so this always applies.
		logger.Notice(ctx, "Enable and edit this copy to test your changes -- it takes precedence over the bundled template while it exists.")
	}
	appenv.InvalidateAppMetaCache()
	return nil
}

// HandleAppTemplateNew scaffolds a brand-new user app template by copying
// the .TEMPLATE reference folder into the user apps folder under a new
// name. File *contents* use bracket-wrapped tokens -- "<APPNAME>" ->
// uppercase, "<AppName>" -> capitalized (user's own casing, see below),
// "<appname>" -> lowercase -- matching the existing
// <__instance>/<__INSTANCE>/<__Instance> convention, which avoids
// mis-renaming any unrelated occurrence of the substring "appname"
// elsewhere in a file (e.g. inside a URL or example value). File *names*
// use the bare "appname" prefix instead (e.g. "appname.yml"), with no
// brackets: "<"/">" are invalid in Windows filenames. The
// <__instance>/<__INSTANCE>/<__Instance> tokens themselves are deliberately
// left untouched in both -- those are a separate substitution
// AppInstanceFile performs later, at app-instance-creation time, not at
// template-authoring time.
func HandleAppTemplateNew(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) == 0 {
		logger.Error(ctx, "Usage: --app-template-new <appname> [user:<subfolder>]")
		return fmt.Errorf("missing app name")
	}
	lower := strings.ToLower(group.Args[0])
	if !appenv.IsAppNameValid(strings.ToUpper(lower)) {
		logger.Error(ctx, "'{{|App|}}%s{{[-]}}' is not a valid app name.", lower)
		return fmt.Errorf("invalid app name: %s", lower)
	}
	upper := strings.ToUpper(lower)
	// The nicename/<AppName> token uses the user's own casing verbatim
	// (e.g. "MyApp" typed on the command line stays "MyApp" in labels.yml's
	// nicename and the README/meta helptext), falling back to a single
	// capitalized letter only when the user typed it all lowercase --
	// otherwise every scaffolded app would read "Myapp" regardless of how
	// they actually wanted it displayed.
	capitalized := group.Args[0]
	if capitalized == lower {
		capitalized = appenv.CapitalizeFirstLetter(lower)
	}

	// Prefer the already-synced local copy (kept current by
	// appenv.SyncUserAppTemplateReference); fall back to reading straight
	// from the templates repo clone if that hasn't run yet.
	srcDir := filepath.Join(paths.GetUserAppsDir(), ".TEMPLATE")
	if info, err := os.Stat(srcDir); err != nil || !info.IsDir() {
		srcDir = filepath.Join(paths.GetTemplatesDir(), constants.TemplatesDirName, ".TEMPLATE")
		if info, err := os.Stat(srcDir); err != nil || !info.IsDir() {
			logger.Error(ctx, "App template reference (.TEMPLATE) not found -- run "+
				"{{|UserCommand|}}--update-templates{{[-]}} first.")
			return fmt.Errorf(".TEMPLATE not found")
		}
	}

	destArg := ""
	if len(group.Args) >= 2 {
		destArg = group.Args[1]
	}
	destBase, err := resolveAppDestSubfolder(destArg)
	if err != nil {
		logger.Error(ctx, "%v", err)
		return err
	}
	destDir := filepath.Join(destBase, lower)
	if _, err := os.Stat(destDir); err == nil {
		logger.Error(ctx, "'"+console.FormatUserFolderPath(paths.GetUserAppsDir(), destDir)+"' already exists.")
		return fmt.Errorf("destination already exists: %s", destDir)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		logger.Error(ctx, "Failed to create directory '"+console.FormatUserFolderPath(paths.GetUserAppsDir(), destDir)+"': %v", err)
		return err
	}

	renameContent := func(s string) string {
		s = strings.ReplaceAll(s, "<APPNAME>", upper)
		s = strings.ReplaceAll(s, "<AppName>", capitalized)
		s = strings.ReplaceAll(s, "<appname>", lower)
		return s
	}
	renameFilename := func(s string) string {
		s = strings.ReplaceAll(s, "APPNAME", upper)
		s = strings.ReplaceAll(s, "AppName", capitalized)
		s = strings.ReplaceAll(s, "appname", lower)
		return s
	}

	copied := 0
	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		dst := filepath.Join(destDir, renameFilename(rel))
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0755); mkErr != nil {
			return mkErr
		}
		if writeErr := os.WriteFile(dst, []byte(renameContent(string(data))), 0644); writeErr != nil {
			return writeErr
		}
		copied++
		return nil
	})
	if err != nil {
		logger.Error(ctx, "Failed to scaffold app template: %v", err)
		return err
	}

	logger.Notice(ctx, "New app template '{{|App|}}%s{{[-]}}' created at: "+console.FormatUserFolderPath(paths.GetUserAppsDir(), destDir)+" (%d %s)", lower, copied, fileWord(copied))
	logger.Notice(ctx, "Fill in the compose snippet and defaults, then enable '{{|App|}}%s{{[-]}}' to test it.", upper)
	appenv.InvalidateAppMetaCache()
	return nil
}
