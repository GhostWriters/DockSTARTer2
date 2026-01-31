package appenv

import (
	"context"
	"os"
	"regexp"
	"strings"

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
func VarNameToAppName(varName string) string {
	if !strings.Contains(varName, "__") {
		return ""
	}
	re := regexp.MustCompile(`^([A-Z][A-Z0-9]*(?:__[A-Z0-9]+)?)__[A-Za-z0-9]`)
	matches := re.FindStringSubmatch(varName)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
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
		niceParts = append(niceParts, strings.Title(strings.ToLower(part)))
	}
	return strings.Join(niceParts, " ")
}

// GetDescription returns the description of an application.
func GetDescription(ctx context.Context, appName string, envFile string) string {
	// Check if user defined
	if IsAppUserDefined(appName, envFile) {
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
