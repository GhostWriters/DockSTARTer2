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
// supplies every child region below it, including the collapsed-preview
// expand indicator (see appearanceLayoutRow.GetHitRegions).
// GetInputCursor implements tui.InputCursorProvider: the terminal cursor
// sits in the scheme search box while it has focus, placed by the box's own
// hit region.
func (s *DisplayOptionsScreen) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	if s.tintSearchMenu == nil || s.tintSearchInput == nil || s.focusedSettingsLeaf() != s.tintSearchMenu {
		return 0, 0, tea.CursorBar, false
	}
	// Hit regions record the input's absolute text X; keep it.
	saved := s.tintSearchInput.ScreenTextX()
	defer s.tintSearchInput.SetScreenTextX(saved)
	for _, r := range s.GetHitRegions(0, 0) {
		if r.ID != tintSearchID+".sinput" {
			continue
		}
		shape = tea.CursorBar
		if s.tintSearchInput.IsOverwrite() {
			shape = tea.CursorBlock
		}
		// CursorColumn counts the prompt; +1 is the section padding.
		return r.X + 1 + s.tintSearchInput.CursorColumn(), r.Y, shape, true
	}
	return 0, 0, tea.CursorBar, false
}

func (s *DisplayOptionsScreen) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	if s.outerMenu == nil {
		return nil
	}
	return s.outerMenu.GetHitRegions(offsetX, offsetY)
}
