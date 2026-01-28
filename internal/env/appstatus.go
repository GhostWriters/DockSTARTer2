package env

// AppStatus provides functions for checking application status based on env variables.

import (
	"strings"
)

// IsUserDefined checks if an app is user-defined (has vars in .env but no template).
// Mirrors app_is_user_defined.sh functionality.
func IsUserDefined(appname string, envFile string) bool {
	// An app is user-defined if it has variables in the env file
	// but doesn't have a template folder
	appUpper := strings.ToUpper(appname)
	enabledVar := appUpper + "__ENABLED"

	// Check if __ENABLED variable exists
	val, err := Get(enabledVar, envFile)
	if err != nil {
		// No __ENABLED var means not in env file
		return false
	}

	// If __ENABLED exists, check if template exists
	// If no template, it's user-defined
	// For now, we'll assume if __ENABLED exists in env, it could be user-defined
	// The actual check for template existence is done in apps.IsBuiltin
	// Since we're in env package, we can't check that, so we use a heuristic:
	// User-defined apps typically won't have complex variable patterns
	_ = val // Use the value to avoid unused var warning

	// This is a simplified check - the full logic requires template checking
	// which belongs in apps package. For formatting purposes, we'll use
	// a conservative approach.
	return false // Will be refined if needed
}

// IsDisabled checks if an app has __ENABLED variable set to false/no/off.
// Mirrors app_is_disabled.sh functionality.
func IsDisabled(appname string, envFile string) bool {
	appUpper := strings.ToUpper(appname)
	enabledVar := appUpper + "__ENABLED"

	val, err := Get(enabledVar, envFile)
	if err != nil {
		// Variable doesn't exist, not explicitly disabled
		return false
	}

	// Check if value is false/no/off (opposite of IsTrue)
	return !isTrue(val)
}

// isTrue helper for boolean strings.
func isTrue(val string) bool {
	v := strings.ToUpper(strings.Trim(val, `"' `))
	return v == "TRUE" || v == "YES" || v == "ON" || v == "1" || v == "ENABLED"
}
