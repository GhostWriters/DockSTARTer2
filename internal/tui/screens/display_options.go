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
		menu.Select(s.config.UI.ShadowLevel)
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
		s.width = msg.Width
		s.height = msg.Height

		// The settings dialog has overhead: Title (2) + Borders (2) + Shadow (1) + Buttons (min 3) = 8
		dialogOverhead := 8
		menuHeight := s.height - dialogOverhead
		if menuHeight < 5 {
			menuHeight = 5
		}

		// We pass a modified WindowSizeMsg to the submenus so they size themselves to our available height
		subMsg := tea.WindowSizeMsg{Width: s.width / 2, Height: menuHeight}

		updatedTheme, _ := s.themeMenu.Update(subMsg)
		if m, ok := updatedTheme.(*tui.MenuModel); ok {
			s.themeMenu = m
		}

		updatedOptions, _ := s.optionsMenu.Update(subMsg)
		if m, ok := updatedOptions.(*tui.MenuModel); ok {
			s.optionsMenu = m
		}

		return s, nil

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
					s.applyPreview(items[i].Tag)
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
	tui.ClearSemanticCache()
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

func (s *DisplayOptionsScreen) Layers() []tui.LayerSpec {
	layout := tui.GetLayout()

	// Calculate known dimensions (same logic as SetSize)
	// s.width and s.height are already the content area (chrome and shadow space accounted for)
	shadowW := 0
	shadowH := 0
	if s.config.UI.Shadow {
		shadowW = layout.ShadowWidth
		shadowH = layout.ShadowHeight
	}

	previewMinWidth := 48
	minDialogWidth := 44 + layout.BorderWidth()
	// Preview fits if: dialog + shadow + gutter + preview fits in content area
	previewFits := s.width >= minDialogWidth+shadowW+layout.GutterWidth+previewMinWidth

	// Calculate dialog content width based on known size, NOT measured content
	// Since s.width is already content area, dialog + shadow must fit within s.width
	var dialogContentWidth int
	if previewFits {
		dialogContentWidth = s.width - shadowW - layout.GutterWidth - previewMinWidth
	} else {
		// Maximized: dialog + shadow fills content area
		dialogContentWidth = s.width - shadowW
	}

	// Menu width = dialog content - outer dialog borders
	// Must match SetSize calculation exactly
	menuWidth := dialogContentWidth - layout.BorderWidth()
	if menuWidth < 40 {
		menuWidth = 40
	}

	// 1. Render Settings Menus
	themeView := tui.ZoneMark("ThemePanel", s.themeMenu.ViewString())
	optionsView := tui.ZoneMark("OptionsPanel", s.optionsMenu.ViewString())

	leftColumn := lipgloss.JoinVertical(lipgloss.Left,
		themeView,
		optionsView,
	)

	// 2. Render Buttons using known width, not measured
	buttons := []tui.ButtonSpec{
		{Text: "Apply", Active: s.focusedButton == 0, ZoneID: "ApplyBtn"},
		{Text: "Back", Active: s.focusedButton == 1, ZoneID: "BackBtn"},
		{Text: "Exit", Active: s.focusedButton == 2, ZoneID: "ExitBtn"},
	}
	buttonRow := tui.RenderCenteredButtons(menuWidth, buttons...)

	// 3. Settings Dialog with known width
	settingsContent := lipgloss.JoinVertical(lipgloss.Left, leftColumn, buttonRow)

	// Calculate target height - s.height is already content area
	// Dialog + shadow must fit within content area
	targetHeight := s.height - shadowH
	if targetHeight < 10 {
		targetHeight = 10
	}

	// Use RenderBorderedBoxCtx with known width instead of RenderDialog which measures content
	settingsDialog := tui.RenderBorderedBoxCtx("Appearance Settings", settingsContent, menuWidth, targetHeight, true, tui.GetActiveContext())

	// Add shadow if enabled in config
	if s.config.UI.Shadow {
		settingsDialog = tui.AddShadow(settingsDialog)
	}

	// Calculate actual dialog width (content + borders + shadow)
	dialogWidth := menuWidth + layout.BorderWidth() + shadowW

	layers := []tui.LayerSpec{
		{Content: settingsDialog, X: 0, Y: 0, Z: 1},
	}

	// 4. Render Preview (if space allows)
	if previewFits {
		previewX := dialogWidth + layout.GutterWidth
		preview := s.renderMockup()
		layers = append(layers, tui.LayerSpec{Content: preview, X: previewX, Y: 0, Z: 1})
	}

	return layers
}

func (s *DisplayOptionsScreen) ViewString() string {
	layers := s.Layers()
	return tui.MultiOverlayWithBounds(s.width, s.height, layers...)
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
	// themeAuthor := ""
	// themeDesc := ""
	// for _, t := range s.themes {
	// 	if t.Name == themeName {
	// 		themeAuthor = t.Author
	// 		themeDesc = t.Description
	// 		break
	// 	}
	// }

	// Header Row (simulate real status bar layout)
	// Left: Host
	leftText := " {{|Preview_Theme_Hostname|}}HOST{{[-]}}"
	leftSec := hStyle.Width(width / 3).Align(lipgloss.Left).Render(tui.RenderThemeText(leftText, hStyle))

	// Center: App Name
	centerText := "{{|Preview_Theme_ApplicationName|}}" + tui.GetPlainText(themeName) + "{{[-]}}"
	centerWidth := lipgloss.Width(tui.GetPlainText(centerText))
	centerSec := hStyle.Width(centerWidth).Align(lipgloss.Center).Render(tui.RenderThemeText(centerText, hStyle))

	// Right: Version
	rightWidth := width - (width / 3) - centerWidth
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
		b = lipgloss.NormalBorder()
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
		ShadowColor:     tui.SemanticRawStyle("Preview_Theme_Shadow").GetBackground(),
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
	dialogBox := tui.RenderBorderedBoxCtx(dTitle, contentStr, 38, 0, true, previewCtx)

	// Add shadow if enabled using our new context-aware helper
	if s.config.UI.Shadow {
		dialogBox = tui.AddShadowCtx(dialogBox, previewCtx)
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

	mockup := lipgloss.JoinVertical(lipgloss.Left,
		headerRow,
		sepRow,
		backdropBlock,
		helpRow,
		logStripRow,
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

	layout := tui.GetLayout()

	// Shadow dimensions (for total dialog size calculation, NOT for content area reduction)
	// The width/height passed in is already the content area (shadow space already accounted for)
	shadowW := 0
	if s.config.UI.Shadow {
		shadowW = layout.ShadowWidth
	}

	// Check if preview fits using same logic as IsMaximized()
	// Note: width is the content area, so total available = width + shadow (if enabled)
	previewMinWidth := 48
	minDialogWidth := 44 + layout.BorderWidth() // Minimum dialog without shadow
	// Preview fits if: dialog + shadow + gutter + preview fits in content area
	previewFits := width >= minDialogWidth+shadowW+layout.GutterWidth+previewMinWidth

	// Calculate available width for dialog content
	// Since width is already content area, dialog + shadow must fit within width
	var dialogContentWidth int
	if previewFits {
		// Dialog shares space with preview
		// dialogContentWidth + shadow + gutter + preview <= width
		dialogContentWidth = width - shadowW - layout.GutterWidth - previewMinWidth
	} else {
		// Dialog fills available width (maximized mode)
		// dialogContentWidth + shadow = width (dialog with shadow fills content area)
		dialogContentWidth = width - shadowW
	}

	// Menu width = dialog content - outer dialog borders
	menuWidth := dialogContentWidth - layout.BorderWidth()
	if menuWidth < 40 {
		menuWidth = 40
	}

	// Vertical Budgeting
	// height is already the content area (chrome already subtracted)
	// Dialog overhead: outer borders(2) + title line(1) + button row(3) + shadow
	shadowH := 0
	if s.config.UI.Shadow {
		shadowH = layout.ShadowHeight
	}
	overhead := layout.BorderHeight() + 1 + layout.ButtonHeight + shadowH

	// Calculate options menu height first (it's dynamic based on flow)
	optionsFlowLines := s.optionsMenu.GetFlowHeight(menuWidth)
	optionsHeight := optionsFlowLines + layout.BorderHeight()

	// Theme list gets remaining height
	themeHeight := height - optionsHeight - overhead
	if themeHeight < 4 {
		themeHeight = 4
	}

	s.themeMenu.SetSize(menuWidth, themeHeight)
	s.optionsMenu.SetSize(menuWidth, optionsHeight)
}

func (s *DisplayOptionsScreen) IsMaximized() bool {
	// Maximized when preview doesn't fit (no side-by-side layout)
	layout := tui.GetLayout()
	previewMinWidth := 48
	shadowW := 0
	if s.config.UI.Shadow {
		shadowW = layout.ShadowWidth
	}

	// Calculate minimum dialog width (without shadow)
	minDialogWidth := 44 + layout.BorderWidth()

	// Preview fits if: dialog + shadow + gutter + preview fits in content area (s.width)
	previewFits := s.width >= minDialogWidth+shadowW+layout.GutterWidth+previewMinWidth

	// When preview doesn't fit, we're in "maximized" mode (dialog fills available width)
	return !previewFits
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
