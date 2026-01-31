package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/paths"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// IsAppNameValid checks if an app name is valid according to DS rules.
func IsAppNameValid(appName string) bool {
	// 1. Strip leading/trailing colons
	name := strings.TrimSpace(appName)
	if strings.HasSuffix(name, ":") {
		name = name[:len(name)-1]
	} else if strings.HasPrefix(name, ":") {
		name = name[1:]
	}

	// 2. Regex check: ^[A-Z][A-Z0-9]*(__[A-Z0-9]+)?$
	re := regexp.MustCompile(`^[A-Z][A-Z0-9]*(__[A-Z0-9]+)?$`)
	if !re.MatchString(name) {
		return false
	}

	// 3. Instance name validation
	instance := AppNameToInstanceName(name)
	if instance != "" {
		if !InstanceNameIsValid(instance) {
			return false
		}
	}

	return true
}

// IsAppBuiltIn checks if the application has a corresponding template folder.
func IsAppBuiltIn(appName string) bool {
	base := AppNameToBaseAppName(appName)
	base = strings.ToLower(base)

	templatesDir := paths.GetTemplatesDir()
	appDir := filepath.Join(templatesDir, ".apps", base)
	info, err := os.Stat(appDir)
	return err == nil && info.IsDir()
}

// IsAppDeprecated checks if an app is marked deprecated in its labels.yml.
func IsAppDeprecated(ctx context.Context, appName string) bool {
	labelsFile, err := AppInstanceFile(ctx, appName, "*.labels.yml")
	if err != nil || labelsFile == "" {
		return false
	}

	content, err := os.ReadFile(labelsFile)
	if err != nil {
		return false
	}

	var labels LabelsFile
	if err := yaml.Unmarshal(content, &labels); err != nil {
		return false
	}

	for _, service := range labels.Services {
		if val, ok := service.Labels["com.dockstarter.appinfo.deprecated"]; ok {
			return strings.ToLower(strings.Trim(val, `"' `)) == "true"
		}
	}
	return false
}

// IsAppUserDefined checks if an app is user-defined (not built-in OR missing ENABLED var).
func IsAppUserDefined(ctx context.Context, appName string, envFile string) bool {
	appUpper := strings.ToUpper(appName)
	if IsAppBuiltIn(appUpper) {
		return false
	}
	exists, _ := EnvVarExists(ctx, appUpper+"__ENABLED", envFile)
	return exists
}

// IsAppAdded checks if an app is both builtin and has an __ENABLED variable.
func IsAppAdded(ctx context.Context, appName string, envFile string) bool {
	appUpper := strings.ToUpper(appName)
	exists, _ := EnvVarExists(ctx, appUpper+"__ENABLED", envFile)
	return IsAppBuiltIn(appUpper) && exists
}

// IsAppRunnable checks if an app has the required YML template files for the current architecture.
func IsAppRunnable(appName string, conf config.AppConfig) bool {
	basename := AppNameToBaseAppName(appName)
	templatesDir := paths.GetTemplatesDir()
	templateFolder := filepath.Join(templatesDir, ".apps", basename)

	// Check for main.yml
	mainYml := filepath.Join(templateFolder, basename+".yml")
	if _, err := os.Stat(mainYml); err != nil {
		return false
	}

	// Check for arch-specific yml
	archYml := filepath.Join(templateFolder, basename+"."+conf.Arch+".yml")
	if _, err := os.Stat(archYml); err != nil {
		return false
	}

	return true
}

// IsAppNonDeprecated is a wrapper that returns the opposite of IsAppDeprecated.
func IsAppNonDeprecated(ctx context.Context, appName string) bool {
	return !IsAppDeprecated(ctx, appName)
}

// IsAppEnabled checks if an app is enabled (ENABLED=true).
func IsAppEnabled(app, envFile string) bool {
	// bash checks value being IsTrue.
	// We need to read the value.
	val, _ := Get(app+"__ENABLED", envFile)
	return IsTrue(val)
}

// IsAppReferenced checks if an app is referenced in .env or compose override.
func IsAppReferenced(ctx context.Context, app string, conf config.AppConfig) bool {
	// Implementation from status.go
	envFile := filepath.Join(conf.ComposeDir, ".env")
	if IsAppEnabled(app, envFile) {
		return true
	}
	// Also check overrides...
	overrideFile := filepath.Join(conf.ComposeDir, "docker-compose.override.yml")
	if _, err := os.Stat(overrideFile); err == nil {
		content, _ := os.ReadFile(overrideFile)
		// Grep for .env.app.APPNAME
		if strings.Contains(string(content), ".env.app."+strings.ToLower(app)) {
			return true
		}
	}
	return false
}

// IsTrue checks if a string value represents true.
func IsTrue(val string) bool {
	val = strings.ToLower(strings.TrimSpace(val))
	return val == "true" || val == "yes" || val == "1" || val == "on"
}

// InstanceNameIsValid checks if an instance name is allowed.
func InstanceNameIsValid(name string) bool {
	invalidNames := map[string]bool{
		"CONTAINER": true, "DEVICE": true, "DEVICES": true, "ENABLED": true,
		"ENVIRONMENT": true, "HOSTNAME": true, "PORT": true, "NETWORK": true,
		"RESTART": true, "STORAGE": true, "STORAGE2": true, "STORAGE3": true,
		"STORAGE4": true, "TAG": true,
	}
	return !invalidNames[strings.ToUpper(name)]
}

// IsGlobalVar checks if a variable name is a global variable.
func IsGlobalVar(varName string) bool {
	appVarPattern := regexp.MustCompile(`^[A-Z][A-Z0-9]*(__[A-Z0-9]+)+\w+`)
	return !appVarPattern.MatchString(varName)
}

// VarNameIsValid validates if a variable name is valid.
func VarNameIsValid(varName string, varType string) bool {
	varType = strings.ToUpper(varType)
	switch varType {
	case "":
		return VarNameIsValid(varName, "_BARE_") || VarNameIsValid(varName, "_APPNAME_")
	case "_BARE_":
		return regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(varName)
	case "_GLOBAL_":
		if !VarNameIsValid(varName, "_BARE_") {
			return false
		}
		return VarNameToAppName(varName) == ""
	case "_APPNAME_":
		if !VarNameIsValid(varName, "_BARE_") {
			return false
		}
		return VarNameToAppName(varName) != ""
	default:
		if strings.HasSuffix(varType, ":") {
			if !strings.Contains(varName, ":") {
				return false
			}
			parts := strings.SplitN(varName, ":", 2)
			return strings.ToUpper(parts[0]) == strings.TrimSuffix(varType, ":") && VarNameIsValid(parts[1], "_BARE_")
		}
		if !VarNameIsValid(varName, "_BARE_") {
			return false
		}
		return strings.ToUpper(VarNameToAppName(varName)) == strings.ToUpper(varType)
	}
}

// EnvVarExists checks if a variable exists in the specified file.
// If key is "APPNAME:VARNAME", it resolves the app instance file.
// Mirrors env_var_exists.sh.
func EnvVarExists(ctx context.Context, key string, file string) (bool, error) {
	targetFile := file
	targetKey := key

	// Check for APPNAME:VARNAME syntax
	if strings.Contains(key, ":") {
		parts := strings.SplitN(key, ":", 2)
		appName := parts[0]
		targetKey = parts[1]

		f, err := AppInstanceFile(ctx, appName, ".env")
		if err != nil {
			return false, err
		}
		targetFile = f
	}

	if targetFile == "" {
		return false, fmt.Errorf("no file specified")
	}

	// Logic: grep -q -E "^\s*${VAR_NAME}\s*="
	line, err := GetLine(targetKey, targetFile)
	if err != nil {
		return false, err
	}
	return line != "", nil
}
