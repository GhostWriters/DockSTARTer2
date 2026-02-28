package screens

import (
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// OptionsMenuScreen is the options menu screen
type OptionsMenuScreen struct {
	menu *tui.MenuModel
}

// NewOptionsMenuScreen creates the options menu
func NewOptionsMenuScreen() *OptionsMenuScreen {
	items := []tui.MenuItem{
		{
			Tag:    "Appearance",
			Desc:   "Themes and Display Options",
			Help:   "Configure color scheme, borders, and effects",
			Action: func() tea.Msg { return tui.NavigateMsg{Screen: NewDisplayOptionsScreen()} },
		},
		{
			Tag:    "Trigger Test Panic",
			Desc:   "{{|Theme_TitleError|}}Test error handling{{[-]}}",
			Help:   "Verify branded recovery and stack trace",
			Action: func() tea.Msg { panic("Manual verification panic (Test: 123)") },
		},
	}

	menu := tui.NewMenuModel(
		"options_menu",
		"Options",
		"Customize settings",
		items,
		navigateBack(),
	)

	return &OptionsMenuScreen{menu: &menu}
}

// Init implements tea.Model
func (s *OptionsMenuScreen) Init() tea.Cmd {
	return s.menu.Init()
}

// Update implements tea.Model
func (s *OptionsMenuScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := s.menu.Update(msg)
	if menu, ok := updated.(*tui.MenuModel); ok {
		s.menu = menu
	}
	return s, cmd
}

// ViewString returns the screen content as a string (for compositing)
func (s *OptionsMenuScreen) ViewString() string {
	return s.menu.ViewString()
}

// View implements tea.Model
func (s *OptionsMenuScreen) View() tea.View {
	return s.menu.View()
}

// Title implements ScreenModel
func (s *OptionsMenuScreen) Title() string {
	return s.menu.Title()
}

// HelpText implements ScreenModel
func (s *OptionsMenuScreen) HelpText() string {
	return s.menu.HelpText()
}

// SetSize implements ScreenModel
func (s *OptionsMenuScreen) SetSize(width, height int) {
	// options leaves 1 blank line before the helpline.
	s.menu.SetSize(width, height)
}

// SetFocused propagates focus state to the inner menu (used by log panel focus)
func (s *OptionsMenuScreen) SetFocused(f bool) {
	s.menu.SetFocused(f)
}

// IsMaximized implements ScreenModel
func (s *OptionsMenuScreen) IsMaximized() bool {
	return s.menu.IsMaximized()
}

// HasDialog implements ScreenModel
func (s *OptionsMenuScreen) HasDialog() bool {
	return s.menu.HasDialog()
}

// MenuName implements ScreenModel
func (s *OptionsMenuScreen) MenuName() string {
	return "options"
}

// Layers implements LayeredView for compositing
func (s *OptionsMenuScreen) Layers() []*lipgloss.Layer {
	return s.menu.Layers()
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (s *OptionsMenuScreen) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	return s.menu.GetHitRegions(offsetX, offsetY)
}
