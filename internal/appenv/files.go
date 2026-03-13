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
// Mirrors app_instance_file.sh exactly.
func AppInstanceFile(ctx context.Context, appName, fileSuffix string) (string, error) {
	templatesDir := paths.GetTemplatesDir()
	instancesDir := paths.GetInstancesDir()

	baseApp := strings.ToLower(AppNameToBaseAppName(appName))
	instance := AppNameToInstanceName(appName)

	// Instance paths (Parity lines 31-36)
	templateFolder := filepath.Join(templatesDir, constants.TemplatesDirName, baseApp)
	instanceTemplateFolder := filepath.Join(instancesDir, constants.TemplatesDirName, strings.ToLower(appName))
	instanceFolder := filepath.Join(instancesDir, strings.ToLower(appName))

	templateFile := filepath.Join(templateFolder, strings.ReplaceAll(fileSuffix, "*", baseApp))
	instanceTemplateFile := filepath.Join(instanceTemplateFolder, strings.ReplaceAll(fileSuffix, "*", strings.ToLower(appName)))
	instanceFile := filepath.Join(instanceFolder, strings.ReplaceAll(fileSuffix, "*", strings.ToLower(appName)))

	// Return InstanceFile (Parity line 38: echo "${InstanceFile}")
	// In Go we proceed with logic but we MUST ensure this path is Returned.
	// We'll capture the return value and finish the side-effects.

	// 0. Ensure instances folder exists (Parity lines 19-24)
	if _, err := os.Stat(instancesDir); os.IsNotExist(err) {
		if err := os.MkdirAll(instancesDir, 0755); err == nil {
			system.SetPermissions(ctx, instancesDir)
		}
	}

	// 1. Template folder check (Parity lines 41-52)
	if _, err := os.Stat(templateFolder); os.IsNotExist(err) {
		// Remove instance folders if template folder is gone
		for _, folder := range []string{instanceTemplateFolder, instanceFolder} {
			if _, err := os.Stat(folder); err == nil {
				system.SetPermissions(ctx, folder)
				_ = os.RemoveAll(folder)
			}
		}
		return instanceFile, nil
	}

	// 2. Template file check (Parity lines 54-66)
	templateContent, err := os.ReadFile(templateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Remove instance files if template file is gone
			for _, file := range []string{instanceTemplateFile, instanceFile} {
				if _, err := os.Stat(file); err == nil {
					system.SetPermissions(ctx, file)
					_ = os.Remove(file)
				}
			}
			return instanceFile, nil
		}
		return instanceFile, err
	}

	// 3. Skip check (Parity lines 68-71)
	if _, err := os.Stat(instanceFile); err == nil {
		if _, err := os.Stat(instanceTemplateFile); err == nil {
			if cmpContent(templateFile, instanceTemplateFile) {
				return instanceFile, nil
			}
		}
	}

	// 4. Create instance folder (Parity lines 76-82)
	if _, err := os.Stat(instanceFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(instanceFolder, 0755); err != nil {
			return instanceFile, err
		}
		system.SetPermissions(ctx, instanceFolder)
	}

	// 5. Create instance file (Parity lines 85-96)
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

	// 6. Create original template folder and file (Parity lines 98-111)
	if _, err := os.Stat(instanceTemplateFolder); os.IsNotExist(err) {
		if err := os.MkdirAll(instanceTemplateFolder, 0755); err != nil {
			return instanceFile, err
		}
		system.SetPermissions(ctx, instanceTemplateFolder)
	}

	if err := os.WriteFile(instanceTemplateFile, templateContent, 0644); err != nil {
		return instanceFile, err
	}
	system.SetPermissions(ctx, instanceTemplateFile)

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
