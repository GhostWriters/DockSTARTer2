package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/paths"
	"context"
	"os"
	"testing"
)

// TestVarDefaultValue mirrors test_var_default_value in var_default_value.sh.
// Tests structural defaults that don't depend on system state (PUID/PGID/TZ/etc.)
// by using an empty temp dir so AppInstanceFile finds no default file.
func TestVarDefaultValue(t *testing.T) {
	tempDir := t.TempDir()
	origState := paths.StateHomeOverride
	origTemplates := paths.TemplatesDirOverride
	paths.StateHomeOverride = tempDir
	paths.TemplatesDirOverride = tempDir
	defer func() {
		paths.StateHomeOverride = origState
		paths.TemplatesDirOverride = origTemplates
	}()

	ctx := context.Background()
	cfg := config.AppConfig{}

	tests := []struct {
		input    string
		expected string
	}{
		// APP vars — structural defaults when no template file exists
		{"NONEXISTENTAPP__ENABLED", "'false'"},
		{"NONEXISTENTAPP__CONTAINER_NAME", "'nonexistentapp'"},
		{"NONEXISTENTAPP__RESTART", "'unless-stopped'"},
		{"NONEXISTENTAPP__TAG", "'latest'"},
		{"NONEXISTENTAPP__NETWORK_MODE", "''"},
		{"NONEXISTENTAPP__PORT_80", "'80'"},
		{"NONEXISTENTAPP__PORT_8080", "'8080'"},
		{"NONEXISTENTAPP__VARNAME", "''"}, // unknown suffix
		{"NONEXISTENTAPP__HOSTNAME", "'Nonexistentapp'"},

		// Instanced app
		{"NONEXISTENTAPP__4K__ENABLED", "'false'"},
		{"NONEXISTENTAPP__4K__PORT_7878", "'7878'"},

		// GLOBAL vars — unknown returns ''
		{"NONEXISTENT_GLOBAL_VAR", "''"},
		{"TRULY_NONEXISTENT_XYZ123", "''"},
	}

	for _, test := range tests {
		result := VarDefaultValue(ctx, test.input, cfg)
		if result != test.expected {
			t.Errorf("VarDefaultValue(%q) = %q; want %q", test.input, result, test.expected)
		}
	}
}

// TestVarDefaultValueGlobalConfig tests GLOBAL vars that depend on config fields.
func TestVarDefaultValueGlobalConfig(t *testing.T) {
	// Ensure StateHomeOverride won't interfere; no need for templates here.
	origState := paths.StateHomeOverride
	paths.StateHomeOverride = t.TempDir()
	defer func() { paths.StateHomeOverride = origState }()

	ctx := context.Background()
	cfg := config.AppConfig{
		Paths: config.PathConfig{
			ConfigFolder:  "/opt/docker/config",
			ComposeFolder: "/opt/docker/compose",
		},
	}

	if got := VarDefaultValue(ctx, "DOCKER_CONFIG_FOLDER", cfg); got != "/opt/docker/config" {
		t.Errorf("VarDefaultValue(DOCKER_CONFIG_FOLDER) = %q; want %q", got, "/opt/docker/config")
	}
	if got := VarDefaultValue(ctx, "DOCKER_COMPOSE_FOLDER", cfg); got != "/opt/docker/compose" {
		t.Errorf("VarDefaultValue(DOCKER_COMPOSE_FOLDER) = %q; want %q", got, "/opt/docker/compose")
	}
}

// TestVarDefaultValueAppEnv tests the APPENV case (colon-format vars).
func TestVarDefaultValueAppEnv(t *testing.T) {
	tempDir := t.TempDir()
	origState := paths.StateHomeOverride
	origTemplates := paths.TemplatesDirOverride
	paths.StateHomeOverride = tempDir
	paths.TemplatesDirOverride = tempDir
	defer func() {
		paths.StateHomeOverride = origState
		paths.TemplatesDirOverride = origTemplates
	}()

	ctx := context.Background()
	cfg := config.AppConfig{}

	// Without a default .env.app.* template file, APPENV vars return ''
	if got := VarDefaultValue(ctx, "WATCHTOWER:WATCHTOWER_CLEANUP", cfg); got != "''" {
		t.Errorf("VarDefaultValue(WATCHTOWER:WATCHTOWER_CLEANUP) = %q; want \"''\"", got)
	}

	// Create a mock .env.app.watchtower template with a known default
	appDir := t.TempDir()
	appEnvFile := appDir + "/watchtower.env.app.watchtower"
	if err := os.WriteFile(appEnvFile, []byte("WATCHTOWER_CLEANUP='true'\n"), 0644); err != nil {
		t.Fatal(err)
	}
}
