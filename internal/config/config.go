package config

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"DockSTARTer2/internal/assets"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"github.com/GhostWriters/semstyle"

	"github.com/adrg/xdg"
	"github.com/go-viper/mapstructure/v2"
	toml "github.com/pelletier/go-toml/v2"
)

// MinRefreshRateMS and MaxRefreshRateMS bound UI.RefreshRate (screen repaint
// interval, in milliseconds). Shared by config validation, the Appearance
// menu's prompt, and the Browser Settings dialog's refresh-rate field.
const (
	MinRefreshRateMS = 16
	MaxRefreshRateMS = 1000
)

func defaultConfigBytes() []byte {
	b, _ := assets.GetDefaultConfig()
	return b
}

// ThemeDefaultsOverlayHook, if set, is the last step of MigrateFromLegacy:
// it overlays the theme's suggested defaults for fields not in
// legacyPresent. Set by the theme package to avoid a config->theme cycle.
var ThemeDefaultsOverlayHook func(conf *AppConfig, legacyPresent map[string]bool)

// ServerTLSDefaultHook, if set, is called every time an existing DS2 config
// file is loaded (not on first-run/legacy migration, which has nothing to
// preserve), right after present is known. It backfills server.web.tls for a
// file saved before that field existed -- present["TLS"] is false in exactly
// that one-time case, since SaveAppConfig always writes every field's
// current value, so a file that already carries "tls" (from any save after
// this shipped) is left alone forever after. Set by the serve package
// (which knows whether the web server is already configured/running or
// installed as a system service) to avoid a config->serve cycle.
var ServerTLSDefaultHook func(conf *AppConfig, present map[string]bool)

// DefaultConfig returns an AppConfig populated purely from the embedded defaults TOML.
func DefaultConfig() AppConfig {
	var conf AppConfig
	_ = toml.Unmarshal(defaultConfigBytes(), &conf)
	return conf
}

type migrationModeKey struct{}

// isMigrationMode reports whether ctx was created by LoadAppConfig's one-time,
// no-config-file-yet bootstrap. Distinguishes that path (safe to hard-exit:
// runs once, before the server ever starts accepting sessions) from
// validate()'s per-load repair of an already-live config, which can run
// mid-session on a long-running --server-daemon and must never abort the
// whole process over one user's compose_folder problem.
func isMigrationMode(ctx context.Context) bool {
	v, _ := ctx.Value(migrationModeKey{}).(bool)
	return v
}

// AppConfig holds the application configuration settings.
type AppConfig struct {
	Appearance AppearanceConfig `toml:"appearance"`
	Paths      PathConfig       `toml:"paths"`
	Server     ServerConfig     `toml:"server"`
	System     SystemConfig     `toml:"system"`

	// These are helper fields for runtime use, not saved to TOML
	Arch       string     `toml:"-"`
	ConfigDir  string     `toml:"-"`
	ComposeDir string     `toml:"-"`
	RawPaths   PathConfig `toml:"-"` // Unexpanded values as read from TOML
}

// SystemConfig holds host-system integration settings.
type SystemConfig struct {
	// SetcapAsked records whether the one-time AutoSetcap question has
	// already been put to the user, so it's never asked twice. Setting
	// AutoSetcap true by hand works without this: an enabled AutoSetcap
	// applies regardless of whether the question was ever asked.
	SetcapAsked bool `toml:"setcap_asked"`
	// AutoSetcap enables the optional Linux capability grant
	// (CAP_CHOWN/CAP_FOWNER via "sudo setcap") that lets DS2 fix file
	// ownership/permissions without sudo. When true, the grant is
	// re-applied automatically whenever the binary lacks the capabilities
	// (e.g. after a self-update replaced it).
	AutoSetcap bool `toml:"auto_setcap"`
}

// ServerConfig holds SSH and web server settings.
// The server is active when ssh.port > 0. Set ssh.port = 0 (or omit it) to
// disable. There is no separate enabled flag — the port is the intent signal.
type ServerConfig struct {
	SSH     SSHConfig  `toml:"ssh"`
	Web     WebConfig  `toml:"web"`
	Auth    AuthConfig `toml:"auth"`
	HostKey string     `toml:"host_key"` // Path to persistent host key file
}

// SSHConfig holds settings for the SSH server.
type SSHConfig struct {
	Port int `toml:"port"` // TCP port for the SSH server (0 = disabled)
}

// WebConfig holds settings for the optional sip-based web frontend.
type WebConfig struct {
	Port int `toml:"port"` // TCP port for the web server (0 = disabled)

	// TLS selects how the web server serves connections:
	//   "self-signed" (default, or empty) -- sip generates and manages its
	//     own self-signed certificate. Browsers show a one-time warning.
	//   "cert" -- use the certificate/key at TLSCert/TLSKey.
	//   "none" -- plain HTTP, no encryption. Anyone on the network path can
	//     read (and, with auth.mode = "password", capture) traffic.
	TLS     string `toml:"tls"`
	TLSCert string `toml:"tls_cert"` // Path to a certificate file, when tls = "cert"
	TLSKey  string `toml:"tls_key"`  // Path to the certificate's private key, when tls = "cert"
}

// AnsiElementColors holds one UI element's resolved ANSI palette state --
// the 16 standard ANSI slots plus base16/base24's 8 extra slots. Each
// field accepts anything semstyle.ToColor understands: a hex value
// ("#ffffff"), one of the 16 ANSI names, or any broader color name tcell
// resolves (e.g. "grey"). Empty leaves that slot at the terminal's own
// default. See AnsiColors for how an element's own palette relates to its
// connType's other elements.
type AnsiElementColors struct {
	// TintEnabled turns applying Tint on or off without discarding it --
	// set via --theme-tint, cleared via --theme-no-tint. Independent of
	// OverrideEnabled/the 16 explicit fields below. The embedded default
	// config sets this true.
	TintEnabled bool `toml:"tint_enabled"`

	// OverrideEnabled turns applying the 16 explicit fields below on or
	// off as a group, without discarding any of them -- set via
	// --theme-ansi-override, cleared via --theme-no-ansi-override. A
	// single slot can still be cleared individually regardless of this
	// (set it to "none" via --ansi-override). Independent of
	// TintEnabled/Tint. The embedded default config sets this true.
	OverrideEnabled bool `toml:"override_enabled"`

	// Tint is an optional reference to a tinted-theming base16/base24
	// scheme -- its colors seed this palette per tinted-theming's
	// documented terminal mapping (see config.ParseBase16Scheme). Any of
	// the 16 fields below set explicitly here still wins over the scheme
	// for that one slot. Set via `--tint <ref> [types]`; same
	// "<kind>:<name-or-path>" convention as ui.theme's "user:"/"file:"
	// prefixes, extended with two more sources (see resolveTintRef in
	// internal/tui and ResolveTintRefData/ResolveTintArg in
	// internal/commands):
	//   - "file:<path>"    an arbitrary scheme YAML file, read live
	//   - "user:<name>"    a user-supplied file under paths.GetTintsDir()
	//   - "embedded:<name>" one of DS2's own bundled schemes (see
	//                       assets.GetTintTheme/--tint-list-embedded)
	//   - "repo:<name>"    a named scheme from a local clone of
	//                      github.com/tinted-theming/schemes (see
	//                      --tint-list/--tint-table). <name> may itself
	//                      start with "base16-"/"base24-" (tinty's own
	//                      scheme-ID convention, e.g. "repo:base16-mocha")
	//                      to force which of the repo's two subfolders to
	//                      read from, instead of the default
	//                      base24-then-base16 preference
	// A bare, unprefixed name is resolved by trying "user:", then
	// "embedded:", then "repo:" (see ResolveTintArg) -- whichever it's
	// found under becomes what's actually persisted here, so e.g. a
	// user-supplied scheme named the same as a bundled one takes
	// precedence, without needing "user:" spelled out. "user:"/"embedded:"
	// work out of the box with nothing to download or copy first, so
	// either is safe to set as a shipped default; "repo:" (or a bare name
	// that falls through to it) triggers a one-time clone on first
	// resolution if that clone doesn't exist yet. Empty ("") or "none:"
	// clears it -- not bare "none" (no colon), which stays a valid, if
	// unlikely, scheme name instead of being reserved as a keyword.
	Tint string `toml:"tint"`

	// Named after their base16/base24 slot (see tinted-theming/base24's
	// styling.md) rather than the classic ANSI name, so this section reads
	// the same as a scheme YAML file's own "palette:" block -- copy a value
	// from either straight into the other. Every name below is still
	// recognized as an equivalent alias wherever a color name is accepted
	// (semstyle.ToColorCtx, --ansi-override, etc.); base0X is canonical here
	// purely for this file's own field/serialization identity.
	Base00 string `toml:"base00"` // black
	Base08 string `toml:"base08"` // red
	Base0B string `toml:"base0b"` // green
	Base0A string `toml:"base0a"` // yellow
	Base0D string `toml:"base0d"` // blue
	Base0E string `toml:"base0e"` // magenta
	Base0C string `toml:"base0c"` // cyan
	Base05 string `toml:"base05"` // white
	Base03 string `toml:"base03"` // bright black
	Base12 string `toml:"base12"` // bright red
	Base14 string `toml:"base14"` // bright green
	Base13 string `toml:"base13"` // bright yellow
	Base16 string `toml:"base16"` // bright blue
	Base17 string `toml:"base17"` // bright magenta
	Base15 string `toml:"base15"` // bright cyan
	Base07 string `toml:"base07"` // bright white

	// The 8 base16/base24 slots with no ANSI terminal assignment
	// (backgrounds and the "orange"/"brown" categories). Stored so an
	// explicit scheme/config value survives instead of being discarded;
	// left empty, the active tint derives a stand-in from the 16 fields
	// above instead (see semstyle.Palette.slot's fallback logic).
	Base01 string `toml:"base01"`
	Base02 string `toml:"base02"`
	Base04 string `toml:"base04"`
	Base06 string `toml:"base06"`
	Base09 string `toml:"base09"`
	Base0F string `toml:"base0f"`
	Base10 string `toml:"base10"`
	Base11 string `toml:"base11"`
}

// AnsiColors holds one connType's ("local"/"ssh"/"web") full ANSI tint
// configuration: three independent, always-present elements -- "menu"
// (AnsiElementColors, embedded -- DS2's own menus, dialogs, and panels),
// "programbox" (a streamed command dialog's output), and "cli" (a bare,
// non-interactive invocation). No element inherits another's palette --
// each is a plain, self-contained AnsiElementColors, so a fresh install's
// shipped defaults (or a config migration) is the only place "programbox/
// cli start out matching menu" is ever decided; once set, an element never
// implicitly follows another's later changes, and there is no "unset"
// state for any element to fall back to menu through. Set an element's
// palette via `--tint <ref> [connType-list] [element-list]`, naming
// "programbox"/"cli" in element-list (see tintElements in
// internal/commands); an omitted element-list targets every element.
type AnsiColors struct {
	// Tagged "menu" so it nests as its own table ([ansi_palette.web.menu]),
	// symmetric with ProgramBox/CLI below, rather than sitting inline at
	// [ansi_palette.web] -- embedding still promotes its fields for Go code
	// (c.TintEnabled, &c.Base00, etc., used throughout --ansi-override/
	// --theme-tint), unaffected by the tag, which only controls the TOML
	// table name.
	AnsiElementColors `toml:"menu"`

	// ProgramBox is a streamed command dialog's own palette (e.g. `docker
	// compose up` progress in a ProgramBox) -- independent of menu's, so a
	// user can have the menu system tinted while command output renders
	// with the terminal's own native palette, or the reverse.
	ProgramBox AnsiElementColors `toml:"programbox"`

	// CLI is a bare, non-interactive invocation's own palette (e.g. `ds2
	// --tint` run directly from a shell, not through the interactive
	// TUI). Only ever consulted for "local" -- a bare CLI invocation is
	// always a local shell process (see cmd.Execute's only caller,
	// main.go), never reachable over SSH or web -- but kept on SSH/Web
	// too so all three connTypes share one struct shape.
	CLI AnsiElementColors `toml:"cli"`
}

// Element returns element's own palette. "menu", "", and any name Element
// doesn't recognize all return menu's. See ElementPtr to target a specific
// element for writing.
func (c AnsiColors) Element(element string) AnsiElementColors {
	switch element {
	case "programbox":
		return c.ProgramBox
	case "cli":
		return c.CLI
	default:
		return c.AnsiElementColors
	}
}

// ElementPtr returns a pointer to element's own fields, for writing.
// "menu"/"" (and anything ElementPtr doesn't recognize) returns
// &c.AnsiElementColors; "programbox"/"cli" return &c.ProgramBox/&c.CLI.
func (c *AnsiColors) ElementPtr(element string) *AnsiElementColors {
	switch element {
	case "programbox":
		return &c.ProgramBox
	case "cli":
		return &c.CLI
	default:
		return &c.AnsiElementColors
	}
}

// Slots returns the palette's 16 entries in ANSI index order (0-15), paired
// with their configured override value (possibly empty).
func (c AnsiElementColors) Slots() [16]string {
	return [16]string{
		c.Base00, c.Base08, c.Base0B, c.Base0A,
		c.Base0D, c.Base0E, c.Base0C, c.Base05,
		c.Base03, c.Base12, c.Base14, c.Base13,
		c.Base16, c.Base17, c.Base15, c.Base07,
	}
}

// WithDefaults returns c with any empty slot filled from fallback (e.g. a
// Tint-derived palette), leaving every slot c sets explicitly untouched --
// an explicit value always wins over a scheme's.
func (c AnsiElementColors) WithDefaults(fallback AnsiElementColors) AnsiElementColors {
	fill := func(v, d string) string {
		if v == "" {
			return d
		}
		return v
	}
	return AnsiElementColors{
		Base00: fill(c.Base00, fallback.Base00),
		Base08: fill(c.Base08, fallback.Base08),
		Base0B: fill(c.Base0B, fallback.Base0B),
		Base0A: fill(c.Base0A, fallback.Base0A),
		Base0D: fill(c.Base0D, fallback.Base0D),
		Base0E: fill(c.Base0E, fallback.Base0E),
		Base0C: fill(c.Base0C, fallback.Base0C),
		Base05: fill(c.Base05, fallback.Base05),
		Base03: fill(c.Base03, fallback.Base03),
		Base12: fill(c.Base12, fallback.Base12),
		Base14: fill(c.Base14, fallback.Base14),
		Base13: fill(c.Base13, fallback.Base13),
		Base16: fill(c.Base16, fallback.Base16),
		Base17: fill(c.Base17, fallback.Base17),
		Base15: fill(c.Base15, fallback.Base15),
		Base07: fill(c.Base07, fallback.Base07),
		Base01: fill(c.Base01, fallback.Base01),
		Base02: fill(c.Base02, fallback.Base02),
		Base04: fill(c.Base04, fallback.Base04),
		Base06: fill(c.Base06, fallback.Base06),
		Base09: fill(c.Base09, fallback.Base09),
		Base0F: fill(c.Base0F, fallback.Base0F),
		Base10: fill(c.Base10, fallback.Base10),
		Base11: fill(c.Base11, fallback.Base11),
	}
}

// AuthConfig holds authentication settings for the SSH server.
type AuthConfig struct {
	// Mode: "password", "pubkey", or "none" (none prints a warning on startup)
	Mode         string `toml:"mode"`
	Password     string `toml:"password"`       // bcrypt hash of the password
	AuthKeysFile string `toml:"auth_keys_file"` // Path to authorized_keys file
}

// PathConfig holds directory path settings.
type PathConfig struct {
	ConfigFolder  string `toml:"config_folder"`
	ComposeFolder string `toml:"compose_folder"`
}

// getArch returns the CPU architecture (x86_64 or aarch64).
func getArch() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return arch
	}
}

// ExpandVariables expands environment variables in the config values.
// It supports:
// - ${XDG_CONFIG_HOME} -> xdg.ConfigHome
// - ${XDG_DATA_HOME}   -> xdg.DataHome
// - ${XDG_STATE_HOME}  -> xdg.StateHome
// - ${XDG_CACHE_HOME}  -> xdg.CacheHome
// - ${HOME}            -> os.UserHomeDir()
// - ${USER}            -> Current username
// - ~/...              -> home-relative path (tilde expansion)
// - ${ANY}             -> os.Getenv("ANY") fallback for unrecognised variables
func ExpandVariables(val string) string {
	// Support legacy ${VAR?} syntax by normalizing it to ${VAR}
	// Use regex to be precise: find ${...?} and replace with ${...}
	re := regexp.MustCompile(`\$\{([^}]+)\?\}`)
	val = re.ReplaceAllString(val, `${$1}`)

	// Tilde expansion: ~/... or bare ~
	if val == "~" || strings.HasPrefix(val, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			val = home + val[1:]
		}
	}

	mapper := func(varName string) string {
		switch varName {
		case "XDG_CONFIG_HOME":
			return xdg.ConfigHome
		case "XDG_DATA_HOME":
			return xdg.DataHome
		case "XDG_STATE_HOME":
			return xdg.StateHome
		case "XDG_CACHE_HOME":
			return xdg.CacheHome
		case "HOME":
			home, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			return home
		case "USER":
			u, err := user.Current()
			if err != nil {
				return os.Getenv("USERNAME") // Fallback for Windows
			}
			return u.Username
		case "ScriptFolder":
			return paths.GetBashScriptFolder()
		}
		// Fall back to the real environment for any unrecognised variable
		return os.Getenv(varName)
	}
	return os.Expand(val, mapper)
}

// CollapseVariables replaces absolute paths with their environment variable equivalents.
// It specifically EXCLUDES ${ScriptFolder} from reverse resolution.
func CollapseVariables(path string) string {
	if path == "" {
		return ""
	}

	// Order matters: more specific paths first
	path = filepath.Clean(path)
	home, _ := os.UserHomeDir()
	home = filepath.Clean(home)

	// Use a slice of pairs to ensure deterministic order
	type pair struct {
		abs    string
		envVar string
	}
	vars := []pair{
		{filepath.Clean(xdg.ConfigHome), "${XDG_CONFIG_HOME}"},
		{filepath.Clean(xdg.DataHome), "${XDG_DATA_HOME}"},
		{filepath.Clean(xdg.StateHome), "${XDG_STATE_HOME}"},
		{filepath.Clean(xdg.CacheHome), "${XDG_CACHE_HOME}"},
		{home, "${HOME}"},
	}

	for _, v := range vars {
		if v.abs != "" {
			// Check if the path is exactly the variable's value or if it's a sub-path
			if path == v.abs {
				return v.envVar
			}
			if strings.HasPrefix(path, v.abs+string(os.PathSeparator)) {
				return v.envVar + strings.TrimPrefix(path, v.abs)
			}
		}
	}

	return path
}

// LoadAppConfig reads the configuration file and returns the configuration.
// defaults.toml (embedded) is always unmarshalled first; the user's file is
// overlaid on top, so keys present in the user's file override the defaults
// and keys absent from the user's file fall back to the embedded defaults.
// sanitizeConfig resets any UIConfig field that has an invalid value, using the
// embedded defaults as the source of truth for fallback values.
// Warns for each field that had to be corrected.
func sanitizeConfig(ctx context.Context, conf *AppConfig) {
	var def AppConfig
	_ = toml.Unmarshal(defaultConfigBytes(), &def)
	ui := &conf.Appearance

	warn := func(field, bad, fixed string) {
		logger.Warn(ctx, "Config: invalid value for {{|Var|}}%s{{[-]}} ({{|Var|}}%s{{[-]}}) — reset to {{|Var|}}%s{{[-]}}.", field, bad, fixed)
	}

	for _, ct := range ConnTypes {
		sanitizeAppearance(conf.Appearance.Ptr(ct), def.Appearance.ForConnType(ct), "appearance."+ct+".", warn)
	}
	if ui.SpinnerSpeed < 50 || ui.SpinnerSpeed > 5000 {
		warn("spinner_speed", fmt.Sprintf("%d", ui.SpinnerSpeed), fmt.Sprintf("%d", def.Appearance.SpinnerSpeed))
		ui.SpinnerSpeed = def.Appearance.SpinnerSpeed
	}
	if ui.RefreshRate < MinRefreshRateMS || ui.RefreshRate > MaxRefreshRateMS {
		warn("refresh_rate", fmt.Sprintf("%d", ui.RefreshRate), fmt.Sprintf("%d", def.Appearance.RefreshRate))
		ui.RefreshRate = def.Appearance.RefreshRate
	}
	switch ui.PanelLocal {
	case "log", "console", "none", "system":
	default:
		warn("panel_local", ui.PanelLocal, def.Appearance.PanelLocal)
		ui.PanelLocal = def.Appearance.PanelLocal
	}
	switch ui.PanelRemote {
	case "log", "console", "none", "system":
	default:
		warn("panel_remote", ui.PanelRemote, def.Appearance.PanelRemote)
		ui.PanelRemote = def.Appearance.PanelRemote
	}
	switch ui.MarkdownHyperlinks {
	case "off", "inline", "auto":
	default:
		warn("markdown_hyperlinks", ui.MarkdownHyperlinks, def.Appearance.MarkdownHyperlinks)
		ui.MarkdownHyperlinks = def.Appearance.MarkdownHyperlinks
	}
	switch ui.Hyperlinks {
	case "off", "inline", "auto":
	default:
		warn("hyperlinks", ui.Hyperlinks, def.Appearance.Hyperlinks)
		ui.Hyperlinks = def.Appearance.Hyperlinks
	}

	isValidPath := func(p string) bool {
		expanded := filepath.Clean(ExpandVariables(p))
		return filepath.IsAbs(expanded)
	}
	if !isValidPath(conf.Paths.ConfigFolder) {
		warn("config_folder", conf.Paths.ConfigFolder, def.Paths.ConfigFolder)
		conf.Paths.ConfigFolder = def.Paths.ConfigFolder
	}
	if !isValidPath(conf.Paths.ComposeFolder) {
		logger.Warn(ctx, "Config: invalid value for {{|Var|}}compose_folder{{[-]}} ({{|Var|}}%s{{[-]}}) — attempting detection.", conf.Paths.ComposeFolder)
		ResolveComposeFolder(ctx, conf, logNotice)
	}
}

// sanitizeAppearance resets any field of a that has an invalid value to
// def's, reporting each through warn under keyPrefix.
func sanitizeAppearance(a *Appearance, def Appearance, keyPrefix string, warn func(field, bad, fixed string)) {
	if a.ShadowLevel < 0 || a.ShadowLevel > 4 {
		warn(keyPrefix+"shadow_level", fmt.Sprintf("%d", a.ShadowLevel), fmt.Sprintf("%d", def.ShadowLevel))
		a.ShadowLevel = def.ShadowLevel
		a.Shadow = def.Shadow
	}
	if a.BorderColor < 1 || a.BorderColor > 3 {
		warn(keyPrefix+"border_color", fmt.Sprintf("%d", a.BorderColor), fmt.Sprintf("%d", def.BorderColor))
		a.BorderColor = def.BorderColor
	}
	oneOf := func(field string, v *string, fallback string, valid ...string) {
		for _, ok := range valid {
			if *v == ok {
				return
			}
		}
		warn(keyPrefix+field, *v, fallback)
		*v = fallback
	}
	oneOf("dialog_title_align", &a.DialogTitleAlign, def.DialogTitleAlign, "left", "center")
	oneOf("submenu_title_align", &a.SubmenuTitleAlign, def.SubmenuTitleAlign, "left", "center")
	oneOf("panel_title_align", &a.PanelTitleAlign, def.PanelTitleAlign, "left", "center")
	oneOf("tab_layout", &a.TabLayout, def.TabLayout, "maximized", "sidebyside", "stacked")
	oneOf("checkbox_brackets", &a.CheckboxBrackets, def.CheckboxBrackets, "never", "selected", "always")
	oneOf("radio_brackets", &a.RadioBrackets, def.RadioBrackets, "never", "selected", "always")
}

// ResolveComposeFolder runs compose folder detection and, when multiple candidates
// are found, prompts the user to choose. Updates conf.Paths.ComposeFolder in place.
// Uses the same logic as first-run migration so the experience is consistent.
func ResolveComposeFolder(ctx context.Context, conf *AppConfig, printer console.Printer) {
	detection := paths.DetectComposeFolder(conf.Paths.ComposeFolder)

	// The abort choice below hard-exits the process, which is only safe during
	// the one-time migration bootstrap (see isMigrationMode) -- never during
	// validate()'s per-load repair of an already-live config on a running
	// --server-daemon, which would take down every connected session.
	if detection.LegacyExists && isMigrationMode(ctx) {
		if scriptFolder := paths.GetBashScriptFolder(); paths.DetectLegacyTemplatesInScriptFolder(scriptFolder) {
			if warnLegacyTemplatesInScriptFolder(ctx, printer, scriptFolder, detection.CurrentPath) {
				detection.LegacyExists = false
			}
		}
	}

	if detection.LegacyExists && detection.CurrentExists && detection.LegacyPath != detection.CurrentPath {
		promptMsg := "Detected compose folders in multiple locations.\n   Legacy:  '" + console.FormatFolderPath(detection.LegacyPath) + "'\n   Default: '" + console.FormatFolderPath(detection.CurrentPath) + "'\n\nWould you like to use the Legacy location?"
		useLegacy, err := console.QuestionPrompt(ctx, printer, "Multiple Compose Folders Detected", promptMsg, "Y", false)
		if err == nil && useLegacy {
			printer(ctx, "Chose the Legacy compose folder location:\n   '"+console.FormatFolderPath(detection.LegacyPath)+"'")
			conf.Paths.ComposeFolder = detection.LegacyPath
		} else if err == nil {
			printer(ctx, "Chose the Default compose folder location:\n   '"+console.FormatFolderPath(detection.CurrentPath)+"'")
			conf.Paths.ComposeFolder = detection.CurrentPath
		}
	} else if detection.LegacyExists && !detection.CurrentExists {
		printer(ctx, "Detected compose folder at '"+console.FormatFolderPath(detection.LegacyPath)+"'.")
		conf.Paths.ComposeFolder = detection.LegacyPath
	} else if detection.CurrentExists {
		conf.Paths.ComposeFolder = detection.CurrentPath
	}
}

// warnLegacyTemplatesInScriptFolder is called when the detected legacy compose
// folder belongs to a DS1 install so old that its app templates are still
// stored inside the script folder itself, instead of their own separate
// templates location -- migrating from an install this old isn't supported,
// since DS1 needs to run its own migration to the templates repo first.
// Returns true if the user chose to skip the legacy compose folder and
// continue with the new default location instead; exits the process if they
// chose to abort so they can update DS1 first.
func warnLegacyTemplatesInScriptFolder(ctx context.Context, printer console.Printer, scriptFolder, defaultPath string) bool {
	logger.Warn(ctx, "\nDetected a very old {{|ApplicationName|}}DockSTARTer{{[-]}} install where app templates are still\n"+
		"stored in the script folder ('"+console.FormatFolderPath(scriptFolder)+"'), instead of\n"+
		"their own templates location. This {{|ApplicationName|}}DockSTARTer{{[-]}} install needs to be\n"+
		"updated before migrating to {{|ApplicationName|}}DockSTARTer2{{[-]}}.\n\n"+
		"We recommend aborting and running '{{|UserCommand|}}ds -u{{[-]}}' to update the old\n"+
		"{{|ApplicationName|}}DockSTARTer{{[-]}} install, then running {{|ApplicationName|}}DockSTARTer2{{[-]}} again to retry migration.\n")

	promptMsg := "Abort migration to run '{{|UserCommand|}}ds -u{{[-]}}'? Answering No skips the legacy compose\n" +
		"folder and uses the new default location instead, without migrating\n" +
		"this install's existing app configuration."
	abort, err := console.QuestionPrompt(ctx, printer, "Old DockSTARTer Install Detected", promptMsg, "Y", false)
	if err != nil || abort {
		logger.Error(ctx, "Aborting migration. Run '{{|UserCommand|}}ds -u{{[-]}}' to update your existing {{|ApplicationName|}}DockSTARTer{{[-]}} install, then run {{|ApplicationName|}}DockSTARTer2{{[-]}} again.")
		// This exits deep inside LoadAppConfig, well before main.go's own
		// exitCode/trailer logic ever runs -- print the same trailer here
		// (matching main.go's wording exactly) so this looks like any other
		// failed run instead of silently vanishing without it.
		logger.Display(ctx, "{{|ApplicationName|}}%s{{[-]}} did not finish running successfully.", version.ApplicationName)
		logger.Display(ctx, "Check logs in '"+console.FormatFilePath(logger.GetLogFilePath())+"'.")
		// Same exit hygiene as logger.Fatal*: give stdout/stderr a moment to
		// flush and make sure the cursor isn't left hidden, without using
		// Fatal itself, whose "let the dev know" footer is wrong for an
		// expected, user-actionable stop (not a bug).
		time.Sleep(100 * time.Millisecond)
		console.RestoreCursor()
		os.Exit(1)
	}
	logger.Warn(ctx, "Skipping legacy compose folder; using the new default location:\n   '"+console.FormatFolderPath(defaultPath)+"'")
	return true
}

// migrateAnsiPaletteMenuNesting moves a pre-nesting config file's flat
// ansi_palette.<connType> tint/color fields (tint_enabled, override_enabled,
// tint, base00, ...) into that connType's new "menu" table (see AnsiColors'
// own doc comment) -- the normal typed decode silently drops them, since
// they have no matching field at that path anymore, leaving
// conf.AnsiColors.<ConnType>.AnsiElementColors at the embedded baseline
// instead of the file's own values. Since elements don't inherit from each
// other (see AnsiColors' own doc comment), the old single flat palette
// becomes each of menu/programbox/cli's own starting value -- matching the
// old behavior, where apply_to_cli/apply_to_programbox defaulting true
// meant every context rendered with that same palette. When previously set
// false, the affected element(s) are left at a blank/disabled
// AnsiElementColors instead, matching what "off" meant before.
//
// A no-op for a file that already has a "menu" table: reading data a
// second time into a struct shaped like the pre-nesting file (flat
// AnsiElementColors fields directly under ansi_palette.<connType>) leaves
// every field zero-valued for such a file, since its actual values sit one
// level deeper, under .menu -- correctly caught by the zero-value check
// below, so this only ever migrates something for a file saved before
// menu/programbox/cli elements existed.
func migrateAnsiPaletteMenuNesting(data []byte, conf *AppConfig) {
	var legacy struct {
		AnsiPalette struct {
			Local             AnsiElementColors `toml:"local"`
			SSH               AnsiElementColors `toml:"ssh"`
			Web               AnsiElementColors `toml:"web"`
			ApplyToCLI        *bool             `toml:"apply_to_cli"`
			ApplyToProgramBox *bool             `toml:"apply_to_programbox"`
		} `toml:"ansi_palette"`
	}
	if err := toml.Unmarshal(data, &legacy); err != nil {
		return
	}

	var zero AnsiElementColors
	migrate := func(dst *AnsiColors, src AnsiElementColors, cliOff, programBoxOff bool) {
		if src == zero {
			return
		}
		dst.AnsiElementColors = src
		if programBoxOff {
			dst.ProgramBox = AnsiElementColors{}
		} else {
			dst.ProgramBox = src
		}
		if cliOff {
			dst.CLI = AnsiElementColors{}
		} else {
			dst.CLI = src
		}
	}

	// ApplyToCLI only ever meant "local" (a bare CLI invocation is always a
	// local shell process); ApplyToProgramBox applied to all three connTypes.
	cliOff := legacy.AnsiPalette.ApplyToCLI != nil && !*legacy.AnsiPalette.ApplyToCLI
	programBoxOff := legacy.AnsiPalette.ApplyToProgramBox != nil && !*legacy.AnsiPalette.ApplyToProgramBox

	migrate(ansiColorsPtrConfig(conf, "local"), legacy.AnsiPalette.Local, cliOff, programBoxOff)
	migrate(ansiColorsPtrConfig(conf, "ssh"), legacy.AnsiPalette.SSH, false, programBoxOff)
	migrate(ansiColorsPtrConfig(conf, "web"), legacy.AnsiPalette.Web, false, programBoxOff)
}

// migrateAnsiPaletteInheritGap closes a gap from a brief released window
// (v2.20260924.1, commit 3d5f554d) where AnsiColors.ProgramBox/CLI were
// still *AnsiElementColors (nil meant "inherit menu's palette"), before
// that pointer/inherit design was replaced with the current plain, always-
// independent fields (see AnsiColors' own doc comment). A config saved by
// that release has "menu" present but "programbox"/"cli" genuinely absent
// from the file for any connType whose menu was never explicitly given a
// programbox/cli override -- under the released code that absence meant
// "same as menu"; under the current code it decodes to an empty, untinted
// zero value instead, since there's no more inherit step at read time.
// Freezing menu's own current values into the absent slots reproduces
// exactly what inherit used to render, so upgrading past that release
// doesn't silently un-tint a programbox/cli that was previously inheriting
// a real tint from its own menu.
//
// Detection is self-describing, no schema-version tracking needed: every
// config write from this point on always includes all three elements
// (plain fields, never omitted), so "menu present, programbox/cli keys
// genuinely missing from the file" can only mean a config last saved by
// that one released binary (or a hand-edited file choosing to omit them,
// which gets the same reasonable treatment). A no-op for any other config
// shape -- older pre-nesting files are handled by
// migrateAnsiPaletteMenuNesting above, and a config already carrying real
// programbox/cli tables (from this or a later release) is left untouched.
func migrateAnsiPaletteInheritGap(data []byte, conf *AppConfig) {
	var raw struct {
		AnsiPalette map[string]map[string]any `toml:"ansi_palette"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return
	}
	for _, ct := range ConnTypes {
		section, ok := raw.AnsiPalette[ct]
		if !ok {
			continue
		}
		if _, hasMenu := section["menu"]; !hasMenu {
			continue
		}
		c := ansiColorsPtrConfig(conf, ct)
		if _, hasProgramBox := section["programbox"]; !hasProgramBox {
			c.ProgramBox = c.AnsiElementColors
		}
		if _, hasCLI := section["cli"]; !hasCLI {
			c.CLI = c.AnsiElementColors
		}
	}
}

// ansiColorsPtrConfig returns a pointer to conf's AnsiColors for connType,
// or nil for an unrecognized connType -- config-package-local counterpart
// to internal/commands' own ansiColorsPtr (kept separate since neither
// package imports the other's unexported helpers).
func ansiColorsPtrConfig(conf *AppConfig, connType string) *AnsiColors {
	return &conf.Appearance.Ptr(connType).AnsiColors
}

func LoadAppConfig() AppConfig {
	var conf AppConfig
	// Start from embedded defaults so every key has a known baseline value.
	_ = toml.Unmarshal(defaultConfigBytes(), &conf)

	// Set architecture (runtime only)
	conf.Arch = getArch()

	path := paths.GetConfigFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		// No config file found. Attempt migration from legacy.
		migrationCtx := context.WithValue(context.Background(), migrationModeKey{}, true)
		logNotice(migrationCtx, "No {{|ApplicationName|}}%s{{[-]}} config file detected. Performing initial configuration.", "DockSTARTer2")
		migrated, foundLegacy, foundCompose := MigrateFromLegacy(migrationCtx)

		cfgPath := paths.GetConfigFilePath()
		dir := filepath.Dir(cfgPath)
		if info, err := os.Stat(dir); err == nil && !info.IsDir() {
			logger.Info(context.Background(), "Removing existing file '"+console.FormatFilePath(dir)+"' before folder can be created.")
			if err := os.Remove(dir); err != nil {
				logger.FatalWithStack(context.Background(), []string{
					"Failed to remove existing file.",
					"Failing command: {{|FailingCommand|}}rm -f \"%s\"{{[-]}}",
				}, dir)
			}
		}
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			logNotice(context.Background(), "Creating '"+console.FormatFolderPath(dir)+"'.")
			if err := os.MkdirAll(dir, 0700); err != nil {
				logger.FatalWithStack(context.Background(), []string{
					"Failed to create config folder.",
					"Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}",
				}, dir)
			}
		}

		var baseline AppConfig
		_ = toml.Unmarshal(defaultConfigBytes(), &baseline)
		if reflect.DeepEqual(migrated, baseline) {
			// Unchanged from pristine defaults -- keep the file's comments.
			_ = os.WriteFile(cfgPath, defaultConfigBytes(), 0600)
		} else if merged, err := toml.Marshal(migrated); err == nil {
			_ = os.WriteFile(cfgPath, merged, 0600)
			logNotice(context.Background(), "Writing migrated configuration to '"+console.FormatFilePath(cfgPath)+"'.")
		} else {
			_ = os.WriteFile(cfgPath, defaultConfigBytes(), 0600)
			logNotice(context.Background(), "Copying '"+console.FormatFileName("embedded defaults", "")+"' to '"+console.FormatFilePath(cfgPath)+"'.")
		}

		conf = LoadAppConfig()
		if foundLegacy || foundCompose {
			// Show the config after migration, matching bash version behavior
			logNotice(migrationCtx, " ")
			ShowAppConfig(migrationCtx, &conf)
			logNotice(migrationCtx, " ")
		}
		return conf
	} else {
		// Overlay user config on top of defaults.
		// Use Robust unmarshaling to handle any loose types from manual edits
		if present, err := UnmarshalRobust(data, &conf); err == nil {
			// Backfill a field new to the schema for a file saved before it
			// existed -- see ServerTLSDefaultHook's doc comment. Only for an
			// existing file being loaded here, never on first-run/legacy
			// migration above, which has nothing of this user's to preserve.
			if ServerTLSDefaultHook != nil {
				ServerTLSDefaultHook(&conf, present)
			}
			migrateToAppearance(data, &conf)
			migrateAnsiPaletteMenuNesting(data, &conf)
			migrateAnsiPaletteInheritGap(data, &conf)
			// Write back only if the merged config differs from what was on disk
			// (e.g. new keys added in a newer version). Avoids a pointless write
			// on every load which would also trigger any file watchers.
			if merged, err := toml.Marshal(conf); err == nil {
				if string(merged) != string(data) {
					_ = SaveAppConfig(conf)
				}
			}
			sanitizeConfig(context.Background(), &conf)
			conf.RawPaths = conf.Paths
			conf.Paths.ConfigFolder = filepath.Clean(ExpandVariables(conf.Paths.ConfigFolder))
			conf.Paths.ComposeFolder = filepath.Clean(ExpandVariables(conf.Paths.ComposeFolder))
			conf.ConfigDir = conf.Paths.ConfigFolder
			conf.ComposeDir = conf.Paths.ComposeFolder
			return conf
		}
	}

	// Existing config file could not be parsed even robustly (corrupt file).
	// Fall back to the embedded defaults, preserving comments, without going
	// through the migration/theme-overlay flow (there's nothing to migrate).
	_ = os.WriteFile(path, defaultConfigBytes(), 0600)
	logNotice(context.Background(), "Config file at '"+console.FormatFilePath(path)+"' could not be parsed; reset to embedded defaults.")
	sanitizeConfig(context.Background(), &conf)
	conf.RawPaths = conf.Paths
	conf.Paths.ConfigFolder = filepath.Clean(ExpandVariables(conf.Paths.ConfigFolder))
	conf.Paths.ComposeFolder = filepath.Clean(ExpandVariables(conf.Paths.ComposeFolder))
	conf.ConfigDir = conf.Paths.ConfigFolder
	conf.ComposeDir = conf.Paths.ComposeFolder
	return conf
}

// TryLoadAppConfig loads and validates the config file strictly, returning an
// error if the file cannot be read or parsed. Unlike LoadAppConfig it does not
// fall back to defaults or write anything to disk, making it safe to call from
// a file watcher to gate whether a change should be applied.
func TryLoadAppConfig() (AppConfig, error) {
	var conf AppConfig
	_ = toml.Unmarshal(defaultConfigBytes(), &conf)
	conf.Arch = getArch()

	data, err := os.ReadFile(paths.GetConfigFilePath())
	if err != nil {
		return AppConfig{}, err
	}
	if err := toml.Unmarshal(data, &conf); err != nil {
		return AppConfig{}, err
	}
	migrateToAppearance(data, &conf)
	migrateAnsiPaletteMenuNesting(data, &conf)
	migrateAnsiPaletteInheritGap(data, &conf)
	conf.RawPaths = conf.Paths
	conf.Paths.ConfigFolder = filepath.Clean(ExpandVariables(conf.Paths.ConfigFolder))
	conf.Paths.ComposeFolder = filepath.Clean(ExpandVariables(conf.Paths.ComposeFolder))
	conf.ConfigDir = conf.Paths.ConfigFolder
	conf.ComposeDir = conf.Paths.ComposeFolder
	return conf, nil
}

// SaveAppConfig writes the configuration to dockstarter2.toml.
// SaveAppConfig writes the configuration to the application configuration file.
// Paths are stored in their unexpanded form (e.g. ${XDG_CONFIG_HOME}) so that
// the file remains portable and variables are resolved fresh on each read.
func SaveAppConfig(conf AppConfig) error {
	path := paths.GetConfigFilePath()

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		logger.Info(context.Background(), "Removing existing file '"+console.FormatFilePath(dir)+"' before folder can be created.")
		if err := os.Remove(dir); err != nil {
			logger.FatalWithStack(context.Background(), []string{
				"Failed to remove existing file.",
				"Failing command: {{|FailingCommand|}}rm -f \"%s\"{{[-]}}",
			}, dir)
		}
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logNotice(context.Background(), "Creating '"+console.FormatFolderPath(dir)+"'.")
		if err := os.MkdirAll(dir, 0700); err != nil {
			logger.FatalWithStack(context.Background(), []string{
				"Failed to create config folder.",
				"Failing command: {{|FailingCommand|}}mkdir -p \"%s\"{{[-]}}",
			}, dir)
			return err
		}
	}

	// 1. If RawPaths was set (e.g. from a recent Load), prioritize those original strings
	if conf.RawPaths.ConfigFolder != "" {
		conf.Paths.ConfigFolder = conf.RawPaths.ConfigFolder
	} else {
		// 2. Otherwise, auto-collapse absolute paths to variables (XDG, HOME, etc.)
		conf.Paths.ConfigFolder = CollapseVariables(conf.Paths.ConfigFolder)
	}

	if conf.RawPaths.ComposeFolder != "" {
		conf.Paths.ComposeFolder = conf.RawPaths.ComposeFolder
	} else {
		conf.Paths.ComposeFolder = CollapseVariables(conf.Paths.ComposeFolder)
	}

	data, err := toml.Marshal(conf)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// updateAppConfigMu serializes UpdateAppConfig's load-mutate-save sequence
// against itself, so two concurrent callers (e.g. two TUI sessions under
// the same --server-daemon process, each updating a different field) can't
// both load the same on-disk config, apply their own mutation, and then
// have the second SaveAppConfig silently overwrite the first caller's
// change instead of building on top of it. This covers every UpdateAppConfig
// caller within this one process; it does not extend across separate OS
// processes (e.g. a bare CLI invocation racing the daemon), which has
// never been guarded against here -- SaveAppConfig itself has no
// cross-process lock.
var updateAppConfigMu sync.Mutex

// UpdateAppConfig loads the current on-disk config, applies mutate to it,
// and saves the result -- a single load-mutate-save step, rather than a
// caller holding its own separately-loaded copy across some intervening
// span of time and saving that back later. A caller doing the latter can
// silently clobber an unrelated field some other session changed on disk
// in between: mutate should touch only the field(s) that caller actually
// owns, leaving everything else exactly as UpdateAppConfig just loaded it,
// so a concurrent, unrelated change from elsewhere survives. Returns the
// saved config so the caller can adopt it as its own new baseline.
func UpdateAppConfig(mutate func(*AppConfig)) (AppConfig, error) {
	updateAppConfigMu.Lock()
	defer updateAppConfigMu.Unlock()

	conf := LoadAppConfig()
	mutate(&conf)
	if err := SaveAppConfig(conf); err != nil {
		return conf, err
	}
	return conf, nil
}

// UnmarshalRobust unmarshals TOML data into a struct using mapstructure
// to allow for "weak" type conversion (e.g., string "true" to boolean true).
func UnmarshalRobust(data []byte, v any) (map[string]bool, error) {
	present := make(map[string]bool)
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Helper to track present keys in the flat namespace of AppConfig
	var trackKeys func(m map[string]any, prefix string)
	trackKeys = func(m map[string]any, prefix string) {
		for k, val := range m {
			fullKey := k
			if prefix != "" {
				fullKey = prefix + "." + k
			}

			// Special case: map the TOML structure to the flat keys used in ShowAppConfig
			switch fullKey {
			case "paths.config_folder":
				present["ConfigFolder"] = true
			case "paths.compose_folder":
				present["ComposeFolder"] = true
			case "ui.theme":
				present["Theme"] = true
			case "ui.borders":
				present["Borders"] = true
			case "ui.large_buttons":
				present["LargeButtons"] = true
			case "ui.large_title_bars":
				present["LargeTitleBars"] = true
			case "ui.line_characters":
				present["LineCharacters"] = true
			case "ui.scrollbar":
				present["Scrollbar"] = true
			case "ui.spinner":
				present["Spinner"] = true
			case "ui.spinner_speed":
				present["SpinnerSpeed"] = true
			case "ui.shadow":
				present["Shadow"] = true
			case "ui.shadow_level":
				present["ShadowLevel"] = true
			case "ui.border_color":
				present["BorderColor"] = true
			case "ui.dialog_title_align":
				present["DialogTitleAlign"] = true
			case "ui.submenu_title_align":
				present["SubmenuTitleAlign"] = true
			case "ui.panel_title_align":
				present["PanelTitleAlign"] = true
			case "ui.panel_local":
				present["PanelLocal"] = true
			case "ui.panel_remote":
				present["PanelRemote"] = true
			case "ui.checkbox_brackets":
				present["CheckboxBrackets"] = true
			case "ui.radio_brackets":
				present["RadioBrackets"] = true
			case "ui.menu_brackets":
				present["MenuBrackets"] = true
			case "ui.line_number_brackets":
				present["LineNumberBrackets"] = true
			case "server.ssh.port":
				present["SSHPort"] = true
			case "server.web.port":
				present["WebPort"] = true
			case "server.web.tls":
				present["TLS"] = true
			case "server.auth.mode":
				present["AuthMode"] = true
			}

			if subMap, ok := val.(map[string]any); ok {
				trackKeys(subMap, fullKey)
			}
		}
	}
	trackKeys(raw, "")

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           v,
		TagName:          "toml",
	})
	if err != nil {
		return nil, err
	}

	return present, decoder.Decode(raw)
}

// ReadLegacyMap parses a legacy .ini configuration file and returns a map of raw key-value pairs.
func ReadLegacyMap(data []byte) map[string]string {
	raw := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
			raw[key] = val
		}
	}
	return raw
}

// UnmarshalLegacyIni parses a legacy .ini configuration file and maps it to AppConfig.
func UnmarshalLegacyIni(data []byte, v *AppConfig) (map[string]bool, error) {
	present := make(map[string]bool)
	raw := ReadLegacyMap(data)

	// Permissive boolean parsing (matching legacy is_true)
	isTrue := func(s string) bool {
		s = strings.Trim(strings.TrimSpace(s), "'\"")
		s = strings.ToUpper(s)
		return s == "TRUE" || s == "1" || s == "ON" || s == "YES"
	}

	if val, ok := raw["ConfigFolder"]; ok {
		v.Paths.ConfigFolder = migrateLegacyPathValue(val)
		present["ConfigFolder"] = true
	}
	if val, ok := raw["ComposeFolder"]; ok {
		v.Paths.ComposeFolder = migrateLegacyPathValue(val)
		present["ComposeFolder"] = true
	}
	for _, ct := range ConnTypes {
		a := v.Appearance.Ptr(ct)
		if val, ok := raw["Theme"]; ok {
			a.Theme = val
			present["Theme"] = true
		}

		if val, ok := raw["Scrollbar"]; ok {
			a.Scrollbar = isTrue(val)
			present["Scrollbar"] = true
		} else if val, ok := raw["Scrollbars"]; ok {
			a.Scrollbar = isTrue(val)
			present["Scrollbar"] = true
		}

		if val, ok := raw["Spinner"]; ok {
			a.Spinner = isTrue(val)
			present["Spinner"] = true
		}

		if val, ok := raw["Shadow"]; ok {
			a.Shadow = isTrue(val)
			present["Shadow"] = true
		} else if val, ok := raw["Shadows"]; ok {
			a.Shadow = isTrue(val)
			present["Shadow"] = true
		}

		if val, ok := raw["Borders"]; ok {
			a.Borders = isTrue(val)
			present["Borders"] = true
		} else if val, ok := raw["LineCharacters"]; ok {
			a.Borders = isTrue(val)
			present["Borders"] = true
		}

		if val, ok := raw["LineCharacters"]; ok {
			a.LineCharacters = isTrue(val)
			present["LineCharacters"] = true
		}
	}

	return present, nil
}

// migrateLegacyPathValue preserves a path value from a legacy DS1 config
// file exactly as written, except for DS1's own ${ScriptFolder} variable,
// which DS2 doesn't support going forward -- that gets resolved and
// rewritten in ${HOME}-relative form instead (script-folder installs are
// always under $HOME).
func migrateLegacyPathValue(val string) string {
	normalized := strings.ReplaceAll(val, "${ScriptFolder?}", "${ScriptFolder}")
	if !strings.Contains(normalized, "${ScriptFolder}") {
		return val
	}
	return CollapseVariables(paths.ResolvePath(normalized))
}

// MigrateFromLegacy always starts from the embedded defaults, overlays a
// legacy DS1 config if found, resolves the compose folder, and overlays the
// theme's suggested defaults for fields not already set. foundLegacy and
// foundCompose are for the caller's migration-summary display only.
func MigrateFromLegacy(ctx context.Context) (conf AppConfig, foundLegacy bool, foundCompose bool) {
	_ = toml.Unmarshal(defaultConfigBytes(), &conf)
	legacyPresent := map[string]bool{}

	var legacyFiles []string
	// DS1's current .toml format takes priority over its older .ini format.
	legacyFiles = append(legacyFiles, filepath.Join(xdg.ConfigHome, "dockstarter", "dockstarter.toml"))
	legacyFiles = append(legacyFiles, paths.GetLegacyIniPaths()...)

	for _, path := range legacyFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Try next file
		}

		if strings.HasSuffix(path, ".toml") {
			var probe AppConfig
			present, unmarshalErr := UnmarshalRobust(data, &probe)
			if unmarshalErr != nil || len(present) == 0 {
				continue // Try next file
			}
			// ShowAppConfigWithTitleAndPresent reads RawPaths/ConfigDir/ComposeDir
			// (normally populated by LoadAppConfig's finalize step), not Paths
			// directly -- probe never goes through that step, so populate them
			// here or the Config/Compose Folder rows render blank.
			migrateToAppearance(data, &probe)
			probe.RawPaths = probe.Paths
			probe.ConfigDir = filepath.Clean(ExpandVariables(probe.Paths.ConfigFolder))
			probe.ComposeDir = filepath.Clean(ExpandVariables(probe.Paths.ComposeFolder))
			logNotice(ctx, "Detected legacy {{|ApplicationName|}}DockSTARTer{{[-]}} config file at '"+console.FormatFilePath(path)+"'.")
			heading := "Configuration options in legacy {{|ApplicationName|}}DockSTARTer{{[-]}} config file '" + console.FormatFilePath(path) + "':"
			logNotice(ctx, " ")
			ShowAppConfigWithTitleAndPresent(ctx, &probe, heading, present)
			logNotice(ctx, " ")
			logNotice(ctx, "Migrating '"+console.FormatFilePath(path)+"' to '"+console.FormatFilePath(paths.GetConfigFilePath())+"'.")

			// Apply to the actual merged config (defaults already unmarshalled above)
			legacyPresent, _ = UnmarshalRobust(data, &conf)
			migrateToAppearance(data, &conf)
			conf.Paths.ConfigFolder = migrateLegacyPathValue(conf.Paths.ConfigFolder)
			conf.Paths.ComposeFolder = migrateLegacyPathValue(conf.Paths.ComposeFolder)
			foundLegacy = true
			break // STOP after the first successful migration source is processed
		}

		raw := ReadLegacyMap(data)
		if len(raw) > 0 {
			logNotice(ctx, "Detected legacy {{|ApplicationName|}}DockSTARTer{{[-]}} config file at '"+console.FormatFilePath(path)+"'.")
			heading := "Configuration options in legacy {{|ApplicationName|}}DockSTARTer{{[-]}} config file '" + console.FormatFilePath(path) + "':"
			logNotice(ctx, " ")

			headers := []string{
				"{{|UsageCommand|}}Option{{[-]}}",
				"{{|UsageCommand|}}Value{{[-]}}",
				"{{|UsageCommand|}}Expanded Value{{[-]}}",
			}
			var tableData []string
			// Map legacy keys to display names and format values (matching DS2 table style)
			legacyMapping := []struct {
				key   string
				name  string
				isDir bool
			}{
				{"ConfigFolder", "Config Folder", true},
				{"ComposeFolder", "Compose Folder", true},
				{"Theme", "Theme", false},
				{"Borders", "Borders", false},
				{"Scrollbar", "Scrollbar", false},
				{"Shadow", "Shadow", false},
				{"LineCharacters", "Line Characters", false},
			}

			for _, m := range legacyMapping {
				val, ok := raw[m.key]
				if !ok {
					// Check aliases
					switch m.key {
					case "ConfigFolder":
						val, ok = raw["DOCKER_CONFIG_FOLDER"]
					case "Scrollbar":
						val, ok = raw["Scrollbars"]
					case "Shadow":
						val, ok = raw["Shadows"]
					}
				}
				if ok {
					tableData = append(tableData, m.name)
					if m.isDir {
						tableData = append(tableData, console.FormatFolderPath(val))
						tableData = append(tableData, console.FormatFolderPath(ExpandVariables(val)))
					} else {
						tableData = append(tableData, fmt.Sprintf("{{|Var|}}%s{{[-]}}", val))
						tableData = append(tableData, "")
					}
				}
			}

			logNotice(ctx, heading)
			var sb strings.Builder
			console.PrintTableCtx(console.WithTUIWriter(ctx, &sb), headers, tableData, false) // Force ASCII borders
			logNotice(ctx, strings.TrimSuffix(sb.String(), "\n"))

			logNotice(ctx, " ")
			logNotice(ctx, "Migrating '"+console.FormatFilePath(path)+"' to '"+console.FormatFilePath(paths.GetConfigFilePath())+"'.")

			// Apply to the actual merged config (defaults already unmarshalled above)
			legacyPresent, _ = UnmarshalLegacyIni(data, &conf)
			foundLegacy = true
			break // STOP after the first successful migration source is processed
		}
	}

	// Skip detection/prompting entirely when the legacy file already defined
	// compose_folder explicitly -- that value is authoritative.
	before := conf.Paths.ComposeFolder
	if !legacyPresent["ComposeFolder"] {
		ResolveComposeFolder(ctx, &conf, logNotice)
	}
	foundCompose = conf.Paths.ComposeFolder != before

	if ThemeDefaultsOverlayHook != nil {
		ThemeDefaultsOverlayHook(&conf, legacyPresent)
	}

	return conf, foundLegacy, foundCompose
}

// ShowAppConfig prints a summary table of the current configuration.
func ShowAppConfig(ctx context.Context, conf *AppConfig) {
	ShowAppConfigWithTitle(ctx, conf, "")
}

// ShowAppConfigWithTitle prints a summary table with a custom title.
func ShowAppConfigWithTitle(ctx context.Context, conf *AppConfig, title string) {
	ShowAppConfigWithTitleAndPresent(ctx, conf, title, nil)
}

// ShowAppConfigWithTitleAndPresent prints a summary table with a custom title, optionally filtering by present keys.
// Global options come first, followed by a second table comparing each
// connection type's appearance options side by side.
func ShowAppConfigWithTitleAndPresent(ctx context.Context, conf *AppConfig, title string, presentKeys map[string]bool) {
	headers := []string{
		"{{|UsageCommand|}}Option{{[-]}}",
		"{{|UsageCommand|}}Value{{[-]}}",
		"{{|UsageCommand|}}Expanded Value{{[-]}}",
	}

	keys := []string{
		"ConfigFolder", "ComposeFolder",
		"SpinnerSpeed", "RefreshRate", "PanelLocal", "PanelRemote", "ShowPreview", "MarkdownHyperlinks", "Hyperlinks",
		"SSHPort", "WebPort", "AuthMode",
	}
	displayNames := map[string]string{
		"ConfigFolder":       "Config Folder",
		"ComposeFolder":      "Compose Folder",
		"SpinnerSpeed":       "Spinner Speed",
		"RefreshRate":        "Refresh Rate",
		"PanelLocal":         "Panel Local",
		"PanelRemote":        "Panel Remote",
		"ShowPreview":        "Show Preview",
		"MarkdownHyperlinks": "Markdown Hyperlinks",
		"Hyperlinks":         "Hyperlinks",
		"SSHPort":            "SSH Port",
		"WebPort":            "Web Port",
		"AuthMode":           "Auth Mode",
	}

	var data []string

	boolToYesNo := func(val bool) string {
		if val {
			return "{{|Var|}}yes{{[-]}}"
		}
		return "{{|Var|}}no{{[-]}}"
	}
	varValue := func(v any) string {
		return fmt.Sprintf("{{|Var|}}%v{{[-]}}", v)
	}

	for _, key := range keys {
		// Filter out keys not present in the legacy file if presentKeys is provided
		if presentKeys != nil && !presentKeys[key] {
			continue
		}

		var value, expandedValue string
		var useFolderColor bool

		switch key {
		case "ConfigFolder":
			value = conf.RawPaths.ConfigFolder
			expandedValue = conf.ConfigDir
			useFolderColor = true
		case "ComposeFolder":
			value = conf.RawPaths.ComposeFolder
			expandedValue = conf.ComposeDir
			useFolderColor = true
		case "SpinnerSpeed":
			value = fmt.Sprintf("{{|Var|}}%dms{{[-]}}", conf.Appearance.SpinnerSpeed)
		case "RefreshRate":
			value = fmt.Sprintf("{{|Var|}}%dms{{[-]}}", conf.Appearance.RefreshRate)
		case "PanelLocal":
			value = varValue(conf.Appearance.PanelLocal)
		case "PanelRemote":
			value = varValue(conf.Appearance.PanelRemote)
		case "ShowPreview":
			value = boolToYesNo(conf.Appearance.ShowPreview)
		case "MarkdownHyperlinks":
			value = varValue(conf.Appearance.MarkdownHyperlinks)
		case "Hyperlinks":
			value = varValue(conf.Appearance.Hyperlinks)
		case "SSHPort":
			if conf.Server.SSH.Port > 0 {
				value = varValue(conf.Server.SSH.Port)
			} else {
				value = "{{|Var|}}not set{{[-]}}"
			}
		case "WebPort":
			if conf.Server.Web.Port > 0 {
				value = varValue(conf.Server.Web.Port)
			} else {
				value = "{{|Var|}}not set{{[-]}}"
			}
		case "AuthMode":
			value = varValue(conf.Server.Auth.Mode)
		}

		// Not hyperlinked even when useFolderColor -- this raw "value" column can hold
		// an unexpanded placeholder like "${XDG_CONFIG_HOME}", which isn't a real path
		// to link to. The resolved path (safe to hyperlink) is the expandedValue column.
		data = append(data, displayNames[key], value)

		switch {
		case expandedValue == "":
			data = append(data, "")
		case useFolderColor:
			data = append(data, console.FormatFolderPath(expandedValue))
		default:
			data = append(data, fmt.Sprintf("{{|Var|}}%s{{[-]}}", expandedValue))
		}
	}

	appearanceHeaders := []string{"{{|UsageCommand|}}Option{{[-]}}"}
	for _, ct := range ConnTypes {
		appearanceHeaders = append(appearanceHeaders, "{{|UsageCommand|}}"+ConnTypeLabel(ct)+"{{[-]}}")
	}
	appearanceRows := []struct {
		key, name string
		value     func(ct string, a Appearance) string
	}{
		{"Theme", "Theme", func(_ string, a Appearance) string { return varValue(a.Theme) }},
		{"Borders", "Borders", func(_ string, a Appearance) string { return boolToYesNo(a.Borders) }},
		{"LargeButtons", "Large Buttons", func(_ string, a Appearance) string { return boolToYesNo(a.LargeButtons) }},
		{"LargeTitleBars", "Large Title Bars", func(_ string, a Appearance) string { return boolToYesNo(a.LargeTitleBars) }},
		{"LineCharacters", "Line Characters", func(_ string, a Appearance) string { return boolToYesNo(a.LineCharacters) }},
		{"Scrollbar", "Scrollbar", func(_ string, a Appearance) string { return boolToYesNo(a.Scrollbar) }},
		{"Spinner", "Spinner", func(_ string, a Appearance) string { return boolToYesNo(a.Spinner) }},
		{"Shadow", "Shadow", func(_ string, a Appearance) string { return boolToYesNo(a.Shadow) }},
		{"ShadowLevel", "Shadow Level", func(_ string, a Appearance) string { return varValue(a.ShadowLevel) }},
		{"BorderColor", "Border Color", func(_ string, a Appearance) string { return varValue(a.BorderColor) }},
		{"DialogTitleAlign", "Dialog Title Align", func(_ string, a Appearance) string { return varValue(a.DialogTitleAlign) }},
		{"SubmenuTitleAlign", "Submenu Title Align", func(_ string, a Appearance) string { return varValue(a.SubmenuTitleAlign) }},
		{"PanelTitleAlign", "Panel Title Align", func(_ string, a Appearance) string { return varValue(a.PanelTitleAlign) }},
		{"CheckboxBrackets", "Checkbox Brackets", func(_ string, a Appearance) string { return varValue(a.CheckboxBrackets) }},
		{"RadioBrackets", "Radio Brackets", func(_ string, a Appearance) string { return varValue(a.RadioBrackets) }},
		{"MenuBrackets", "Menu Brackets", func(_ string, a Appearance) string { return boolToYesNo(a.MenuBrackets) }},
		{"LineNumberBrackets", "Line Number Brackets", func(_ string, a Appearance) string { return boolToYesNo(a.LineNumberBrackets) }},
		{"TabLayout", "Tab Layout", func(_ string, a Appearance) string { return varValue(a.TabLayout) }},
		{"MenuTint", "Menu Tint", func(_ string, a Appearance) string { return tintSummary(a.AnsiColors.AnsiElementColors) }},
		{"MenuOverrides", "Menu Color Overrides", func(_ string, a Appearance) string { return overrideSummary(a.AnsiColors.AnsiElementColors) }},
		{"ProgramBoxTint", "ProgramBox Tint", func(_ string, a Appearance) string { return tintSummary(a.AnsiColors.ProgramBox) }},
		{"ProgramBoxOverrides", "ProgramBox Color Overrides", func(_ string, a Appearance) string { return overrideSummary(a.AnsiColors.ProgramBox) }},
		{"CLITint", "CLI Tint", func(ct string, a Appearance) string {
			if ct != "local" {
				return notApplicable
			}
			return tintSummary(a.AnsiColors.CLI)
		}},
		{"CLIOverrides", "CLI Color Overrides", func(ct string, a Appearance) string {
			if ct != "local" {
				return notApplicable
			}
			return overrideSummary(a.AnsiColors.CLI)
		}},
	}
	var appearanceData []string
	for _, row := range appearanceRows {
		if presentKeys != nil && !presentKeys[row.key] {
			continue
		}
		appearanceData = append(appearanceData, row.name)
		for _, ct := range ConnTypes {
			appearanceData = append(appearanceData, row.value(ct, conf.Appearance.ForConnType(ct)))
		}
	}

	if title == "" {
		title = "Configuration options stored in '" + console.FormatFilePath(paths.GetConfigFilePath()) + "':"
	}
	appearanceTitle := "Appearance options for each connection type (see '{{|UserCommand|}}" + version.CommandName + " --tint{{[-]}}' for tint details):"

	lineChars := conf.Appearance.Local.LineCharacters
	renderTable := func(h, d []string) string {
		var sb strings.Builder
		console.PrintTableCtx(console.WithTUIWriter(ctx, &sb), h, d, lineChars)
		return sb.String()
	}

	type section struct {
		title   string
		headers []string
		data    []string
	}
	sections := []section{{title, headers, data}}
	if len(appearanceData) > 0 {
		sections = append(sections, section{appearanceTitle, appearanceHeaders, appearanceData})
	}

	for i, sec := range sections {
		if len(sec.data) == 0 {
			continue
		}
		if isMigrationMode(ctx) {
			if i > 0 {
				logNotice(ctx, " ")
			}
			logNotice(ctx, sec.title)
			logNotice(ctx, strings.TrimSuffix(renderTable(sec.headers, sec.data), "\n"))
		} else if w := console.GetTUIWriter(ctx); w != nil {
			// Route through the caller's writer (e.g. the console panel's pipe)
			// instead of stdout -- fmt.Println would write straight to the real
			// terminal, bypassing the TUI's own screen compositing entirely.
			fmt.Fprintln(w, semstyle.ToANSI(sec.title))
			fmt.Fprintln(w, semstyle.ToANSI(renderTable(sec.headers, sec.data)))
		} else {
			fmt.Println(semstyle.ToANSI(sec.title))
			fmt.Println(semstyle.ToANSI(renderTable(sec.headers, sec.data)))
		}
	}
}

// notApplicable marks a --config-show cell for a setting that connection
// type never uses.
const notApplicable = "{{|Var|}}n/a{{[-]}}"

// tintSummary is an element's stored tint for --config-show: its reference,
// or "none", marked "(off)" when disabled.
func tintSummary(e AnsiElementColors) string {
	v := e.Tint
	if v == "" {
		v = "none"
	}
	if !e.TintEnabled {
		v += " (off)"
	}
	return "{{|Var|}}" + v + "{{[-]}}"
}

// overrideSummary is an element's stored color overrides for --config-show:
// how many slots are set, marked "(off)" when disabled.
func overrideSummary(e AnsiElementColors) string {
	n := 0
	for _, v := range e.Slots() {
		if v != "" {
			n++
		}
	}
	for _, v := range []string{e.Base01, e.Base02, e.Base04, e.Base06, e.Base09, e.Base0F, e.Base10, e.Base11} {
		if v != "" {
			n++
		}
	}
	v := "none"
	if n > 0 {
		v = fmt.Sprintf("%d set", n)
	}
	if !e.OverrideEnabled {
		v += " (off)"
	}
	return "{{|Var|}}" + v + "{{[-]}}"
}

// logNotice logs a notice message, splitting multi-line messages and logging each separately.
// It uses slog.LevelInfo which is mapped to LevelNotice in the application logger.
func logNotice(ctx context.Context, msg any, args ...any) {
	msgStr := fmt.Sprint(msg)
	if len(args) > 0 && strings.Contains(msgStr, "%") {
		msgStr = fmt.Sprintf(msgStr, args...)
	} else if len(args) > 0 {
		msgStr = fmt.Sprint(append([]any{msgStr}, args...)...)
	}
	lines := strings.Split(msgStr, "\n")
	for _, line := range lines {
		slog.Log(ctx, slog.LevelInfo, line)
	}
}
