package env

import (
	"DockSTARTer2/internal/logger"
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Update regenerates the .env file to ensure correct sorting and headers.
// Mirrors env_update.sh functionality.
func Update(ctx context.Context, file string) error {
	logger.Info(ctx, "Updating '{{_File_}}%s{{|-|}}'.", file)

	// 1. Backup
	bak := file + ".bak"
	if err := CopyFile(file, bak); err != nil {
		logger.Warn(ctx, "Failed to backup .env: %v", err)
	}

	// 2. Read all lines and parse variables
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	var globals []string
	appVars := make(map[string][]string) // Map of AppPrefix -> []VariableLines

	scanner := bufio.NewScanner(f)
	reVar := regexp.MustCompile(`^\s*([a-zA-Z0-9_]+)=(.*)$`)

	seen := make(map[string]bool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip comments and empty lines during parse, we rebuild them
		}

		matches := reVar.FindStringSubmatch(line)
		if matches != nil {
			key := matches[1]
			val := matches[2]

			if seen[key] {
				continue // Skip duplicates, keep first occurrence
			}
			seen[key] = true

			// Reconstruct line to ensure clean format if needed, or use original line?
			// Bash env_update re-quotes things.
			// We should probably keep the line closely valid but clean up quotes if possible.
			// For now, let's use the key=val structure we found, but maybe trust the original line content for value accuracy?
			// Actually, if we want to ensure sorting, we need the key.
			// Let's store the full line "KEY=VALUE".
			fullLine := fmt.Sprintf("%s=%s", key, val)

			if strings.Contains(key, "__") {
				// App Variable
				// Determine prefix. Usually APPNAME__VAR
				parts := strings.Split(key, "__")
				prefix := parts[0]

				// Special case for sub-apps like RADARR__4K__VAR -> Prefix RADARR
				// Wait, Bash env_update logic is:
				// Global: ! [[ ${line} == *"__"* ]]
				// Apps: everything else.
				// Grouping:
				// Iterates through variables.
				// Uses `appname_from_servicename` equivalent?
				// Actually `env_update.sh` calls `env_update_global` then loops through ALL defined apps.
				// It iterates `yq` list of apps.
				// Then for each app, it greps `^${A}__` from the file.

				// Since we don't want to depend on yq/app list here if possible, let's infer from the keys?
				// Or should we assume the prefix is the app name?
				// If we have RADARR__4K__ENABLED, the prefix is RADARR.
				// If we have RADARR__ENABLED, the prefix is RADARR.

				// Let's use the first part as the grouping key.
				// Exception: maybe some vars have multiple underscores?
				// Standard DS pattern is APPNAME__VAR.

				appVars[prefix] = append(appVars[prefix], fullLine)
			} else {
				// Global Variable
				globals = append(globals, fullLine)
			}
		}
	}

	// 3. Sort Globals
	sort.Strings(globals)

	// 4. Sort Apps
	var appNames []string
	for app := range appVars {
		appNames = append(appNames, app)
	}
	sort.Strings(appNames)
	// Sort vars within apps
	for _, app := range appNames {
		sort.Strings(appVars[app])
	}

	// 5. Re-Write File
	fNew, err := os.Create(file)
	if err != nil {
		return err
	}
	defer fNew.Close()

	w := bufio.NewWriter(fNew)

	// Header
	// Bash adds a generic header or just starts?
	// env_update_global adds "# Global settings"
	w.WriteString("# Global settings\n")
	for _, line := range globals {
		w.WriteString(line + "\n")
	}
	w.WriteString("\n")

	// Apps
	for _, app := range appNames {
		// Need "Nice Name" for header?
		// We can try to format the app name nicely (Capitalize)
		// Or if we have access to the app definition, use it.
		// `update.go` is in `env` package, might not have `apps` package ref to avoid cycle?
		// `apps` imports `env`. `env` cannot import `apps`.
		// So we can't call `apps.NiceName`.
		// We'll just title case the key for now.
		niceName := TitleCase(strings.ToLower(app))

		// Handle sub-instances for header?
		// If sorting groups by prefix `RADARR`, `RADARR__4K` vars would be in `RADARR` group if we split by `__`?
		// Wait, `RADARR__4K__ENABLED`: `parts[0]` is `RADARR`.
		// So `RADARR__4K` vars end up in `RADARR` block.
		// Is that what we want?
		// Bash: `env_update` loops over `mapfile -t APP_LIST < <(params_list)`.
		// `params_list` gets all known apps.
		// It greps using the full app name. `RADARR` and `RADARR__4K` are separate apps in `params_list`?
		// If so, we should group by the full app prefix.

		// However, we are inferring groups from the existing file, we don't know the list of valid apps here without circular dependency.
		// If we group strictly by first token before `__`, `RADARR__4K` variables go into `RADARR`.
		// If `RADARR` and `RADARR__4K` are distinct apps, we might want to separate them.
		// But `RADARR__4K` starts with `RADARR__`.

		// Let's refine the grouping.
		// If we strictly follow the 'split by first __' rule:
		// RADARR__ENABLED -> RADARR
		// RADARR__4K__ENABLED -> RADARR
		// The header will be `# Radarr settings`.
		// And inside, `RADARR__4K__ENABLED` will just be sorted alphabetically next to `RADARR__ENABLED`.
		// This seems acceptable and robust enough without knowing the full app list.

		w.WriteString(fmt.Sprintf("# %s settings\n", niceName))
		for _, line := range appVars[app] {
			w.WriteString(line + "\n")
		}
		w.WriteString("\n")
	}

	return w.Flush()
}

func CopyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

func TitleCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
