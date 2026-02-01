package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/envutil"
	"DockSTARTer2/internal/paths"
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ListAddedApps returns a sorted list of all added applications (those with __ENABLED variables).
func ListAddedApps(ctx context.Context, envFile string) ([]string, error) {
	vars, err := ListVars(envFile)
	if err != nil {
		return nil, err
	}

	var added []string
	for key := range vars {
		// key: APPNAME__ENABLED
		if strings.HasSuffix(key, "__ENABLED") {
			appName := strings.TrimSuffix(key, "__ENABLED")
			if IsAppNameValid(appName) && IsAppBuiltIn(appName) {
				added = append(added, appName)
			}
		}
	}

	sort.Strings(added)
	return added, nil
}

// ListBuiltinApps returns a sorted list of all builtin applications.
func ListBuiltinApps() ([]string, error) {
	templatesDir := paths.GetTemplatesDir()
	appsDir := filepath.Join(templatesDir, ".apps")

	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return nil, err
	}

	var builtin []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			builtin = append(builtin, strings.ToUpper(entry.Name()))
		}
	}

	sort.Strings(builtin)
	return builtin, nil
}

// ListDeprecatedApps returns a sorted list of all deprecated applications.
func ListDeprecatedApps(ctx context.Context) ([]string, error) {
	builtin, err := ListBuiltinApps()
	if err != nil {
		return nil, err
	}

	var deprecated []string
	for _, appName := range builtin {
		if IsAppDeprecated(ctx, appName) {
			deprecated = append(deprecated, appName)
		}
	}

	sort.Strings(deprecated)
	return deprecated, nil
}

// ListDisabledApps returns a sorted list of disabled applications.
func ListDisabledApps(envFile string) ([]string, error) {
	vars, err := ListVars(envFile)
	if err != nil {
		return nil, err
	}

	var disabled []string
	for key, val := range vars {
		if strings.HasSuffix(key, "__ENABLED") && !IsTrue(val) {
			appName := strings.TrimSuffix(key, "__ENABLED")
			if IsAppNameValid(appName) && IsAppBuiltIn(appName) {
				disabled = append(disabled, appName)
			}
		}
	}

	sort.Strings(disabled)
	return disabled, nil
}

// ListNonDeprecatedApps returns all builtin applications that are NOT deprecated.
func ListNonDeprecatedApps(ctx context.Context) ([]string, error) {
	builtin, err := ListBuiltinApps()
	if err != nil {
		return nil, err
	}

	var nonDeprecated []string
	for _, app := range builtin {
		if !IsAppDeprecated(ctx, app) {
			nonDeprecated = append(nonDeprecated, app)
		}
	}

	sort.Strings(nonDeprecated)
	return nonDeprecated, nil
}

// ListReferencedApps returns all applications mentioned in .env, app-specific envs, or override file.
func ListReferencedApps(ctx context.Context, conf config.AppConfig) ([]string, error) {
	referenced := make(map[string]bool)

	enabled, _ := ListEnabledApps(conf)
	for _, app := range enabled {
		referenced[app] = true
	}

	entries, _ := os.ReadDir(conf.ComposeDir)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".env.app.") {
			appName := strings.TrimPrefix(entry.Name(), ".env.app.")
			referenced[strings.ToUpper(appName)] = true
		}
	}

	overrideFile := filepath.Join(conf.ComposeDir, "docker-compose.override.yml")
	if _, err := os.Stat(overrideFile); err == nil {
		content, _ := os.ReadFile(overrideFile)
		re := regexp.MustCompile(`\.env\.app\.([a-z0-9_]+)`)
		matches := re.FindAllStringSubmatch(string(content), -1)
		for _, m := range matches {
			if len(m) > 1 {
				referenced[strings.ToUpper(m[1])] = true
			}
		}
	}

	var result []string
	for app := range referenced {
		if IsAppNameValid(app) {
			result = append(result, app)
		}
	}
	sort.Strings(result)
	return result, nil
}

// ListEnabledApps returns a sorted list of enabled applications.
func ListEnabledApps(conf config.AppConfig) ([]string, error) {
	envFile := filepath.Join(conf.ComposeDir, ".env")
	vars, err := ListVars(envFile)
	if err != nil {
		return nil, err
	}

	var enabled []string
	for key, val := range vars {
		if strings.HasSuffix(key, "__ENABLED") && IsTrue(val) {
			appName := strings.TrimSuffix(key, "__ENABLED")
			if IsAppNameValid(appName) && IsAppBuiltIn(appName) {
				enabled = append(enabled, appName)
			}
		}
	}

	sort.Strings(enabled)
	return enabled, nil
}

// ListAppVars returns a list of variable names for the specified app.
func ListAppVars(ctx context.Context, appName string, conf config.AppConfig) ([]string, error) {
	envFile := filepath.Join(conf.ComposeDir, ".env")

	// If appName ends with ":", use app-specific env file
	if strings.HasSuffix(appName, ":") {
		baseApp := strings.TrimSuffix(appName, ":")
		targetFile := filepath.Join(conf.ComposeDir, ".env.app."+strings.ToLower(baseApp))
		varsMap, err := ListVars(targetFile)
		if err != nil {
			return nil, err
		}
		var result []string
		for k := range varsMap {
			result = append(result, k)
		}
		sort.Strings(result)
		return result, nil
	}

	// Otherwise, use AppVarsLines to filter from global .env
	content, err := os.ReadFile(envFile)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	appVarLines := AppVarsLines(strings.ToUpper(appName), lines)

	var result []string
	for _, line := range appVarLines {
		// Extract variable name from line
		idx := strings.Index(line, "=")
		if idx > 0 {
			varName := strings.TrimSpace(line[:idx])
			result = append(result, varName)
		}
	}
	sort.Strings(result)
	return result, nil
}

// ListVars returns a map of all variable keys and values found in the file.
func ListVars(file string) (map[string]string, error) {
	vars := make(map[string]string)
	f, err := os.Open(file)
	if err != nil {
		if os.IsNotExist(err) {
			return vars, nil
		}
		return nil, err
	}
	defer f.Close()

	re := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)=(.*)`)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if matches != nil {
			key := matches[1]
			val := strings.Trim(matches[2], `"' `)
			vars[key] = val
		}
	}
	return vars, scanner.Err()
}

// AppVarsLines filters environment variable lines to only those belonging to the specified app.
func AppVarsLines(appName string, lines []string) []string {
	if appName == "" {
		var globals []string
		// Search for all variables not for an app
		// Exclude empty lines and comments
		reEmpty := regexp.MustCompile(`^\s*$`)
		reComment := regexp.MustCompile(`^\s*#`)

		for _, line := range lines {
			if reEmpty.MatchString(line) || reComment.MatchString(line) {
				continue
			}

			// Check if line contains a variable assignment
			if idx := strings.Index(line, "="); idx > 0 {
				varName := strings.TrimSpace(line[:idx])
				if IsGlobalVar(varName) {
					globals = append(globals, line)
				}
			}
		}
		return globals
	}

	// Search for all variables for app "appName"
	// Match lines starting with appName__
	pattern := fmt.Sprintf(`^\s*%s__([A-Za-z0-9_]+)\s*=`, regexp.QuoteMeta(appName))
	re := regexp.MustCompile(pattern)

	var appVars []string
	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			suffix := matches[1]
			// Bash: APPNAME__(?![A-Za-z0-9]+__)\w+
			// This means the suffix should NOT contain another __
			// (which would indicate an instance of this app, which is a separate app)
			if !strings.Contains(suffix, "__") {
				appVars = append(appVars, line)
			}
		}
	}

	return appVars
}

// ListAppVarLines returns a list of variable lines (KEY=VALUE) for the specified app.
func ListAppVarLines(ctx context.Context, appName string, conf config.AppConfig) ([]string, error) {
	appName = strings.ToUpper(appName)
	envFile := filepath.Join(conf.ComposeDir, ".env")

	// If appName ends with ":", use app-specific env file
	if strings.HasSuffix(appName, ":") {
		baseApp := strings.TrimSuffix(appName, ":")
		targetFile := filepath.Join(conf.ComposeDir, ".env.app."+strings.ToLower(baseApp))
		return envutil.ReadLines(targetFile)
	}

	// Otherwise, use AppVarsLines to filter from global .env
	content, err := os.ReadFile(envFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	result := AppVarsLines(appName, lines)
	sort.Strings(result)
	return result, nil
}
