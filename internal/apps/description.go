package apps

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/paths"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GetDescription returns the description for an app.
// Mirrors app_description.sh functionality.
func GetDescription(appName string) string {
	// Construct envFile path without using paths to avoid cycle
	// Use config to get compose folder
	conf := config.LoadAppConfig()
	envFile := filepath.Join(conf.ComposeFolder, ".env")

	if IsUserDefined(appName, envFile) {
		niceName := GetNiceName(appName)
		return niceName + " is a user defined application"
	}
	return GetDescriptionFromTemplate(appName)
}

// GetDescriptionFromTemplate extracts the description from the app's labels.yml file.
// Mirrors app_description_from_template.sh functionality.
func GetDescriptionFromTemplate(appName string) string {
	appLower := strings.ToLower(appName)

	// Get base app name (strip instance suffix)
	baseApp := appname_to_baseappname(appLower)

	// Check if builtin
	templatesDir := paths.GetTemplatesDir()
	labelsFile := filepath.Join(templatesDir, ".apps", baseApp, baseApp+".labels.yml")

	if _, err := os.Stat(labelsFile); err != nil {
		niceName := GetNiceName(appName)
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
		// Remove quotes if present (sed -E 's/^([^"].*[^"])$/"\1"/' | xargs)
		desc = strings.Trim(desc, `"' `)
		return desc
	}

	return "! Missing description !"
}

// GetNiceName returns a nicely formatted app name.
// Converts "radarr__4k" to "Radarr 4K", etc.
func GetNiceName(appName string) string {
	appUpper := strings.ToUpper(appName)

	// Split on __ for instances
	parts := strings.Split(appUpper, "__")

	var niceParts []string
	for _, part := range parts {
		// Title case the part
		niceParts = append(niceParts, strings.Title(strings.ToLower(part)))
	}

	return strings.Join(niceParts, " ")
}
