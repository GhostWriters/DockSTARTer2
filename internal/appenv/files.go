package appenv

import (
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/system"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AppInstanceFile handles template processing for app instances.
// Mirrors app_instance_file.sh with one change: the original template copy is
// stored as instanceFile+".original" (alongside the instance file) instead of
// in a separate instances/.apps/ subfolder.
func AppInstanceFile(ctx context.Context, appName, fileSuffix string) (string, error) {
	templatesDir := paths.GetTemplatesDir()
	instancesDir := paths.GetInstancesDir()

	baseApp := strings.ToLower(AppNameToBaseAppName(appName))
	instance := AppNameToInstanceName(appName)

	templateFolder := filepath.Join(templatesDir, constants.TemplatesDirName, baseApp)
	instanceFolder := filepath.Join(instancesDir, strings.ToLower(appName))

	templateFile := filepath.Join(templateFolder, strings.ReplaceAll(fileSuffix, "*", baseApp))
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
		__Instance = "__" + strings.Title(strings.ToLower(instance))
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

// IsAnyFileNewer checks if any file in the given root (recursively) is newer than the reference time.
func IsAnyFileNewer(root string, referenceTime time.Time) bool {
	newer := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.ModTime().After(referenceTime) {
			newer = true
			return filepath.SkipDir // Found one, can stop
		}
		return nil
	})
	return newer
}
