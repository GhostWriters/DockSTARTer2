package config

import (
	"os"
	"path/filepath"
	"testing"

	"DockSTARTer2/internal/paths"

	toml "github.com/pelletier/go-toml/v2"
)

// loadFromBytes mirrors LoadAppConfig's overlay-and-migrate steps for an
// in-memory config file.
func loadFromBytes(t *testing.T, data string) AppConfig {
	t.Helper()
	conf := DefaultConfig()
	if _, err := UnmarshalRobust([]byte(data), &conf); err != nil {
		t.Fatalf("UnmarshalRobust: %v", err)
	}
	migrateToAppearance([]byte(data), &conf)
	migrateAnsiPaletteMenuNesting([]byte(data), &conf)
	migrateAnsiPaletteInheritGap([]byte(data), &conf)
	return conf
}

func TestMigrateFlatUIToAppearance(t *testing.T) {
	conf := loadFromBytes(t, `
[ui]
theme = "Firehouse"
borders = false
shadow_level = 4
spinner_speed = 200
hyperlinks = "off"
`)
	if conf.Appearance.SpinnerSpeed != 200 || conf.Appearance.Hyperlinks != "off" {
		t.Errorf("shared settings not migrated: spinner_speed=%d hyperlinks=%q", conf.Appearance.SpinnerSpeed, conf.Appearance.Hyperlinks)
	}
	def := DefaultConfig().Appearance.Local
	for _, ct := range ConnTypes {
		a := conf.Appearance.ForConnType(ct)
		if a.Theme != "Firehouse" || a.Borders || a.ShadowLevel != 4 {
			t.Errorf("%s: theme=%q borders=%v shadow_level=%d, want Firehouse/false/4", ct, a.Theme, a.Borders, a.ShadowLevel)
		}
		if a.TabLayout != def.TabLayout {
			t.Errorf("%s: unset tab_layout = %q, want default %q", ct, a.TabLayout, def.TabLayout)
		}
	}
}

func TestMigrateTopLevelAnsiPalette(t *testing.T) {
	conf := loadFromBytes(t, `
[ansi_palette.web.menu]
tint = "embedded:dracula"

[ansi_palette.web.programbox]
tint = "embedded:nord"
`)
	web := conf.Appearance.Web.AnsiColors
	if web.Tint != "embedded:dracula" || web.ProgramBox.Tint != "embedded:nord" {
		t.Errorf("web palette not migrated: menu=%q programbox=%q", web.Tint, web.ProgramBox.Tint)
	}
	// menu present, cli absent: the released inherit gap freezes menu's tint into cli.
	if web.CLI.Tint != "embedded:dracula" {
		t.Errorf("web cli tint = %q, want menu's %q", web.CLI.Tint, "embedded:dracula")
	}
}

func TestMigrateLegacyFlatAnsiPalette(t *testing.T) {
	conf := loadFromBytes(t, `
[ansi_palette]
apply_to_cli = false

[ansi_palette.local]
tint = "embedded:gruvbox"
`)
	local := conf.Appearance.Local.AnsiColors
	if local.Tint != "embedded:gruvbox" || local.ProgramBox.Tint != "embedded:gruvbox" {
		t.Errorf("local palette not migrated: menu=%q programbox=%q", local.Tint, local.ProgramBox.Tint)
	}
	if local.CLI.Tint != "" {
		t.Errorf("local cli tint = %q, want empty (apply_to_cli = false)", local.CLI.Tint)
	}
}

func TestAppearanceFileNotRemigrated(t *testing.T) {
	conf := loadFromBytes(t, `
[ui]
theme = "Firehouse"

[appearance.ssh]
theme = "Murica"
`)
	if got := conf.Appearance.SSH.Theme; got != "Murica" {
		t.Errorf("ssh theme = %q, want Murica", got)
	}
	if got := conf.Appearance.Local.Theme; got == "Firehouse" {
		t.Errorf("stale [ui] migrated over an existing [appearance] table")
	}
}

func TestMigratedConfigDropsOldTables(t *testing.T) {
	conf := loadFromBytes(t, `
[ui]
theme = "Firehouse"

[ansi_palette.web.menu]
tint = "embedded:dracula"
`)
	data, err := toml.Marshal(conf)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ui", "ansi_palette"} {
		if _, ok := raw[key]; ok {
			t.Errorf("saved config still has a top-level [%s] table", key)
		}
	}
}

// TestLoadAppConfigMigratesOldFile runs the migration through LoadAppConfig
// itself, the way a real startup reaches it.
func TestLoadAppConfigMigratesOldFile(t *testing.T) {
	dir := t.TempDir()
	prevConfig, prevState := paths.ConfigHomeOverride, paths.StateHomeOverride
	paths.ConfigHomeOverride, paths.StateHomeOverride = dir, dir
	t.Cleanup(func() { paths.ConfigHomeOverride, paths.StateHomeOverride = prevConfig, prevState })

	cfgPath := paths.GetConfigFilePath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	old := `
[ui]
theme = 'RetroDockSTARTer'
spinner_speed = 200

[paths]
config_folder = '${XDG_CONFIG_HOME}'
compose_folder = '${XDG_CONFIG_HOME}/compose'

[ansi_palette.web.menu]
tint = 'embedded:dracula'
`
	if err := os.WriteFile(cfgPath, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	conf := LoadAppConfig()
	for _, ct := range ConnTypes {
		if got := conf.Appearance.ForConnType(ct).Theme; got != "RetroDockSTARTer" {
			t.Errorf("%s theme = %q, want RetroDockSTARTer", ct, got)
		}
	}
	if conf.Appearance.SpinnerSpeed != 200 {
		t.Errorf("spinner_speed = %d, want 200", conf.Appearance.SpinnerSpeed)
	}
	if got := conf.Appearance.Web.AnsiColors.Tint; got != "embedded:dracula" {
		t.Errorf("web tint = %q, want embedded:dracula", got)
	}

	// The saved file must now be in the new layout and load back the same.
	if again := LoadAppConfig(); again.Appearance.Local.Theme != "RetroDockSTARTer" {
		t.Errorf("reloaded local theme = %q, want RetroDockSTARTer", again.Appearance.Local.Theme)
	}
}
