package appenv

import (
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/envutil"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"DockSTARTer2/internal/strutil"
)

// FormatLinesCore processes environment variable lines to match DockSTARTer formatting.
// currentLines contains the staged variable values (already in memory).
// defaultLines contains the template lines (nil = no template).
// envLines, when non-nil, is scanned for APPNAME__ENABLED to determine the heading.
// When envLines is nil, composeEnvFile is read from disk instead (non-editor callers).
// For the global .env tab, pass currentLines as envLines.
// For the .env.app.appname tab, pass the global tab's staged lines as envLines.
func FormatLinesCore(ctx context.Context, currentLines, defaultLines, envLines []string, appName, composeEnvFile string) []string {
	appUpper := strings.ToUpper(appName)

	// Local variables for tags (Parity with env_format_lines.sh lines 15-20)
	globalVarsHeading := "Global Variables"
	appDeprecatedTag := " [*DEPRECATED*]"
	appDisabledTag := " (Disabled)"
	appUserDefinedTag := " (User Defined)"
	appUserDefinedVarsTag := " (User Defined Variables)"
	userDefinedGlobalVarsTag := " (User Defined)"

	var formattedEnvLines []string

	// Resolve app status once — used by both the heading block and template inclusion.
	var appIsUserDefined, appEnabled bool
	var appDescription string
	if appUpper != "" {
		if envLines != nil {
			appIsUserDefined = IsAppUserDefinedFromLines(ctx, appUpper, envLines)
			appEnabled = IsAppEnabledFromLines(appUpper, envLines)
			appDescription = GetDescriptionFromLines(ctx, appUpper, envLines)
		} else {
			appIsUserDefined = IsAppUserDefined(ctx, appUpper, composeEnvFile)
			appEnabled = IsAppEnabled(appUpper, composeEnvFile)
			appDescription = GetDescription(ctx, appUpper, composeEnvFile)
		}
	}

	// 2. Add App Heading if APPNAME is specified (Parity with env_format_lines.sh lines 31-56)
	if appUpper != "" {
		appNameNice := GetNiceName(ctx, appUpper)

		headingTitle := appNameNice
		if appIsUserDefined {
			headingTitle += appUserDefinedTag
		} else {
			if IsAppDeprecated(ctx, appUpper) {
				headingTitle += appDeprecatedTag
			}
			if !appEnabled {
				headingTitle += appDisabledTag
			}
		}

		// Parity lines 46-55: Adds ### wrapping including before/after descriptions.
		// Description is only shown for built-in apps (not user-defined).
		formattedEnvLines = append(formattedEnvLines, "###")
		formattedEnvLines = append(formattedEnvLines, "### "+headingTitle)
		formattedEnvLines = append(formattedEnvLines, "###")
		if appDescription != "" {
			descLines := strutil.WordWrapToSlice(console.StripSemanticTags(appDescription), 75)
			for _, line := range descLines {
				trimmed := strings.TrimRight(line, " \r\t")
				if trimmed == "" {
					formattedEnvLines = append(formattedEnvLines, "###")
				} else {
					formattedEnvLines = append(formattedEnvLines, "### "+trimmed)
				}
			}
			formattedEnvLines = append(formattedEnvLines, "###")
		}
	}

	// 3. Add Template Contents Verbatim (Parity with env_format_lines.sh lines 57-64)
	// Skip the template when the app is user-defined — it has no built-in template structure.
	processedTemplate := false
	if len(defaultLines) > 0 && !appIsUserDefined {
		formattedEnvLines = append(formattedEnvLines, defaultLines...)
		processedTemplate = true
	}

	// Bash line 62: adds a blank ONLY IF template section was processed/found
	if processedTemplate && len(formattedEnvLines) > 0 {
		formattedEnvLines = append(formattedEnvLines, "")
	}

	// 4. Index existing variables in formattedEnvLines (Parity lines 66-78)
	varRe := regexp.MustCompile(`^([A-Za-z0-9_]+)\s*=`)
	formattedEnvVarIndex := make(map[string]int)
	for i, line := range formattedEnvLines {
		matches := varRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			formattedEnvVarIndex[matches[1]] = i
		}
	}

	// 5. Update values from currentLines (Parity lines 80-91)
	if len(currentLines) > 0 {
		consumed := make([]bool, len(currentLines))
		for i, line := range currentLines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) > 1 {
				varName := strings.TrimSpace(parts[0])
				if idx, exists := formattedEnvVarIndex[varName]; exists {
					formattedEnvLines[idx] = line
					consumed[i] = true
				}
			}
		}

		// 6. Handle remaining currentLines (User Defined) (Parity lines 93-124)
		var remaining []string
		for i, line := range currentLines {
			if !consumed[i] {
				remaining = append(remaining, line)
			}
		}

		if len(remaining) > 0 {
			// Add User Defined heading (Parity lines 93-109)
			var appIsUserDefined2 bool
			if envLines != nil {
				appIsUserDefined2 = IsAppUserDefinedFromLines(ctx, appUpper, envLines)
			} else {
				appIsUserDefined2 = IsAppUserDefined(ctx, appUpper, composeEnvFile)
			}
			if appUpper == "" || !appIsUserDefined2 {
				headingTitle := ""
				if appUpper != "" {
					headingTitle = GetNiceName(ctx, appUpper) + appUserDefinedVarsTag
				} else {
					headingTitle = globalVarsHeading + userDefinedGlobalVarsTag
				}

				// Parity lines 102-108
				formattedEnvLines = append(formattedEnvLines, "###")
				formattedEnvLines = append(formattedEnvLines, "### "+headingTitle)
				formattedEnvLines = append(formattedEnvLines, "###")
			}

			// Add the remaining variables (Parity lines 111-122)
			for _, line := range remaining {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) > 1 {
					varName := strings.TrimSpace(parts[0])
					// Parity line 116 check: update if exists (handle duplicates in currentLines)
					if idx, exists := formattedEnvVarIndex[varName]; exists {
						formattedEnvLines[idx] = line
					} else {
						// Variable is new, add it (Parity line 119)
						formattedEnvLines = append(formattedEnvLines, line)
						formattedEnvVarIndex[varName] = len(formattedEnvLines) - 1
					}
				}
			}
			// Parity line 123
			formattedEnvLines = append(formattedEnvLines, "")
		}
	} else {
		// Parity line 126 fallback
		formattedEnvLines = append(formattedEnvLines, "")
	}

	return formattedEnvLines
}

// ReadDefaultLines loads the default/template lines for the given file path.
// Returns nil if the file is not found or cannot be read.
func ReadDefaultLines(defaultEnvFile string) []string {
	if defaultEnvFile == "" {
		return nil
	}
	if filepath.Base(defaultEnvFile) == constants.EnvExampleFileName {
		data, err := assets.GetDefaultEnv()
		if err != nil {
			return nil
		}
		lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		// readarray -t strips the final newline character from the file
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		return lines
	}
	if info, err := os.Stat(defaultEnvFile); err != nil || info.IsDir() {
		return nil
	}
	data, err := os.ReadFile(defaultEnvFile)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// FormatLines processes environment variable lines to match DockSTARTer formatting.
// Matches env_format_lines.sh exactly. Reads files from disk and delegates to FormatLinesCore.
func FormatLines(ctx context.Context, currentEnvFile, defaultEnvFile, appName, composeEnvFile string) ([]string, error) {
	var currentLines []string
	if currentEnvFile != "" {
		var err error
		currentLines, err = envutil.ReadLines(currentEnvFile)
		if err != nil {
			return nil, err
		}
	}
	defaultLines := ReadDefaultLines(defaultEnvFile)
	return FormatLinesCore(ctx, currentLines, defaultLines, nil, appName, composeEnvFile), nil
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
