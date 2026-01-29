package env

import (
	"DockSTARTer2/internal/envutil"
	"DockSTARTer2/internal/paths"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FormatLines formats environment variable lines with proper headers and structure.
// Mirrors Bash env_format_lines.sh functionality.
//
// Parameters:
//   - currentEnvFile:  File containing current variable values (or empty string for new vars)
//   - defaultEnvFile:  Template file to preserve exact formatting (e.g., .env.example or app template)
//   - appName:         Application name (empty for globals)
//   - composeEnvFile:  Path to global .env file (for checking IsUserDefined/IsDisabled)
//
// Returns: Array of formatted lines ready to write to file
func FormatLines(currentEnvFile, defaultEnvFile, appName, composeEnvFile string) ([]string, error) {
	var result []string

	// Tags/constants matching Bash (lines 15-20)
	const (
		globalVarsHeading        = "Global Variables"
		appDeprecatedTag         = " [*DEPRECATED*]"
		appDisabledTag           = " (Disabled)"
		appUserDefinedTag        = " (User Defined)"
		userDefinedVarsTag       = " (User Defined Variables)"
		userDefinedGlobalVarsTag = " (User Defined)"
	)

	// 1. If appName specified, add app header (lines 31-56)
	// For globals (empty appName), skip header generation - .env.example has its own
	// But we still need these vars for the user-defined section later
	appIsUserDefined := false
	var appNiceName string

	if appName != "" {
		// Check if user defined (full Bash parity)
		appIsUserDefined = IsUserDefinedApp(appName, composeEnvFile)

		// Get nice name (inline implementation)
		appNiceName = getNiceName(appName)

		// Get description from labels.yml (full Bash parity)
		appDescription := getDescriptionFromLabels(appName, appIsUserDefined)

		// Build heading title
		headingTitle := appNiceName
		if appIsUserDefined {
			headingTitle += appUserDefinedTag
		} else {
			// Check deprecated/disabled status (full Bash parity)
			if IsDeprecatedApp(appName) {
				headingTitle += appDeprecatedTag
			}
			if IsDisabled(appName, composeEnvFile) {
				headingTitle += appDisabledTag
			}
		}

		// Wrap description at 75 chars (line 37)
		wrapped := wordWrap(appDescription, 75)

		// Create header (lines 46-55)
		result = append(result, "###")
		result = append(result, "### "+headingTitle)
		result = append(result, "###")
		for _, line := range wrapped {
			result = append(result, "### "+line)
		}
		result = append(result, "###")
	}

	// 2. If defaultEnvFile exists, read verbatim (lines 57-64)
	if defaultEnvFile != "" {
		if info, err := os.Stat(defaultEnvFile); err == nil && !info.IsDir() {
			content, err := os.ReadFile(defaultEnvFile)
			if err == nil {
				lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
				result = append(result, lines...)
				if len(result) > 0 {
					result = append(result, "") // blank line
				}
			}
		}
	}

	// 3. Build index of variables already in result (lines 66-78)
	formattedEnvVarIndex := make(map[string]int)

	// Regex to extract variable names from lines (line 71)
	varRe := regexp.MustCompile(`^([A-Za-z0-9_]+)=`)
	for i, line := range result {
		matches := varRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			formattedEnvVarIndex[matches[1]] = i
		}
	}

	// 4. Read current env lines (lines 22-25, 80-91)
	var currentEnvLines []string
	if currentEnvFile != "" {
		if info, err := os.Stat(currentEnvFile); err == nil && !info.IsDir() {
			lines, err := envutil.ReadLines(currentEnvFile)
			if err == nil {
				currentEnvLines = lines
			}
		}
	}

	// Update existing variables and track remaining (lines 82-90)
	var remainingVars []string
	for _, line := range currentEnvLines {
		matches := varRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			varName := matches[1]
			if idx, exists := formattedEnvVarIndex[varName]; exists {
				// Update existing variable
				result[idx] = line
			} else {
				// Track for adding later
				remainingVars = append(remainingVars, line)
			}
		}
	}

	// 5. Add user-defined variables section if needed (lines 92-123)
	if len(remainingVars) > 0 {
		// Add "User Defined" heading if not already user-defined app (lines 93-108)
		// CRITICAL: For globals, DON'T add this header if we successfully read .env.example
		//           The variables should have been updated in-place above
		shouldAddHeader := false
		if appName != "" {
			// For apps, add header unless it's a user-defined app
			shouldAddHeader = !appIsUserDefined
		} else {
			// For globals, ONLY add header if we didn't read a default file
			// If we DID read .env.example, something went wrong and we shouldn't
			// add a misleading "User Defined" header
			shouldAddHeader = (defaultEnvFile == "")
		}

		if shouldAddHeader {
			var headingTitle string
			if appName != "" {
				headingTitle = appNiceName + userDefinedVarsTag
			} else {
				headingTitle = globalVarsHeading + userDefinedGlobalVarsTag
			}

			result = append(result, "###")
			result = append(result, "### "+headingTitle)
			result = append(result, "###")
		}

		// Add the remaining variables (lines 110-121)
		for _, line := range remainingVars {
			result = append(result, line)
		}
		result = append(result, "")
	} else if len(result) > 0 {
		// Just add blank line if no user vars (line 125)
		result = append(result, "")
	}

	return result, nil
}

// FormatLinesForApp is a convenience wrapper for FormatLines that handles app-specific logic.
// It determines the default template .env file based on whether the app is user-defined.
func FormatLinesForApp(currentEnvFile, appName, templatesDir, composeEnvFile string) ([]string, error) {
	var defaultEnvFile string

	// Get default app .env file if not user-defined
	if !IsUserDefinedApp(appName, composeEnvFile) {
		// Use the processed instance file (contains correct values/placeholders replaced)
		instancesDir := paths.GetInstancesDir()
		processedInstanceFile := filepath.Join(instancesDir, appName, ".env")
		if _, err := os.Stat(processedInstanceFile); err == nil {
			defaultEnvFile = processedInstanceFile
		}
	}

	return FormatLines(currentEnvFile, defaultEnvFile, appName, composeEnvFile)
}

// GetReferencedApps returns a list of apps referenced in the compose env file.
func GetReferencedApps(composeEnvFile string) ([]string, error) {
	lines, err := envutil.ReadLines(composeEnvFile)
	if err != nil {
		return nil, err
	}

	appMap := make(map[string]bool)
	for _, line := range lines {
		varName := line
		if idx := strings.Index(line, "="); idx > 0 {
			varName = strings.TrimSpace(line[:idx])
		}
		appName := VarNameToAppName(varName)
		if appName != "" && AppNameIsValid(appName) {
			appMap[appName] = true
		}
	}

	var result []string
	for app := range appMap {
		result = append(result, app)
	}
	return result, nil
}

// getBaseAppName returns the base app name without instance suffix.
func getBaseAppName(appName string) string {
	appLower := strings.ToLower(appName)
	if idx := strings.Index(appLower, "__"); idx > 0 {
		return appLower[:idx]
	}
	return appLower
}

// getNiceName returns a nicely formatted app name.
func getNiceName(appName string) string {
	appUpper := strings.ToUpper(appName)
	parts := strings.Split(appUpper, "__")

	var niceParts []string
	for _, part := range parts {
		niceParts = append(niceParts, strings.Title(strings.ToLower(part)))
	}

	return strings.Join(niceParts, " ")
}

// getSimpleDescription returns a simple description for an app.
func getSimpleDescription(appName string, isUserDefined bool) string {
	niceName := getNiceName(appName)
	if isUserDefined {
		return niceName + " is a user defined application"
	}
	return niceName + " application"
}

// wordWrap wraps text at the specified width, breaking on word boundaries.
func wordWrap(text string, width int) []string {
	var lines []string
	words := strings.Fields(text)

	if len(words) == 0 {
		return lines
	}

	var currentLine string
	for _, word := range words {
		if currentLine == "" {
			currentLine = word
		} else if len(currentLine)+1+len(word) <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}
