package appenv

import (
	"context"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// LabelsFile structure for unmarshaling labels.yml
type LabelsFile struct {
	Services map[string]struct {
		Labels map[string]string `yaml:"labels"`
	} `yaml:"services"`
}

// AppNameToBaseAppName extracts the base application name.
func AppNameToBaseAppName(appName string) string {
	if strings.Contains(appName, "__") {
		parts := strings.Split(appName, "__")
		return parts[0]
	}
	return appName
}

// AppNameToInstanceName extracts the instance suffix from an app name.
func AppNameToInstanceName(appName string) string {
	if strings.Contains(appName, "__") {
		parts := strings.SplitN(appName, "__", 2)
		return parts[1]
	}
	return ""
}

// VarNameToAppName returns the DS application name based on the variable name passed.
// Mirrors varname_to_appname.sh: if the name contains ":", the part before the colon
// is the app name; otherwise the app name is extracted via the double-underscore pattern.
func VarNameToAppName(varName string) string {
	// APPNAME:VARNAME format (used for .env.app.* vars)
	if idx := strings.Index(varName, ":"); idx > 0 {
		return strings.ToUpper(varName[:idx])
	}
	if !strings.Contains(varName, "__") {
		return ""
	}
	// Regex matches:
	// Group 1: The App Name (can be builtin or instance)
	// __: The separator
	// Group 2: The starting character of the variable name (can be _ or alphanumeric)
	// followed by the rest.
	// 1. Try to match APP__INST__VAR
	re3 := regexp.MustCompile(`^([A-Z][A-Z0-9]*)__([A-Z0-9]+)__([A-Za-z0-9_].*)`)
	m3 := re3.FindStringSubmatch(varName)
	if len(m3) > 3 {
		return m3[1] + "__" + m3[2]
	}

	// 2. Try to match APP__VAR
	re2 := regexp.MustCompile(`^([A-Z][A-Z0-9]*)__([A-Za-z0-9_].*)`)
	m2 := re2.FindStringSubmatch(varName)
	if len(m2) > 2 {
		return m2[1]
	}

	return ""
}

// CapitalizeFirstLetter lowercases s then capitalizes the first Unicode letter,
// skipping any leading digits or non-letter characters.
// Examples: "4K" → "4K", "4k" → "4K", "23kkk" → "23Kkk", "abc" → "Abc".
func CapitalizeFirstLetter(s string) string {
	s = strings.ToLower(s)
	for i, r := range s {
		if unicode.IsLetter(r) {
			return s[:i] + string(unicode.ToUpper(r)) + s[i+utf8.RuneLen(r):]
		}
	}
	return s
}

// InstanceDisplayName returns the display label for an instance sub-row.
// If appName has no instance suffix it returns baseNiceName unchanged;
// otherwise it appends "__<TitleCasedSuffix>" (e.g. "Radarr__4k").
func InstanceDisplayName(baseNiceName, appName string) string {
	suffix := AppNameToInstanceName(appName)
	if suffix == "" {
		return baseNiceName
	}
	return baseNiceName + "__" + CapitalizeFirstLetter(suffix)
}

// GetNiceName returns a nicely formatted app name.
// Checks template labels first, then falls back to title casing.
func GetNiceName(ctx context.Context, appName string) string {
	// 1. Try to get from labels
	labelsFile, err := AppInstanceFile(ctx, appName, "*.labels.yml")
	if err == nil && labelsFile != "" {
		content, err := os.ReadFile(labelsFile)
		if err == nil {
			var labels LabelsFile
			if err := yaml.Unmarshal(content, &labels); err == nil {
				for _, service := range labels.Services {
					if name, ok := service.Labels["com.dockstarter.appinfo.nicename"]; ok {
						return strings.Trim(name, `"' `)
					}
				}
			}
		}
	}

	// 2. Fallback
	appUpper := strings.ToUpper(appName)
	parts := strings.Split(appUpper, "__")
	var niceParts []string
	for _, part := range parts {
		niceParts = append(niceParts, CapitalizeFirstLetter(part))
	}
	return strings.Join(niceParts, " ")
}

// GetDescription returns the description of an application.
func GetDescription(ctx context.Context, appName string, envFile string) string {
	// Check if user defined (not built-in OR missing ENABLED var)
	if IsAppUserDefined(ctx, appName, envFile) {
		return GetNiceName(ctx, appName) + " is a user defined application"
	}

	// Try to get from labels
	labelsFile, err := AppInstanceFile(ctx, appName, "*.labels.yml")
	if err != nil || labelsFile == "" {
		return "! Missing description !"
	}

	content, err := os.ReadFile(labelsFile)
	if err != nil {
		return "! Missing description !"
	}

	var labels LabelsFile
	if err := yaml.Unmarshal(content, &labels); err != nil {
		return "! Missing description !"
	}

	for _, service := range labels.Services {
		if desc, ok := service.Labels["com.dockstarter.appinfo.description"]; ok {
			return strings.Trim(desc, `"' `)
		}
	}

	return "! Missing description !"
}

// GetDescriptionFromTemplate returns the description of an application.
func GetDescriptionFromTemplate(ctx context.Context, appName string, envFile string) string {
	// Check if user defined (not built-in OR missing ENABLED var)
	if !IsAppBuiltIn(appName) {
		return GetNiceName(ctx, appName) + " is a user defined application"
	}

	// Try to get from labels
	labelsFile, err := AppInstanceFile(ctx, appName, "*.labels.yml")
	if err != nil || labelsFile == "" {
		return "! Missing description !"
	}

	content, err := os.ReadFile(labelsFile)
	if err != nil {
		return "! Missing description !"
	}

	var labels LabelsFile
	if err := yaml.Unmarshal(content, &labels); err != nil {
		return "! Missing description !"
	}

	for _, service := range labels.Services {
		if desc, ok := service.Labels["com.dockstarter.appinfo.description"]; ok {
			return strings.Trim(desc, `"' `)
		}
	}

	return "! Missing description !"
}
