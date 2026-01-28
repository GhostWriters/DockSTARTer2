package format

import (
	"DockSTARTer2/internal/apps"
	"DockSTARTer2/internal/env"
	"os"
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
	appIsUserDefined := false
	var appNiceName string

	if appName != "" {
		// Check if user defined
		appIsUserDefined = apps.IsUserDefined(appName, composeEnvFile)

		// Get nice name and description
		appNiceName = apps.GetNiceName(appName)
		appDescription := apps.GetDescription(appName)

		// Build heading title
		headingTitle := appNiceName
		if appIsUserDefined {
			headingTitle += appUserDefinedTag
		} else {
			if apps.IsDeprecated(appName) {
				headingTitle += appDeprecatedTag
			}
			if apps.IsDisabled(appName, composeEnvFile) {
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
			lines, err := env.ReadLines(currentEnvFile)
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
		if appName == "" || !appIsUserDefined {
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
