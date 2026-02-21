package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/system"
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MigrateAppVars performs variable migrations for an application based on a .migrate file.
// It mirrors the logic of appvars_migrate.sh.
func MigrateAppVars(ctx context.Context, appName string, conf config.AppConfig) error {
	appName = strings.ToUpper(appName)

	migrateFile, err := AppInstanceFile(ctx, appName, "*.migrate")
	if err != nil || migrateFile == "" {
		return nil // No migration file, nothing to do
	}

	if _, err := os.Stat(migrateFile); os.IsNotExist(err) {
		return nil
	}

	f, err := os.Open(migrateFile)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Remove comments
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Split by whitespace
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			toVar := parts[0]
			fromVar := parts[1]

			if err := EnvMigrate(ctx, fromVar, toVar, conf); err != nil {
				logger.Warn(ctx, "Failed to migrate variable %s to %s: %v", fromVar, toVar, err)
			}
		}
	}

	return scanner.Err()
}

// EnvMigrate renames a variable while preserving its value.
func EnvMigrate(ctx context.Context, fromVar, toVar string, conf config.AppConfig) error {
	fromVarResolved := fromVar
	fromFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if strings.Contains(fromVar, ":") {
		parts := strings.SplitN(fromVar, ":", 2)
		f, err := AppInstanceFile(ctx, parts[0], constants.EnvFileName)
		if err == nil && f != "" {
			fromFile = f
			fromVarResolved = parts[1]
		}
	}

	toVarResolved := toVar
	toFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if strings.Contains(toVar, ":") {
		parts := strings.SplitN(toVar, ":", 2)
		f, err := AppInstanceFile(ctx, parts[0], constants.EnvFileName)
		if err == nil && f != "" {
			toFile = f
			toVarResolved = parts[1]
		}
	}

	// Get value from source
	val, err := Get(fromVar, fromFile)
	if err != nil || val == "" {
		return nil // Source variable not found or empty
	}

	// Check if target already exists
	toVal, _ := Get(toVar, toFile)
	if toVal != "" {
		logger.Debug(ctx, "Migration target %s already exists, skipping.", toVar)
		return nil
	}

	logger.Notice(ctx, "Migrating variable {{|Var|}}%s{{[-]}} to {{|Var|}}%s{{[-]}}.", fromVar, toVar)

	// Set new variable
	if err := SetLiteral(ctx, toVarResolved, val, toFile); err != nil {
		return err
	}

	// Remove old variable
	if err := unsetVarInFile(ctx, fromVarResolved, fromFile); err != nil {
		return err
	}

	// If we're in the global .env, also check the override file
	if fromFile == filepath.Join(conf.ComposeDir, constants.EnvFileName) {
		return OverrideVarRename(ctx, fromVarResolved, toVarResolved, conf)
	}

	return nil
}

func unsetVarInFile(ctx context.Context, varName, file string) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	pattern := fmt.Sprintf(`^\s*%s\s*=`, regexp.QuoteMeta(varName))
	re := regexp.MustCompile(pattern)

	found := false
	for _, line := range lines {
		if re.MatchString(line) {
			found = true
			continue
		}
		newLines = append(newLines, line)
	}

	if found {
		output := strings.Join(newLines, "\n")
		if err := os.WriteFile(file, []byte(output), 0644); err != nil {
			return err
		}
		system.SetPermissions(ctx, file)
	}
	return nil
}

// OverrideVarRename renames a variable in the docker-compose.override.yml file.
// Matches bash parity: global search/replace for variable substitution syntax.
func OverrideVarRename(ctx context.Context, fromVar, toVar string, conf config.AppConfig) error {
	overrideFile := filepath.Join(conf.ComposeDir, constants.ComposeOverrideFileName)
	if _, err := os.Stat(overrideFile); os.IsNotExist(err) {
		return nil
	}

	content, err := os.ReadFile(overrideFile)
	if err != nil {
		return err
	}

	// Bash regex: s/([$]\{?)${FromVar}\b/\1${ToVar}/g
	// This handles both $VAR and ${VAR} syntax.
	// We use regexp.QuoteMeta to safely inject the variable names.
	pattern := fmt.Sprintf(`([$]\{?)%s\b`, regexp.QuoteMeta(fromVar))
	re := regexp.MustCompile(pattern)
	replacement := fmt.Sprintf(`${1}%s`, toVar)

	if re.Match(content) {
		logger.Notice(ctx, "Renaming variable in {{|File|}}%s{{[-]}}:", filepath.Base(overrideFile))
		logger.Notice(ctx, "\t{{|Var|}}%s{{[-]}} to {{|Var|}}%s{{[-]}}", fromVar, toVar)

		newContent := re.ReplaceAll(content, []byte(replacement))
		if err := os.WriteFile(overrideFile, newContent, 0644); err != nil {
			return err
		}
		system.SetPermissions(ctx, overrideFile)
	}

	return nil
}
