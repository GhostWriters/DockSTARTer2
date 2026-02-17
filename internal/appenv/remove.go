package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Remove (purge) prompts to remove variables for the specified apps or all disabled
// Mirrors appvars_purge.sh and appvars_purge_all.sh functionality.
func Remove(ctx context.Context, appNames []string, conf config.AppConfig, assumeYes bool) error {
	// If no apps specified, purge all disabled apps
	if len(appNames) == 0 {
		return removeAllDisabled(ctx, conf, assumeYes)
	}

	// Otherwise purge specific apps
	for _, appName := range appNames {
		appName = strings.TrimSpace(strings.ToLower(appName))
		if err := removeApp(ctx, appName, conf, assumeYes); err != nil {
			return err
		}
	}

	return nil
}

func removeAllDisabled(ctx context.Context, conf config.AppConfig, assumeYes bool) error {
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	disabledApps, err := ListDisabledApps(envFile)
	if err != nil {
		return err
	}

	if len(disabledApps) == 0 {
		logger.Notice(ctx, "'{{|File|}}%s{{[-]}}' does not contain any disabled ", envFile)
		return nil
	}

	// Ask once for all disabled apps
	question := "Would you like to purge variables for all disabled apps?"
	if !assumeYes && !promptYesNo(ctx, question) {
		return nil
	}

	logger.Info(ctx, "Purging disabled app variables.")
	for _, appName := range disabledApps {
		// Don't prompt again for individual apps when doing "all"
		if err := removeApp(ctx, appName, conf, true); err != nil {
			return err
		}
	}

	return nil
}

func removeApp(ctx context.Context, appName string, conf config.AppConfig, assumeYes bool) error {
	appUpper := strings.ToUpper(appName)
	nice := GetNiceName(ctx, appUpper)
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))

	// Get current and default variables
	currentGlobalVars, err := listAppVars(appName, envFile)
	if err != nil {
		return err
	}
	defaultGlobalVars, err := listDefaultGlobalVars(ctx, appName)
	if err != nil {
		return err
	}
	globalVarsToRemove := intersection(currentGlobalVars, defaultGlobalVars)
	globalLinesToRemove, _ := getVarLines(globalVarsToRemove, envFile)

	currentAppVars, err := listAppVars(appName+":", appEnvFile)
	if err != nil {
		return err
	}
	defaultAppVars, err := listDefaultAppVars(ctx, appName)
	if err != nil {
		return err
	}
	appVarsToRemove := intersection(currentAppVars, defaultAppVars)
	appLinesToRemove, _ := getVarLines(appVarsToRemove, appEnvFile)

	// Check if there's anything to remove
	if len(globalVarsToRemove) == 0 && len(appVarsToRemove) == 0 {
		logger.Warn(ctx, "'{{|App|}}%s{{[-]}}' has no variables to remove.", nice)
		return nil
	}

	// Build the question showing what will be removed (matching Bash format exactly)
	indent := "\t"
	question := fmt.Sprintf("Would you like to purge these settings for '{{|App|}}%s{{[-]}}'?\n", nice)

	if len(globalLinesToRemove) > 0 {
		question += fmt.Sprintf("%s{{|Folder|}}%s{{[-]}}:\n", indent, envFile)
		for _, line := range globalLinesToRemove {
			question += fmt.Sprintf("%s%s{{|Var|}}%s{{[-]}}\n", indent, indent, line)
		}
	}

	if len(appLinesToRemove) > 0 {
		question += fmt.Sprintf("%s{{|Folder|}}%s{{[-]}}:\n", indent, appEnvFile)
		for _, line := range appLinesToRemove {
			question += fmt.Sprintf("%s%s{{|Var|}}%s{{[-]}}\n", indent, indent, line)
		}
	}

	// Prompt for confirmation
	if !assumeYes && !promptYesNo(ctx, question) {
		logger.Info(ctx, "Keeping '{{|App|}}%s{{[-]}}' variables.", nice)
		return nil
	}

	logger.Info(ctx, "Purging '{{|App|}}%s{{[-]}}' variables.", nice)

	// Remove global variables (matching Bash multi-line notice format)
	if len(globalVarsToRemove) > 0 {
		// Build multi-line message
		msg := fmt.Sprintf("Removing variables from {{|File|}}%s{{[-]}}:", envFile)
		for _, line := range globalLinesToRemove {
			msg += fmt.Sprintf("\n%s{{|Var|}}%s{{[-]}}", indent, line)
		}
		logger.Notice(ctx, msg)

		if err := removeVarsFromFile(globalVarsToRemove, envFile); err != nil {
			return fmt.Errorf("failed to purge '%s' variables: %w", nice, err)
		}
	}

	// Remove app-specific variables (matching Bash multi-line notice format)
	if len(appVarsToRemove) > 0 {
		// Build multi-line message
		msg := fmt.Sprintf("Removing variables from {{|File|}}%s{{[-]}}:", appEnvFile)
		for _, line := range appLinesToRemove {
			msg += fmt.Sprintf("\n%s{{|Var|}}%s{{[-]}}", indent, line)
		}
		logger.Notice(ctx, msg)

		if err := removeVarsFromFile(appVarsToRemove, appEnvFile); err != nil {
			return fmt.Errorf("failed to purge '%s' variables: %w", nice, err)
		}
	}

	return nil
}

// listAppVars lists variable names for an app from a file (e.g., APPNAME__ENABLED)
func listAppVars(prefix string, filePath string) ([]string, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []string{}, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	vars := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	upperPrefix := strings.ToUpper(prefix)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, upperPrefix) {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) > 0 {
				varName := strings.TrimSpace(parts[0])
				vars[varName] = true
			}
		}
	}

	result := make([]string, 0, len(vars))
	for v := range vars {
		result = append(result, v)
	}
	slices.Sort(result)
	return result, scanner.Err()
}

// listDefaultGlobalVars lists default global variables for an app
func listDefaultGlobalVars(ctx context.Context, appName string) ([]string, error) {
	processedFile, err := AppInstanceFile(ctx, appName, constants.EnvFileName)
	if err != nil {
		return nil, err
	}
	if processedFile == "" {
		return []string{}, nil
	}
	return listAppVars(strings.ToUpper(appName), processedFile)
}

// listDefaultAppVars lists default app-specific variables
func listDefaultAppVars(ctx context.Context, appName string) ([]string, error) {
	processedFile, err := AppInstanceFile(ctx, appName, fmt.Sprintf("%s*", constants.AppEnvFileNamePrefix))
	if err != nil {
		return nil, err
	}
	if processedFile == "" {
		return []string{}, nil
	}
	return listAppVars(strings.ToUpper(appName), processedFile)
}

// intersection returns common elements between two slices
func intersection(a, b []string) []string {
	set := make(map[string]bool)
	for _, item := range a {
		set[item] = true
	}

	result := []string{}
	for _, item := range b {
		if set[item] {
			result = append(result, item)
		}
	}
	slices.Sort(result)
	return result
}

// getVarLines gets the actual lines containing the variables from a file
func getVarLines(vars []string, filePath string) ([]string, error) {
	if len(vars) == 0 {
		return []string{}, nil
	}
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []string{}, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	varSet := make(map[string]bool)
	for _, v := range vars {
		varSet[v] = true
	}

	lines := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		for varName := range varSet {
			matched, _ := regexp.MatchString(`^\s*`+regexp.QuoteMeta(varName)+`\s*=`, line)
			if matched {
				lines = append(lines, line)
				break
			}
		}
	}

	return lines, scanner.Err()
}

// removeVarsFromFile removes variables from a file
func removeVarsFromFile(vars []string, filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	file.Close()

	if err := scanner.Err(); err != nil {
		return err
	}

	// Filter out lines matching the variables
	varSet := make(map[string]bool)
	for _, v := range vars {
		varSet[v] = true
	}

	filteredLines := []string{}
	for _, line := range lines {
		shouldRemove := false
		for varName := range varSet {
			matched, _ := regexp.MatchString(`^\s*`+regexp.QuoteMeta(varName)+`\s*=`, line)
			if matched {
				shouldRemove = true
				break
			}
		}
		if !shouldRemove {
			filteredLines = append(filteredLines, line)
		}
	}

	// Write back
	return os.WriteFile(filePath, []byte(strings.Join(filteredLines, "\n")+"\n"), 0644)
}

// promptYesNo prompts the user for yes/no confirmation
func promptYesNo(ctx context.Context, question string) bool {
	logger.Display(ctx, question)
	fmt.Print("(Y/n): ")

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return response == "y" || response == "yes" || response == ""
	}

	return false
}
