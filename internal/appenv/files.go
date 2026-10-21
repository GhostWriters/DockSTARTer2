package appenv

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/system"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogAppTemplateOverride logs (unconditionally -- callers decide when to
// call this, not AppInstanceFile itself) that appName's template is
// currently resolved from a user override rather than the bundled repo
// copy, if that's the case; a no-op otherwise. AppInstanceFile itself is
// called many times per app (once per file it resolves: .env,
// .env.app.*, .meta.toml, .labels.yml, ...), so it doesn't log this on
// every call -- a caller that performs one user-visible action per app
// (CreateApp, compose merge, ...) calls this once, at the point where it's
// already about to announce that action for the app.
func LogAppTemplateOverride(ctx context.Context, appName string) {
	baseApp := strings.ToLower(AppNameToBaseAppName(appName))
	templateFolder, isUserOverride := resolveAppTemplateFolder(baseApp)
	if !isUserOverride {
		return
	}
	logger.Info(ctx, "Using user app template for '%s' in '"+
		console.FormatFolderPath(templateFolder)+"'.", StyledAppName(ctx, baseApp))
}

// AppInstanceFile handles template processing for app instances.
// Mirrors app_instance_file.sh with one change: the original template copy is
// stored as instanceFile+".original" (alongside the instance file) instead of
// in a separate instances/.apps/ subfolder.
func AppInstanceFile(ctx context.Context, appName, fileSuffix string) (string, error) {
	instancesDir := paths.GetInstancesDir()

	baseApp := strings.ToLower(AppNameToBaseAppName(appName))
	instance := AppNameToInstanceName(appName)

	templateFolder := TemplateFolder(baseApp)
	instanceFolder := filepath.Join(instancesDir, strings.ToLower(appName))

	templateFile := TemplateFile(baseApp, fileSuffix)
	instanceFile := filepath.Join(instanceFolder, strings.ReplaceAll(fileSuffix, "*", strings.ToLower(appName)))
	// Original template copy stored alongside the instance file (not in a separate .apps folder).
	originalFile := instanceFile + ".original"

	// 0. Ensure instances folder exists.
	if _, err := os.Stat(instancesDir); os.IsNotExist(err) {
		if err := os.MkdirAll(instancesDir, 0755); err == nil {
			system.SetPermissions(ctx, instancesDir)
		}
	}

	// 1. Template folder check — clean up instance folder if template folder is gone.
	if _, err := os.Stat(templateFolder); os.IsNotExist(err) {
		if _, err := os.Stat(instanceFolder); err == nil {
			system.SetPermissions(ctx, instanceFolder)
			_ = os.RemoveAll(instanceFolder)
		}
		return instanceFile, nil
	}

	// 2. Template file check — clean up instance files if template file is gone.
	templateContent, err := os.ReadFile(templateFile)
	if err != nil {
		if os.IsNotExist(err) {
			for _, file := range []string{originalFile, instanceFile} {
				if _, err := os.Stat(file); err == nil {
					system.SetPermissions(ctx, file)
					_ = os.Remove(file)
				}
			}
			return instanceFile, nil
		}
		return instanceFile, err
	}

	// 3. Skip check — if instance file exists and the original copy matches the
	// current template, nothing has changed.
	if _, err := os.Stat(instanceFile); err == nil {
		if _, err := os.Stat(originalFile); err == nil {
			if cmpContent(templateFile, originalFile) {
				return instanceFile, nil
			}
		}
	}

	// 4. Create instance folder if needed.
	if _, err := os.Stat(instanceFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(instanceFolder, 0755); err != nil {
			return instanceFile, err
		}
		system.SetPermissions(ctx, instanceFolder)
	}

	// 5. Create instance file by substituting <__INSTANCE> markers.
	content := string(templateContent)
	var __INSTANCE, __Instance, __instance string
	if instance != "" {
		__INSTANCE = "__" + strings.ToUpper(instance)
		__Instance = "__" + CapitalizeFirstLetter(instance)
		__instance = "__" + strings.ToLower(instance)
	}

	content = strings.ReplaceAll(content, "<__INSTANCE>", __INSTANCE)
	content = strings.ReplaceAll(content, "<__Instance>", __Instance)
	content = strings.ReplaceAll(content, "<__instance>", __instance)

	if err := os.WriteFile(instanceFile, []byte(content), 0644); err != nil {
		return instanceFile, err
	}
	system.SetPermissions(ctx, instanceFile)

	// 6. Save a copy of the original template alongside the instance file.
	if err := os.WriteFile(originalFile, templateContent, 0644); err != nil {
		return instanceFile, err
	}
	system.SetPermissions(ctx, originalFile)

	return instanceFile, nil
}

// cmpContent compares two files (equivalent to cmp -s).
func cmpContent(file1, file2 string) bool {
	f1, err := os.ReadFile(file1)
	if err != nil {
		return false
	}
	f2, err := os.ReadFile(file2)
	if err != nil {
		return false
	}
	return bytes.Equal(f1, f2)
}

// CopyFile copies a file from src to dst.
func CopyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

// CompareFiles returns true if two files have identical contents.
func CompareFiles(file1, file2 string) bool {
	f1, err := os.ReadFile(file1)
	if err != nil {
		return false
	}
	f2, err := os.ReadFile(file2)
	if err != nil {
		return false
	}
	return bytes.Equal(f1, f2)
}

// AppTemplateChangedSince reports whether any file in appName's template
// folder changed after t. The folder is resolved like every other template
// lookup (TemplateFolder), so a user app template, or a user override of a
// repo app, is the one checked.
func AppTemplateChangedSince(appName string, t time.Time) bool {
	return IsAnyFileNewer(TemplateFolder(appName), t)
}

// templateSourcePrefix names the marker RecordTemplateSource writes per app.
const templateSourcePrefix = "TemplateSource_"

// TemplateSourceChanged reports whether appName's template folder
// (TemplateFolder) differs from the one RecordTemplateSource last saved in
// markerDir. Adding, removing or switching a user override changes which
// folder is used without making its files any newer than the last sync, so
// AppTemplateChangedSince alone can't see it. A missing record counts as a
// change.
func TemplateSourceChanged(markerDir, appName string) bool {
	saved, err := os.ReadFile(filepath.Join(markerDir, templateSourcePrefix+strings.ToUpper(appName)))
	return err != nil || string(saved) != TemplateFolder(appName)
}

// RecordTemplateSource saves appName's current template folder in markerDir,
// for TemplateSourceChanged.
func RecordTemplateSource(markerDir, appName string) {
	_ = os.WriteFile(filepath.Join(markerDir, templateSourcePrefix+strings.ToUpper(appName)), []byte(TemplateFolder(appName)), 0644)
}

// IsAnyFileNewer reports whether any file under root (recursively) is newer
// than referenceTime. A missing root has nothing newer; anything under it
// that can't be read counts as newer, so a template DS2 can't fully inspect
// is treated as changed rather than silently skipped.
func IsAnyFileNewer(root string, referenceTime time.Time) bool {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return false
	}
	newer := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || (!info.IsDir() && info.ModTime().After(referenceTime)) {
			newer = true
			return filepath.SkipAll
		}
		return nil
	})
	return newer
}
