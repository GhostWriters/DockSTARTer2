package apps

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/env"
	"DockSTARTer2/internal/paths"
	"os"
	"path/filepath"
	"strings"
)

// appname_to_baseappname extracts the base application name from a potentially instanced app name.
// Example: "RADARR__4K" returns "radarr", "RADARR" returns "radarr"
// Always returns lowercase to match Bash's 'local -l' behavior
func appname_to_baseappname(appname string) string {
	appname = strings.ToLower(appname)
	if strings.Contains(appname, "__") {
		parts := strings.Split(appname, "__")
		return parts[0]
	}
	return appname
}

// appname_to_instancename extracts the instance suffix from an instanced app name.
// Example: "RADARR__4K" returns "4k", "RADARR" returns ""
// Always returns lowercase to match Bash's 'local -l' behavior
func appname_to_instancename(appname string) string {
	appname = strings.ToLower(appname)
	if strings.Contains(appname, "__") {
		parts := strings.Split(appname, "__")
		return parts[1]
	}
	return ""
}

// IsUserDefined checks if an app is user-defined (not built-in OR missing ENABLED var).
// Mirrors app_is_user_defined.sh functionality.
//
// Logic from Bash:
// - If app is not builtin, it's user-defined
// - If app is builtin but has no __ENABLED var, it's user-defined
func IsUserDefined(appname string, envFile string) bool {
	appUpper := strings.ToUpper(appname)

	// Check if app is builtin
	baseapp := appname_to_baseappname(appUpper)
	templatesDir := paths.GetTemplatesDir()
	templatePath := filepath.Join(templatesDir, ".apps", baseapp)
	_, err := os.Stat(templatePath)
	isBuiltin := err == nil

	// If not builtin, it's user-defined
	if !isBuiltin {
		return true
	}

	// If builtin but no ENABLED var exists, it's user-defined
	if !env.VarExists(appUpper+"__ENABLED", envFile) {
		return true
	}

	return false
}

// IsAdded checks if an app is both builtin and has an __ENABLED variable.
// Mirrors app_is_added.sh functionality.
func IsAdded(appname string, envFile string) bool {
	appUpper := strings.ToUpper(appname)
	return IsBuiltin(appUpper) && env.VarExists(appUpper+"__ENABLED", envFile)
}

// IsRunnable checks if an app has the required YML template files for the current architecture.
// Mirrors app_is_runnable.sh functionality.
func IsRunnable(appname string, conf config.AppConfig) bool {
	basename := appname_to_baseappname(appname)

	// Get template folder
	templatesDir := paths.GetTemplatesDir()
	templateFolder := filepath.Join(templatesDir, ".apps", basename)

	// Check for main.yml
	mainYml := filepath.Join(templateFolder, basename+".yml")
	if _, err := os.Stat(mainYml); err != nil {
		return false
	}

	// Check for arch-specific yml
	archYml := filepath.Join(templateFolder, basename+"."+conf.Arch+".yml")
	if _, err := os.Stat(archYml); err != nil {
		return false
	}

	return true
}

// IsNonDeprecated is a wrapper that returns the opposite of IsDeprecated.
// Mirrors app_is_nondeprecated.sh functionality.
func IsNonDeprecated(appname string) bool {
	return !IsDeprecated(appname)
}

// VarsList returns a list of variable names for the specified app.
// Mirrors appvars_list.sh functionality.
//
// If appName ends with ":", it lists variables from the app's .env.app.appname file.
// Otherwise, it lists variables from the global .env file using the Bash regex pattern.
func VarsList(appName string, conf config.AppConfig) ([]string, error) {
	envFile := filepath.Join(conf.ComposeFolder, ".env")

	// If appName ends with ":", use app-specific env file
	if strings.HasSuffix(appName, ":") {
		appName = strings.TrimSuffix(appName, ":")
		appLower := strings.ToLower(appName)
		appEnvFile := filepath.Join(conf.ComposeFolder, ".env.app."+appLower)
		vars, err := env.ListVars(appEnvFile)
		if err != nil {
			return nil, err
		}
		var result []string
		for k := range vars {
			result = append(result, k)
		}
		return result, nil
	}

	// Otherwise, use AppVarsLines to filter from global .env
	content, err := os.ReadFile(envFile)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	appVarLines := env.AppVarsLines(strings.ToUpper(appName), lines)

	var result []string
	for _, line := range appVarLines {
		// Extract variable name from line
		idx := strings.Index(line, "=")
		if idx > 0 {
			varName := strings.TrimSpace(line[:idx])
			result = append(result, varName)
		}
	}

	return result, nil
}
