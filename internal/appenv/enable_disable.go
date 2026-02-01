package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"context"
	"path/filepath"
	"strings"
)

// Enable sets the __ENABLED variable to 'true' for the specified app(s).
// Mirrors enable_app.sh functionality.
func Enable(ctx context.Context, appNames []string, conf config.AppConfig) error {
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)

	for _, appName := range appNames {
		appUpper := strings.TrimSpace(strings.ToUpper(appName))
		niceName := GetNiceName(ctx, appUpper)

		if IsAppBuiltIn(appUpper) {
			enabledVar := appUpper + "__ENABLED"
			logger.Info(ctx, "Enabling application '{{_App_}}%s{{|-|}}'", niceName)
			logger.Notice(ctx, "Setting variable in '{{_File_}}%s{{|-|}}':", envFile)
			logger.Notice(ctx, "   {{_Var_}}%s='true'{{|-|}}", enabledVar)

			if err := Set(enabledVar, "true", envFile); err != nil {
				return err
			}
		} else {
			logger.Warn(ctx, "Application '{{_App_}}%s{{|-|}}' does not exist.", niceName)
		}
	}

	return nil
}

// Disable sets the __ENABLED variable to 'false' for the specified app(s).
// Mirrors disable_app.sh functionality.
func Disable(ctx context.Context, appNames []string, conf config.AppConfig) error {
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)

	for _, appName := range appNames {
		appUpper := strings.TrimSpace(strings.ToUpper(appName))
		niceName := GetNiceName(ctx, appUpper)

		if IsAppBuiltIn(appUpper) {
			enabledVar := appUpper + "__ENABLED"
			logger.Info(ctx, "Disabling application '{{_App_}}%s{{|-|}}'", niceName)
			logger.Notice(ctx, "Setting variable in '{{_File_}}%s{{|-|}}':", envFile)
			logger.Notice(ctx, "   {{_Var_}}%s='false'{{|-|}}", enabledVar)

			if err := Set(enabledVar, "false", envFile); err != nil {
				return err
			}
		} else {
			logger.Warn(ctx, "Application '{{_App_}}%s{{|-|}}' does not exist.", niceName)
		}
	}

	return nil
}
