package appenv

import (
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/envutil"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// FormatLines processes environment variable lines to match DockSTARTer formatting.
// Matches env_format_lines.sh exactly.
func FormatLines(ctx context.Context, currentEnvFile, defaultEnvFile, appName, composeEnvFile string) ([]string, error) {
	appUpper := strings.ToUpper(appName)

	// Local variables for tags (Parity with env_format_lines.sh lines 15-20)
	globalVarsHeading := "Global Variables"
	appDeprecatedTag := " [*DEPRECATED*]"
	appDisabledTag := " (Disabled)"
	appUserDefinedTag := " (User Defined)"
	appUserDefinedVarsTag := " (User Defined Variables)"
	userDefinedGlobalVarsTag := " (User Defined)"

	// 1. Load CurrentEnvLines (Parity with env_format_lines.sh lines 22-25)
	var currentEnvLines []string
	if currentEnvFile != "" {
		var err error
		currentEnvLines, err = envutil.ReadLines(currentEnvFile)
		if err != nil {
			return nil, err
		}
	}

	var formattedEnvLines []string

	// 2. Add App Heading if APPNAME is specified (Parity with env_format_lines.sh lines 31-56)
	if appUpper != "" {
		appIsUserDefined := IsAppUserDefined(ctx, appUpper, composeEnvFile)
		appNameNice := GetNiceName(ctx, appUpper)
		appDescription := GetDescription(ctx, appUpper, composeEnvFile)

		headingTitle := appNameNice
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

		formattedEnvLines = append(formattedEnvLines, "###")
		formattedEnvLines = append(formattedEnvLines, "### "+headingTitle)
		formattedEnvLines = append(formattedEnvLines, "###")
		if appDescription != "" {
			descLines := strings.Split(appDescription, "\n")
			for _, line := range descLines {
				trimmed := strings.TrimRight(line, " \r\t")
				formattedEnvLines = append(formattedEnvLines, "### "+trimmed)
			}
			formattedEnvLines = append(formattedEnvLines, "###")
		}
	}

	// 3. Add Template Contents Verbatim (Parity with env_format_lines.sh lines 57-64)
	if defaultEnvFile != "" {
		var templateLines []string
		// Use embedded asset for global .env.example
		if filepath.Base(defaultEnvFile) == constants.EnvExampleFileName {
			data, err := assets.GetDefaultEnv()
			if err == nil {
				templateLines = strings.Split(string(data), "\n")
				// readarray -t strips the final newline character from the file
				if len(templateLines) > 0 && templateLines[len(templateLines)-1] == "" {
					templateLines = templateLines[:len(templateLines)-1]
				}
			}
		} else {
			// Read app templates from disk
			if data, err := os.ReadFile(defaultEnvFile); err == nil {
				templateLines = strings.Split(string(data), "\n")
				if len(templateLines) > 0 && templateLines[len(templateLines)-1] == "" {
					templateLines = templateLines[:len(templateLines)-1]
				}
			}
		}

		if len(templateLines) > 0 {
			formattedEnvLines = append(formattedEnvLines, templateLines...)
			// Bash line 62: adds a blank if template was added
			formattedEnvLines = append(formattedEnvLines, "")
		}
	}

	// 4. Index existing variables in formattedEnvLines (Parity lines 66-78)
	varRe := regexp.MustCompile(`^([A-Za-z0-9_]+)=`)
	formattedEnvVarIndex := make(map[string]int)
	for i, line := range formattedEnvLines {
		matches := varRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			formattedEnvVarIndex[matches[1]] = i
		}
	}

	// 5. Update values from CurrentEnvLines (Parity lines 80-91)
	if len(currentEnvLines) > 0 {
		consumed := make([]bool, len(currentEnvLines))
		for i, line := range currentEnvLines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) > 1 {
				varName := parts[0]
				if idx, exists := formattedEnvVarIndex[varName]; exists {
					formattedEnvLines[idx] = line
					consumed[i] = true
				}
			}
		}

		// 6. Handle remaining CurrentEnvLines (User Defined) (Parity lines 93-124)
		var remaining []string
		for i, line := range currentEnvLines {
			if !consumed[i] {
				remaining = append(remaining, line)
			}
		}

		if len(remaining) > 0 {
			// Add User Defined heading
			headingTitle := ""
			if appUpper != "" {
				headingTitle = GetNiceName(ctx, appUpper) + appUserDefinedVarsTag
			} else {
				headingTitle = globalVarsHeading + userDefinedGlobalVarsTag
			}

			formattedEnvLines = append(formattedEnvLines, "###")
			formattedEnvLines = append(formattedEnvLines, "### "+headingTitle)
			formattedEnvLines = append(formattedEnvLines, "###")
			formattedEnvLines = append(formattedEnvLines, remaining...)
			formattedEnvLines = append(formattedEnvLines, "")
		}
	} else {
		// Parity line 126 fallback
		formattedEnvLines = append(formattedEnvLines, "")
	}

	return formattedEnvLines, nil
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
