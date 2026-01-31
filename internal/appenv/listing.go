package appenv

import (
	"DockSTARTer2/internal/config"
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

	entries, _ := os.ReadDir(conf.ComposeFolder)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".env.app.") {
			appName := strings.TrimPrefix(entry.Name(), ".env.app.")
			referenced[strings.ToUpper(appName)] = true
		}
	}

	overrideFile := filepath.Join(conf.ComposeFolder, "docker-compose.override.yml")
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
	envFile := filepath.Join(conf.ComposeFolder, ".env")
	content, err := os.ReadFile(envFile)
	if err != nil {
		return nil, err
	}

	var added []string
	lines := strings.Split(string(content), "\n")
	re := regexp.MustCompile(`^([A-Z0-9_]+)__ENABLED=true$`)

	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			key := matches[1]
			appName := strings.TrimSuffix(key, "__ENABLED")
			if IsAppNameValid(appName) && IsAppBuiltIn(appName) {
				added = append(added, appName)
			}
		}
	}

	sort.Strings(added)
	return added, nil
}

// ListAppVars returns a list of variable names for the specified app.
func ListAppVars(ctx context.Context, appName string, conf config.AppConfig) ([]string, error) {
	envFile := filepath.Join(conf.ComposeFolder, ".env")

	// If appName ends with ":", use app-specific env file
	if strings.HasSuffix(appName, ":") {
		baseApp := strings.TrimSuffix(appName, ":")
		targetFile := filepath.Join(conf.ComposeFolder, ".env.app."+strings.ToLower(baseApp))
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
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			matches := regexp.MustCompile(`^([A-Za-z0-9_]+)=`).FindStringSubmatch(line)
			if len(matches) > 1 {
				varName := matches[1]
				if IsGlobalVar(varName) {
					globals = append(globals, line)
				}
			}
		}
		return globals
	}

	pattern := fmt.Sprintf(`^\s*(%s__\w+)\s*=`, regexp.QuoteMeta(appName))
	re := regexp.MustCompile(pattern)

	var appVars []string
	for _, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			varName := matches[1]
			suffix := strings.TrimPrefix(varName, appName+"__")
			if regexp.MustCompile(`^[A-Za-z0-9]+__`).MatchString(suffix) {
				continue
			}
			appVars = append(appVars, strings.TrimSpace(line))
		}
	}

	return appVars
}

// ListAppVarLines returns a list of variable lines (KEY=VALUE) for the specified app.
func ListAppVarLines(ctx context.Context, appName string, conf config.AppConfig) ([]string, error) {
	envFile := filepath.Join(conf.ComposeFolder, ".env")

	// If appName ends with ":", use app-specific env file
	if strings.HasSuffix(appName, ":") {
		baseApp := strings.TrimSuffix(appName, ":")
		targetFile := filepath.Join(conf.ComposeFolder, ".env.app."+strings.ToLower(baseApp))
		content, err := os.ReadFile(targetFile)
		if err != nil {
			return nil, err
		}
		var lines []string
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		return lines, scanner.Err()
	}

	// Otherwise, use AppVarsLines to filter from global .env
	content, err := os.ReadFile(envFile)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	return AppVarsLines(strings.ToUpper(appName), lines), nil
}
