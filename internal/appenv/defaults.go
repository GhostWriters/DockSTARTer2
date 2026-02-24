package appenv

import (
	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/system"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// VarDefaultValue returns the default value for a given variable.
// It mirrors the logic of var_default_value.sh.
func VarDefaultValue(ctx context.Context, key string, conf config.AppConfig) string {
	var appName, cleanVarName, varType string

	derivedAppName := VarNameToAppName(key)
	if derivedAppName != "" {
		appName = derivedAppName
		if strings.Contains(key, ":") {
			varType = "APPENV"
			parts := strings.SplitN(key, ":", 2)
			cleanVarName = parts[1]
		} else {
			varType = "APP"
			cleanVarName = key
		}
	} else {
		varType = "GLOBAL"
		cleanVarName = key
	}

	switch varType {
	case "APP":
		defFile, _ := AppInstanceFile(ctx, appName, constants.EnvFileName)
		if defFile != "" {
			exists, _ := EnvVarExists(ctx, cleanVarName, defFile)
			if exists {
				val, _ := GetLiteral(cleanVarName, defFile)
				return val
			}
		}

		suffix := strings.TrimPrefix(cleanVarName, appName+"__")
		appNiceName := GetNiceName(ctx, appName)
		appLower := strings.ToLower(appName)

		switch suffix {
		case "CONTAINER_NAME":
			return fmt.Sprintf("'%s'", appLower)
		case "ENABLED":
			return "'false'"
		case "HOSTNAME":
			return fmt.Sprintf("'%s'", appNiceName)
		case "NETWORK_MODE":
			return "''"
		case "RESTART":
			return "'unless-stopped'"
		case "TAG":
			return "'latest'"
		case "VOLUME_DOCKER_SOCKET":
			return "\"${DOCKER_VOLUME_DOCKER_SOCKET?}\""
		default:
			matched, _ := regexp.MatchString(`^PORT_[0-9]+$`, suffix)
			if matched {
				portVal := strings.TrimPrefix(suffix, "PORT_")
				return fmt.Sprintf("'%s'", portVal)
			}
			return "''"
		}

	case "APPENV":
		defFile, _ := AppInstanceFile(ctx, appName, fmt.Sprintf("%s*", constants.AppEnvFileNamePrefix))
		if defFile != "" {
			exists, _ := EnvVarExists(ctx, cleanVarName, defFile)
			if exists {
				val, _ := GetLiteral(cleanVarName, defFile)
				return val
			}
		}
		return "''"

	case "GLOBAL":
		switch cleanVarName {
		case "DOCKER_COMPOSE_FOLDER":
			return conf.Paths.ComposeFolder
		case "DOCKER_CONFIG_FOLDER":
			return conf.Paths.ConfigFolder
		case "DOCKER_VOLUME_DOCKER_SOCKET":
			return fmt.Sprintf("'%s'", DetectDockerSocket())
		case "DOCKER_GID":
			return fmt.Sprintf("'%s'", GroupId(ctx, "docker"))
		case "DOCKER_HOSTNAME":
			h, _ := os.Hostname()
			return fmt.Sprintf("'%s'", h)
		case "GLOBAL_LAN_NETWORK":
			return fmt.Sprintf("'%s'", DetectLANNetwork())
		case "PGID":
			_, pgid := system.GetIDs()
			return fmt.Sprintf("'%d'", pgid)
		case "PUID":
			puid, _ := system.GetIDs()
			return fmt.Sprintf("'%d'", puid)
		case "TZ":
			return fmt.Sprintf("'%s'", detectTZ())
		default:
			// Fallback to .env.example from embedded assets
			defs := getExampleDefaults()
			if val, ok := defs[cleanVarName]; ok {
				// Don't wrap in quotes if it already looks quoted or is a shell expression
				if strings.Contains(val, "$") || strings.HasPrefix(val, "'") || strings.HasPrefix(val, "\"") {
					return val
				}
				return fmt.Sprintf("'%s'", val)
			}
			return "''"
		}
	}
	return "''"
}

var (
	exampleDefaults map[string]string
	exampleOnce     sync.Once
)

func getExampleDefaults() map[string]string {
	exampleOnce.Do(func() {
		exampleDefaults = make(map[string]string)
		data, err := assets.GetDefaultEnv()
		if err != nil {
			return
		}

		re := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)=(.*)`)
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := scanner.Text()
			matches := re.FindStringSubmatch(line)
			if matches != nil {
				key := matches[1]
				val := matches[2] // Keep original for now, VarDefaultValue handles quotes
				exampleDefaults[key] = val
			}
		}
	})
	return exampleDefaults
}

func detectTZ() string {
	tz := "Etc/UTC"

	// 1. Try /etc/timezone (standard on many distros)
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		tz = strings.TrimSpace(string(data))
		if tz != "" {
			return tz
		}
	}

	// 2. Try parsing /etc/localtime symlink
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		// Example: /usr/share/zoneinfo/America/New_York
		if strings.Contains(link, "zoneinfo/") {
			parts := strings.SplitN(link, "zoneinfo/", 2)
			if len(parts) == 2 {
				return parts[1]
			}
		}
	}

	return tz
}
