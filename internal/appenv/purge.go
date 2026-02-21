package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/system"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PurgeAppVars removes application-specific variables from environment files.
// It mirrors the intent of appvars_purge.sh.
func PurgeAppVars(ctx context.Context, appName string, conf config.AppConfig) error {
	appUpper := strings.ToUpper(appName)
	niceName := GetNiceName(ctx, appUpper)

	globalEnv := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	appSpecificEnv := filepath.Join(conf.ComposeDir, constants.AppEnvFileNamePrefix+strings.ToLower(appUpper))

	logger.Notice(ctx, "Purging '{{|App|}}%s{{[-]}}' variables.", niceName)

	// 1. Purge from global .env
	if _, err := os.Stat(globalEnv); err == nil {
		if err := purgeFromFile(ctx, appUpper, globalEnv); err != nil {
			return err
		}
	}

	// 2. Purge from app-specific .env
	if _, err := os.Stat(appSpecificEnv); err == nil {
		// For the app-specific file, we purge ALL variables (matching bash appvars_purge logic for .env.app.*)
		if err := purgeFromFile(ctx, "", appSpecificEnv); err != nil {
			return err
		}
	}

	// 3. Cleanup orphaned files (might be redundant if called through flows that already cleanup, but safe)
	return CleanupOrphanedEnvFiles(ctx, conf)
}

func purgeFromFile(ctx context.Context, appName string, file string) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	var removedLines []string

	// If appName is "", purge ALL variables
	// If appName is set, purge only appName__... variables
	var pattern string
	if appName == "" {
		pattern = `^\s*([a-zA-Z_][a-zA-Z0-9_]*)=`
	} else {
		pattern = fmt.Sprintf(`^\s*%s__([A-Za-z0-9_]+)\s*=`, regexp.QuoteMeta(appName))
	}
	re := regexp.MustCompile(pattern)

	for _, line := range lines {
		if re.MatchString(line) {
			removedLines = append(removedLines, strings.TrimSpace(line))
		} else {
			newLines = append(newLines, line)
		}
	}

	if len(removedLines) > 0 {
		logger.Notice(ctx, "Removing variables from '{{|File|}}%s{{[-]}}':", filepath.Base(file))
		for _, rl := range removedLines {
			logger.Notice(ctx, "\t{{|Var|}}%s{{[-]}}", rl)
		}

		output := strings.Join(newLines, "\n")
		// Ensure trailing newline if file had any lines
		if len(newLines) > 0 && !strings.HasSuffix(output, "\n") {
			// output = output + "\n" // Join already adds \n between lines, but if last line was empty...?
			// Actually strings.Split and Join handles it.
		}

		if err := os.WriteFile(file, []byte(output), 0644); err != nil {
			return err
		}
		system.SetPermissions(ctx, file)
	}

	return nil
}
