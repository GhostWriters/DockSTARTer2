package appenv

import (
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/adrg/xdg"
)

// EnvCreate initializes the DockSTARTer environment file.
//
// This function mirrors env_create.sh and performs the following steps:
//  1. Creates the compose folder if it doesn't exist
//  2. If .env exists: backs it up and sanitizes it
//  3. If .env missing: creates it from the default template and sanitizes it
//  4. If no apps are added: automatically enables WATCHTOWER as the default app
//
// The sanitization process ensures all required variables are present and sets
// platform-specific defaults for PUID, PGID, TZ, HOME, and Docker paths.
//
// Returns an error if critical operations like folder creation or file writing fail.
func EnvCreate(ctx context.Context, conf config.AppConfig) error {
	// 1. Ensure ComposeFolder exists
	if _, err := os.Stat(conf.Paths.ComposeFolder); os.IsNotExist(err) {
		logger.Notice(ctx, "Creating folder '{{|Folder|}}%s{{[-]}}'.", conf.Paths.ComposeFolder)
		if err := os.MkdirAll(conf.Paths.ComposeFolder, 0755); err != nil {
			logger.Fatal(ctx, "Failed to create compose folder: %v", err)
		}
	} else if err != nil {
		return err
	}

	mainEnvPath := filepath.Join(conf.Paths.ComposeFolder, constants.EnvFileName)

	// 2. Initialize .env file if missing
	if _, err := os.Stat(mainEnvPath); os.IsNotExist(err) {
		logger.Warn(ctx, "File '{{|File|}}%s{{[-]}}' not found. Copying example template.", mainEnvPath)

		defaultContent, err := assets.GetDefaultEnv()
		if err != nil {
			logger.Fatal(ctx, "Failed to load default env template: %v", err)
		}

		if err := os.WriteFile(mainEnvPath, defaultContent, 0644); err != nil {
			logger.Fatal(ctx, "Failed to create default env file: %v", err)
		}
	} else {
		// 3. Backup if existing
		if err := BackupEnv(ctx, mainEnvPath, conf); err != nil {
			logger.Warn(ctx, "Failed to backup .env: %v", err)
		}
	}

	// 4. Sanitize (ensures required variables like HOME, Docker paths, etc. are set)
	if err := SanitizeEnv(ctx, mainEnvPath, conf); err != nil {
		logger.Warn(ctx, "Failed to sanitize .env: %v", err)
	}

	// 5. Ensure default apps are enabled if none are referenced
	referenced, err := ListReferencedApps(ctx, conf)
	if err != nil {
		return err
	}
	if len(referenced) == 0 {
		logger.Notice(ctx, "Installing default applications.")
		// Add Watchtower
		if err := Enable(ctx, []string{"WATCHTOWER"}, conf); err != nil {
			return err
		}
	}

	return nil
}

// SanitizeEnv sanitizes the environment file by setting default values
func SanitizeEnv(ctx context.Context, file string, conf config.AppConfig) error {
	// 1. Merge default values (if keys missing)
	tmpDefault := filepath.Join(os.TempDir(), "ds2_default.env")
	defaultContent, err := assets.GetDefaultEnv()
	if err != nil {
		return fmt.Errorf("failed to load default env for sanitization: %w", err)
	}
	if err := os.WriteFile(tmpDefault, defaultContent, 0644); err != nil {
		return err
	}
	defer os.Remove(tmpDefault)

	if _, err := MergeNewOnly(ctx, file, tmpDefault); err != nil {
		logger.Error(ctx, "Failed to merge defaults: %v", err)
	}

	// 2. Load all variables for expansion context
	vars, err := ListVars(file)
	if err != nil {
		return fmt.Errorf("failed to read env vars: %w", err)
	}

	expansionContext := make(map[string]string)
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) == 2 {
			expansionContext[pair[0]] = pair[1]
		}
	}

	// Inject XDG variables if missing
	if _, ok := expansionContext["XDG_CONFIG_HOME"]; !ok {
		expansionContext["XDG_CONFIG_HOME"] = xdg.ConfigHome
	}
	if _, ok := expansionContext["XDG_DATA_HOME"]; !ok {
		expansionContext["XDG_DATA_HOME"] = xdg.DataHome
	}
	if _, ok := expansionContext["XDG_CACHE_HOME"]; !ok {
		expansionContext["XDG_CACHE_HOME"] = xdg.CacheHome
	}
	if _, ok := expansionContext["XDG_STATE_HOME"]; !ok {
		expansionContext["XDG_STATE_HOME"] = xdg.StateHome
	}

	for k, v := range vars {
		expansionContext[k] = v
	}

	getExpanded := func(key string) string {
		val := vars[key]
		return ExpandVars(val, expansionContext)
	}

	// 2. Collect Updates
	var updatedVars []string
	updates := make(map[string]string)

	detectedHome, _ := os.UserHomeDir()

	addUpdate := func(key, value string) {
		updatedVars = append(updatedVars, key)
		updates[key] = value
		expansionContext[key] = value // Update context with potential literal? Or value?
		// For Sanitization loop, we assume value is what we want to use.
		// If explicit quotes are needed, 'value' should have them.
	}

	// HOME
	// Bash: HOME="${DETECTED_HOMEDIR}" (No quotes added by env_sanitize)
	currentHomeExpanded := getExpanded("HOME")
	if currentHomeExpanded != detectedHome {
		addUpdate("HOME", detectedHome)
	}

	// DOCKER_CONFIG_FOLDER
	rawConfig := conf.Paths.ConfigFolder

	// Create strict context for config expansion as per global_variables.sh
	configExpansionContext := map[string]string{
		"XDG_CONFIG_HOME": xdg.ConfigHome,
		"HOME":            detectedHome,
	}

	expandedConfig := ExpandVars(rawConfig, configExpansionContext)
	expandedConfig = filepath.Clean(expandedConfig)
	if runtime.GOOS == "windows" {
		expandedConfig = filepath.ToSlash(expandedConfig)
	}

	// Normalize detectedHome for replacement matching
	replHome := filepath.Clean(detectedHome)
	if runtime.GOOS == "windows" {
		replHome = filepath.ToSlash(replHome)
	}

	homeMap := map[string]string{"HOME": replHome}
	targetConfigLiteral := ReplaceWithVars(expandedConfig, homeMap)

	currentConfigLiteral := vars["DOCKER_CONFIG_FOLDER"]
	if currentConfigLiteral != targetConfigLiteral {
		addUpdate("DOCKER_CONFIG_FOLDER", targetConfigLiteral)
	}

	// DOCKER_COMPOSE_FOLDER
	rawCompose := conf.Paths.ComposeFolder

	// Strict context for compose expansion
	composeExpansionContext := map[string]string{
		"ConfigFolder":         expandedConfig,
		"DOCKER_CONFIG_FOLDER": expandedConfig,
		"XDG_CONFIG_HOME":      xdg.ConfigHome,
		"HOME":                 detectedHome,
	}

	expandedCompose := ExpandVars(rawCompose, composeExpansionContext)
	expandedCompose = filepath.Clean(expandedCompose)
	if runtime.GOOS == "windows" {
		expandedCompose = filepath.ToSlash(expandedCompose)
	}

	composeMap := map[string]string{
		"DOCKER_CONFIG_FOLDER": expandedConfig, // Use expanded path for replacement matching
		"HOME":                 replHome,
	}

	targetComposeLiteral := ReplaceWithVars(expandedCompose, composeMap)

	currentComposeLiteral := vars["DOCKER_COMPOSE_FOLDER"]
	if currentComposeLiteral != targetComposeLiteral {
		addUpdate("DOCKER_COMPOSE_FOLDER", targetComposeLiteral)
	}

	// Apply defaults using VarDefaultValue (Global variables)
	defaultsList1 := []string{"DOCKER_HOSTNAME", "TZ"}
	for _, key := range defaultsList1 {
		val := vars[key]
		if val == "" {
			def := VarDefaultValue(ctx, key, conf)
			if def != "" {
				addUpdate(key, def)
			}
		}
	}

	defaultsList2 := []string{"GLOBAL_LAN_NETWORK", "DOCKER_GID", "PGID", "PUID"}
	for _, key := range defaultsList2 {
		val := vars[key]
		if val == "" || strings.Contains(val, "x") {
			def := VarDefaultValue(ctx, key, conf)
			if def != "" {
				addUpdate(key, def)
			}
		}
	}

	// Volumes - ReplaceWithVars Logic
	volumeVars := []string{
		"DOCKER_VOLUME_CONFIG",
		"DOCKER_VOLUME_STORAGE",
		"DOCKER_VOLUME_STORAGE2",
		"DOCKER_VOLUME_STORAGE3",
		"DOCKER_VOLUME_STORAGE4",
	}

	// Normalize specific paths for volume replacements
	replHomeVol := filepath.Clean(detectedHome)
	if runtime.GOOS == "windows" {
		replHomeVol = filepath.ToSlash(replHomeVol)
	}
	replacements := map[string]string{
		"DOCKER_CONFIG_FOLDER": expandedConfig,
		"HOME":                 replHomeVol,
	}

	for _, vVar := range volumeVars {
		currentValExpanded := getExpanded(vVar)
		sanitized := filepath.Clean(currentValExpanded)
		if runtime.GOOS == "windows" {
			sanitized = filepath.ToSlash(sanitized)
		}

		restored := ReplaceWithVars(sanitized, replacements)

		// In Bash: UpdatedValue like "${HOME}/storage"
		// Logic: VarsToUpdate+=("${VarName}")
		// UpdatedVarValue["${VarName}"]="\"${UpdatedValue}\""
		// It EXPLICITLY quotes the restored value with double quotes.

		// Check overlap with current literal
		currentLiteral := vars[vVar]

		// Bash checks if ${Value} (expanded/current) != ${UpdatedValue} (restored)
		// Wait, Bash check: if [[ ${Value} != "${UpdatedValue}" ]]; then
		// Value is get_env (literal? or expanded?). env_get returns value.
		// If env_get returns literal, then we compare Literal vs Restored.
		// If env_get returns expanded (vars expanded?), then we compare Expanded vs Restored.

		// Actually env_get source simply sources the file and echoes the var.
		// So `env_get` returns the EXPANDED value if the file has `VAR=${OTHER}`.

		// So Bash compares EXPANDED vs RESTORED.
		// Example: File has `STORAGE=/home/user/storage`.
		// Expanded: `/home/user/storage`.
		// Restored: `${HOME}/storage`.
		// Are they equal? NO.
		// So strict equality fails -> Update.
		// Update set to `"\"${UpdatedValue}\""` => `"${HOME}/storage"`.

		// If File has `STORAGE="${HOME}/storage"`.
		// Expanded: `/home/user/storage`.
		// Restored: `${HOME}/storage`.
		// Equality fails -> Update.
		// Update set to `"${HOME}/storage"`. (Same literal).

		// If `vars[vVar]` (literal) is same as `"${restored}"` (quoted restored), skipping?
		// But Bash logic seems to force update if expanded != restored.
		// Which means it effectively enforces standard variable syntax?

		// FIX: ListVars returns cleaned values (unquoted). targetVal is quoted.
		// Comparing cleaned vs quoted causes infinite loop.
		// Compare current cleaned value vs restored (unquoted) value.
		targetVal := fmt.Sprintf("\"%s\"", restored)
		if currentLiteral != restored {
			addUpdate(vVar, targetVal)
		}
	}

	// 3. Log and Apply Updates
	if len(updatedVars) > 0 {
		logger.Notice(ctx, "Setting variables in '{{|File|}}%s{{[-]}}':", file)
		for _, key := range updatedVars {
			val := updates[key]
			logger.Notice(ctx, "\t{{|Var|}}%s=%s{{[-]}}", key, val)
			SetLiteral(key, val, file)
		}
	}

	return nil
}
