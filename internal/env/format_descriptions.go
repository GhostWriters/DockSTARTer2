package env

import (
	"DockSTARTer2/internal/paths"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// getDescriptionFromLabels reads the description from labels.yml.
// Mirrors app_description_from_template.sh functionality.
func getDescriptionFromLabels(appName string, isUserDefined bool) string {
	niceName := getNiceName(appName)
	if isUserDefined {
		return niceName + " is a user defined application "
	}

	// Get base app name
	appLower := strings.ToLower(appName)
	baseApp := appLower
	if idx := strings.Index(appLower, "__"); idx > 0 {
		baseApp = appLower[:idx]
	}

	// Get templates dir and labels file
	templatesDir := paths.GetTemplatesDir()
	labelsFile := filepath.Join(templatesDir, ".apps", baseApp, baseApp+".labels.yml")

	// Check if labels file exists
	if _, err := os.Stat(labelsFile); err != nil {
		return niceName + " is a user defined application"
	}

	// Read labels file
	content, err := os.ReadFile(labelsFile)
	if err != nil {
		return "! Missing application !"
	}

	// Extract description using regex
	// Bash: grep -Po "\scom\.dockstarter\.appinfo\.description: \K.*"
	re := regexp.MustCompile(`\s+com\.dockstarter\.appinfo\.description:\s+(.+)`)
	matches := re.FindStringSubmatch(string(content))

	if len(matches) > 1 {
		desc := matches[1]
		// Remove quotes if present (sed -E 's/^([^"].*[^"])$/"\\1"/' | xargs)
		desc = strings.Trim(desc, `"' `)
		return desc
	}

	return "! Missing description !"
}
