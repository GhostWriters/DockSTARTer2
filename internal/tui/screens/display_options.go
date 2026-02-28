package screens

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	themeMenu     *tui.MenuModel
	optionsMenu   *tui.MenuModel
	focusedPanel  DisplayOptionsFocus
	focusedButton int // 0=Apply, 1=Cancel

	config       config.AppConfig
	themes       []theme.ThemeMetadata
	currentTheme string
	previewTheme string // Theme currently being highlighted in the list

	width  int
	height int

	baseConfig    config.AppConfig                // Original exact config before previewing
	themeDefaults map[string]*theme.ThemeDefaults // Cache parsed defaults
}

// updateDisplayOptionMsg is sent when an option is changed in the menu
type updateDisplayOptionMsg struct {
	update func(*config.AppConfig)
}

// NewDisplayOptionsScreen creates a new consolidated display options screen.
func NewDisplayOptionsScreen() *DisplayOptionsScreen {
	themes, _ := theme.List()
	current := theme.Current.Name

	s := &DisplayOptionsScreen{
		config:        config.LoadAppConfig(),
		baseConfig:    config.LoadAppConfig(),
		themes:        themes,
		currentTheme:  current,
		previewTheme:  current,
		themeDefaults: make(map[string]*theme.ThemeDefaults),
	}
	s.themeDefaults[current], _ = theme.Load(current, "Preview")

	s.initMenus()
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

func (s *DisplayOptionsScreen) updateFocusStates() {
	s.themeMenu.SetSubFocused(s.focusedPanel == FocusThemes)
	s.optionsMenu.SetSubFocused(s.focusedPanel == FocusOptions)
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
			if msg.Button == tea.MouseWheelUp {
				if s.focusedButton > 0 {
					s.focusedButton--
				}
			} else if msg.Button == tea.MouseWheelDown {
				if s.focusedButton < 2 {
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
			s.focusedButton = 1
			return s, navigateBack()
		case tui.IDExitButton:
			s.focusedButton = 2
			return s, tea.Quit
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
			// Activate the currently focused button
			switch s.focusedButton {
			case 0:
				return s, s.handleApply()
			case 1:
				theme.Unload("Preview")
				return s, navigateBack()
			case 2:
				theme.Unload("Preview")
				return s, tea.Quit
			}
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
				s.focusedButton = 2
			}
			return s, nil
		}
		if key.Matches(msg, tui.Keys.Right) {
			s.focusedButton++
			if s.focusedButton > 2 {
				s.focusedButton = 0
			}
			return s, nil
		}

		if key.Matches(msg, tui.Keys.Enter) {
			switch s.focusedButton {
			case 0:
				return s, s.handleApply()
			case 1:
				theme.Unload("Preview")
				return s, navigateBack()
			case 2:
				theme.Unload("Preview")
				return s, tea.Quit
			}
		}

		// Esc: Cancel
		if key.Matches(msg, tui.Keys.Esc) {
			theme.Unload("Preview")
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
	s.optionsMenu.SetItems(items)
}

func (s *DisplayOptionsScreen) ViewString() (result string) {
	defer func() {
		if r := recover(); r != nil {
			result = "(rendering error — theme may still be loading)"
		}
	}()
	if s.optionsMenu == nil || s.themeMenu == nil {
		return ""
	}
	layout := tui.GetLayout()

	// s.width and s.height are already the content area from layout.ContentArea()
	// which has already subtracted shadow space. Dialog body fits here,
	// shadow extends past into edge indent area.

	// If dimensions not yet set, use terminal dimensions as fallback
	// This handles the initial render before WindowSizeMsg arrives
	width, height := s.width, s.height
	if width == 0 || height == 0 {
		termW, termH, _ := console.GetTerminalSize()
		if termW > 0 && termH > 0 {
			// Apply content area calculation
			hasShadow := s.config.UI.Shadow
			header := tui.NewHeaderModel()
			header.SetWidth(termW - 2)
			headerH := header.Height()
			width, height = layout.ContentArea(termW, termH, hasShadow, headerH)
		}
	}

	previewMinWidth := 48
	minDialogWidth := 44 + layout.BorderWidth()
	// Preview fits if: dialog + gutter + preview fits in content area
	// (shadow extends past content area, not counted here)
	previewFits := width >= minDialogWidth+layout.GutterWidth+previewMinWidth

	// Calculate dialog content width - use local width variable consistently
	// (which may have been set from terminal size fallback)
	var dialogContentWidth int
	if previewFits {
		// Dialog shares space with preview
		dialogContentWidth = width - layout.GutterWidth - previewMinWidth
	} else {
		// Maximized: dialog fills entire content area
		dialogContentWidth = width
	}

	// Menu width = dialog content - outer dialog borders
	menuWidth := dialogContentWidth - layout.BorderWidth()
	if menuWidth < 40 {
		menuWidth = 40
	}

	// Calculate menu widths (same logic as SetSize)
	// These are local and don't modify state, so fine for View pass
	menuWidth = dialogContentWidth - layout.BorderWidth()
	if menuWidth < 40 {
		menuWidth = 40
	}

	// 1. Render Settings Menus
	themeView := s.themeMenu.ViewString()
	optionsView := s.optionsMenu.ViewString()

	// Trim newlines before joining to prevent extra gaps
	leftColumnParts := []string{themeView, optionsView}
	for i, p := range leftColumnParts {
		leftColumnParts[i] = strings.TrimRight(p, "\n")
	}
	leftColumn := lipgloss.JoinVertical(lipgloss.Left, leftColumnParts...)

	// 2. Render Buttons using known width, not measured
	buttons := []tui.ButtonSpec{
		{Text: "Apply", Active: s.focusedButton == 0},
		{Text: "Back", Active: s.focusedButton == 1},
		{Text: "Exit", Active: s.focusedButton == 2},
	}
	buttonRow := tui.RenderCenteredButtons(menuWidth, buttons...)

	// 3. Settings Dialog with known width
	settingsContent := lipgloss.JoinVertical(lipgloss.Left, leftColumn, buttonRow)

	// Target height = content area height (shadow extends past, not counted)
	// Use local height variable for consistency with width
	targetHeight := height
	if targetHeight < 10 {
		targetHeight = 10
	}

	// Use RenderBorderedBoxCtx with known width instead of RenderDialog which measures content
	// targetHeight uses width/height which are local copies of the intended layout
	settingsDialog := tui.RenderBorderedBoxCtx("Appearance Settings", settingsContent, menuWidth, s.height, true, false, tui.GetActiveContext())

	// Add shadow if enabled in global config (not preview config)
	// The preview mockup shows what shadow would look like, but the
	// settings dialog itself uses the current active setting
	if tui.IsShadowEnabled() {
		settingsDialog = tui.AddShadow(settingsDialog)
	}

	// If preview doesn't fit, just return the settings dialog
	if !previewFits {
		return settingsDialog
	}

	// Calculate settings height for preview to match
	styles := tui.GetStyles()
	settingsHeight := lipgloss.Height(settingsDialog)

	// 4. Render Preview and compose side-by-side
	// Pass target height so preview can maximize vertically
	preview := s.renderMockup(settingsHeight)

	// Match preview height to settings dialog to prevent black gaps
	previewHeight := lipgloss.Height(preview)
	previewWidth := lipgloss.Width(preview)

	if previewHeight > settingsHeight {
		// Truncate preview to match settings height
		previewLines := strings.Split(preview, "\n")
		if len(previewLines) > settingsHeight {
			previewLines = previewLines[:settingsHeight]
		}
		preview = strings.Join(previewLines, "\n")
	} else if previewHeight < settingsHeight {
		// Pad preview with Screen background to match settings height
		padStyle := lipgloss.NewStyle().
			Width(previewWidth).
			Background(styles.Screen.GetBackground())
		padLine := padStyle.Render("")
		for i := previewHeight; i < settingsHeight; i++ {
			preview += "\n" + padLine
		}
	}

	// Create gutter with explicit Screen background color
	// Height matches settings dialog (preview is already matched)
	gutterStyle := lipgloss.NewStyle().Background(styles.Screen.GetBackground())
	gutterLines := make([]string, settingsHeight)
	for i := range gutterLines {
		// Render spaces with Screen background for each line
		gutterLines[i] = gutterStyle.Render(strutil.Repeat(" ", layout.GutterWidth))
	}
	gutter := strings.Join(gutterLines, "\n")

	// Join horizontally: settings | gutter | preview
	return lipgloss.JoinHorizontal(lipgloss.Top, settingsDialog, gutter, preview)
}

func alignCenter(w int, text string) string {
	plain := tui.GetPlainText(text)
	wt := lipgloss.Width(plain)
	if wt >= w {
		return text
	}
	left := (w - wt) / 2
	right := w - wt - left
	return strutil.Repeat(" ", left) + text + strutil.Repeat(" ", right)
}

func (s *DisplayOptionsScreen) renderMockup(targetHeight int) string {
	width := 44 // Reduced width to fit the screen better

	paddedLine := func(text string, style lipgloss.Style, fallback string) string {
		rendered := tui.RenderThemeText(text, style)
		plain := tui.GetPlainText(rendered)
		wt := lipgloss.Width(plain)
		if wt < width {
			return style.Render(rendered + strutil.Repeat(fallback, width-wt))
		}
		return style.Render(plain[:width])
	}

	hStyle := tui.SemanticRawStyle("Preview_Theme_Screen")

	themeName := s.previewTheme

	// Header Row (simulate real status bar layout)
	// Use thirds for proper centering
	leftWidth := width / 3
	centerWidth := width / 3
	rightWidth := width - leftWidth - centerWidth // Handles odd widths

	// Left: Host
	leftText := " {{|Preview_Theme_Hostname|}}HOST{{[-]}}"
	leftSec := hStyle.Width(leftWidth).Align(lipgloss.Left).Render(tui.RenderThemeText(leftText, hStyle))

	// Center: App Name (centered within its third)
	centerText := "{{|Preview_Theme_ApplicationName|}}" + tui.GetPlainText(themeName) + "{{[-]}}"
	centerSec := hStyle.Width(centerWidth).Align(lipgloss.Center).Render(tui.RenderThemeText(centerText, hStyle))

	// Right: Version
	rightText := "{{|Preview_Theme_ApplicationVersion|}}A:[{{[-]}}{{|Preview_Theme_ApplicationVersion|}}2.1{{[-]}}{{|Preview_Theme_ApplicationVersion|}}]{{[-]}} "
	rightSec := hStyle.Width(rightWidth).Align(lipgloss.Right).Render(tui.RenderThemeText(rightText, hStyle))

	headerRow := lipgloss.JoinHorizontal(lipgloss.Top, leftSec, centerSec, rightSec)

	sepChar := "-"
	if s.config.UI.LineCharacters {
		sepChar = "─"
	} else {
		sepChar = "-"
	}
	// The real separator is indented by 1 character on each side and uses the header/screen background
	sepLine := strutil.Repeat(sepChar, width-2)
	sepRow := hStyle.Render(" " + sepLine + " ")

	bgStyle := tui.SemanticRawStyle("Preview_Theme_Screen")
	dContent := tui.SemanticRawStyle("Preview_Theme_Dialog")

	dBorder1 := tui.SemanticRawStyle("Preview_Theme_Border")
	dBorder2 := tui.SemanticRawStyle("Preview_Theme_Border2")

	// Adjust border colors based on setting
	switch s.config.UI.BorderColor {
	case 1:
		dBorder2 = dBorder1
	case 2:
		dBorder1 = dBorder2
	}

	var b lipgloss.Border
	if !s.config.UI.Borders {
		b = lipgloss.HiddenBorder()
	} else if s.config.UI.LineCharacters {
		b = lipgloss.RoundedBorder()
	} else {
		b = tui.RoundedAsciiBorder // Use exported variant from tui package
	}

	// Build StyleContext for the preview
	previewCtx := tui.StyleContext{
		LineCharacters:  s.config.UI.LineCharacters,
		DrawBorders:     s.config.UI.Borders,
		Screen:          bgStyle,
		Dialog:          dContent,
		DialogTitle:     tui.SemanticRawStyle("Preview_Theme_Title"),
		DialogTitleHelp: tui.SemanticRawStyle("Preview_Theme_TitleHelp"),
		Border:          b,
		BorderColor:     dBorder1.GetForeground(),
		Border2Color:    dBorder2.GetForeground(),
		ButtonActive:    tui.SemanticRawStyle("Preview_Theme_ButtonActive"),
		ButtonInactive:  tui.SemanticRawStyle("Preview_Theme_ButtonInactive"),
		ItemNormal:      tui.SemanticRawStyle("Preview_Theme_Item"),
		ItemSelected:    tui.SemanticRawStyle("Preview_Theme_ItemSelected"),
		TagNormal:       tui.SemanticRawStyle("Preview_Theme_Tag"),
		TagSelected:     tui.SemanticRawStyle("Preview_Theme_TagSelected"),
		TagKey:          tui.SemanticRawStyle("Preview_Theme_TagKey"),
		TagKeySelected:  tui.SemanticRawStyle("Preview_Theme_TagKeySelected"),
		Shadow:          tui.SemanticRawStyle("Preview_Theme_Shadow"),
		ShadowColor:     getPreviewShadowColor(),
		ShadowLevel:     s.config.UI.ShadowLevel,
		HelpLine:        tui.SemanticRawStyle("Preview_Theme_Helpline"),
		StatusSuccess:   tui.SemanticRawStyle("Preview_Theme_TitleNotice"),
		StatusWarn:      tui.SemanticRawStyle("Preview_Theme_TitleWarn"),
		StatusError:     tui.SemanticRawStyle("Preview_Theme_TitleError"),
	}

	// Backdrop Content (Dialog Simulation)
	// Shortened strings to prevent overflow in the 44-cell width
	contentLines := []string{
		" {{|Preview_Theme_Subtitle|}}A Subtitle Line{{[-]}}",
		"   {{|Preview_Theme_CommandLine|}}ds2 --theme{{[-]}}",
		"",
		" Heading: {{|Preview_Theme_HeadingValue|}}Value{{[-]}} {{|Preview_Theme_HeadingTag|}}[*Tag*]{{[-]}}",
		"",
		"    Caps: {{|Preview_Theme_KeyCap|}}[up]{{[-]}} {{|Preview_Theme_KeyCap|}}[down]{{[-]}} {{|Preview_Theme_KeyCap|}}[left]{{[-]}} {{|Preview_Theme_KeyCap|}}[right]",
		"",
		" Normal text",
		" {{|Preview_Theme_Highlight|}}Highlighted text{{[-]}}",
		"",
		// Menu Items Simulation
		" {{|Preview_Theme_Item|}}Item 1      Item Description{{[-]}}",
		" {{|Preview_Theme_Item|}}Item 2      {{|Preview_Theme_ListAppUserDefined|}}User Description{{[-]}}",
		"",
		" {{|Preview_Theme_LineHeading|}}*** .env ***{{[-]}}",
		" {{|Preview_Theme_LineComment|}}### Sample comment{{[-]}}",
		" {{|Preview_Theme_LineVar|}}Var='Default'{{[-]}}",
		" {{|Preview_Theme_LineModifiedVar|}}Var='Modified'{{[-]}}",
	}

	for i, l := range contentLines {
		contentLines[i] = tui.RenderThemeText(l, dContent)
	}
	contentStr := lipgloss.JoinVertical(lipgloss.Left, contentLines...)

	// Multi-segment title on the border
	titleParts := []string{
		"{{|Preview_Theme_Title|}}Title{{[-]}}",
		"{{|Preview_Theme_TitleSuccess|}}S{{[-]}}",
		"{{|Preview_Theme_TitleWarning|}}W{{[-]}}",
		"{{|Preview_Theme_TitleError|}}E{{[-]}}",
		"{{|Preview_Theme_TitleQuestion|}}Q{{[-]}}",
	}
	dTitle := strings.Join(titleParts, " ")

	// Use our new context-aware bordered box renderer for perfect parity
	// We use 38 to ensure width (38+2 borders + 2 shadow = 42) leaves 1 space indent on both sides of a 44 width backdrop.
	dialogBox := tui.RenderBorderedBoxCtx(dTitle, contentStr, 38, 0, true, false, previewCtx)

	// Add shadow if enabled using our new context-aware helper
	if s.config.UI.Shadow {
		dialogBox = tui.AddShadowCtx(dialogBox, previewCtx)
	}

	// Backdrop - calculate height to fill available space
	// Structure: headerRow(1) + sepRow(1) + backdrop + helpRow(1) + logStripRow(1) + outer borders(2) + shadow(1 if enabled)
	fixedLines := 6 // header + sep + help + logstrip + 2 borders
	if tui.IsShadowEnabled() {
		fixedLines++ // shadow adds 1 line
	}
	backdropHeight := targetHeight - fixedLines
	if backdropHeight < 10 {
		backdropHeight = 10 // minimum height for content visibility
	}
	backdropLines := make([]string, backdropHeight)
	filler := bgStyle.Render(strutil.Repeat(" ", width))
	for i := range backdropLines {
		backdropLines[i] = filler
	}
	backdropBlock := lipgloss.JoinVertical(lipgloss.Left, backdropLines...)

	backdropBlock = tui.Overlay(dialogBox, backdropBlock, tui.OverlayCenter, tui.OverlayCenter, 0, 0)

	helpStyle := tui.SemanticRawStyle("Preview_Theme_Helpline")
	helpRow := paddedLine(" Help: [Tab] Cycle | [Esc] Back", helpStyle, " ")

	// Log Toggle Strip (Border)
	stripSepChar := "-"
	if s.config.UI.LineCharacters {
		stripSepChar = "─"
	} else {
		stripSepChar = "-"
	}
	marker := "^"
	label := " " + marker + " Log " + marker + " "
	stripStyle := lipgloss.NewStyle().
		Foreground(tui.SemanticRawStyle("Preview_Theme_LogPanel").GetForeground()).
		Background(helpStyle.GetBackground())

	labelW := lipgloss.Width(label)
	dashW := (width - labelW) / 2
	leftDashes := strutil.Repeat(stripSepChar, dashW)
	rightTotal := width - dashW - labelW
	rightDashes := strutil.Repeat(stripSepChar, rightTotal)

	logStripRow := stripStyle.Render(leftDashes + label + rightDashes)

	mockupParts := []string{
		headerRow,
		sepRow,
		backdropBlock,
		helpRow,
		logStripRow,
	}
	for i, p := range mockupParts {
		mockupParts[i] = strings.TrimRight(p, "\n")
	}
	mockup := lipgloss.JoinVertical(lipgloss.Left, mockupParts...)

	// Wrap in a standard dialog using the current (active) theme
	// The mockup content uses preview theme colors, but the outer dialog uses active theme
	mockupWidth := lipgloss.Width(mockup)
	preview := tui.RenderBorderedBoxCtx("Preview", mockup, mockupWidth, 0, false, false, tui.GetActiveContext())

	// Add shadow if enabled in global config (same as settings dialog)
	if tui.IsShadowEnabled() {
		preview = tui.AddShadow(preview)
	}

	return preview
}

func (s *DisplayOptionsScreen) View() tea.View {
	v := tea.NewView(s.ViewString())
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// Layers implements LayeredView
func (s *DisplayOptionsScreen) Layers() []*lipgloss.Layer {
	// Simple layer - just the rendered content
	// Hit testing is handled separately by GetHitRegions()
	return []*lipgloss.Layer{
		lipgloss.NewLayer(s.ViewString()).Z(tui.ZScreen),
	}
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
	// height passed in already accounts for global GapBeforeHelpline
	s.height = height

	// Guard: menus may not be initialized yet (e.g. called during screen registration)
	if s.optionsMenu == nil || s.themeMenu == nil {
		return
	}

	layout := tui.GetLayout()

	// The width/height passed in is already the content area from layout.ContentArea()
	// which has already subtracted shadow space. The dialog body fits here,
	// and the shadow extends past into the edge indent area.

	// Check if preview fits
	previewMinWidth := 48
	minDialogWidth := 44 + layout.BorderWidth() // Minimum dialog content + borders
	// Preview fits if: dialog + gutter + preview fits in content area
	// (shadow extends past content area, not counted here)
	previewFits := width >= minDialogWidth+layout.GutterWidth+previewMinWidth

	// Calculate available width for dialog content
	var dialogContentWidth int
	if previewFits {
		// Dialog shares space with preview
		// dialog + gutter + preview = width (content area)
		dialogContentWidth = width - layout.GutterWidth - previewMinWidth
	} else {
		// Dialog fills entire content area (maximized mode)
		dialogContentWidth = width
	}

	// Menu width = dialog content - outer dialog borders
	menuWidth := dialogContentWidth - layout.BorderWidth()
	if menuWidth < 40 {
		menuWidth = 40
	}

	// Use centralized layout helper for vertical budgeting
	// Dialog overhead: outer borders, subtitle(0), buttons, shadow
	hasShadow := tui.IsShadowEnabled()
	optionsContentHeight := layout.DialogContentHeight(height, 0, true, hasShadow)
	// overhead = total height - content height
	overhead := height - optionsContentHeight

	// Calculate options menu height first (it's dynamic based on flow)
	optionsFlowLines := s.optionsMenu.GetFlowHeight(menuWidth)
	optionsHeight := optionsFlowLines + layout.BorderHeight()

	// Theme list gets remaining height
	themeHeight := s.height - optionsHeight - overhead
	if themeHeight < 4 {
		themeHeight = 4
	}

	s.themeMenu.SetSize(menuWidth, themeHeight)
	s.optionsMenu.SetSize(menuWidth, optionsHeight)
}

func (s *DisplayOptionsScreen) IsMaximized() bool {
	// Always return true to get left-aligned positioning from AppModel
	// Both side-by-side and single-dialog modes should start at EdgeIndent
	return true
}

func (s *DisplayOptionsScreen) HasDialog() bool {
	if s.themeMenu == nil || s.optionsMenu == nil {
		return false
	}
	return s.themeMenu.HasDialog() || s.optionsMenu.HasDialog()
}

func (s *DisplayOptionsScreen) MenuName() string {
	return "display_options"
}

func (s *DisplayOptionsScreen) SetFocused(f bool) {
	// If the entire screen loses focus to e.g. the log panel
	if !f {
		s.themeMenu.SetSubFocused(false)
		s.optionsMenu.SetSubFocused(false)
	} else {
		s.updateFocusStates()
	}
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (s *DisplayOptionsScreen) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	var regions []tui.HitRegion

	// Content starts at (1, 1) relative to root because of the outer border
	const contentX = 1
	const contentY = 1

	// Theme menu regions
	themeRegions := s.themeMenu.GetHitRegions(offsetX+contentX, offsetY+contentY)
	regions = append(regions, themeRegions...)

	// Theme panel hit region
	regions = append(regions, tui.HitRegion{
		ID:     tui.IDThemePanel,
		X:      offsetX + contentX,
		Y:      offsetY + contentY,
		Width:  s.themeMenu.Width(),
		Height: s.themeMenu.Height(),
		ZOrder: tui.ZScreen + 1,
	})

	// Options menu regions
	optionsY := contentY + s.themeMenu.Height()
	optionsRegions := s.optionsMenu.GetHitRegions(offsetX+contentX, offsetY+optionsY)
	regions = append(regions, optionsRegions...)

	// Options panel hit region
	regions = append(regions, tui.HitRegion{
		ID:     tui.IDOptionsPanel,
		X:      offsetX + contentX,
		Y:      offsetY + optionsY,
		Width:  s.optionsMenu.Width(),
		Height: s.optionsMenu.Height(),
		ZOrder: tui.ZScreen + 1,
	})

	// Button row regions
	buttonY := optionsY + s.optionsMenu.Height()
	btnRowWidth := s.themeMenu.Width()

	// Button panel background: covers the full button row so hover+scroll/middle-click
	// can reach it even between buttons. Lower ZOrder than individual button regions
	// so left-clicks on a specific button still hit the correct button.
	regions = append(regions, tui.HitRegion{
		ID:     tui.IDButtonPanel,
		X:      offsetX + contentX,
		Y:      offsetY + buttonY,
		Width:  btnRowWidth,
		Height: tui.DialogButtonHeight,
		ZOrder: tui.ZScreen + 1,
	})

	// Individual button hit regions (higher ZOrder → take priority for left-click)
	regions = append(regions, tui.GetButtonHitRegions(
		"", offsetX+contentX, offsetY+buttonY, btnRowWidth, tui.ZScreen+2,
		tui.ButtonSpec{Text: "Apply", ZoneID: tui.IDApplyButton},
		tui.ButtonSpec{Text: "Back", ZoneID: tui.IDBackButton},
		tui.ButtonSpec{Text: "Exit", ZoneID: tui.IDExitButton},
	)...)

	return regions
}

// getPreviewShadowColor extracts the shadow color from the preview theme
// Prefers foreground (for shade chars), falls back to background
func getPreviewShadowColor() color.Color {
	shadowStyle := tui.SemanticRawStyle("Preview_Theme_Shadow")
	if fg := shadowStyle.GetForeground(); fg != nil {
		return fg
	}
	return shadowStyle.GetBackground()
}
