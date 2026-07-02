package tui

import (
	"DockSTARTer2/internal/displayengine"

	"charm.land/lipgloss/v2"
)

// newProgramBoxHeaderSection builds a borderless, fixed-height, non-focusable
// displayengine.Content section rendering a ProgramBoxModel's subtitle/task-list/progress-bar
// header (box.renderHeaderUI/box.calculateHeaderHeight, kept as ProgramBoxModel
// methods in dialog_programbox_render.go, called here rather than duplicated).
// Always present (never conditionally added/removed) -- calculateHeaderHeight
// already naturally returns 0 when there's nothing to show, so this section
// simply renders at height 0 rather than needing index-shifting section-count
// logic elsewhere.
func newProgramBoxHeaderSection(id string, box *ProgramBoxModel) *displayengine.MenuModel {
	m := displayengine.NewMenuModel(id, "", "", nil)
	m.SetSubMenuMode(true)
	m.SetIsDialog(false)
	m.SetButtons([]displayengine.ButtonDef{})
	m.SetMaximized(true)
	m.SetVariableHeight(false)
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)
	m.SetBorderless(true)
	m.SetNonFocusable(true)

	m.SectionHeightOverride = func(width int) int {
		return box.calculateHeaderHeight(width)
	}
	m.ContentRenderer = func(contentWidth int) string {
		return box.renderHeaderUI(contentWidth)
	}

	return m
}

// newProgramBoxCommandSection builds a borderless, fixed-height, non-focusable
// displayengine.Content section rendering a ProgramBoxModel's command-line display
// (box.command). Only added when box.command != "" -- unlike the header
// section, the command string never changes after construction, so there's
// no dynamic resize-of-section-count concern.
func newProgramBoxCommandSection(id string, box *ProgramBoxModel) *displayengine.MenuModel {
	m := displayengine.NewMenuModel(id, "", "", nil)
	m.SetSubMenuMode(true)
	m.SetIsDialog(false)
	m.SetButtons([]displayengine.ButtonDef{})
	m.SetMaximized(true)
	m.SetVariableHeight(false)
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)
	m.SetBorderless(true)
	m.SetNonFocusable(true)

	m.SectionHeightOverride = func(width int) int {
		return 1
	}
	m.ContentRenderer = func(contentWidth int) string {
		ctx := displayengine.GetActiveContext()
		renderedCmd := displayengine.RenderThemeText(box.command, ctx.Dialog)
		return lipgloss.NewStyle().
			Width(contentWidth).
			Background(ctx.Dialog.GetBackground()).
			Render(renderedCmd)
	}

	return m
}
