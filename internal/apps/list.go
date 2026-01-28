package apps

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/env"
	"DockSTARTer2/internal/paths"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ListAdded returns a list of enabled applications from the .env file.
// Matches Bash app_list_added: finds APPNAME__ENABLED vars that are built-in templates.
func ListAdded(envFile string) ([]string, error) {
	keys, err := env.ListVars(envFile)
	if err != nil {
		return nil, err
	}

	var added []string
	for key := range keys {
		// key: APPNAME__ENABLED
		if strings.HasSuffix(key, "__ENABLED") {
			appName := strings.TrimSuffix(key, "__ENABLED")
			if IsAppNameValid(appName) && IsBuiltin(appName) {
				added = append(added, appName)
			}
		}
	}
	sort.Strings(added)
	return added, nil
}

// ListBuiltin returns all applications available in the templates folder.
func ListBuiltin() ([]string, error) {
	templatesDir := paths.GetTemplatesDir()
	appsDir := filepath.Join(templatesDir, ".apps")
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
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

// ListDeprecated returns all builtin applications marked as deprecated.
func ListDeprecated() ([]string, error) {
	builtin, err := ListBuiltin()
	if err != nil {
		return nil, err
	}

	var deprecated []string
	for _, app := range builtin {
		if IsDeprecated(app) {
			deprecated = append(deprecated, app)
		}
	}
	return deprecated, nil
}

// ListEnabled returns all applications that are explicitly enabled in the .env.
func ListEnabled(envFile string) ([]string, error) {
	keys, err := env.ListVars(envFile)
	if err != nil {
		return nil, err
	}

	var enabled []string
	for key := range keys {
		if strings.HasSuffix(key, "__ENABLED") {
			val, _ := env.Get(key, envFile)
			if IsTrue(val) {
				appName := strings.TrimSuffix(key, "__ENABLED")
				// Bash checks if instance folder exists, but we'll stick to enabled status for now
				enabled = append(enabled, appName)
			}
		}
	}
	sort.Strings(enabled)
	return enabled, nil
}

// ListDisabled returns all applications that are referenced but not enabled, or builtin but not added.
func ListDisabled(envFile string) ([]string, error) {
	// Bash logic: (DISABLED_APPS + BUILTIN_APPS) | sort | uniq -d
	// This finds apps that are in both lists.
	// Actually, Bash's list_disabled is:
	// find apps in .env with __ENABLED != true
	// find apps in builtin
	// return the intersection? No, that's what sort | uniq -d does.
	// It means apps that are builtin AND present in .env but not enabled.

	keys, err := env.ListVars(envFile)
	if err != nil {
		return nil, err
	}

	disabledInEnv := make(map[string]bool)
	for key := range keys {
		if strings.HasSuffix(key, "__ENABLED") {
			val, _ := env.Get(key, envFile)
			if !IsTrue(val) {
				disabledInEnv[strings.TrimSuffix(key, "__ENABLED")] = true
			}
		}
	}

	builtin, _ := ListBuiltin()
	var disabled []string
	for _, app := range builtin {
		if disabledInEnv[app] {
			disabled = append(disabled, app)
		}
	}
	sort.Strings(disabled)
	return disabled, nil
}

// ListNonDeprecated returns all builtin apps that are NOT deprecated.
func ListNonDeprecated() ([]string, error) {
	builtin, err := ListBuiltin()
	if err != nil {
		return nil, err
	}

	var nonDeprecated []string
	for _, app := range builtin {
		if !IsDeprecated(app) {
			nonDeprecated = append(nonDeprecated, app)
		}
	}
	return nonDeprecated, nil
}

// ListReferenced returns all applications mentioned in .env, app-specific envs, or override file.
func ListReferenced(ctx context.Context, conf config.AppConfig) ([]string, error) {
	referenced := make(map[string]bool)

	// 1. From global .env
	envFile := filepath.Join(conf.ComposeFolder, ".env")
	keys, _ := env.ListVars(envFile)
	re := regexp.MustCompile(`^([A-Z][A-Z0-9]*(__[A-Z0-9]+)?)__`)
	for key := range keys {
		matches := re.FindStringSubmatch(key)
		if len(matches) > 1 {
			if IsAppNameValid(matches[1]) {
				referenced[matches[1]] = true
			}
		}
	}

	// 2. From app-specific env files
	entries, _ := os.ReadDir(conf.ComposeFolder)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".env.app.") {
			appName := strings.TrimPrefix(entry.Name(), ".env.app.")
			if appName != "" && IsAppNameValid(appName) {
				referenced[strings.ToUpper(appName)] = true
			}
		}
	}

	// 3. From override file
	overrideFile := filepath.Join(conf.ComposeFolder, "docker-compose.override.yml")
	if _, err := os.Stat(overrideFile); err == nil {
		content, _ := os.ReadFile(overrideFile)
		// Regex for .env.app.appName
		reOverride := regexp.MustCompile(`\.env\.app\.([a-z][a-z0-9]*(?:__[a-z0-9]+)?)`)
		matches := reOverride.FindAllStringSubmatch(string(content), -1)
		for _, m := range matches {
			if len(m) > 1 {
				if IsAppNameValid(m[1]) {
					referenced[strings.ToUpper(m[1])] = true
				}
			}
		}
	}

	var result []string
	for app := range referenced {
		result = append(result, app)
	}
	sort.Strings(result)
	return result, nil
}

// ListHasVarFile returns all apps that have a .env.app.* file.
// Mirrors app_list_hasvarfile.sh functionality.
func ListHasVarFile(composeFolder string) ([]string, error) {
	pattern := filepath.Join(composeFolder, ".env.app.*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var result []string
	for _, match := range matches {
		// Extract app name from filename: .env.app.appname -> appname
		basename := filepath.Base(match)
		if !strings.HasPrefix(basename, ".env.app.") {
			continue
		}

		appname := strings.TrimPrefix(basename, ".env.app.")
		// Convert to uppercase (Bash uses varfile_to_appname_pipe which uppercases)
		result = append(result, strings.ToUpper(appname))
	}

	// Sort for consistent output
	sort.Strings(result)
	return result, nil
}
