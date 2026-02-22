package screens

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	"fmt"
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
			Desc:          desc,
			Help:          fmt.Sprintf("Preview the %s theme", t.Name),
			IsRadioButton: true,
			Checked:       s.currentTheme == t.Name,
		}
	}

	themeMenu := tui.NewMenuModel("theme_list", "Select Theme", "", themeItems, nil)
	s.themeMenu = &themeMenu
	s.themeMenu.SetSubMenuMode(true)
	s.themeMenu.SetShowExit(false)

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

	// Initial Focus
	s.focusedPanel = FocusThemes
	s.updateFocusStates()
}

func (s *DisplayOptionsScreen) updateFocusStates() {
	s.themeMenu.SetSubFocused(s.focusedPanel == FocusThemes)
	s.optionsMenu.SetSubFocused(s.focusedPanel == FocusOptions)
}

func (s *DisplayOptionsScreen) shadowLevelToDesc(l int) string {
	levels := []string{"Off", "Light (░)", "Medium (▒)", "Dark (▓)", "Solid (█)"}
	if l < 0 || l >= len(levels) {
		l = 0
	}
	return levels[l]
}

func (s *DisplayOptionsScreen) borderColorToDesc(c int) string {
	modes := map[int]string{1: "Border 1", 2: "Border 2", 3: "Both (3D)"}
	return modes[c]
}

func (s *DisplayOptionsScreen) dropdownDesc(val string) string {
	return fmt.Sprintf("{{[-]}}{{|Theme_TitleNotice|}}%s{{[-]}} {{[-]}}{{|Theme_LineComment|}}▼{{[-]}}", val)
}

func (s *DisplayOptionsScreen) showShadowDropdown() tea.Cmd {
	return func() tea.Msg {
		levels := []string{"Off", "Light (░)", "Medium (▒)", "Dark (▓)", "Solid (█)"}
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
		return tui.ShowDialogMsg{Dialog: &menu}
	}
}

func (s *DisplayOptionsScreen) showBorderColorDropdown() tea.Cmd {
	return func() tea.Msg {
		modes := []int{1, 2, 3}
		labels := map[int]string{1: "Border 1 (Theme Focus)", 2: "Border 2 (Theme Accent)", 3: "Both (3D Effect)"}
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
	case tea.MouseClickMsg:
		// Focus routing via zones
		if tui.ZoneClick(msg, "ThemePanel") {
			s.focusedPanel = FocusThemes
			s.updateFocusStates()

			// Check if a specific theme item was clicked
			items := s.themeMenu.GetItems()
			for i := range items {
				zoneID := fmt.Sprintf("item-theme_list-%d", i)
				if tui.ZoneClick(msg, zoneID) {
					for j := range items {
						items[j].Checked = (i == j)
					}
					s.themeMenu.SetItems(items)
					s.themeMenu.Select(i)
					s.previewTheme = items[i].Tag
					break
				}
			}
		} else if tui.ZoneClick(msg, "OptionsPanel") {
			s.focusedPanel = FocusOptions
			s.updateFocusStates()
		} else if tui.ZoneClick(msg, "ApplyBtn") {
			s.focusedButton = 0
			return s, s.handleApply()
		} else if tui.ZoneClick(msg, "BackBtn") {
			s.focusedButton = 1
			return s, navigateBack()
		} else if tui.ZoneClick(msg, "ExitBtn") {
			s.focusedButton = 2
			return s, tea.Quit
		}

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
					s.previewTheme = items[cursor].Tag

					// Reset to base configs
					s.config = s.baseConfig

					defaults, ok := s.themeDefaults[s.previewTheme]
					if !ok {
						var err error
						defaults, err = theme.Load(s.previewTheme, "Preview")
						if err != nil {
							s.previewTheme = "ERR: " + err.Error()
						}
						s.themeDefaults[s.previewTheme] = defaults
					}

					if defaults != nil {
						theme.ApplyThemeDefaults(&s.config, *defaults)
					}
					s.syncOptionsMenu()
					tui.ClearSemanticCache()
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

func (s *DisplayOptionsScreen) ViewString() string {
	// 1. Render Settings
	themeView := tui.ZoneMark("ThemePanel", s.themeMenu.ViewString())
	optionsView := tui.ZoneMark("OptionsPanel", s.optionsMenu.ViewString())

	leftColumn := lipgloss.JoinVertical(lipgloss.Left,
		themeView,
		optionsView,
	)

	// 2. Render Buttons
	// Calculate button width to match the column
	contentWidth := lipgloss.Width(leftColumn)
	buttons := []tui.ButtonSpec{
		{Text: "Apply", Active: s.focusedButton == 0, ZoneID: "ApplyBtn"},
		{Text: "Back", Active: s.focusedButton == 1, ZoneID: "BackBtn"},
		{Text: "Exit", Active: s.focusedButton == 2, ZoneID: "ExitBtn"},
	}
	buttonRow := tui.RenderCenteredButtons(contentWidth, buttons...)

	// 3. Wrap Settings + Buttons in a Dialog Box
	// targetHeight is s.height - 1 (to account for shadow)
	settingsContent := lipgloss.JoinVertical(lipgloss.Left, leftColumn, buttonRow)
	settingsDialog := tui.RenderDialog("Appearance Settings", settingsContent, true, s.height-1)

	// 4. Render Preview (only if there is space)
	if s.width >= 100 {
		preview := s.renderMockup()
		return lipgloss.JoinHorizontal(lipgloss.Top, settingsDialog, "    ", preview)
	}

	return settingsDialog
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

func (s *DisplayOptionsScreen) renderMockup() string {
	width := 44 // Reduced width to fit the screen better

	paddedLine := func(text string, style lipgloss.Style, fallback string) string {
		rendered := tui.RenderThemeText(text)
		plain := tui.GetPlainText(rendered)
		wt := lipgloss.Width(plain)
		if wt < width {
			return style.Render(rendered + strutil.Repeat(fallback, width-wt))
		}
		return style.Render(plain[:width])
	}

	hStyle := tui.SemanticRawStyle("Preview_Theme_Screen")
	vStyle := tui.SemanticRawStyle("Preview_Theme_Version")
	headerRow := paddedLine(fmt.Sprintf(" {{|Preview_Theme_Title|}}DS2{{[-]}} %s", vStyle.Render("v2.1")), hStyle, " ")

	sepStyle := tui.SemanticRawStyle("Preview_Theme_Border")
	sepChar := "-"
	if s.config.UI.LineCharacters {
		sepChar = "─"
	}
	sepRow := sepStyle.Render(strutil.Repeat(sepChar, width))

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

	dTitle := tui.SemanticRawStyle("Preview_Theme_Title")
	titleStr := dTitle.Render(" Preview: " + s.previewTheme + " ")

	// Content lines (abbreviated)
	contentLines := []string{
		" {{|Preview_Theme_TitleNotice|}}Notice{{[-]}}   {{|Preview_Theme_TitleSuccess|}}Success{{[-]}}",
		" {{|Preview_Theme_TitleWarning|}}Warning{{[-]}}  {{|Preview_Theme_TitleError|}}Error{{[-]}}",
		"",
		" {{|Preview_Theme_TitleHelp|}}Heading Styl{{[-]}}",
		" {{|Preview_Theme_HelpTag|}}[Tag]{{[-]}} {{|Preview_Theme_HelpItem|}}Value{{[-]}}",
		"",
		" {{|Preview_Theme_TagKey|}}Highlight{{[-]}}",
		" {{|Preview_Theme_Tag|}}[Key]{{[-]}} {{|Preview_Theme_TagKeySelected|}}[Cap]{{[-]}}",
		"",
		" {{|Preview_Theme_ItemHelp|}}*** Variables ***{{[-]}}",
		" {{|Preview_Theme_Shadow|}}### Comment{{[-]}}",
		" {{|Preview_Theme_Item|}}VAR=\"Value\"{{[-]}}",
		" {{|Preview_Theme_ItemSelected|}}NEW=\"New\"{{[-]}}",
		"",
		"  {{|Preview_Theme_ButtonActive|}} <ADD> {{[-]}}  ",
	}

	for i, l := range contentLines {
		contentLines[i] = tui.RenderThemeText(l)
	}
	contentStr := lipgloss.JoinVertical(lipgloss.Left, contentLines...)
	dialogInner := lipgloss.JoinVertical(lipgloss.Center, titleStr, contentStr)

	var b lipgloss.Border
	if !s.config.UI.Borders {
		b = lipgloss.HiddenBorder()
	} else if s.config.UI.LineCharacters {
		b = lipgloss.RoundedBorder()
	} else {
		b = lipgloss.NormalBorder()
	}

	dialogBox := lipgloss.NewStyle().
		Border(b).
		BorderTopForeground(dBorder1.GetForeground()).
		BorderLeftForeground(dBorder1.GetForeground()).
		BorderBottomForeground(dBorder2.GetForeground()).
		BorderRightForeground(dBorder2.GetForeground()).
		BorderBackground(dContent.GetBackground()).
		Background(dContent.GetBackground()).
		Padding(0, 1).
		Render(dialogInner)

	// Apply shadow manually if enabled
	if s.config.UI.Shadow {
		lines := strings.Split(dialogBox, "\n")
		dialogWidth := 0
		for _, line := range lines {
			if w := lipgloss.Width(line); w > dialogWidth {
				dialogWidth = w
			}
		}

		var shadowCell, bottomShadowChars string
		if s.config.UI.LineCharacters {
			shadowStyle := tui.SemanticRawStyle("Preview_Theme_Shadow").
				Background(tui.SemanticRawStyle("Preview_Theme_Screen").GetBackground())

			var shadeChar string
			switch s.config.UI.ShadowLevel {
			case 1:
				shadeChar = "░"
			case 2:
				shadeChar = "▒"
			case 3:
				shadeChar = "▓"
			case 4:
				shadeChar = "█"
			default:
				shadeChar = "▓"
			}
			shadowCell = shadowStyle.Render(strutil.Repeat(shadeChar, 2))
			bottomShadowChars = shadowStyle.Render(strutil.Repeat(shadeChar, dialogWidth-1))
		} else {
			shadowCell = tui.SemanticRawStyle("Preview_Theme_Shadow").Width(2).Render("  ")
			bottomShadowChars = tui.SemanticRawStyle("Preview_Theme_Shadow").Width(dialogWidth - 1).Render(strutil.Repeat(" ", dialogWidth-1))
		}

		spacerCell := lipgloss.NewStyle().Background(bgStyle.GetBackground()).Width(2).Render("  ")
		spacer1 := lipgloss.NewStyle().Background(bgStyle.GetBackground()).Width(1).Render(" ")

		var result strings.Builder
		for i, line := range lines {
			if i == 0 {
				result.WriteString(line + spacerCell + "\n")
			} else {
				result.WriteString(line + shadowCell + "\n")
			}
		}
		result.WriteString(spacer1 + bottomShadowChars)
		dialogBox = result.String()
	}

	// Backdrop
	backdropHeight := 18
	backdropLines := make([]string, backdropHeight)
	filler := bgStyle.Render(strutil.Repeat(" ", width))
	for i := range backdropLines {
		backdropLines[i] = filler
	}
	backdropBlock := lipgloss.JoinVertical(lipgloss.Left, backdropLines...)

	backdropBlock = tui.Overlay(dialogBox, backdropBlock, tui.OverlayCenter, tui.OverlayCenter, 0, 0)

	logStyle := tui.SemanticRawStyle("Preview_Theme_LogPanel")
	logRow := paddedLine(" {{|Preview_Theme_TitleNotice|}}LOG:{{[-]}} Ready", logStyle, " ")

	helpStyle := tui.SemanticRawStyle("Preview_Theme_Helpline")
	helpRow := paddedLine(" Help: [Tab] Cycle | [Esc] Back", helpStyle, " ")

	mockup := lipgloss.JoinVertical(lipgloss.Left,
		headerRow,
		sepRow,
		backdropBlock,
		logRow,
		helpRow,
	)

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Background(lipgloss.Color("#000000")).
		Padding(0, 1)

	return containerStyle.Render(mockup)
}

func (s *DisplayOptionsScreen) View() tea.View {
	return tea.NewView(s.ViewString())
}

func (s *DisplayOptionsScreen) Title() string {
	return "Display Options"
}

func (s *DisplayOptionsScreen) HelpText() string {
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

	menuWidth := width - 55
	if menuWidth < 40 {
		menuWidth = 40
	}

	// Vertical Budgeting:
	// Total available height is 'height'
	// Flow options menu height is dynamic
	optionsFlowLines := s.optionsMenu.GetFlowHeight(menuWidth)
	optionsHeight := optionsFlowLines + 2 // +2 for borders

	// Main dialog overhead: Title(1) + Buttons(1) + Borders(2) + separator(1) = 5
	overhead := 5

	themeHeight := height - optionsHeight - overhead
	if themeHeight < 4 {
		themeHeight = 4
	}

	s.themeMenu.SetSize(menuWidth, themeHeight)
	s.optionsMenu.SetSize(menuWidth, optionsHeight)
}

func (s *DisplayOptionsScreen) IsMaximized() bool {
	return false
}

func (s *DisplayOptionsScreen) HasDialog() bool {
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
