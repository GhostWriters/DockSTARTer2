package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"context"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"runtime"
	"strings"
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
		case "DOCKER_GID":
			return fmt.Sprintf("'%s'", GroupId(ctx, "docker"))
		case "DOCKER_HOSTNAME":
			h, _ := os.Hostname()
			return fmt.Sprintf("'%s'", h)
		case "GLOBAL_LAN_NETWORK":
			return fmt.Sprintf("'%s'", DetectLANNetwork())
		case "PGID":
			return fmt.Sprintf("'%s'", getDefaultPgid())
		case "PUID":
			return fmt.Sprintf("'%s'", getDefaultPuid())
		case "TZ":
			tz := "Etc/UTC"
			if data, err := os.ReadFile("/etc/timezone"); err == nil {
				tz = strings.TrimSpace(string(data))
			}
			return fmt.Sprintf("'%s'", tz)
		default:
			return "''"
		}
	}
	return "''"
}

func getDefaultPuid() string {
	currentUser, err := user.Current()
	if err == nil {
		if runtime.GOOS == "windows" {
			return "1000"
		}
		return currentUser.Uid
	}
	return "1000"
}

func getDefaultPgid() string {
	currentUser, err := user.Current()
	if err == nil {
		if runtime.GOOS == "windows" {
			return "1000"
		}
		return currentUser.Gid
	}
	return "1000"
}
