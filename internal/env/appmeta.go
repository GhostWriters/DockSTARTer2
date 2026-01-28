package env

// AppMeta provides metadata checking functions that can access paths/config.
// These are separate from appstatus.go to avoid import issues.

import (
	"DockSTARTer2/internal/paths"
	"os"
	"path/filepath"
	"strings"
)

// IsBuiltinApp checks if an app has a template folder.
// Mirrors app_is_builtin.sh logic.
func IsBuiltinApp(appname string) bool {
	appLower := strings.ToLower(appname)
	baseApp := appLower
	if idx := strings.Index(appLower, "__"); idx > 0 {
		baseApp = appLower[:idx]
	}

	templatesDir := paths.GetTemplatesDir()
	templatePath := filepath.Join(templatesDir, ".apps", baseApp)

	if info, err := os.Stat(templatePath); err == nil && info.IsDir() {
		return true
	}
	return false
}

// IsDeprecatedApp checks if an app template has deprecated.app marker file.
// Mirrors app_is_deprecated.sh logic.
func IsDeprecatedApp(appname string) bool {
	appLower := strings.ToLower(appname)
	baseApp := appLower
	if idx := strings.Index(appLower, "__"); idx > 0 {
		baseApp = appLower[:idx]
	}

	templatesDir := paths.GetTemplatesDir()
	deprecatedMarker := filepath.Join(templatesDir, ".apps", baseApp, "deprecated.app")

	if _, err := os.Stat(deprecatedMarker); err == nil {
		return true
	}
	return false
}

// IsUserDefinedApp checks if an app is user-defined (has vars but no template).
// Full implementation matching app_is_user_defined.sh.
func IsUserDefinedApp(appname string, envFile string) bool {
	// Bash logic: !app_is_builtin && app_has_vars
	if IsBuiltinApp(appname) {
		return false
	}

	// Check if has vars in env
	appUpper := strings.ToUpper(appname)
	enabledVar := appUpper + "__ENABLED"

	_, err := Get(enabledVar, envFile)
	return err == nil // Has vars and not builtin = user defined
}
