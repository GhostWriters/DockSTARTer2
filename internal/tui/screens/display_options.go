package screens

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// DisplayOptionsFocus defines which area of the screen has focus
type DisplayOptionsFocus int

const (
	FocusThemes DisplayOptionsFocus = iota
	FocusOptions
	FocusButtons
)

// DisplayOptionsScreen allows the user to configure UI settings and themes together.
type DisplayOptionsScreen struct {
	themeMenu    *tui.MenuModel
	optionsMenu  *tui.MenuModel
	focusedPanel DisplayOptionsFocus
	// focusedButton index: 0=Apply, 1=Back (or Exit when isRoot), 2=Exit (only when !isRoot)
	focusedButton int
	isRoot        bool // true when launched directly via -M appearance; hides Back button

	config       config.AppConfig
	themes       []theme.ThemeMetadata
	currentTheme string
	previewTheme string // Theme currently being highlighted in the list

	width  int
	height int

	outerMenu *tui.MenuModel // outer "Appearance Settings" dialog with sections + buttons

	focused bool // tracks global screen focus (header/log panel interaction)

	baseConfig    config.AppConfig                // Original exact config before previewing
	themeDefaults map[string]*theme.ThemeDefaults // Cache parsed defaults
	themeFileCache map[string]theme.ThemeFile     // Cache GetThemeFile results for help text

	connType string // "local", "ssh", or "web"
}

// updateDisplayOptionMsg is sent when an option is changed in the menu
type updateDisplayOptionMsg struct {
	update func(*config.AppConfig)
}

// NewDisplayOptionsScreen creates a new consolidated display options screen.
// isRoot suppresses the Back button when this screen is the entry point.
func NewDisplayOptionsScreen(isRoot bool, connType string) *DisplayOptionsScreen {
	themes, _ := theme.List()
	cfg := config.LoadAppConfig()
	current := cfg.UI.Theme // ConfigValue e.g. "DockSTARTer" or "user:MyTheme"

	s := &DisplayOptionsScreen{
		isRoot:         isRoot,
		connType:       connType,
		config:         cfg,
		baseConfig:     cfg,
		themes:         themes,
		currentTheme:   current,
		previewTheme:   current,
		themeDefaults:  make(map[string]*theme.ThemeDefaults),
		themeFileCache: make(map[string]theme.ThemeFile),
	}
	s.themeDefaults[current], _ = theme.Load(current, "Preview")

	s.initMenus()
	s.focused = true // Default to focused initially
	return s
}

func (s *DisplayOptionsScreen) initMenus() {
	// 1. Theme Selection Menu
	themeItems := make([]tui.MenuItem, len(s.themes))
	foundCurrent := false
	for i, t := range s.themes {
		desc := t.Description
		if t.Author != "" {
			desc += fmt.Sprintf(" [by %s]", t.Author)
		}
		descTag := "{{|ListTheme|}}"
		if t.IsUserTheme {
			descTag = "{{|ListThemeUserDefined|}}"
		}
		checked := s.currentTheme == t.ConfigValue
		if checked {
			foundCurrent = true
		}
		themeItems[i] = tui.MenuItem{
			Tag:           t.Name,
			Desc:          descTag + desc,
			Help:          desc,
			IsRadioButton: true,
			Checked:       checked,
			IsInvalid:     t.IsInvalid,
			IsUserDefined: t.IsUserTheme,
			IsDestructive: true,
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
		themeItems = append([]tui.MenuItem{{
			Tag:           displayName,
			Desc:          "{{|ListThemeUserDefined|}}Source file not found — using cached version",
			Help:          "Theme source file is missing. The cached version remains active until you choose another theme.",
			IsRadioButton: true,
			Checked:       true,
			IsUserDefined: true,
			Metadata:      map[string]string{"config_value": s.currentTheme},
		}}, themeItems...)
	}

	themeMenu := tui.NewMenuModel(tui.IDThemePanel, "Select Theme", "", themeItems, nil)
	s.themeMenu = themeMenu
	s.themeMenu.SetHelpItemPrefix("Theme")
	s.themeMenu.SetItemHelpFunc(s.buildThemeItemHelp)
	s.themeMenu.SetHelpPageText("Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.")
	s.themeMenu.SetSubMenuMode(true)
	s.themeMenu.SetVariableHeight(false)
	s.themeMenu.SetIsDialog(false) // Part of a screen, not a modal
	s.themeMenu.SetShowExit(false)
	s.themeMenu.SetMaximized(true) // Fill available width
	s.themeMenu.SetShowLockGutter(false)

	// 2. Options Menu
	optionItems := []tui.MenuItem{
		{
			Tag:         "Borders",
			Desc:        "Show borders on all dialogs",
			Help:        "Toggle border visibility (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.Borders,
			Selectable:  true,
			SpaceAction: s.toggleBorders(),
			IsDestructive: true,
		},
		{
			Tag:         "Button Borders",
			Desc:        "Show borders on buttons",
			Help:        "Toggle bordered vs flat button style (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.ButtonBorders,
			Selectable:  true,
			SpaceAction: s.toggleButtonBorders(),
			IsDestructive: true,
		},
		{
			Tag:         "Line Characters",
			Desc:        "Use unicode line drawing characters",
			Help:        "Use ┌─ instead of +- for borders (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.LineCharacters,
			Selectable:  true,
			SpaceAction: s.toggleLineChars(),
			IsDestructive: true,
		},
		{
			Tag:         "Shadow",
			Desc:        "Enable drop shadows",
			Help:        "Toggle drop shadow effect (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.Shadow,
			Selectable:  true,
			SpaceAction: s.toggleShadow(),
			IsDestructive: true,
		},
		{
			Tag:         "Scrollbar",
			Desc:        "Show scrollbar in lists",
			Help:        "Toggle scrollbar in scrollable lists (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.UI.Scrollbar,
			Selectable:  true,
			SpaceAction: s.toggleScrollbar(),
			IsDestructive: true,
		},
		{
			Tag:    "Shadow Level",
			Desc:   s.dropdownDesc(s.shadowLevelToDesc(s.config.UI.ShadowLevel)),
			Help:   "Adjust the density of the shadow (Select/Enter for list)",
			Action: s.showShadowDropdown(),
			IsDestructive: true,
		},
		{
			Tag:    "Border Color",
			Desc:   s.dropdownDesc(s.borderColorToDesc(s.config.UI.BorderColor)),
			Help:   "Choose theme colors for borders (Select/Enter for list)",
			Action: s.showBorderColorDropdown(),
			IsDestructive: true,
		},
		{
			Tag:  "Dialog Title",
			Desc: s.dropdownDesc(titleAlignDesc(s.config.UI.DialogTitleAlign)),
			Help: "Alignment of titles in dialog borders (Enter for options)",
			Action: s.showTitleAlignDropdown("dialog_title_align", "Dialog Title Align",
				func() string { return s.config.UI.DialogTitleAlign },
				func(cfg *config.AppConfig, v string) { cfg.UI.DialogTitleAlign = v }),
			IsDestructive: true,
		},
		{
			Tag:  "Submenu Title",
			Desc: s.dropdownDesc(titleAlignDesc(s.config.UI.SubmenuTitleAlign)),
			Help: "Alignment of subtitle rows inside menus (Enter for options)",
			Action: s.showTitleAlignDropdown("submenu_title_align", "Submenu Title Align",
				func() string { return s.config.UI.SubmenuTitleAlign },
				func(cfg *config.AppConfig, v string) { cfg.UI.SubmenuTitleAlign = v }),
			IsDestructive: true,
		},
		{
			Tag:  "Log Title",
			Desc: s.dropdownDesc(titleAlignDesc(s.config.UI.LogTitleAlign)),
			Help: "Alignment of the log panel strip label (Enter for options)",
			Action: s.showTitleAlignDropdown("log_title_align", "Log Title Align",
				func() string { return s.config.UI.LogTitleAlign },
				func(cfg *config.AppConfig, v string) { cfg.UI.LogTitleAlign = v }),
			IsDestructive: true,
		},
	}

	if s.connType == "local" {
		optionItems = append(optionItems, tui.MenuItem{
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
		optionItems = append(optionItems, tui.MenuItem{
			Tag:           label,
			Desc:          s.dropdownDesc(s.panelModeToDesc(s.config.UI.PanelRemote)),
			Help:          "Choose the panel mode for SSH and Web sessions (Console restricted).",
			Action:        s.showPanelDropdown(false),
			IsDestructive: true,
		})
	}

	optionsMenu := tui.NewMenuModel(tui.IDOptionsPanel, "Options", "", optionItems, nil)
	s.optionsMenu = optionsMenu
	s.optionsMenu.SetHelpItemPrefix("Option")
	s.optionsMenu.SetHelpPageText("Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.")
	s.optionsMenu.SetSubMenuMode(true)
	s.optionsMenu.SetIsDialog(false) // Part of a screen, not a modal
	s.optionsMenu.SetShowExit(false)
	s.optionsMenu.SetFlowMode(true)
	s.optionsMenu.SetMaximized(true) // Fill available width
	s.optionsMenu.SetShowLockGutter(false)

	// 3. Outer "Appearance Settings" dialog (sections container + buttons)
	var outerBack tea.Cmd
	if !s.isRoot {
		outerBack = navigateBack()
	}
	outerMenu := tui.NewMenuModel("appearance_outer", "Appearance Settings", "", nil, outerBack)
	outerMenu.SetShowExit(true)
	outerMenu.SetButtonLabels("Apply", "Back", "Exit")
	outerMenu.AddContentSection(themeMenu)
	outerMenu.AddContentSection(optionsMenu)
	s.outerMenu = outerMenu

	// Initial Focus
	s.focusedPanel = FocusThemes
	s.updateFocusStates()
}

// maxFocusedButton returns the highest valid focusedButton index.
// When isRoot there is no Back button: Apply=0, Exit=1 (two buttons).
// Otherwise: Apply=0, Back=1, Exit=2 (three buttons).
func (s *DisplayOptionsScreen) maxFocusedButton() int {
	if s.isRoot {
		return 1
	}
	return 2
}

// execFocusedButton runs the action for the current focusedButton index.
func (s *DisplayOptionsScreen) execFocusedButton() (tea.Model, tea.Cmd) {
	switch s.focusedButton {
	case 0:
		return s, s.handleApply()
	case 1:
		if s.isRoot {
			theme.Unload("Preview")
			return s, tui.ConfirmExitAction()
		}
		theme.Unload("Preview")
		return s, navigateBack()
	case 2:
		theme.Unload("Preview")
		return s, tui.ConfirmExitAction()
	}
	return s, nil
}

func (s *DisplayOptionsScreen) updateFocusStates() {
	s.themeMenu.SetSubFocused(s.focused && s.focusedPanel == FocusThemes)
	s.optionsMenu.SetSubFocused(s.focused && s.focusedPanel == FocusOptions)

	if s.outerMenu == nil {
		return
	}
	s.outerMenu.SetFocused(s.focused)
	if s.focusedPanel == FocusButtons {
		switch s.focusedButton {
		case 0:
			s.outerMenu.SetFocusedItem(tui.FocusSelectBtn)
		case 1:
			if s.isRoot {
				s.outerMenu.SetFocusedItem(tui.FocusExitBtn)
			} else {
				s.outerMenu.SetFocusedItem(tui.FocusBackBtn)
			}
		case 2:
			s.outerMenu.SetFocusedItem(tui.FocusExitBtn)
		}
	} else {
		s.outerMenu.SetFocusedItem(tui.FocusList)
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
	if d.ButtonBorders != nil {
		lines = append(lines, fmt.Sprintf("  Button Borders: %s", boolStr(*d.ButtonBorders)))
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
	if d.BorderColor != nil {
		lines = append(lines, fmt.Sprintf("  Border Color: %d", *d.BorderColor))
	}
	if d.DialogTitleAlign != nil {
		lines = append(lines, fmt.Sprintf("  Dialog Title: %s", *d.DialogTitleAlign))
	}
	if d.SubmenuTitleAlign != nil {
		lines = append(lines, fmt.Sprintf("  Submenu Title: %s", *d.SubmenuTitleAlign))
	}
	if d.LogTitleAlign != nil {
		lines = append(lines, fmt.Sprintf("  Log Title: %s", *d.LogTitleAlign))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Defaults applied by this theme:\n" + strings.Join(lines, "\n")
}

// buildThemeItemHelp returns enriched (itemTitle, itemText) for a theme menu item.
// Used by itemHelpFunc (right-click) and HelpContext (F1).
func (s *DisplayOptionsScreen) buildThemeItemHelp(item tui.MenuItem) (itemTitle, itemText string) {
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
	if defaultsText := formatThemeDefaults(tf.Defaults); defaultsText != "" {
		parts = append(parts, defaultsText)
	}
	if len(parts) == 0 {
		return "", ""
	}
	return item.Tag, strings.Join(parts, "\n\n")
}

// HelpContext implements tui.HelpContextProvider.
func (s *DisplayOptionsScreen) HelpContext(maxWidth int) tui.HelpContext {
	screenName := s.outerMenu.Title()
	pageText := "Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options."

	var inner tui.HelpContext
	switch s.focusedPanel {
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
			"({{|OptionValue|}} )",
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
		items := []tui.MenuItem{
			{Tag: "Left", Help: "Align title to the left", Action: s.titleAlignAction(apply, "left")},
			{Tag: "Center", Help: "Center the title", Action: s.titleAlignAction(apply, "center")},
		}
		menu := tui.NewMenuModel(menuName, label, "Select alignment", items, tui.CloseDialog())
		menu.SetShowExit(false)
		if current == "left" {
			menu.Select(0)
		} else {
			menu.Select(1)
		}
		return tui.ShowDialogMsg{Dialog: menu}
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
				return tui.ShowDialogMsg{Dialog: confirm}
			}
		}

		var items []tui.MenuItem

		// None option: always available
		items = append(items, tui.MenuItem{
			Tag:  "None",
			Desc: "Hide the panel entirely",
			Help: "Removes the panel and stretches content to the bottom of the screen.",
			Action: func() tea.Msg { return confirmChange("none")() },
		})

		// Log option: always available
		items = append(items, tui.MenuItem{
			Tag:  "Log",
			Desc: "Show read-only log viewer",
			Help: "Displays application logs but hides the command input bar.",
			Action: func() tea.Msg { return confirmChange("log")() },
		})

		// Console (ds2-only): always available for both local and remote —
		// it only accepts ds2 subcommands so it is safe in all session types.
		items = append(items, tui.MenuItem{
			Tag:  "Console",
			Desc: "ds2 commands only",
			Help: "Accepts ds2 subcommands only. Safe for remote sessions.",
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
				return tui.ShowDialogMsg{Dialog: confirm}
			}
			return applyChange("system")()
		}
		items = append(items, tui.MenuItem{
			Tag:  "System Console",
			Desc: "Full shell access",
			Help: "Passes commands directly to the OS shell. Use with caution for remote sessions.",
			Action: systemAction,
		})


		title := "Remote Panel Mode"
		if isLocalSetting {
			title = "Local Panel Mode"
		}
		menu := tui.NewMenuModel("panel_dropdown", title, "Choose layout", items, tui.CloseDialog())
		menu.SetShowExit(false)

		// Set initial selection — "system" maps to tag "System Console"
		current := strings.ToLower(currentMode)
		for i, item := range items {
			if strings.ToLower(item.Tag) == current {
				menu.Select(i)
				break
			}
		}

		return tui.ShowDialogMsg{Dialog: menu}
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
				{"Solid", "{{|OptionValue|}}( ){{[-]}}"},
			}
		}
		var items []tui.MenuItem
		for i, e := range entries {
			level := i
			items = append(items, tui.MenuItem{
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
		menu := tui.NewMenuModel("shadow_dropdown", "Shadow Level", "Select shadow fill pattern", items, tui.CloseDialog())
		menu.SetShowExit(false)
		menu.Select(s.config.UI.ShadowLevel)
		return tui.ShowDialogMsg{Dialog: menu}
	}
}

func (s *DisplayOptionsScreen) showBorderColorDropdown() tea.Cmd {
	return func() tea.Msg {
		type borderEntry struct {
			mode        int
			label, value string
		}
		entries := []borderEntry{
			{1, "Border 1 (Theme Focus)", "{{|OptionValue|}}(1){{[-]}}"},
			{2, "Border 2 (Theme Accent)", "{{|OptionValue|}}(2){{[-]}}"},
			{3, "Both (3D Effect)", "{{|OptionValue|}}(3D){{[-]}}"},
		}
		var items []tui.MenuItem
		for _, e := range entries {
			mode := e.mode
			items = append(items, tui.MenuItem{
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
		menu := tui.NewMenuModel("border_dropdown", "Border Coloring", "Select which theme colors highlight borders", items, tui.CloseDialog())
		menu.SetShowExit(false)
		menu.Select(s.config.UI.BorderColor - 1)
		return tui.ShowDialogMsg{Dialog: menu}
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

func (s *DisplayOptionsScreen) toggleButtonBorders() tea.Cmd {
	return func() tea.Msg {
		newState := !s.config.UI.ButtonBorders
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			cfg.UI.ButtonBorders = newState
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

func (s *DisplayOptionsScreen) handleApply() tea.Cmd {
	return func() tea.Msg {
		// Do not apply if any destructive settings are locked
		if s.optionsMenu.AnyLocked() || s.themeMenu.AnyLocked() {
			return nil
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
		_ = config.SaveAppConfig(s.config)
		s.baseConfig = s.config

		// 3. Trigger synchronized style update
		return tui.ConfigChangedMsg{Config: s.config}
	}
}

func (s *DisplayOptionsScreen) MenuName() string {
	return "appearance"
}

func (s *DisplayOptionsScreen) IsDestructive() bool {
	return false
}

func (s *DisplayOptionsScreen) Init() tea.Cmd {
	return tea.Batch(s.themeMenu.Init(), s.optionsMenu.Init())
}
