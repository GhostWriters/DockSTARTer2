package apps

import (
	"DockSTARTer2/internal/env"
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ListVars returns a list of variable names for the given app.
// If appName ends with ":", it lists all variables from the app-specific .env file.
// Otherwise, it lists variables from the global .env that match APPNAME__ pattern.
func ListVars(ctx context.Context, appName string, composeFolder string) ([]string, error) {
	var vars []string

	if strings.HasSuffix(appName, ":") {
		// Remove the trailing colon
		baseApp := strings.TrimSuffix(appName, ":")
		appEnvFile := filepath.Join(composeFolder, ".env.app."+strings.ToLower(baseApp))

		// List all variables in the app-specific env file
		keys, err := env.ListVars(appEnvFile)
		if err != nil {
			return nil, err
		}

		for key := range keys {
			vars = append(vars, key)
		}
	} else {
		// List variables from global .env that match APPNAME__ pattern
		globalEnv := filepath.Join(composeFolder, ".env")

		// The regex pattern: APPNAME__(?![A-Za-z0-9]+__)\w+
		// This matches APPNAME__VAR but not APPNAME__SUBAPP__VAR
		upperAppName := strings.ToUpper(appName)
		pattern := `^` + regexp.QuoteMeta(upperAppName) + `__\w+`

		f, err := os.Open(globalEnv)
		if err != nil {
			if os.IsNotExist(err) {
				return vars, nil
			}
			return nil, err
		}
		defer f.Close()

		// Regex to match variable lines
		re := regexp.MustCompile(pattern + `\s*=`)

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if re.MatchString(strings.TrimSpace(line)) {
				// Extract the variable name
				parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
				if len(parts) > 0 {
					varName := parts[0]
					// Check if it's not a sub-app variable (no double underscore after APPNAME__)
					suffix := strings.TrimPrefix(varName, upperAppName+"__")
					if !strings.Contains(suffix, "__") {
						vars = append(vars, varName)
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	return vars, nil
}

// ListVarLines returns a list of "KEY=VALUE" lines for the given app.
// If appName ends with ":", it lists all lines from the app-specific .env file.
// If appName is empty, it lists all non-app variables from the global .env.
// Otherwise, it lists lines from the global .env that match APPNAME__ pattern.
func ListVarLines(ctx context.Context, appName string, composeFolder string) ([]string, error) {
	var lines []string

	if appName == "" {
		// List all non-app variables from global .env
		globalEnv := filepath.Join(composeFolder, ".env")

		// Pattern to match app variables: [A-Z][A-Z0-9]*(__[A-Z0-9]+)+\w+
		appVarPattern := regexp.MustCompile(`^\s*[A-Z][A-Z0-9]*__[A-Z0-9_]+\w*\s*=`)
		commentOrEmptyPattern := regexp.MustCompile(`^\s*$|^\s*#`)

		f, err := os.Open(globalEnv)
		if err != nil {
			if os.IsNotExist(err) {
				return lines, nil
			}
			return nil, err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			// Skip empty lines and comments
			if commentOrEmptyPattern.MatchString(line) {
				continue
			}
			// Skip app variables
			if !appVarPattern.MatchString(line) {
				lines = append(lines, line)
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
	} else if strings.HasSuffix(appName, ":") {
		// Remove the trailing colon
		baseApp := strings.TrimSuffix(appName, ":")
		appEnvFile := filepath.Join(composeFolder, ".env.app."+strings.ToLower(baseApp))

		f, err := os.Open(appEnvFile)
		if err != nil {
			if os.IsNotExist(err) {
				return lines, nil
			}
			return nil, err
		}
		defer f.Close()

		commentOrEmptyPattern := regexp.MustCompile(`^\s*$|^\s*#`)

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			// Skip empty lines and comments
			if commentOrEmptyPattern.MatchString(line) {
				continue
			}
			lines = append(lines, line)
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
	} else {
		// List lines from global .env that match APPNAME__ pattern
		globalEnv := filepath.Join(composeFolder, ".env")

		upperAppName := strings.ToUpper(appName)
		pattern := `^\s*` + regexp.QuoteMeta(upperAppName) + `__\w+\s*=`

		f, err := os.Open(globalEnv)
		if err != nil {
			if os.IsNotExist(err) {
				return lines, nil
			}
			return nil, err
		}
		defer f.Close()

		re := regexp.MustCompile(pattern)

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if re.MatchString(line) {
				// Check if it's not a sub-app variable
				parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
				if len(parts) > 0 {
					varName := parts[0]
					suffix := strings.TrimPrefix(varName, upperAppName+"__")
					if !strings.Contains(suffix, "__") {
						lines = append(lines, line)
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	return lines, nil
}
