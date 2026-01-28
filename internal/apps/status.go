package apps

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/env"
	"DockSTARTer2/internal/paths"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// IsEnabled checks if an app is enabled in the .env file.
func IsEnabled(app, envFile string) bool {
	if !IsBuiltin(app) {
		return false
	}
	val, _ := env.Get(app+"__ENABLED", envFile)
	return IsTrue(val)
}

// IsReferenced checks if an app is referenced in any configuration.
// Mirrors app_is_referenced.sh functionality.
func IsReferenced(ctx context.Context, app string, conf config.AppConfig) bool {
	appUpper := strings.ToUpper(app)
	appLower := strings.ToLower(app)

	// 1. Check for app variables in the global .env file
	// Bash: run_script 'appvars_list' "${APPNAME}"
	vars, err := VarsList(appUpper, conf)
	if err == nil && len(vars) > 0 {
		return true
	}

	// 2. Check for app variables in the .env.app.appname file
	// Bash: run_script 'appvars_list' "${APPNAME}:"
	varsApp, err := VarsList(appUpper+":", conf)
	if err == nil && len(varsApp) > 0 {
		return true
	}

	// 3. Check for uncommented reference to .env.app.appname in the override file
	// Bash regex: ^(?:[^#]*)(?:\s|^)(?<Q>['"]?)(?:[.]\/)?${SearchString}(?=\k<Q>(?:\s|$))
	overrideFile := filepath.Join(conf.ComposeFolder, "docker-compose.override.yml")
	if _, err := os.Stat(overrideFile); err == nil {
		content, err := os.ReadFile(overrideFile)
		if err == nil {
			// Pattern matches uncommented lines with .env.app.appname (with optional quotes and ./)
			appEnvFile := "\\.env\\.app\\." + regexp.QuoteMeta(appLower)
			// Handle both quoted and unquoted, with optional ./
			pattern := fmt.Sprintf(`(?m)^(?:[^#]*)(?:\s|^)(?:['"]?)(?:[.]\/)?%s(?:['"]?)(?:\s|$)`, appEnvFile)
			re := regexp.MustCompile(pattern)
			if re.MatchString(string(content)) {
				return true
			}
		}
	}

	return false
}

// IsDisabled checks if an app has __ENABLED variable set to false/no/off.
// Mirrors app_is_disabled.sh functionality.
func IsDisabled(appname string, envFile string) bool {
	appUpper := strings.ToUpper(appname)
	enabledVar := appUpper + "__ENABLED"

	val, err := env.Get(enabledVar, envFile)
	if err != nil {
		// Variable doesn't exist, not explicitly disabled
		return false
	}

	// Check if value is false/no/off (opposite of IsTrue)
	return !IsTrue(val)
}

// IsTrue helper for boolean strings.
func IsTrue(val string) bool {
	v := strings.ToUpper(strings.Trim(val, `"' `))
	return v == "1" || v == "ON" || v == "TRUE" || v == "YES"
}

// Status returns a string describing the current status of an app.
func Status(ctx context.Context, app string, conf config.AppConfig) string {
	appUpper := strings.ToUpper(app)
	envFile := filepath.Join(conf.ComposeFolder, ".env")
	nice := NiceName(appUpper)

	if !IsAppNameValid(appUpper) {
		return fmt.Sprintf("{{_App_}}%s{{|-|}} is not a valid application name.", nice)
	}

	if IsReferenced(ctx, appUpper, conf) {
		addedVars, _ := env.ListVars(envFile)
		isAdded := false
		for k := range addedVars {
			if strings.HasPrefix(k, appUpper+"__ENABLED") {
				isAdded = true
				break
			}
		}

		if isAdded {
			if IsEnabled(appUpper, envFile) {
				return fmt.Sprintf("{{_App_}}%s{{|-|}} is enabled.", nice)
			}
			return fmt.Sprintf("{{_App_}}%s{{|-|}} is disabled.", nice)
		}
		return fmt.Sprintf("{{_App_}}%s{{|-|}} is referenced.", nice)
	}

	if IsBuiltin(appUpper) {
		return fmt.Sprintf("{{_App_}}%s{{|-|}} is not added.", nice)
	}

	return fmt.Sprintf("{{_App_}}%s{{|-|}} does not exist.", nice)
}

// NiceName returns the "NiceName" of the app.
// Matches Bash app_nicename: checks template labels or defaults to title case.
func NiceName(app string) string {
	base := appname_to_baseappname(app)
	instance := appname_to_instancename(app)

	// Default: titleize base (first letter upper, rest lower)
	displayName := ""
	if base != "" {
		displayName = strings.ToUpper(base[:1]) + strings.ToLower(base[1:])
	}

	// Try to get from template
	templatesDir := paths.GetTemplatesDir()
	labelsFile := filepath.Join(templatesDir, ".apps", strings.ToLower(base), strings.ToLower(base)+".labels.yml")
	if _, err := os.Stat(labelsFile); err == nil {
		content, err := os.ReadFile(labelsFile)
		if err == nil {
			re := regexp.MustCompile(`(?m)^\s*com\.dockstarter\.appinfo\.nicename:\s*(.*)`)
			matches := re.FindStringSubmatch(string(content))
			if len(matches) > 1 {
				displayName = strings.Trim(matches[1], `"' `)
			}
		}
	}

	// Remove any placeholders from the template's nicename (mirroring Bash behavior of using processed base file)
	displayName = strings.ReplaceAll(displayName, "<__INSTANCE>", "")
	displayName = strings.ReplaceAll(displayName, "<__Instance>", "")
	displayName = strings.ReplaceAll(displayName, "<__instance>", "")

	if instance != "" {
		displayName += "__" + strings.ToUpper(instance[:1]) + strings.ToLower(instance[1:])
	}

	return displayName
}
