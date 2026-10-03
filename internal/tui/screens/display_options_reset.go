package screens

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/displayengine"

	tea "charm.land/bubbletea/v2"
)

// sectionResetIcons returns a list's reset icon, disabled unless changed
// reports it has unapplied changes (see
// displayengine.MenuModel.SetTitleIcons).
func sectionResetIcons(changed func() bool) func() []displayengine.WidgetDef {
	return func() []displayengine.WidgetDef {
		icon := displayengine.WidgetRefresh
		icon.ID = displayengine.IDTitleWidgetReset
		icon.Label = "Reset"
		icon.HelpText = "Discard this list's unapplied changes on the shown tab (Alt+R)."
		icon.Disabled = !changed()
		return []displayengine.WidgetDef{icon}
	}
}

// sectionResetID returns the hit region ID of menu's reset icon.
func sectionResetID(menu *displayengine.MenuModel) string {
	return menu.TitleIconsID() + "." + displayengine.IDTitleWidgetReset
}

// resetSection discards the unapplied changes of the list whose reset icon
// id belongs to, on the shown tab and element; ok is false for any other id.
func (s *DisplayOptionsScreen) resetSection(id string) (cmd tea.Cmd, ok bool) {
	for _, menu := range []*displayengine.MenuModel{s.themeMenu, s.tintMenu, s.overrideMenu, s.optionsMenu} {
		if id == sectionResetID(menu) {
			return s.resetList(menu), true
		}
	}
	return nil, false
}

// resetFocusedList discards the unapplied changes of the list holding
// focus, its Find box included.
func (s *DisplayOptionsScreen) resetFocusedList() tea.Cmd {
	switch s.focusedSettingsLeaf() {
	case nil:
		return nil
	case s.themeMenu, s.themeFindMenu:
		return s.resetList(s.themeMenu)
	case s.tintMenu, s.tintSearchMenu:
		return s.resetList(s.tintMenu)
	case s.overrideMenu:
		return s.resetList(s.overrideMenu)
	case s.optionsMenu:
		return s.resetList(s.optionsMenu)
	}
	return nil
}

// resetList discards menu's unapplied changes on the shown tab and element.
func (s *DisplayOptionsScreen) resetList(menu *displayengine.MenuModel) tea.Cmd {
	switch menu {
	case s.themeMenu:
		return s.resetTheme()
	case s.tintMenu:
		return s.resetElementPart(func(e *config.AnsiElementColors, base config.AnsiElementColors) {
			e.Tint, e.TintEnabled = base.Tint, base.TintEnabled
		})
	case s.overrideMenu:
		return s.resetElementPart(func(e *config.AnsiElementColors, base config.AnsiElementColors) {
			base.Tint, base.TintEnabled = e.Tint, e.TintEnabled
			*e = base
		})
	case s.optionsMenu:
		s.themeChangedFields = nil
		return func() tea.Msg {
			return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
				a := cfg.Appearance.Ptr(s.editType)
				base := s.baseConfig.Appearance.ForConnType(s.editType)
				base.Theme, base.AnsiColors = a.Theme, a.AnsiColors
				*a = base
			}}
		}
	}
	return nil
}

// resetTheme puts the shown tab's saved theme back, without staging its
// suggested options (see loadThemeDefaults).
func (s *DisplayOptionsScreen) resetTheme() tea.Cmd {
	loadDefaults := s.loadThemeDefaults
	s.loadThemeDefaults = false
	s.applyPreview(s.baseConfig.Appearance.ForConnType(s.editType).Theme)
	s.loadThemeDefaults = loadDefaults
	s.syncThemeList()
	s.syncOptionsMenu()
	if s.outerMenu != nil {
		s.outerMenu.InvalidateCache()
	}
	return nil
}

// resetElementPart applies reset to the shown element's staged settings
// with its saved ones.
func (s *DisplayOptionsScreen) resetElementPart(reset func(e *config.AnsiElementColors, base config.AnsiElementColors)) tea.Cmd {
	element := s.tintElement
	return func() tea.Msg {
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			reset(cfg.Appearance.Ptr(s.editType).AnsiColors.ElementPtr(element),
				s.baseConfig.Appearance.ForConnType(s.editType).AnsiColors.Element(element))
		}}
	}
}
