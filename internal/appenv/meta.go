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
// Variable names are matched case-insensitively.
// Returns zero value and false if no metadata exists.
func (m *AppMeta) GetVarMeta(varName string) (VarMeta, bool) {
	if m == nil {
		return VarMeta{}, false
	}
	v, ok := m.Variables[strings.ToUpper(varName)]
	return v, ok
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
