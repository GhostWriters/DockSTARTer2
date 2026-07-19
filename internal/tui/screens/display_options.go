package screens

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
)

// DisplayOptionsFocus defines which area of the screen has focus
type DisplayOptionsFocus int

const (
	FocusLoadDefaults DisplayOptionsFocus = iota
	FocusThemes
	FocusOptions
	FocusButtons
)

// DisplayOptionsScreen allows the user to configure UI settings and themes together.
type DisplayOptionsScreen struct {
	loadDefaultsMenu *displayengine.MenuModel
	themeMenu        *displayengine.MenuModel
	optionsMenu      *displayengine.MenuModel
	focusedPanel     DisplayOptionsFocus
	// focusedButton index: 0=Apply, 1=Reset, 2=Back (or Exit when isRoot), 3=Exit (only when !isRoot)
	focusedButton int
	buttonFocused bool // true when a button is highlighted while a submenu also stays focused
	isRoot        bool // true when launched directly via -M appearance; hides Back button

	config       config.AppConfig
	themes       []theme.ThemeMetadata
	currentTheme string
	previewTheme string // Theme currently being highlighted in the list

	width  int
	height int

	outerMenu *displayengine.MenuModel // outer "Appearance Settings" dialog with sections + buttons

	focused bool // tracks global screen focus (header/log panel interaction)

	baseConfig     config.AppConfig                // Original exact config before previewing
	themeDefaults  map[string]*theme.ThemeDefaults // Cache parsed defaults
	themeFileCache map[string]theme.ThemeFile      // Cache GetThemeFile results for help text

	// loadThemeDefaults controls whether focusing a theme in the list stages
	// that theme's own [defaults] table on top of the current options. This is
	// a screen-local preference, not part of config.AppConfig -- it only
	// affects behavior while this screen is open, not anything persisted.
	loadThemeDefaults bool

	// themeChangedFields holds the config.UIConfig struct field names whose
	// value the most recent applyPreview call actually changed via the
	// theme's own [defaults] table. Drives the transient "changed" marker
	// (same glyph/tag as App Select's just-added/renamed marker) shown in
	// front of the corresponding Options row until the next interaction.
	themeChangedFields map[string]bool

	connType string // "local", "ssh", or "web"
}

// toggleLoadThemeDefaultsMsg flips loadThemeDefaults. Handled directly rather
// than via updateDisplayOptionMsg since it's not a config.AppConfig field.
type toggleLoadThemeDefaultsMsg struct{}

// updateDisplayOptionMsg is sent when an option is changed in the menu
type updateDisplayOptionMsg struct {
	update func(*config.AppConfig)
}

// displayOptionsAbortMsg is sent when Apply is attempted but blocked (e.g. command lock).
// Handled by Update to clear the processing spinner without applying changes.
type displayOptionsAbortMsg struct{}

// NewDisplayOptionsScreen creates a new consolidated display options screen.
// isRoot suppresses the Back button when this screen is the entry point.
func NewDisplayOptionsScreen(isRoot bool, connType string) *DisplayOptionsScreen {
	themes, _ := theme.List()
	cfg := config.LoadAppConfig()
	current := cfg.UI.Theme // ConfigValue e.g. "DockSTARTer" or "user:MyTheme"

	s := &DisplayOptionsScreen{
		isRoot:            isRoot,
		connType:          connType,
		config:            cfg,
		baseConfig:        cfg,
		themes:            themes,
		currentTheme:      current,
		previewTheme:      current,
		themeDefaults:     make(map[string]*theme.ThemeDefaults),
		themeFileCache:    make(map[string]theme.ThemeFile),
		loadThemeDefaults: true,
	}
	s.themeDefaults[current], _ = theme.Load(current, "Preview")

	s.initMenus()
	s.focused = true // Default to focused initially
	return s
}

func (s *DisplayOptionsScreen) initMenus() {
	// 1. Theme Selection Menu
	themeItems := make([]displayengine.MenuItem, len(s.themes))
	foundCurrent := false
	for i, t := range s.themes {
		desc := t.Description
		if t.Author != "" {
			desc += fmt.Sprintf(" [by %s]", t.Author)
		}
		descTag := "{{|ListItem|}}"
		if t.IsUserTheme {
			descTag = "{{|ListItemUserDefined|}}"
		}
		checked := s.currentTheme == t.ConfigValue
		if checked {
			foundCurrent = true
		}
		themeItems[i] = displayengine.MenuItem{
			Tag:           t.Name,
			Desc:          descTag + desc,
			Help:          desc,
			IsRadioButton: true,
			Checked:       checked,
			IsInvalid:     t.IsInvalid,
			IsUserDefined: t.IsUserTheme,
			Metadata:      map[string]string{"config_value": t.ConfigValue},
		}
	}
	// If the configured theme no longer exists on disk, prepend a placeholder so the
	// user can see what is active and optionally switch away from it.
	if !foundCurrent && s.currentTheme != "" {
		shortURI := s.currentTheme
		if strings.HasPrefix(s.currentTheme, "file:") {
			shortURI = "file:" + theme.ThemeDisplayName(s.currentTheme)
		}
		displayName := "(missing) " + shortURI
		themeItems = append([]displayengine.MenuItem{{
			Tag:           displayName,
			Desc:          "{{|ListItemUserDefined|}}Source file not found — using cached version",
			Help:          "Theme source file is missing. The cached version remains active until you choose another theme.",
			IsRadioButton: true,
			Checked:       true,
			IsUserDefined: true,
			Metadata:      map[string]string{"config_value": s.currentTheme},
		}}, themeItems...)
	}

	themeMenu := displayengine.NewMenuModel(displayengine.IDThemePanel, "Select Theme", "", themeItems)
	s.themeMenu = themeMenu
	s.themeMenu.SetHelpItemPrefix("Theme")
	s.themeMenu.SetItemHelpFunc(s.buildThemeItemHelp)
	s.themeMenu.SetHelpPageText("Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.")
	s.themeMenu.SetSubMenuMode(true)
	s.themeMenu.SetVariableHeight(true)
	s.themeMenu.SetIsDialog(false) // Part of a screen, not a modal
	s.themeMenu.SetButtons([]displayengine.ButtonDef{})
	s.themeMenu.SetMaximized(true) // Fill available width
	s.themeMenu.SetShowLockGutter(false)
	s.themeMenu.SetNoLeftMargin(true)

	// 2. Load Theme Defaults Menu (own section, above the theme list, since
	// toggling it affects how focusing a theme below behaves)
	loadDefaultsItems := []displayengine.MenuItem{
		{
			Tag:        "Load Theme Defaults",
			Desc:       "Stage a theme's suggested options when focused",
			Help:       "When on, browsing themes stages that theme's own suggested options below (Space to toggle)",
			IsCheckbox: true,
			Checked:    s.loadThemeDefaults,
			Selectable: true,
			SpaceAction: func() tea.Msg {
				return toggleLoadThemeDefaultsMsg{}
			},
		},
	}
	loadDefaultsMenu := displayengine.NewMenuModel(displayengine.IDLoadDefaultsPanel, "", "", loadDefaultsItems)
	s.loadDefaultsMenu = loadDefaultsMenu
	s.loadDefaultsMenu.SetHelpItemPrefix("Option")
	s.loadDefaultsMenu.SetHelpPageText("Controls whether focusing a theme below stages that theme's own suggested options.")
	s.loadDefaultsMenu.SetSubMenuMode(true)
	s.loadDefaultsMenu.SetIsDialog(false)
	s.loadDefaultsMenu.SetButtons([]displayengine.ButtonDef{})
	s.loadDefaultsMenu.SetFlowMode(true)
	s.loadDefaultsMenu.SetMaximized(true)
	s.loadDefaultsMenu.SetShowLockGutter(false)

	// 3. Options Menu
	// Grouped: visual toggles, then brackets, then title alignment, then performance.
	optionItems := []displayengine.MenuItem{
		// -- Visual toggles --
		{
			Tag:         "Borders",
			Desc:        "Show borders on all dialogs",
			Help:        "Toggle border visibility (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.Borders,
			Selectable:  true,
			SpaceAction: s.toggleBorders(),
		},
		{
			Tag:         "Large Buttons",
			Desc:        "Show large (bordered) buttons",
			Help:        "Toggle large (bordered) vs flat button style (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.LargeButtons,
			Selectable:  true,
			SpaceAction: s.toggleLargeButtons(),
		},
		{
			Tag:         "Large Title Bars",
			Desc:        "Show title in a separate row above content",
			Help:        "Toggle large title bar style (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.LargeTitleBars,
			Selectable:  true,
			SpaceAction: s.toggleLargeTitleBars(),
		},
		{
			Tag:         "Line Characters",
			Desc:        "Use unicode line drawing characters",
			Help:        "Use ┌─ instead of +- for borders (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.LineCharacters,
			Selectable:  true,
			SpaceAction: s.toggleLineChars(),
		},
		{
			Tag:         "Shadows",
			Desc:        "Enable drop shadows",
			Help:        "Toggle drop shadow effect (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.Shadow,
			Selectable:  true,
			SpaceAction: s.toggleShadow(),
		},
		{
			Tag:    "Shadow Level",
			Desc:   s.dropdownDesc(s.shadowLevelToDesc(s.config.UI.ShadowLevel)),
			Help:   "Adjust the density of the shadow (Select/Enter for list)",
			Action: s.showShadowDropdown(),
		},
		{
			Tag:    "Border Color",
			Desc:   s.dropdownDesc(s.borderColorToDesc(s.config.UI.BorderColor)),
			Help:   "Choose theme colors for borders (Select/Enter for list)",
			Action: s.showBorderColorDropdown(),
		},
		{
			Tag:         "Scrollbars",
			Desc:        "Show scrollbar in lists",
			Help:        "Toggle scrollbar in scrollable lists (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.Scrollbar,
			Selectable:  true,
			SpaceAction: s.toggleScrollbar(),
		},
		{
			Tag:         "Spinners",
			Desc:        "Show loading spinner animations",
			Help:        "Toggle spinner animations during loading (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.Spinner,
			Selectable:  true,
			SpaceAction: s.toggleSpinner(),
		},
		{
			Tag:         "Menu Brackets",
			Desc:        "Wrap the focused menu item in [brackets]",
			Help:        "Bracket the focused row's tag (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.MenuBrackets,
			Selectable:  true,
			SpaceAction: s.toggleMenuBrackets(),
		},
		{
			Tag:         "Line Number Brackets",
			Desc:        "Wrap the focused line number in [brackets]",
			Help:        "Bracket the focused line's number in the env editor (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.LineNumberBrackets,
			Selectable:  true,
			SpaceAction: s.toggleLineNumberBrackets(),
		},

		// -- Brackets --
		{
			Tag:  "Checkbox Brackets",
			Desc: s.dropdownDesc(bracketModeDesc(s.config.UI.CheckboxBrackets)),
			Help: "When checkbox brackets are shown in lists (Enter for options)",
			Action: s.showBracketModeDropdown("checkbox_brackets", "Checkbox Brackets",
				func() string { return s.config.UI.CheckboxBrackets },
				func(cfg *config.AppConfig, v string) { cfg.UI.CheckboxBrackets = v }),
		},
		{
			Tag:  "Radio Brackets",
			Desc: s.dropdownDesc(bracketModeDesc(s.config.UI.RadioBrackets)),
			Help: "When radio button brackets are shown in lists (Enter for options)",
			Action: s.showBracketModeDropdown("radio_brackets", "Radio Brackets",
				func() string { return s.config.UI.RadioBrackets },
				func(cfg *config.AppConfig, v string) { cfg.UI.RadioBrackets = v }),
		},

		// -- Title alignment --
		{
			Tag:  "Dialog Title",
			Desc: s.dropdownDesc(titleAlignDesc(s.config.UI.DialogTitleAlign)),
			Help: "Alignment of titles in dialog borders (Enter for options)",
			Action: s.showTitleAlignDropdown("dialog_title_align", "Dialog Title Align",
				func() string { return s.config.UI.DialogTitleAlign },
				func(cfg *config.AppConfig, v string) { cfg.UI.DialogTitleAlign = v }),
		},
		{
			Tag:  "Submenu Title",
			Desc: s.dropdownDesc(titleAlignDesc(s.config.UI.SubmenuTitleAlign)),
			Help: "Alignment of subtitle rows inside menus (Enter for options)",
			Action: s.showTitleAlignDropdown("submenu_title_align", "Submenu Title Align",
				func() string { return s.config.UI.SubmenuTitleAlign },
				func(cfg *config.AppConfig, v string) { cfg.UI.SubmenuTitleAlign = v }),
		},
		{
			Tag:  "Panel Title",
			Desc: s.dropdownDesc(titleAlignDesc(s.config.UI.PanelTitleAlign)),
			Help: "Alignment of the panel strip label (Enter for options)",
			Action: s.showTitleAlignDropdown("panel_title_align", "Panel Title Align",
				func() string { return s.config.UI.PanelTitleAlign },
				func(cfg *config.AppConfig, v string) { cfg.UI.PanelTitleAlign = v }),
		},

		// -- Performance --
		{
			Tag:        "Refresh Rate",
			Desc:       fmt.Sprintf("{{|OptionValue|}}%dms{{[-]}}", s.config.UI.RefreshRate),
			Help:       "Screen repaint interval in milliseconds (Enter to change). Applies on restart.",
			Action:     s.promptRefreshRate(),
			Selectable: true,
		},
		{
			Tag:        "Spinner Speed",
			Desc:       fmt.Sprintf("{{|OptionValue|}}%dms{{[-]}}", s.config.UI.SpinnerSpeed),
			Help:       "Spinner frame speed in milliseconds (Enter to change)",
			Action:     s.promptSpinnerSpeed(),
			Selectable: true,
		},
	}

	if s.connType != "web" {
		optionItems = append(optionItems, displayengine.MenuItem{
			Tag:           "Local Panel Mode",
			Desc:          s.dropdownDesc(s.panelModeToDesc(s.config.UI.PanelLocal)),
			Help:          "Choose the panel mode for local terminal sessions (Console allowed).",
			Action:        s.showPanelDropdown(true),
			IsDestructive: true,
		})
	}

	if s.connType != "web" {
		label := "Remote Panel Mode"
		if s.connType == "local" {
			label = "Remote Panel Mode"
		}
		optionItems = append(optionItems, displayengine.MenuItem{
			Tag:           label,
			Desc:          s.dropdownDesc(s.panelModeToDesc(s.config.UI.PanelRemote)),
			Help:          "Choose the panel mode for SSH and Web sessions (Console restricted).",
			Action:        s.showPanelDropdown(false),
			IsDestructive: true,
		})
	}

	optionsMenu := displayengine.NewMenuModel(displayengine.IDOptionsPanel, "Options", "", optionItems)
	s.optionsMenu = optionsMenu
	s.optionsMenu.SetHelpItemPrefix("Option")
	s.optionsMenu.SetHelpPageText("Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.")
	s.optionsMenu.SetSubMenuMode(true)
	s.optionsMenu.SetIsDialog(false) // Part of a screen, not a modal
	s.optionsMenu.SetButtons([]displayengine.ButtonDef{})
	s.optionsMenu.SetFlowMode(true)
	s.optionsMenu.SetMaximized(true) // Fill available width
	s.optionsMenu.SetShowLockGutter(true)

	// 4. Outer "Appearance Settings" dialog (sections container + buttons)
	outerMenu := displayengine.NewMenuModel("appearance_outer", "Appearance Settings", "", nil)
	if s.isRoot {
		outerMenu.SetButtons([]displayengine.ButtonDef{
			{Label: "Apply", ZoneID: displayengine.IDApplyButton, Help: "Apply and save appearance settings."},
			{Label: "Reset", ZoneID: displayengine.IDResetButton, Help: "Discard staged changes and revert to the current saved settings."},
			{Label: "Exit", ZoneID: displayengine.IDExitButton, Action: tui.ConfirmExitAction(), Help: "Exit the application."},
		})
	} else {
		outerMenu.SetButtons([]displayengine.ButtonDef{
			{Label: "Apply", ZoneID: displayengine.IDApplyButton, Help: "Apply and save appearance settings."},
			{Label: "Reset", ZoneID: displayengine.IDResetButton, Help: "Discard staged changes and revert to the current saved settings."},
			{Label: "Back", ZoneID: displayengine.IDBackButton, Action: navigateBack(), Help: "Return to the previous screen."},
			{Label: "Exit", ZoneID: displayengine.IDExitButton, Action: tui.ConfirmExitAction(), Help: "Exit the application."},
		})
	}
	// Title-bar refresh icon mirrors the Reset button, matching the tabbed
	// vars editor's use of the same widget for its own reload action. Extra
	// widgets go before Help/Close, which stay rightmost by convention (see
	// TitleBarFocus doc comment).
	outerMenu.ConfigureWidgets(displayengine.WidgetRefresh, displayengine.WidgetHelp, displayengine.WidgetClose)
	outerMenu.AddContentSection(loadDefaultsMenu)
	outerMenu.AddContentSection(themeMenu)
	outerMenu.AddContentSection(optionsMenu)
	s.outerMenu = outerMenu

	// Set initial focus state — applied properly when SetFocused(true) is called by AppModel.
	s.focusedPanel = FocusLoadDefaults
	s.buttonFocused = true
	s.focusedButton = 0
}

// maxFocusedButton returns the highest valid focusedButton index.
// When isRoot there is no Back button: Apply=0, Reset=1, Exit=2 (three buttons).
// Otherwise: Apply=0, Reset=1, Back=2, Exit=3 (four buttons).
func (s *DisplayOptionsScreen) maxFocusedButton() int {
	if s.isRoot {
		return 2
	}
	return 3
}

// execFocusedButton runs the action for the current focusedButton index.
func (s *DisplayOptionsScreen) execFocusedButton() (tea.Model, tea.Cmd) {
	switch s.focusedButton {
	case 0:
		return s, s.outerMenu.SetProcessingBtnDeferred(displayengine.IDApplyButton, s.handleApply())
	case 1:
		return s, s.outerMenu.SetProcessingBtnDeferred(displayengine.IDResetButton, s.handleReset())
	case 2:
		if s.isRoot {
			theme.Unload("Preview")
			return s, s.outerMenu.SetProcessingBtnDeferred(displayengine.IDExitButton, tui.ConfirmExitAction())
		}
		theme.Unload("Preview")
		return s, s.outerMenu.SetProcessingBtnDeferred(displayengine.IDBackButton, navigateBack())
	case 3:
		theme.Unload("Preview")
		return s, s.outerMenu.SetProcessingBtnDeferred(displayengine.IDExitButton, tui.ConfirmExitAction())
	}
	return s, nil
}

func (s *DisplayOptionsScreen) updateFocusStates() {
	// Explicit button panel focus clears the dual-focus state.
	if s.focusedPanel == FocusButtons {
		s.buttonFocused = false
	}
	// Submenus stay visually focused when a button is also highlighted (buttonFocused).
	s.loadDefaultsMenu.SetSubFocused(s.focused && s.focusedPanel == FocusLoadDefaults)
	s.themeMenu.SetSubFocused(s.focused && s.focusedPanel == FocusThemes)
	s.optionsMenu.SetSubFocused(s.focused && s.focusedPanel == FocusOptions)

	if s.outerMenu == nil {
		return
	}
	s.outerMenu.SetFocused(s.focused)
	if s.focusedPanel == FocusButtons || s.buttonFocused {
		s.outerMenu.SetFocusedBtnIndex(s.focusedButton)
	} else {
		s.outerMenu.SetFocusedItem(displayengine.FocusList)
	}
	s.outerMenu.InvalidateCache()
}

// SetFocused updates the global focus state for this screen
func (s *DisplayOptionsScreen) SetFocused(f bool) {
	s.focused = f
	s.updateFocusStates()
}

// getThemeFile returns a cached ThemeFile for the given config value.
func (s *DisplayOptionsScreen) getThemeFile(configValue string) theme.ThemeFile {
	if tf, ok := s.themeFileCache[configValue]; ok {
		return tf
	}
	tf, _ := theme.GetThemeFile(configValue)
	s.themeFileCache[configValue] = tf
	return tf
}

// formatThemeDefaults produces a human-readable list of defaults a theme will apply.
// Returns an empty string when no defaults are set.
func formatThemeDefaults(d *theme.ThemeDefaults) string {
	if d == nil {
		return ""
	}
	boolStr := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	var lines []string
	if d.Borders != nil {
		lines = append(lines, fmt.Sprintf("  Borders: %s", boolStr(*d.Borders)))
	}
	if d.LargeButtons != nil {
		lines = append(lines, fmt.Sprintf("  Large Buttons: %s", boolStr(*d.LargeButtons)))
	}
	if d.LargeTitleBars != nil {
		lines = append(lines, fmt.Sprintf("  Large Title Bars: %s", boolStr(*d.LargeTitleBars)))
	}
	if d.LineCharacters != nil {
		lines = append(lines, fmt.Sprintf("  Line Characters: %s", boolStr(*d.LineCharacters)))
	}
	if d.Shadow != nil {
		lines = append(lines, fmt.Sprintf("  Shadow: %s", boolStr(*d.Shadow)))
	}
	if d.ShadowLevel != nil {
		lines = append(lines, fmt.Sprintf("  Shadow Level: %d", *d.ShadowLevel))
	}
	if d.Scrollbar != nil {
		lines = append(lines, fmt.Sprintf("  Scrollbar: %s", boolStr(*d.Scrollbar)))
	}
	if d.MenuBrackets != nil {
		lines = append(lines, fmt.Sprintf("  Menu Brackets: %s", boolStr(*d.MenuBrackets)))
	}
	if d.BorderColor != nil {
		lines = append(lines, fmt.Sprintf("  Border Color: %d", *d.BorderColor))
	}
	if d.DialogTitleAlign != nil {
		lines = append(lines, fmt.Sprintf("  Dialog Title: %s", *d.DialogTitleAlign))
	}
	if d.SubmenuTitleAlign != nil {
		lines = append(lines, fmt.Sprintf("  Submenu Title: %s", *d.SubmenuTitleAlign))
	}
	if d.PanelTitleAlign != nil {
		lines = append(lines, fmt.Sprintf("  Log Title: %s", *d.PanelTitleAlign))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Defaults applied by this theme:\n" + strings.Join(lines, "\n")
}

// buildThemeItemHelp returns enriched (itemTitle, itemText) for a theme menu item.
// Used by itemHelpFunc (right-click) and HelpContext (F1).
func (s *DisplayOptionsScreen) buildThemeItemHelp(item displayengine.MenuItem) (itemTitle, itemText string) {
	cv, ok := item.Metadata["config_value"]
	if !ok || cv == "" {
		return "", ""
	}
	tf := s.getThemeFile(cv)

	var parts []string
	desc := tf.Metadata.Description
	if desc == "" {
		// Fallback to what was shown in the list (ThemeMetadata.Description)
		for _, tm := range s.themes {
			if tm.ConfigValue == cv {
				desc = tm.Description
				break
			}
		}
	}
	if desc != "" {
		parts = append(parts, desc)
	}
	if tf.Metadata.Author != "" {
		parts = append(parts, "By: "+tf.Metadata.Author)
	}
	if defaults, derr := theme.FileDefaults(tf); derr == nil {
		if defaultsText := formatThemeDefaults(defaults); defaultsText != "" {
			parts = append(parts, defaultsText)
		}
	}
	if len(parts) == 0 {
		return "", ""
	}
	return item.Tag, strings.Join(parts, "\n\n")
}

// HelpContext implements displayengine.HelpContextProvider.
func (s *DisplayOptionsScreen) HelpContext(maxWidth int) displayengine.HelpContext {
	screenName := s.outerMenu.Title()
	pageText := "Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options."

	var inner displayengine.HelpContext
	switch s.focusedPanel {
	case FocusLoadDefaults:
		inner = s.loadDefaultsMenu.HelpContext(maxWidth)
	case FocusThemes:
		inner = s.themeMenu.HelpContext(maxWidth)
		// Enrich with theme description, author, and defaults
		items := s.themeMenu.GetItems()
		idx := s.themeMenu.Index()
		if idx >= 0 && idx < len(items) {
			if t, txt := s.buildThemeItemHelp(items[idx]); txt != "" {
				if t != "" {
					inner.ItemTitle = t
				}
				inner.ItemText = txt
			}
		}
	case FocusOptions:
		inner = s.optionsMenu.HelpContext(maxWidth)
	}

	inner.ScreenName = screenName
	inner.PageTitle = "Description"
	inner.PageText = pageText

	return inner
}

func (s *DisplayOptionsScreen) shadowLevelToDesc(l int) string {
	var levels []string
	if s.config.UI.LineCharacters {
		levels = []string{"(Off)", "(░)", "(▒)", "(▓)", "(█)"}
	} else {
		levels = []string{
			"(Off)",
			"({{|Shadow|}}.{{|OptionValue|}})",
			"({{|Shadow|}}:{{|OptionValue|}})",
			"({{|Shadow|}}#{{|OptionValue|}})",
			"({{|Shadow|}}█{{|OptionValue|}})",
		}
	}
	if l < 0 || l >= len(levels) {
		l = 0
	}
	return levels[l]
}

func (s *DisplayOptionsScreen) borderColorToDesc(c int) string {
	modes := map[int]string{1: "(1)", 2: "(2)", 3: "(3D)"}
	return modes[c]
}

func (s *DisplayOptionsScreen) dropdownDesc(val string) string {
	return fmt.Sprintf("{{|OptionValue|}}%s▼{{[-]}}", val)
}

func titleAlignDesc(v string) string {
	if v == "left" {
		return "Left"
	}
	return "Center"
}

func bracketModeDesc(v string) string {
	switch strings.ToLower(v) {
	case "never":
		return "Never"
	case "always":
		return "Always"
	default:
		return "Selected"
	}
}

func (s *DisplayOptionsScreen) panelModeToDesc(v string) string {
	switch strings.ToLower(v) {
	case "none":
		return "None"
	case "log":
		return "Log"
	case "console":
		return "Console"
	case "system":
		return "System Console"
	default:
		return "Default"
	}
}

func (s *DisplayOptionsScreen) titleAlignAction(apply func(*config.AppConfig, string), val string) func() tea.Msg {
	return func() tea.Msg {
		return tea.Batch(
			func() tea.Msg {
				return updateDisplayOptionMsg{func(cfg *config.AppConfig) { apply(cfg, val) }}
			},
			tui.CloseDialog(),
		)()
	}
}

func (s *DisplayOptionsScreen) showTitleAlignDropdown(menuName, label string, getter func() string, apply func(*config.AppConfig, string)) tea.Cmd {
	return func() tea.Msg {
		current := getter()
		items := []displayengine.MenuItem{
			{Tag: "Left", Help: "Align title to the left", Action: s.titleAlignAction(apply, "left")},
			{Tag: "Center", Help: "Center the title", Action: s.titleAlignAction(apply, "center")},
		}
		menu := displayengine.NewMenuModel(menuName, label, "Select alignment", items)
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Select", ZoneID: "btn-select", Help: "Confirm and execute the selected action."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		if current == "left" {
			menu.Select(0)
		} else {
			menu.Select(1)
		}
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

// showBracketModeDropdown shows the Never/Selected/Always picker for
// ui.checkbox_brackets/ui.radio_brackets. Reuses titleAlignAction, which is
// generic (apply a value, close the dialog) despite its title-align-specific
// name.
func (s *DisplayOptionsScreen) showBracketModeDropdown(menuName, label string, getter func() string, apply func(*config.AppConfig, string)) tea.Cmd {
	return func() tea.Msg {
		current := strings.ToLower(getter())
		items := []displayengine.MenuItem{
			{Tag: "Never", Desc: "Only the focused row is bracketed", Help: "Only the focused row is bracketed", Action: s.titleAlignAction(apply, "never")},
			{Tag: "Selected", Desc: "Bracketed when focused or checked", Help: "Bracketed when focused or checked", Action: s.titleAlignAction(apply, "selected")},
			{Tag: "Always", Desc: "Every row is bracketed", Help: "Every row is bracketed", Action: s.titleAlignAction(apply, "always")},
		}
		menu := displayengine.NewMenuModel(menuName, label, "Select mode", items)
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Select", ZoneID: "btn-select", Help: "Confirm and execute the selected action."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		switch current {
		case "never":
			menu.Select(0)
		case "always":
			menu.Select(2)
		default:
			menu.Select(1)
		}
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

func (s *DisplayOptionsScreen) showPanelDropdown(isLocalSetting bool) tea.Cmd {
	return func() tea.Msg {
		currentMode := s.config.UI.PanelRemote
		if isLocalSetting {
			currentMode = s.config.UI.PanelLocal
		}

		applyChange := func(mode string) tea.Cmd {
			return func() tea.Msg {
				return tea.Batch(
					func() tea.Msg {
						return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
							if isLocalSetting {
								cfg.UI.PanelLocal = mode
							} else {
								cfg.UI.PanelRemote = mode
							}
						}}
					},
					tui.CloseDialog(),
				)()
			}
		}

		// confirmChange: only warn for remote sessions currently in system/console
		// mode switching to log/none (would lose interactive access).
		isInteractive := strings.ToLower(currentMode) == "system" || strings.ToLower(currentMode) == "console"
		confirmChange := func(mode string) tea.Cmd {
			return func() tea.Msg {
				if s.connType == "local" || !isInteractive {
					return applyChange(mode)()
				}
				title := "Disable Interactive Panel?"
				msg := "You are removing the interactive panel. You will only be able to re-enable it from a local terminal session.\n\nAre you sure you want to proceed?"
				onConfirm := func() tea.Msg {
					return tea.Batch(applyChange(mode), tui.CloseDialog())()
				}
				confirm := tui.NewConfirmModel(title, msg, false, onConfirm, tui.CloseDialog())
				return displayengine.ShowDialogMsg{Dialog: confirm}
			}
		}

		var items []displayengine.MenuItem

		// None option: always available
		items = append(items, displayengine.MenuItem{
			Tag:    "None",
			Desc:   "Hide the panel entirely",
			Help:   "Removes the panel and stretches content to the bottom of the screen.",
			Action: func() tea.Msg { return confirmChange("none")() },
		})

		// Log option: always available
		items = append(items, displayengine.MenuItem{
			Tag:    "Log",
			Desc:   "Show read-only log viewer",
			Help:   "Displays application logs but hides the command input bar.",
			Action: func() tea.Msg { return confirmChange("log")() },
		})

		// Console (ds2-only): always available for both local and remote —
		// it only accepts ds2 subcommands so it is safe in all session types.
		items = append(items, displayengine.MenuItem{
			Tag:    "Console",
			Desc:   "ds2 commands only",
			Help:   "Accepts ds2 subcommands only. Safe for remote sessions.",
			Action: func() tea.Msg { return applyChange("console")() },
		})

		// System Console: full shell access.
		// Always show in the dropdown, but require sudo auth if remote.
		systemAction := func() tea.Msg {
			// Warn and require sudo when enabling System Console for remote sessions.
			if !isLocalSetting && s.connType != "local" {
				title := "Enable Remote System Console?"
				msg := "System Console grants full interactive shell access to all authenticated SSH and web users. Any command, including destructive ones, can be run.\n\nAre you sure you want to proceed?"
				onConfirm := func() tea.Msg {
					// After confirmation, ask for sudo password
					return func() tea.Msg {
						pass, err := tui.PromptText("Sudo Authentication", "Password required to enable System Console:", true)
						if err != nil {
							if err == console.ErrUserAborted {
								return tui.CloseDialog()()
							}
							return tui.ShowMessageDialogMsg{
								Title:   "Authentication Error",
								Message: err.Error(),
								Type:    tui.MessageError,
							}
						}

						// Validate via sudo -S -v
						cmd := exec.Command("sudo", "-S", "-v")
						cmd.Stdin = strings.NewReader(pass + "\n")
						if err := cmd.Run(); err != nil {
							errMsg := "sudo: authentication failed"
							if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
								errMsg = "sudo command not found on this system"
							}
							return tui.ShowMessageDialogMsg{
								Title:   "Authentication Failed",
								Message: errMsg,
								Type:    tui.MessageError,
							}
						}

						// Success: Apply the change persistently and close dialog
						return tea.Batch(applyChange("system"), tui.CloseDialog())()
					}()
				}
				confirm := tui.NewConfirmModel(title, msg, false, onConfirm, tui.CloseDialog())
				return displayengine.ShowDialogMsg{Dialog: confirm}
			}
			return applyChange("system")()
		}
		items = append(items, displayengine.MenuItem{
			Tag:    "System Console",
			Desc:   "Full shell access",
			Help:   "Passes commands directly to the OS shell. Use with caution for remote sessions.",
			Action: systemAction,
		})

		title := "Remote Panel Mode"
		if isLocalSetting {
			title = "Local Panel Mode"
		}
		menu := displayengine.NewMenuModel("panel_dropdown", title, "Choose layout", items)
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Select", ZoneID: "btn-select", Help: "Confirm and execute the selected action."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})

		// Set initial selection — "system" maps to tag "System Console"
		current := strings.ToLower(currentMode)
		for i, item := range items {
			if strings.ToLower(item.Tag) == current {
				menu.Select(i)
				break
			}
		}

		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

func (s *DisplayOptionsScreen) showShadowDropdown() tea.Cmd {
	return func() tea.Msg {
		type shadowEntry struct{ label, value string }
		var entries []shadowEntry
		if s.config.UI.LineCharacters {
			entries = []shadowEntry{
				{"Off", ""},
				{"Light", "{{|OptionValue|}}(░){{[-]}}"},
				{"Medium", "{{|OptionValue|}}(▒){{[-]}}"},
				{"Dark", "{{|OptionValue|}}(▓){{[-]}}"},
				{"Solid", "{{|OptionValue|}}(█){{[-]}}"},
			}
		} else {
			entries = []shadowEntry{
				{"Off", ""},
				{"Light", "{{|OptionValue|}}({{|Shadow|}}.{{|OptionValue|}}){{[-]}}"},
				{"Medium", "{{|OptionValue|}}({{|Shadow|}}:{{|OptionValue|}}){{[-]}}"},
				{"Dark", "{{|OptionValue|}}({{|Shadow|}}#{{|OptionValue|}}){{[-]}}"},
				{"Solid", "{{|OptionValue|}}({{|Shadow|}}█{{|OptionValue|}}){{[-]}}"},
			}
		}
		var items []displayengine.MenuItem
		for i, e := range entries {
			level := i
			items = append(items, displayengine.MenuItem{
				Tag:  e.label,
				Desc: e.value,
				Help: fmt.Sprintf("Set shadow to %s", e.label),
				Action: func() tea.Msg {
					return tea.Batch(
						func() tea.Msg {
							return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
								cfg.UI.ShadowLevel = level
							}}
						},
						tui.CloseDialog(),
					)()
				},
			})
		}
		menu := displayengine.NewMenuModel("shadow_dropdown", "Shadow Level", "Select shadow fill pattern", items)
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Select", ZoneID: "btn-select", Help: "Confirm and execute the selected action."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		menu.Select(s.config.UI.ShadowLevel)
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

func (s *DisplayOptionsScreen) showBorderColorDropdown() tea.Cmd {
	return func() tea.Msg {
		type borderEntry struct {
			mode         int
			label, value string
		}
		entries := []borderEntry{
			{1, "Border 1 (Theme Focus)", "{{|OptionValue|}}(1){{[-]}}"},
			{2, "Border 2 (Theme Accent)", "{{|OptionValue|}}(2){{[-]}}"},
			{3, "Both (3D Effect)", "{{|OptionValue|}}(3D){{[-]}}"},
		}
		var items []displayengine.MenuItem
		for _, e := range entries {
			mode := e.mode
			items = append(items, displayengine.MenuItem{
				Tag:  e.label,
				Desc: e.value,
				Help: fmt.Sprintf("Set border coloring to %s", e.label),
				Action: func() tea.Msg {
					return tea.Batch(
						func() tea.Msg {
							return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
								cfg.UI.BorderColor = mode
							}}
						},
						tui.CloseDialog(),
					)()
				},
			})
		}
		menu := displayengine.NewMenuModel("border_dropdown", "Border Coloring", "Select which theme colors highlight borders", items)
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Select", ZoneID: "btn-select", Help: "Confirm and execute the selected action."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		menu.Select(s.config.UI.BorderColor - 1)
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

func (s *DisplayOptionsScreen) toggleBorders() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.UI.Borders
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.Borders = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleLargeButtons() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.UI.LargeButtons
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.LargeButtons = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleLargeTitleBars() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.UI.LargeTitleBars
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.LargeTitleBars = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleLineChars() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.UI.LineCharacters
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.LineCharacters = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleShadow() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.UI.Shadow
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.Shadow = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleScrollbar() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.UI.Scrollbar
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.Scrollbar = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleMenuBrackets() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.UI.MenuBrackets
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.MenuBrackets = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleLineNumberBrackets() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.UI.LineNumberBrackets
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.LineNumberBrackets = newState
		}}
	}
}

func (s *DisplayOptionsScreen) toggleSpinner() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.UI.Spinner
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.Spinner = newState
		}}
	}
}

func (s *DisplayOptionsScreen) promptSpinnerSpeed() tea.Cmd {
	return func() tea.Msg {
		result, err := console.TextPrompt(context.Background(),
			func(context.Context, any, ...any) {}, "Spinner Speed", "Enter frame speed in milliseconds (50-5000)", false,
			strconv.Itoa(s.config.UI.SpinnerSpeed))
		if err != nil {
			return nil
		}
		ms, err := strconv.Atoi(strings.TrimSpace(result))
		if err != nil || ms < 50 || ms > 5000 {
			return tui.ShowMessageDialogMsg{
				Title:   "Invalid Speed",
				Message: "Spinner speed must be between 50 and 5000 milliseconds.",
				Type:    tui.MessageError,
			}
		}
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.SpinnerSpeed = ms
		}}
	}
}

func (s *DisplayOptionsScreen) promptRefreshRate() tea.Cmd {
	return func() tea.Msg {
		result, err := console.TextPrompt(context.Background(),
			func(context.Context, any, ...any) {}, "Refresh Rate",
			fmt.Sprintf("Enter screen repaint interval in milliseconds (%d-%d)", config.MinRefreshRateMS, config.MaxRefreshRateMS), false,
			strconv.Itoa(s.config.UI.RefreshRate))
		if err != nil {
			return nil
		}
		ms, err := strconv.Atoi(strings.TrimSpace(result))
		if err != nil || ms < config.MinRefreshRateMS || ms > config.MaxRefreshRateMS {
			return tui.ShowMessageDialogMsg{
				Title:   "Invalid Refresh Rate",
				Message: fmt.Sprintf("Refresh rate must be between %d and %d milliseconds.", config.MinRefreshRateMS, config.MaxRefreshRateMS),
				Type:    tui.MessageError,
			}
		}
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.RefreshRate = ms
		}}
	}
}

func (s *DisplayOptionsScreen) handleApply() tea.Cmd {
	return func() tea.Msg {
		// Do not apply if any options settings are locked (e.g. panel command running).
		if s.optionsMenu.AnyLocked() {
			return displayOptionsAbortMsg{}
		}
		// 1. Apply Theme (Find the actually checked radio option)
		themeSelected := s.previewTheme
		for _, item := range s.themeMenu.GetItems() {
			if item.Checked {
				if cv, ok := item.Metadata["config_value"]; ok {
					themeSelected = cv
				} else {
					themeSelected = item.Tag
				}
				break
			}
		}

		_, err := theme.Load(themeSelected, "")
		if err == nil {
			s.currentTheme = themeSelected
			s.config.UI.Theme = themeSelected
		}

		// 2. Save Config
		refreshRateChanged := s.config.UI.RefreshRate != s.baseConfig.UI.RefreshRate
		_ = config.SaveAppConfig(s.config)
		s.baseConfig = s.config

		// 3. Refresh rate can only take effect at program construction time
		// (tea.WithFPS has no live-update API), so it needs a restart rather
		// than the live ConfigChangedMsg sync path used by other settings.
		if refreshRateChanged {
			if tui.IsRestartSafeLocally() {
				tui.RestartForConfigChange(context.Background())
			} else {
				resultChan := make(chan bool, 1)
				go func() {
					if <-resultChan {
						tui.RestartForConfigChange(context.Background())
					}
				}()
				return tui.ShowConfirmDialogMsg{
					Title:      "Restart Required",
					Question:   "Refresh rate changed. You have unsaved changes — restart now to apply it, or keep editing and it'll apply next session?",
					DefaultYes: false,
					ResultChan: resultChan,
				}
			}
		}

		// 4. Trigger synchronized style update
		return displayengine.ConfigChangedMsg{Config: s.config}
	}
}

// handleReset discards every staged change and reverts to baseConfig (the
// settings as of the last Apply, or as loaded on screen entry). Rebuilds all
// three inner menus from scratch via initMenus so their checkbox/radio/dropdown
// states reflect the reverted config, then re-applies focus since initMenus
// only resets the bookkeeping fields, not the new MenuModels' own focus state.
func (s *DisplayOptionsScreen) handleReset() tea.Cmd {
	return func() tea.Msg {
		s.config = s.baseConfig
		s.currentTheme = s.baseConfig.UI.Theme
		s.previewTheme = s.currentTheme
		s.themeChangedFields = nil
		s.themeDefaults[s.currentTheme], _ = theme.Load(s.currentTheme, "Preview")
		s.initMenus()
		s.updateFocusStates()
		return displayengine.ConfigChangedMsg{Config: s.config}
	}
}

func (s *DisplayOptionsScreen) MenuName() string {
	return "appearance"
}

func (s *DisplayOptionsScreen) IsDestructive() bool {
	return false
}

func (s *DisplayOptionsScreen) FocusTitleBar() {
	if s.outerMenu != nil {
		s.outerMenu.FocusTitleBar()
	}
}

func (s *DisplayOptionsScreen) BlurTitleBar() {
	if s.outerMenu != nil {
		s.outerMenu.BlurTitleBar()
	}
}

func (s *DisplayOptionsScreen) TitleBarFocused() bool {
	return s.outerMenu != nil && s.outerMenu.TitleBarFocused()
}

func (s *DisplayOptionsScreen) Init() tea.Cmd {
	return tea.Batch(s.themeMenu.Init(), s.optionsMenu.Init())
}

func (s *DisplayOptionsScreen) AdvanceSpinners(now time.Time) bool {
	a := s.themeMenu.AdvanceSpinners(now)
	b := s.optionsMenu.AdvanceSpinners(now)
	c := s.outerMenu != nil && s.outerMenu.AdvanceSpinners(now)
	return a || b || c
}
