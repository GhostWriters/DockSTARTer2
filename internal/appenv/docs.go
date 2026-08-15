package appenv

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// GetAppMarkdown retrieves the markdown documentation for a given application.
// appName can be a base name or an instance name (e.g., "RADARR" or "RADARR__4K").
// Returns the documentation content as a string.
func GetAppMarkdown(ctx context.Context, appName string) (string, error) {
	if appName == "" {
		return "", fmt.Errorf("application name is empty")
	}

	appUpper := strings.ToUpper(appName)
	if !IsAppNameValid(appUpper) {
		return "", fmt.Errorf("invalid application name: %s", appName)
	}

	if !IsAppBuiltIn(appUpper) {
		return "", fmt.Errorf("application is not a built-in template: %s", appName)
	}

	LogAppTemplateOverride(ctx, appUpper)
	docPath := TemplateFile(appUpper, "README.md")

	if _, err := os.Stat(docPath); os.IsNotExist(err) {
		return "", fmt.Errorf("documentation not found for app: %s", appName)
	}

	content, err := os.ReadFile(docPath)
	if err != nil {
		return "", fmt.Errorf("failed to read documentation file for app %s: %w", appName, err)
	}

	return string(content), nil
}
