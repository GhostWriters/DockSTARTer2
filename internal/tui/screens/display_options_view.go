package screens

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

func (s *DisplayOptionsScreen) ViewString() (result string) {
	defer func() {
		if r := recover(); r != nil {
			result = "(rendering error — theme may still be loading)"
		}
	}()
	if s.outerMenu == nil {
		return ""
	}
	layout := displayengine.GetLayout()

	// If dimensions not yet set, use terminal dimensions as fallback.
	width, height := s.width, s.height
	if width == 0 || height == 0 {
		termW, termH, _ := console.GetTerminalSize()
		if termW > 0 && termH > 0 {
			hasShadow := tui.IsShadowEnabled()
			header := displayengine.NewHeaderModel()
			header.SetWidth(termW - 2)
			headerH := header.Height()
			width, height = layout.ContentArea(termW, termH, hasShadow, false, headerH, layout.HelplineHeight)
		}
	}

	s.outerMenu.SetSize(width, height)
	return s.outerMenu.ViewString()
}

func (s *DisplayOptionsScreen) View() tea.View {
	v := tea.NewView(s.ViewString())
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// GetHitRegions implements HitRegionProvider for mouse hit testing --
// delegates entirely to outerMenu, whose single content section (the
// settings/preview row, via ContentColumn/appearanceLayoutRow) recursively
// supplies every child region below it.
func (s *DisplayOptionsScreen) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	if s.outerMenu == nil {
		return nil
	}
	return s.outerMenu.GetHitRegions(offsetX, offsetY)
}
