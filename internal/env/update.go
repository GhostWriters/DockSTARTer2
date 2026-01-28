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
			// Reconstruct line in clean format
			fullLine := fmt.Sprintf("%s=%s", key, val)

			if strings.Contains(key, "__") {
				// Try to extract app name
				appName := VarNameToAppName(key)
				if appName != "" {
					// Validate the app name
					if !AppNameIsValid(appName) {
						// Invalid app name (e.g., has reserved instance like RADARR__ENABLED)
						// Extract base app and group under that instead
						parts := strings.Split(appName, "__")
						if len(parts) > 0 {
							baseApp := parts[0]
							appVars[baseApp] = append(appVars[baseApp], fullLine)
						} else {
							// Shouldn't happen, but fallback to global
							globals = append(globals, fullLine)
						}
					} else {
						// Valid app name, use it
						appVars[appName] = append(appVars[appName], fullLine)
					}
				} else {
					// Extraction failed, treat as global
					globals = append(globals, fullLine)
				}
			} else {
				// No double underscore, it's a global variable
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
	w.WriteString("# Global settings\n")
	for _, line := range globals {
		w.WriteString(line + "\n")
	}
	w.WriteString("\n")

	// Apps
	for _, app := range appNames {
		niceName := formatTitle(app)

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

func formatTitle(s string) string {
	// RADARR__4K -> Radarr (4k)
	// RADARR -> Radarr

	if strings.Contains(s, "__") {
		parts := strings.SplitN(s, "__", 2)
		base := strings.Title(strings.ToLower(parts[0]))
		instance := strings.ToLower(parts[1])
		return fmt.Sprintf("%s (%s)", base, instance)
	}
	return strings.Title(strings.ToLower(s))
}
