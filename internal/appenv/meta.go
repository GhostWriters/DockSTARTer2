package appenv

import (
	"context"
	"fmt"
	"os"
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
// AppInstanceFile processes the template before loading, substituting
// <__INSTANCE> with the actual instance value (or empty string). So all keys
// in the loaded Variables map are concrete — no pattern matching is needed.
//
// App-specific vars (.env.app.appname): key is "APPNAME[__INSTANCE]:VARNAME"
//   - e.g. "PLEX:VERSION" (no instance) or "PLEX__2:VERSION" (instance "2")
//
// Main .env vars (carry APPNAME__ prefix): key IS the full variable name
//   - e.g. "ADGUARD__ENVIRONMENT_SERVERIP" or "ADGUARD__HOME__ENVIRONMENT_SERVERIP"
//
// Returns zero value and false if no metadata exists.
func (m *AppMeta) GetVarMeta(varName, appName string) (VarMeta, bool) {
	if m == nil {
		return VarMeta{}, false
	}
	upper := strings.ToUpper(varName)
	upperApp := strings.ToUpper(appName)

	// Main .env vars have the APPNAME__ prefix — the full variable name is the key.
	baseApp := strings.ToUpper(AppNameToBaseAppName(appName))
	if strings.HasPrefix(upper, baseApp+"__") {
		v, ok := m.Variables[upper]
		return v, ok
	}

	// App-specific vars — key is "APPNAME[__INSTANCE]:VARNAME".
	v, ok := m.Variables[upperApp+":"+upper]
	return v, ok
}

// LoadAppMeta loads variable metadata for the given app instance.
//
// It uses AppInstanceFile to obtain the processed .meta.toml path, which
// substitutes <__INSTANCE> markers with the actual instance value (or empty
// string for the base app). This means the keys in the returned AppMeta are
// concrete — no wildcard matching is needed in GetVarMeta.
//
// Returns nil (not an error) if no meta file exists for the app.
func LoadAppMeta(ctx context.Context, appName string) (*AppMeta, error) {
	if appName == "" {
		return nil, nil
	}

	metaFile, err := AppInstanceFile(ctx, appName, "*.meta.toml")
	if err != nil {
		return nil, err
	}

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
