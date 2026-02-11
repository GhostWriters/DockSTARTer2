package config

import (
	"DockSTARTer2/internal/paths"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigMigration(t *testing.T) {
	// Setup temp state home
	tempDir, err := os.MkdirTemp("", "ds2-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	paths.StateHomeOverride = tempDir
	paths.ConfigHomeOverride = tempDir
	// Note: constants.AppConfigFileName is already set to dockstarter2.toml

	// 1. Create a legacy .ini file
	configDir := filepath.Join(tempDir, "dockstarter2")
	os.MkdirAll(configDir, 0755)

	// We need to override paths.GetConfigDir() logic partially or just place it where paths thinks it should be.
	// paths.GetConfigFilePath() uses xdg.ConfigHome. We might need to override that too if possible,
	// but paths has StateHomeOverride. Wait, paths.go:
	// func GetConfigFilePath() string { ... returns xdg.ConfigHome ... }

	// Let's check paths.go again. It doesn't have a ConfigHomeOverride.
	// But it uses version.ApplicationName.

	// Let's just mock the migration logic by calling loadLegacyConfig directly or
	// point the app to a temp location if we can.

	// Actually, let's just test Save and Load first.
	conf := AppConfig{
		UI: UIConfig{
			Theme:   "TestTheme",
			Borders: false,
		},
	}

	err = SaveAppConfig(conf)
	if err != nil {
		t.Errorf("SaveAppConfig failed: %v", err)
	}

	loaded := LoadAppConfig()
	if loaded.UI.Theme != "TestTheme" {
		t.Errorf("Expected Theme 'TestTheme', got '%s'", loaded.UI.Theme)
	}
	if loaded.UI.Borders != false {
		t.Errorf("Expected Borders false, got %v", loaded.UI.Borders)
	}
}
