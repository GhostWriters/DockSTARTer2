package screens

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
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

	focused bool // tracks global screen focus (header/log panel interaction)

	baseConfig    config.AppConfig                // Original exact config before previewing
	themeDefaults map[string]*theme.ThemeDefaults // Cache parsed defaults
}

// updateDisplayOptionMsg is sent when an option is changed in the menu
type updateDisplayOptionMsg struct {
	update func(*config.AppConfig)
}

// NewDisplayOptionsScreen creates a new consolidated display options screen.
// NewDisplayOptionsScreen creates a new consolidated display options screen.
// isRoot suppresses the Back button when this screen is the entry point.
func NewDisplayOptionsScreen(isRoot bool) *DisplayOptionsScreen {
	themes, _ := theme.List()
	current := theme.Current.Name

	s := &DisplayOptionsScreen{
		isRoot:        isRoot,
		config:        config.LoadAppConfig(),
		baseConfig:    config.LoadAppConfig(),
		themes:        themes,
		currentTheme:  current,
		previewTheme:  current,
		themeDefaults: make(map[string]*theme.ThemeDefaults),
	}
	s.themeDefaults[current], _ = theme.Load(current, "Preview")

	s.initMenus()
	s.focused = true // Default to focused initially
	return s
}

func (s *DisplayOptionsScreen) initMenus() {
	// 1. Theme Selection Menu
	themeItems := make([]tui.MenuItem, len(s.themes))
	for i, t := range s.themes {
		desc := t.Description
		if t.Author != "" {
			desc += fmt.Sprintf(" [by %s]", t.Author)
		}
		themeItems[i] = tui.MenuItem{
			Tag:           t.Name,
			Desc:          "{{|Theme_ListTheme|}}" + desc,
			Help:          desc,
			IsRadioButton: true,
			Checked:       s.currentTheme == t.Name,
		}
	}

	themeMenu := tui.NewMenuModel("theme_list", "Select Theme", "", themeItems, nil)
	s.themeMenu = &themeMenu
	s.themeMenu.SetSubMenuMode(true)
	s.themeMenu.SetShowExit(false)
	s.themeMenu.SetMaximized(true) // Fill available width

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
			Tag:         "Shadow",
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
			Tag:  "Log Title",
			Desc: s.dropdownDesc(titleAlignDesc(s.config.UI.LogTitleAlign)),
			Help: "Alignment of the log panel strip label (Enter for options)",
			Action: s.showTitleAlignDropdown("log_title_align", "Log Title Align",
				func() string { return s.config.UI.LogTitleAlign },
				func(cfg *config.AppConfig, v string) { cfg.UI.LogTitleAlign = v }),
		},
	}

	optionsMenu := tui.NewMenuModel("options_list", "Options", "", optionItems, nil)
	s.optionsMenu = &optionsMenu
	s.optionsMenu.SetSubMenuMode(true)
	s.optionsMenu.SetShowExit(false)
	s.optionsMenu.SetFlowMode(true)
	s.optionsMenu.SetMaximized(true) // Fill available width

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
}

// SetFocused updates the global focus state for this screen
func (s *DisplayOptionsScreen) SetFocused(f bool) {
	s.focused = f
	s.updateFocusStates()
}

func (s *DisplayOptionsScreen) shadowLevelToDesc(l int) string {
	var levels []string
	if s.config.UI.LineCharacters {
		levels = []string{"(Off)", "(░)", "(▒)", "(▓)", "(█)"}
	} else {
		levels = []string{
			"(Off)",
			"({{|Theme_Shadow|}}.{{|Theme_OptionValue|}})",
			"({{|Theme_Shadow|}}:{{|Theme_OptionValue|}})",
			"({{|Theme_Shadow|}}#{{|Theme_OptionValue|}})",
			"({{|Theme_OptionValue|}} )",
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
	return fmt.Sprintf("{{|Theme_OptionValue|}}%s▼{{[-]}}", val)
}

func titleAlignDesc(v string) string {
	if v == "left" {
		return "Left"
	}
	return "Center"
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
		return tui.ShowDialogMsg{Dialog: &menu}
	}
}

func (s *DisplayOptionsScreen) showShadowDropdown() tea.Cmd {
	return func() tea.Msg {
		var levels []string
		if s.config.UI.LineCharacters {
			levels = []string{
				"Off",
				"Light {{|Theme_OptionValue|}}(░){{[-]}}",
				"Medium {{|Theme_OptionValue|}}(▒){{[-]}}",
				"Dark {{|Theme_OptionValue|}}(▓){{[-]}}",
				"Solid {{|Theme_OptionValue|}}(█){{[-]}}",
			}
		} else {
			levels = []string{
				"Off",
				"Light {{|Theme_OptionValue|}}({{|Theme_Shadow|}}.{{|Theme_OptionValue|}}){{[-]}}",
				"Medium {{|Theme_OptionValue|}}({{|Theme_Shadow|}}:{{|Theme_OptionValue|}}){{[-]}}",
				"Dark {{|Theme_OptionValue|}}({{|Theme_Shadow|}}#{{|Theme_OptionValue|}}){{[-]}}",
				"Solid {{|Theme_OptionValue|}}( ){{[-]}}",
			}
		}
		var items []tui.MenuItem
		for i, label := range levels {
			level := i
			items = append(items, tui.MenuItem{
				Tag:  label,
				Help: fmt.Sprintf("Set shadow to %s", label),
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
		return tui.ShowDialogMsg{Dialog: &menu}
	}
}

func (s *DisplayOptionsScreen) showBorderColorDropdown() tea.Cmd {
	return func() tea.Msg {
		modes := []int{1, 2, 3}
		labels := map[int]string{
			1: "Border 1 (Theme Focus) {{|Theme_OptionValue|}}(1){{[-]}}",
			2: "Border 2 (Theme Accent) {{|Theme_OptionValue|}}(2){{[-]}}",
			3: "Both (3D Effect) {{|Theme_OptionValue|}}(3D){{[-]}}",
		}
		var items []tui.MenuItem
		for _, m := range modes {
			mode := m
			items = append(items, tui.MenuItem{
				Tag:  labels[mode],
				Help: fmt.Sprintf("Set border coloring to %s", labels[mode]),
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
		return tui.ShowDialogMsg{Dialog: &menu}
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

func (s *DisplayOptionsScreen) handleApply() tea.Cmd {
	return func() tea.Msg {
		// 1. Apply Theme (Find the actually checked radio option)
		themeSelected := s.previewTheme
		for _, item := range s.themeMenu.GetItems() {
			if item.Checked {
				themeSelected = item.Tag
				break
			}
		}

		_, err := theme.Load(themeSelected, "")
		if err == nil {
			s.currentTheme = themeSelected
			s.config.UI.Theme = themeSelected
		}

		// 2. Save Config
		config.SaveAppConfig(s.config)

		// 3. Close the screen (navigate back) and trigger synchronized style update
		return tui.ConfigChangedMsg{Config: s.config}
	}
}

func (s *DisplayOptionsScreen) Init() tea.Cmd {
	return tea.Batch(s.themeMenu.Init(), s.optionsMenu.Init())
}

func (s *DisplayOptionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case tea.MouseWheelMsg:
		// ONLY interact with the focused panel, no mouse-over fallback
		if s.focusedPanel == FocusThemes {
			updated, uCmd := s.themeMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.themeMenu = m
			}
			return s, uCmd
		} else if s.focusedPanel == FocusOptions {
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		} else if s.focusedPanel == FocusButtons {
			// Scroll wheel cycles the focused button (up=left, down=right) — clamps, no wrap.
			maxBtn := s.maxFocusedButton()
			if msg.Button == tea.MouseWheelUp {
				if s.focusedButton > 0 {
					s.focusedButton--
				}
			} else if msg.Button == tea.MouseWheelDown {
				if s.focusedButton < maxBtn {
					s.focusedButton++
				}
			}
			return s, nil
		}
		return s, nil

	case tui.LayerHitMsg:
		// 1. Focus routing via panel hit
		if msg.ID == tui.IDThemePanel {
			s.focusedPanel = FocusThemes
			s.updateFocusStates()
			return s, nil
		} else if msg.ID == tui.IDOptionsPanel {
			s.focusedPanel = FocusOptions
			s.updateFocusStates()
			return s, nil
		} else if msg.ID == tui.IDButtonPanel {
			s.focusedPanel = FocusButtons
			s.updateFocusStates()
			return s, nil
		}

		// 2. Component routing (menus)
		if strings.HasPrefix(msg.ID, "item-theme_list-") {
			// Theme selection logic
			s.focusedPanel = FocusThemes
			s.updateFocusStates()

			// Extract index
			var idx int
			fmt.Sscanf(msg.ID, "item-theme_list-%d", &idx)
			items := s.themeMenu.GetItems()
			if idx >= 0 && idx < len(items) {
				for j := range items {
					items[j].Checked = (idx == j)
				}
				s.themeMenu.SetItems(items)
				s.themeMenu.Select(idx)
				s.applyPreview(items[idx].Tag)
			}
			return s, nil
		} else if strings.HasPrefix(msg.ID, "item-options_list-") {
			s.focusedPanel = FocusOptions
			s.updateFocusStates()

			// Forward to options menu
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		}

		// 3. Button actions
		switch msg.ID {
		case tui.IDApplyButton:
			s.focusedButton = 0
			return s, s.handleApply()
		case tui.IDBackButton:
			if s.isRoot {
				return s, nil // Back is not shown when root; ignore stale hits
			}
			s.focusedButton = 1
			theme.Unload("Preview")
			return s, navigateBack()
		case tui.IDExitButton:
			s.focusedButton = s.maxFocusedButton()
			theme.Unload("Preview")
			return s, tui.ConfirmExitAction()
		}

	case tui.ToggleFocusedMsg:
		// Middle click: activate the currently focused item in the hovered panel
		if s.focusedPanel == FocusThemes {
			items := s.themeMenu.GetItems()
			cursor := s.themeMenu.Index()
			if cursor >= 0 && cursor < len(items) {
				for i := range items {
					items[i].Checked = (i == cursor)
				}
				s.themeMenu.SetItems(items)
				s.applyPreview(items[cursor].Tag)
			}
			return s, nil
		} else if s.focusedPanel == FocusOptions {
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		} else if s.focusedPanel == FocusButtons {
			return s.execFocusedButton()
		}
		return s, nil

	case tea.KeyPressMsg:
		// 1. Panel Cycling (Tab / Shift-Tab) - Themes <-> Options only
		if key.Matches(msg, tui.Keys.CycleTab) || key.Matches(msg, tui.Keys.CycleShiftTab) {
			if s.focusedPanel == FocusThemes {
				s.focusedPanel = FocusOptions
			} else {
				s.focusedPanel = FocusThemes
			}
			s.updateFocusStates()
			return s, nil
		}

		// 2. Strict Navigation (Workstation Model)

		// Left/Right: Cycle buttons globally
		if key.Matches(msg, tui.Keys.Left) {
			s.focusedButton--
			if s.focusedButton < 0 {
				s.focusedButton = s.maxFocusedButton()
			}
			return s, nil
		}
		if key.Matches(msg, tui.Keys.Right) {
			s.focusedButton++
			if s.focusedButton > s.maxFocusedButton() {
				s.focusedButton = 0
			}
			return s, nil
		}

		if key.Matches(msg, tui.Keys.Enter) {
			return s.execFocusedButton()
		}

		// Esc: Cancel — navigate back or quit if root
		if key.Matches(msg, tui.Keys.Esc) {
			theme.Unload("Preview")
			if s.isRoot {
				return s, tui.ConfirmExitAction()
			}
			return s, navigateBack()
		}

		// 3. Up/Down/Space: Routed to focused panel
		if s.focusedPanel == FocusThemes {
			// Specific radio logic for Space on theme list
			if key.Matches(msg, tui.Keys.Space) {
				items := s.themeMenu.GetItems()
				cursor := s.themeMenu.Index()
				if cursor >= 0 && cursor < len(items) {
					for i := range items {
						items[i].Checked = (i == cursor)
					}
					s.themeMenu.SetItems(items)
					s.applyPreview(items[cursor].Tag)
					return s, nil
				}
			}

			updated, uCmd := s.themeMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.themeMenu = m
			}
			return s, uCmd
		} else if s.focusedPanel == FocusOptions {
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		}

	case updateDisplayOptionMsg:
		msg.update(&s.config)
		msg.update(&s.baseConfig) // User actively changed an option, save it to base config
		s.syncOptionsMenu()
		return s, nil
	}

	return s, cmd
}

func (s *DisplayOptionsScreen) applyPreview(themeName string) {
	s.previewTheme = themeName

	// Reset to base configs
	s.config = s.baseConfig

	// Always load to ensure tags are registered in registry
	defaults, err := theme.Load(themeName, "Preview")
	if err != nil {
		s.previewTheme = "ERR: " + err.Error()
	}
	s.themeDefaults[themeName] = defaults

	if defaults != nil {
		theme.ApplyThemeDefaults(&s.config, *defaults)
	}
	s.syncOptionsMenu()
	tui.ClearSemanticCachePrefix("Preview_Theme_")
}

func (s *DisplayOptionsScreen) syncOptionsMenu() {
	items := s.optionsMenu.GetItems()
	items[0].Checked = s.config.UI.Borders
	items[1].Checked = s.config.UI.LineCharacters
	items[2].Checked = s.config.UI.Shadow
	// Update dropdown descriptions
	items[3].Desc = s.dropdownDesc(s.shadowLevelToDesc(s.config.UI.ShadowLevel))
	items[4].Desc = s.dropdownDesc(s.borderColorToDesc(s.config.UI.BorderColor))
	items[5].Desc = s.dropdownDesc(titleAlignDesc(s.config.UI.DialogTitleAlign))
	items[6].Desc = s.dropdownDesc(titleAlignDesc(s.config.UI.SubmenuTitleAlign))
	items[7].Desc = s.dropdownDesc(titleAlignDesc(s.config.UI.LogTitleAlign))
	s.optionsMenu.SetItems(items)
}

func (s *DisplayOptionsScreen) Title() string {
	return "Display Options"
}

func (s *DisplayOptionsScreen) HelpText() string {
	if s.themeMenu == nil || s.optionsMenu == nil {
		return ""
	}
	if s.focusedPanel == FocusThemes {
		return s.themeMenu.HelpText()
	}
	if s.focusedPanel == FocusOptions {
		return s.optionsMenu.HelpText()
	}
	return "Tab to cycle panels, Enter to Apply, Esc to Cancel"
}

func (s *DisplayOptionsScreen) SetSize(width, height int) {
	s.width = width
	s.height = height

	if s.optionsMenu == nil || s.themeMenu == nil {
		return
	}

	layout := tui.GetLayout()

	previewMinWidth := 48
	minDialogWidth := 44 + layout.BorderWidth()
	previewFits := width >= minDialogWidth+layout.GutterWidth+previewMinWidth

	var dialogContentWidth int
	if previewFits {
		dialogContentWidth = width - layout.GutterWidth - previewMinWidth
	} else {
		dialogContentWidth = width
	}

	menuWidth := dialogContentWidth - layout.BorderWidth()
	if menuWidth < 40 {
		menuWidth = 40
	}

	hasShadow := tui.IsShadowEnabled()
	optionsContentHeight := layout.DialogContentHeight(height, 0, true, hasShadow)
	overhead := height - optionsContentHeight

	optionsFlowLines := s.optionsMenu.GetFlowHeight(menuWidth)
	optionsHeight := optionsFlowLines + layout.BorderHeight()

	themeHeight := s.height - optionsHeight - overhead
	if themeHeight < 4 {
		themeHeight = 4
	}

	s.themeMenu.SetSize(menuWidth, themeHeight)
	s.optionsMenu.SetSize(menuWidth, optionsHeight)
}

func (s *DisplayOptionsScreen) IsMaximized() bool {
	return true
}

func (s *DisplayOptionsScreen) HasDialog() bool {
	if s.themeMenu == nil || s.optionsMenu == nil {
		return false
	}
	return s.themeMenu.HasDialog() || s.optionsMenu.HasDialog()
}

func (s *DisplayOptionsScreen) MenuName() string {
	return "appearance"
}
