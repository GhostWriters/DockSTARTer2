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
)

// AppInstanceFile handles template processing for app instances.
func AppInstanceFile(ctx context.Context, appName, fileSuffix string) (string, error) {
	templatesDir := paths.GetTemplatesDir()
	instancesDir := paths.GetInstancesDir()

	baseApp := strings.ToLower(AppNameToBaseAppName(appName))
	instance := AppNameToInstanceName(appName)

	// Template paths
	templateFolder := filepath.Join(templatesDir, constants.TemplatesDirName, baseApp)
	templateFilename := strings.ReplaceAll(fileSuffix, "*", baseApp)
	templateFile := filepath.Join(templateFolder, templateFilename)

	// Instance paths
	instanceFolder := filepath.Join(instancesDir, strings.ToLower(appName))
	instanceFilename := strings.ReplaceAll(fileSuffix, "*", strings.ToLower(appName))
	instanceFile := filepath.Join(instanceFolder, instanceFilename)
	instanceOriginalFile := instanceFile + ".original"

	// 0. Ensure instances folder exists and have permissions
	if _, err := os.Stat(instancesDir); os.IsNotExist(err) {
		if err := os.MkdirAll(instancesDir, 0755); err == nil {
			system.SetPermissions(ctx, instancesDir)
		}
	}

	// Check if template folder exists
	if _, err := os.Stat(templateFolder); os.IsNotExist(err) {
		// Parity: remove instance folders if template folder is gone
		system.SetPermissions(ctx, instanceFolder)
		_ = os.RemoveAll(instanceFolder)
		return "", nil
	}

	// Check if template file exists
	templateContent, err := os.ReadFile(templateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Parity: remove instance files if template file is gone
			system.SetPermissions(ctx, instanceFile)
			_ = os.Remove(instanceFile)
			system.SetPermissions(ctx, instanceOriginalFile)
			_ = os.Remove(instanceOriginalFile)
			return "", nil
		}
		return "", err
	}

	// Check if we need to update/recreate
	// Bash logic (adapted for .original):
	// Return early ONLY if InstanceFile exists AND Original exists AND Original == Template.
	if _, err := os.Stat(instanceFile); err == nil {
		// Instance exists, check original
		originalContent, err := os.ReadFile(instanceOriginalFile)
		if err == nil && bytes.Equal(templateContent, originalContent) {
			return instanceFile, nil
		}
	}

	// Create instance folder
	if err := os.MkdirAll(instanceFolder, 0755); err != nil {
		return "", err
	}
	system.SetPermissions(ctx, instanceFolder)

	// Process content (replace placeholders)
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
		return "", err
	}
	system.SetPermissions(ctx, instanceFile)

	// Write Original Template File
	if err := os.WriteFile(instanceOriginalFile, templateContent, 0644); err != nil {
		return "", err
	}
	system.SetPermissions(ctx, instanceOriginalFile)

	return instanceFile, nil
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
