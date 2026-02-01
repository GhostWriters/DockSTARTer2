package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/paths"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AppStatus returns the status of an app.
// Mirrors app_status.sh functionality.
func AppStatus(ctx context.Context, app string, conf config.AppConfig) string {
	if !IsAppBuiltIn(app) {
		return "Builtin application template not found"
	}

	// Bash checks if instance folder exists (line 17 in app_list_enabled.sh)
	// Instance folder path: ${INSTANCES_FOLDER}/${appname}
	baseapp := AppNameToBaseAppName(app)
	instanceFolder := filepath.Join(paths.GetInstancesDir(), strings.ToLower(baseapp))
	if _, err := os.Stat(instanceFolder); os.IsNotExist(err) {
		return "Not installed"
	}

	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if IsAppAdded(ctx, app, envFile) {
		return "Enabled"
	}

	return "Disabled"
}

// Status returns a string describing the current status of an app.
func Status(ctx context.Context, app string, conf config.AppConfig) string {
	appUpper := strings.ToUpper(app)
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	nice := GetNiceName(ctx, appUpper)

	if !IsAppNameValid(appUpper) {
		return fmt.Sprintf("{{_App_}}%s{{|-|}} is not a valid application name.", nice)
	}

	if IsAppReferenced(ctx, appUpper, conf) {
		addedVars, _ := ListVars(envFile)
		isAdded := false
		for k := range addedVars {
			if strings.HasPrefix(k, appUpper+"__ENABLED") {
				isAdded = true
				break
			}
		}

		if isAdded {
			if IsAppEnabled(appUpper, envFile) {
				return fmt.Sprintf("{{_App_}}%s{{|-|}} is enabled.", nice)
			}
			return fmt.Sprintf("{{_App_}}%s{{|-|}} is disabled.", nice)
		}
		return fmt.Sprintf("{{_App_}}%s{{|-|}} is referenced.", nice)
	}

	if IsAppBuiltIn(appUpper) {
		return fmt.Sprintf("{{_App_}}%s{{|-|}} is not added.", nice)
	}

	return fmt.Sprintf("{{_App_}}%s{{|-|}} does not exist.", nice)
}
