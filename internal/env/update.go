package env

import (
	"DockSTARTer2/internal/logger"
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Update regenerates the .env file to ensure correct sorting and headers.
// Mirrors env_update.sh functionality using appvars_lines.sh logic.
func Update(ctx context.Context, file string) error {
	logger.Info(ctx, "Updating '{{_File_}}%s{{|-|}}'.", file)

	// 1. Backup
	bak := file + ".bak"
	if err := CopyFile(file, bak); err != nil {
		logger.Warn(ctx, "Failed to backup .env: %v", err)
	}

	// 2. Read all lines from file
	input, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	allLines := strings.Split(string(input), "\n")

	// 3. Extract globals using AppVarsLines with empty appName
	globals := AppVarsLines("", allLines)

	// 4. Identify all unique app names from the file
	appNamesMap := make(map[string]bool)
	for _, line := range allLines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Extract variable name
		idx := strings.Index(line, "=")
		if idx > 0 {
			varName := strings.TrimSpace(line[:idx])
			appName := VarNameToAppName(varName)
			if appName != "" && AppNameIsValid(appName) {
				appNamesMap[appName] = true
			}
		}
	}

	// 5. For each app, extract its variables using AppVarsLines
	appVars := make(map[string][]string)
	for appName := range appNamesMap {
		appVars[appName] = AppVarsLines(appName, allLines)
	}

	// 6. Sort globals and apps
	sort.Strings(globals)

	var appNames []string
	for app := range appVars {
		appNames = append(appNames, app)
	}
	sort.Strings(appNames)

	// Sort vars within each app
	for _, app := range appNames {
		sort.Strings(appVars[app])
	}

	// 7. Re-write file with headers
	fNew, err := os.Create(file)
	if err != nil {
		return err
	}
	defer fNew.Close()

	w := bufio.NewWriter(fNew)

	// Global header
	w.WriteString("# Global settings\n")
	for _, line := range globals {
		w.WriteString(line + "\n")
	}
	w.WriteString("\n")

	// App sections
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
