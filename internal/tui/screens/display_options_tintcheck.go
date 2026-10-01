package screens

import (
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// tintedThemeName is the bundled theme drawn in tint colors, offered when a
// tint is applied with a theme that isn't.
const tintedThemeName = "TintedTheming"

// tintThemeChoiceMsg carries the answer to the tint-without-a-tinted-theme
// question (see tintThemeMismatches): use the tinted theme on connTypes,
// apply as staged, or neither.
type tintThemeChoiceMsg struct {
	useTinted, apply bool
	connTypes        []string
}

// tintThemeMismatches returns the connection types whose staged settings
// pair a tint with a theme not made for one (see tintWithoutTintedTheme)
// where their saved settings didn't, so a pairing that's already saved is
// never asked about again.
func (s *DisplayOptionsScreen) tintThemeMismatches() []string {
	var out []string
	for _, ct := range config.ConnTypes {
		if tintWithoutTintedTheme(s.config.Appearance.ForConnType(ct)) &&
			!tintWithoutTintedTheme(s.baseConfig.Appearance.ForConnType(ct)) {
			out = append(out, ct)
		}
	}
	return out
}

// tintWithoutTintedTheme reports whether a's Menu tint is on, isn't the
// bundled ANSI one (the standard colors, which any theme looks right with),
// and sits with a theme that isn't drawn in tint colors (see
// theme.Variant). ProgramBox and CLI tints recolor output, not the theme,
// so they don't count.
func tintWithoutTintedTheme(a config.Appearance) bool {
	menu := a.AnsiColors.AnsiElementColors
	if !menu.TintEnabled || menu.Tint == "" || menu.Tint == "embedded:ansi" {
		return false
	}
	tf, err := theme.GetThemeFile(a.Theme)
	if err != nil {
		return false
	}
	variant, _ := theme.Variant(tf)
	return variant != theme.VariantTinted
}

// tintThemeDialog asks whether to use the tinted theme on connTypes (see
// tintThemeMismatches), apply as staged anyway, or go back.
func (s *DisplayOptionsScreen) tintThemeDialog(connTypes []string) tea.Msg {
	// A tint's name without its source, e.g. "0x96f" for "repo:0x96f", and a
	// theme's display name, each in its style.
	tintName := func(ref string) string {
		if _, name, ok := strings.Cut(ref, ":"); ok {
			ref = name
		}
		return "{{|Tint|}}" + ref + "{{[-]}}"
	}
	themeName := func(value string) string {
		// Not s.getThemeFile: this runs off the UI thread, and its cache isn't
		// safe to write from here.
		tf, _ := theme.GetThemeFile(value)
		name := tf.Metadata.Name
		if name == "" {
			name = theme.ThemeDisplayName(value)
		}
		return "{{|Theme|}}" + name + "{{[-]}}"
	}
	var question string
	if len(connTypes) == 1 {
		a := s.config.Appearance.ForConnType(connTypes[0])
		question = "The " + tintName(a.AnsiColors.Tint) + " tint is set for " + config.ConnTypeLabel(connTypes[0]) +
			" connections, but the " + themeName(a.Theme) + " theme isn't made for tints. The colors may look wrong."
	} else {
		lines := []string{"These tints are set with themes that aren't made for tints:", ""}
		for _, ct := range connTypes {
			a := s.config.Appearance.ForConnType(ct)
			lines = append(lines, "  "+config.ConnTypeLabel(ct)+": "+tintName(a.AnsiColors.Tint)+" with "+themeName(a.Theme))
		}
		question = strings.Join(append(lines, "", "The colors may look wrong."), "\n")
	}
	question += "\n\nSwitch to the " + themeName(tintedThemeName) + " theme and check the preview before applying?"
	choose := func(msg tintThemeChoiceMsg) func() tea.Msg {
		msg.connTypes = connTypes
		return tui.CloseDialogThen(func() tea.Msg { return msg })
	}
	outer := displayengine.NewMenuModel("tint_theme_check", "Theme Not Made for Tints", "", nil)
	outer.SetMaximized(false)
	outer.SetIsDialog(true)
	outer.SetDialogType(displayengine.DialogTypeConfirm)
	outer.SetShowButtons(true)
	outer.SetButtons([]displayengine.ButtonDef{
		{Label: "Use Tinted Theming", ZoneID: "btn-select", Action: choose(tintThemeChoiceMsg{useTinted: true}), Help: "Switch to the Tinted Theming theme there, then check the preview before applying."},
		{Label: "Apply Anyway", ZoneID: "btn-apply", Action: choose(tintThemeChoiceMsg{apply: true}), Help: "Apply with the theme as staged."},
		{Label: "Cancel", ZoneID: "btn-cancel", Action: choose(tintThemeChoiceMsg{}), Help: "Go back without applying."},
	})
	outer.SetEscAction(choose(tintThemeChoiceMsg{}))
	outer.SetFocusedBtnIndex(2) // Cancel
	questionSection := displayengine.NewPlainTextSection("tint_theme_check_question", question)
	questionSection.SetPlainTextStyle("", 1)
	outer.AddContentSection(questionSection)
	return displayengine.ShowDialogMsg{Dialog: outer}
}

// stageTintedTheme stages the tinted theme on connTypes as picking it in the
// Theme list would, the shown tab through the preview.
func (s *DisplayOptionsScreen) stageTintedTheme(connTypes []string) {
	for _, ct := range connTypes {
		if ct == s.editType {
			s.applyPreview(tintedThemeName)
			continue
		}
		a := s.config.Appearance.Ptr(ct)
		a.Theme = tintedThemeName
		if !s.loadThemeDefaults {
			continue
		}
		if defaults, err := theme.FileDefaults(s.getThemeFile(tintedThemeName)); err == nil && defaults != nil {
			theme.ApplyThemeDefaults(a, *defaults)
		}
	}
	s.syncThemeList()
	s.refreshChangeMarkers()
	if s.outerMenu != nil {
		s.outerMenu.InvalidateCache()
	}
}
