package appenv

import (
	"DockSTARTer2/internal/envutil"
	"DockSTARTer2/internal/paths"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FormatLines formats environment variable lines with proper headers and structure.
func FormatLines(ctx context.Context, currentEnvFile, defaultEnvFile, appName, composeEnvFile string) ([]string, error) {
	var result []string

	const (
		globalVarsHeading        = "Global Variables"
		appDeprecatedTag         = " [*DEPRECATED*]"
		appDisabledTag           = " (Disabled)"
		appUserDefinedTag        = " (User Defined)"
		userDefinedVarsTag       = " (User Defined Variables)"
		userDefinedGlobalVarsTag = " (User Defined)"
	)

	appIsUserDefined := false
	var appNiceName string

	if appName != "" {
		appIsUserDefined = IsAppUserDefined(appName, composeEnvFile)
		appNiceName = GetNiceName(ctx, appName)
		appDescription := GetDescription(ctx, appName, composeEnvFile)

		headingTitle := appNiceName
		if appIsUserDefined {
			headingTitle += appUserDefinedTag
		} else {
			if IsAppDeprecated(ctx, appName) {
				headingTitle += appDeprecatedTag
			}
			if !IsAppEnabled(appName, composeEnvFile) {
				headingTitle += appDisabledTag
			}
		}

		wrapped := wordWrap(appDescription, 75)
		result = append(result, "###")
		result = append(result, "### "+headingTitle)
		result = append(result, "###")
		for _, line := range wrapped {
			result = append(result, "### "+line)
		}
		result = append(result, "###")
	}

	if defaultEnvFile != "" {
		if info, err := os.Stat(defaultEnvFile); err == nil && !info.IsDir() {
			content, err := os.ReadFile(defaultEnvFile)
			if err == nil {
				lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
				result = append(result, lines...)
				if len(result) > 0 {
					result = append(result, "")
				}
			}
		}
	}

	formattedEnvVarIndex := make(map[string]int)
	varRe := regexp.MustCompile(`^([A-Za-z0-9_]+)=`)
	for i, line := range result {
		matches := varRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			formattedEnvVarIndex[matches[1]] = i
		}
	}

	var currentEnvLines []string
	if currentEnvFile != "" {
		if info, err := os.Stat(currentEnvFile); err == nil && !info.IsDir() {
			lines, err := envutil.ReadLines(currentEnvFile)
			if err == nil {
				currentEnvLines = lines
			}
		}
	}

	var remainingVars []string
	for _, line := range currentEnvLines {
		matches := varRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			varName := matches[1]
			if idx, exists := formattedEnvVarIndex[varName]; exists {
				result[idx] = line
			} else {
				remainingVars = append(remainingVars, line)
			}
		}
	}

	if len(remainingVars) > 0 {
		shouldAddHeader := false
		if appName != "" {
			shouldAddHeader = !appIsUserDefined
		} else {
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

		for _, line := range remainingVars {
			result = append(result, line)
		}
		result = append(result, "")
	} else if len(result) > 0 {
		result = append(result, "")
	}

	return result, nil
}

// FormatLinesForApp convenience wrapper.
func FormatLinesForApp(ctx context.Context, currentEnvFile, appName, templatesDir, composeEnvFile string) ([]string, error) {
	var defaultEnvFile string
	if !IsAppUserDefined(appName, composeEnvFile) {
		instancesDir := paths.GetInstancesDir()
		processedInstanceFile := filepath.Join(instancesDir, appName, ".env")
		if _, err := os.Stat(processedInstanceFile); err == nil {
			defaultEnvFile = processedInstanceFile
		}
	}
	return FormatLines(ctx, currentEnvFile, defaultEnvFile, appName, composeEnvFile)
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

// FormatDescriptions lists applications and their descriptions.
func FormatDescriptions(ctx context.Context, apps []string, envFile string) {
	if len(apps) == 0 {
		fmt.Println("   None")
		return
	}

	for _, appName := range apps {
		desc := GetDescription(ctx, appName, envFile)
		niceName := GetNiceName(ctx, appName)
		fmt.Printf("   %-20s %s\n", appName, niceName)
		fmt.Printf("      %s\n", desc)
	}
}
