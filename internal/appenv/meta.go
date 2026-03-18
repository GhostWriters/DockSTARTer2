package appenv

import (
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/paths"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// VarMeta holds metadata for a single app-specific variable from a .meta.toml file.
type VarMeta struct {
	HelpLine string      `toml:"helpline"`
	HelpText string      `toml:"helptext"`
	Options  []VarOption `toml:"options"`
}

// AppMeta holds all variable metadata loaded from an app's .meta.toml file.
type AppMeta struct {
	Variables map[string]VarMeta
}

// GetVarMeta returns the metadata for a specific variable name.
//
// All .meta.toml keys use the APPNAME<__INSTANCE> pattern where <__INSTANCE>
// denotes an optional instance suffix. The <__INSTANCE> marker (literal text)
// acts as a wildcard that matches any instance or no instance at all.
//
// App-specific vars (.env.app.appname): "appname<__instance>:VARNAME"
//   - e.g. ["plex<__instance>:VERSION"], ["adminer<__instance>:ADMINER_DESIGN"]
//
// Main .env vars (carry APPNAME__ prefix): "APPNAME<__INSTANCE>__VARNAME"
//   - e.g. ["ADGUARD<__INSTANCE>__ENVIRONMENT_SERVERIP"]
//
// Lookup for main .env vars (varName has APPNAME__ prefix):
//  1. Exact key: "APPNAME__VARNAME" (no instance — rare override)
//  2. Instance pattern key: "APPNAME<__INSTANCE>__VARNAME"
//
// Lookup for app-specific vars (no APPNAME__ prefix):
//  1. Exact instance key: "APPNAME__INSTANCE:VARNAME" (instance-specific override)
//  2. Instance pattern key: "APPNAME<__INSTANCE>:VARNAME" (covers all instances)
//
// Returns zero value and false if no metadata exists.
func (m *AppMeta) GetVarMeta(varName, appName string) (VarMeta, bool) {
	if m == nil {
		return VarMeta{}, false
	}
	upper := strings.ToUpper(varName)
	upperApp := strings.ToUpper(appName)

	// Detect main .env vars: those with an APPNAME__ prefix.
	baseApp := strings.ToUpper(AppNameToBaseAppName(appName))
	if prefix := baseApp + "__"; strings.HasPrefix(upper, prefix) {
		varSuffix := upper[len(prefix):]
		// Strip any instance segment so varSuffix is the bare variable name.
		// e.g. ADGUARD__MYINSTANCE__ENVIRONMENT_SERVERIP → ENVIRONMENT_SERVERIP
		if inst := strings.ToUpper(AppNameToInstanceName(appName)); inst != "" {
			instPrefix := inst + "__"
			if strings.HasPrefix(varSuffix, instPrefix) {
				varSuffix = varSuffix[len(instPrefix):]
			}
		}
		// 1. Exact key (no instance): "ADGUARD__ENVIRONMENT_SERVERIP"
		if v, ok := m.Variables[baseApp+"__"+varSuffix]; ok {
			return v, ok
		}
		// 2. Instance pattern key: "ADGUARD<__INSTANCE>__ENVIRONMENT_SERVERIP"
		if v, ok := m.Variables[baseApp+"<__INSTANCE>__"+varSuffix]; ok {
			return v, ok
		}
		return VarMeta{}, false
	}

	// App-specific var (from .env.app.appname) — key is "APPNAME<__INSTANCE>:VARNAME".
	// 1. Exact instance-specific key: "APPNAME__INSTANCE:VARNAME"
	if v, ok := m.Variables[upperApp+":"+upper]; ok {
		return v, ok
	}
	// 2. Instance pattern key using base app name: "APPNAME<__INSTANCE>:VARNAME"
	if v, ok := m.Variables[baseApp+"<__INSTANCE>:"+upper]; ok {
		return v, ok
	}

	return VarMeta{}, false
}

// LoadAppMeta reads the .meta.toml file for the given app from the templates directory.
// Returns nil (not an error) if no meta file exists for the app.
func LoadAppMeta(appName string) (*AppMeta, error) {
	if appName == "" {
		return nil, nil
	}
	baseApp := strings.ToLower(AppNameToBaseAppName(appName))
	templatesDir := paths.GetTemplatesDir()
	metaFile := filepath.Join(templatesDir, constants.TemplatesDirName, baseApp, fmt.Sprintf("%s.meta.toml", baseApp))

	data, err := os.ReadFile(metaFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Top-level TOML keys are variable names, each mapping to a VarMeta table.
	var raw map[string]VarMeta
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", metaFile, err)
	}

	// Normalize all keys to uppercase for case-insensitive lookup.
	normalized := make(map[string]VarMeta, len(raw))
	for k, v := range raw {
		normalized[strings.ToUpper(k)] = v
	}

	return &AppMeta{Variables: normalized}, nil
}
