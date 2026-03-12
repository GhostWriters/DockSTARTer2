package appenv

import (
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/envutil"
	"context"
	"os"
	"regexp"
	"slices"
	"strings"
)

// FormatLines formats environment variable lines with proper headers and structure.
func FormatLines(ctx context.Context, currentEnvFile, defaultEnvFile, appName, composeEnvFile string) ([]string, error) {
	const (
		globalVarsHeading        = "Global Variables"
		appDeprecatedTag         = " [*DEPRECATED*]"
		appDisabledTag           = " (Disabled)"
		appUserDefinedTag        = " (User Defined)"
		userDefinedVarsTag       = " (User Defined Variables)"
		userDefinedGlobalVarsTag = " (User Defined)"
	)

	appUpper := strings.ToUpper(appName)
	appIsUserDefined := false
	var appNiceName string
	var formattedEnvLines []string

	if appUpper != "" {
		if IsAppUserDefined(ctx, appUpper, composeEnvFile) {
			appIsUserDefined = true
		}
		appNiceName = GetNiceName(ctx, appUpper)
		appDescription := GetDescription(ctx, appUpper, composeEnvFile)

		headingTitle := appNiceName
		if appIsUserDefined {
			headingTitle += appUserDefinedTag
		} else {
			if IsAppDeprecated(ctx, appUpper) {
				headingTitle += appDeprecatedTag
			}
			if !IsAppEnabled(appUpper, composeEnvFile) {
				headingTitle += appDisabledTag
			}
		}

		// Bash heading text: empty, title, empty, description, empty
		headingLines := []string{
			"",
			headingTitle,
			"",
		}
		wrappedDescription := wordWrap(appDescription, 75)
		headingLines = append(headingLines, wrappedDescription...)
		headingLines = append(headingLines, "")

		for _, hl := range headingLines {
			trimmed := strings.TrimRight(hl, " \t")
			if trimmed == "" {
				formattedEnvLines = append(formattedEnvLines, "###")
			} else {
				formattedEnvLines = append(formattedEnvLines, "### "+trimmed)
			}
		}
	}

	if defaultEnvFile != "" {
		if info, err := os.Stat(defaultEnvFile); err == nil && !info.IsDir() {
			content, err := os.ReadFile(defaultEnvFile)
			if err == nil {
				lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
				formattedEnvLines = append(formattedEnvLines, lines...)
				if len(formattedEnvLines) > 0 {
					formattedEnvLines = append(formattedEnvLines, "")
				}
			}
		}
	}

	// Indexed mapping of variable names to their line index in formattedEnvLines
	formattedEnvVarIndex := make(map[string]int)
	varRe := regexp.MustCompile(`^([A-Za-z0-9_]+)=`)
	for i, line := range formattedEnvLines {
		matches := varRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			formattedEnvVarIndex[matches[1]] = i
		}
	}

	// Read current environment lines
	var currentEnvLines []string
	if currentEnvFile != "" {
		var err error
		currentEnvLines, err = envutil.ReadLines(currentEnvFile)
		if err != nil {
			return nil, err
		}
	}

	consumed := make([]bool, len(currentEnvLines))
	if len(currentEnvLines) > 0 {
		// Update values in formattedEnvLines from currentEnvLines
		for i, line := range currentEnvLines {
			idx := strings.Index(line, "=")
			if idx > 0 {
				varName := line[:idx]
				if lineIndex, exists := formattedEnvVarIndex[varName]; exists {
					formattedEnvLines[lineIndex] = line
					consumed[i] = true
				}
			}
		}
	}

	// Filter to remaining currentEnvLines
	var remainingLines []string
	for i, line := range currentEnvLines {
		if !consumed[i] {
			remainingLines = append(remainingLines, line)
		}
	}

	// Filter remainingLines to only those that belong to this context
	var filteredRemaining []string
	for _, line := range remainingLines {
		idx := strings.Index(line, "=")
		if idx > 0 {
			varName := line[:idx]
			isGlobal := IsGlobalVar(varName)
			if appUpper == "" {
				// Global pass: only include global variables
				if isGlobal {
					filteredRemaining = append(filteredRemaining, line)
				}
			} else {
				// App pass: only include variables for this app
				if strings.HasPrefix(strings.ToUpper(varName), appUpper+"__") {
					filteredRemaining = append(filteredRemaining, line)
				}
			}
		}
	}

	if len(filteredRemaining) > 0 {
		if appUpper == "" || !appIsUserDefined {
			// Add the "User Defined" heading
			var headingTitle string
			if appUpper != "" {
				headingTitle = appNiceName + userDefinedVarsTag
			} else {
				headingTitle = globalVarsHeading + userDefinedGlobalVarsTag
			}

			headingLines := []string{
				"",
				headingTitle,
				"",
			}

			for _, hl := range headingLines {
				trimmed := strings.TrimRight(hl, " \t")
				if trimmed == "" {
					formattedEnvLines = append(formattedEnvLines, "###")
				} else {
					formattedEnvLines = append(formattedEnvLines, "### "+trimmed)
				}
			}
		}

		// Add the user defined variables
		for _, line := range filteredRemaining {
			idx := strings.Index(line, "=")
			if idx > 0 {
				varName := line[:idx]
				if lineIndex, exists := formattedEnvVarIndex[varName]; exists {
					// Variable already exists (from another app perhaps? or previous pass)
					// Update its value
					formattedEnvLines[lineIndex] = line
				} else {
					// Variable is new, add it
					formattedEnvLines = append(formattedEnvLines, line)
					formattedEnvVarIndex[varName] = len(formattedEnvLines) - 1
				}
			}
		}
		formattedEnvLines = append(formattedEnvLines, "")
	} else if len(formattedEnvLines) == 0 {
		formattedEnvLines = append(formattedEnvLines, "")
	}

	return formattedEnvLines, nil
}

// FormatLinesForApp convenience wrapper.
func FormatLinesForApp(ctx context.Context, currentEnvFile, appName, templatesDir, composeEnvFile string) ([]string, error) {
	var defaultEnvFile string
	appUpper := strings.ToUpper(appName)
	if !IsAppUserDefined(ctx, appUpper, composeEnvFile) {
		// We need to know if we are formatting for global .env or app-specific .env.app.appName
		// FormatLinesForApp is usually called for the global .env sectional pass in env_update logic.
		// Wait, FormatLinesForApp is also used in Update?
		// Let's re-examine FormatLinesForApp usage in update.go.
		// In update.go, it is indeed used in the app sections pass for the GLOBAL .env file.
		// So it should use ".env" as the suffix.

		processedInstanceFile, err := AppInstanceFile(ctx, appUpper, constants.EnvFileName)
		if err == nil && processedInstanceFile != "" {
			defaultEnvFile = processedInstanceFile
		}
	}
	return FormatLines(ctx, currentEnvFile, defaultEnvFile, appUpper, composeEnvFile)
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
		if appName != "" && IsAppNameValid(appName) {
			appMap[appName] = true
		}
	}

	var result []string
	for app := range appMap {
		result = append(result, app)
	}
	slices.Sort(result)
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
