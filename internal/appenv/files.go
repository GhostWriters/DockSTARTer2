package appenv

import (
	"DockSTARTer2/internal/paths"
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
	templateFolder := filepath.Join(templatesDir, ".apps", baseApp)
	templateFilename := strings.ReplaceAll(fileSuffix, "*", baseApp)
	templateFile := filepath.Join(templateFolder, templateFilename)

	// Instance paths
	instanceFolder := filepath.Join(instancesDir, strings.ToLower(appName))
	instanceFilename := strings.ReplaceAll(fileSuffix, "*", strings.ToLower(appName))
	instanceFile := filepath.Join(instanceFolder, instanceFilename)
	instanceOriginalFile := instanceFile + ".original"

	// Check if template folder exists
	if _, err := os.Stat(templateFolder); os.IsNotExist(err) {
		return "", nil // No template, no instance
	}

	// Read template
	templateContent, err := os.ReadFile(templateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // Some apps don't have all files
		}
		return "", err
	}

	// Create instance folder
	if err := os.MkdirAll(instanceFolder, 0755); err != nil {
		return "", err
	}

	// Check if we need to update/recreate
	originalContent, _ := os.ReadFile(instanceOriginalFile)
	if !bytes.Equal(templateContent, originalContent) {
		// Template changed or missing, recreate instance
		if err := os.WriteFile(instanceOriginalFile, templateContent, 0644); err != nil {
			return "", err
		}

		// Process content (replace placeholders)
		content := string(templateContent)
		if instance != "" {
			content = strings.ReplaceAll(content, "<__INSTANCE>", "__"+instance)
		} else {
			content = strings.ReplaceAll(content, "<__INSTANCE>", "")
		}

		if err := os.WriteFile(instanceFile, []byte(content), 0644); err != nil {
			return "", err
		}
	}

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
