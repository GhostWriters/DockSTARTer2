package tui

import (
	"DockSTARTer2/internal/displayengine"

	"charm.land/lipgloss/v2"
)

// newProgramBoxHeaderSection builds a borderless, fixed-height, non-focusable
// displayengine.Content section rendering a ProgramBoxModel's task-list/
// progress-bar header (box.renderHeaderUI/box.calculateHeaderHeight, kept as
// ProgramBoxModel methods in dialog_programbox_render.go, called here rather
// than duplicated). Subtitle is a separate, standard plain-text Content
// section (see newProgramBox's subtitleSection) -- matching every other
// dialog's subtitle instead of hand-rolled layout/render logic -- not
// handled here. Always present (never conditionally added/removed) --
// calculateHeaderHeight already naturally returns 0 when there's nothing to
// show, so this section simply renders at height 0 rather than needing
// index-shifting section-count logic elsewhere.
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

// newProgramBoxCommandSection builds a borderless, non-focusable
// displayengine.Content section rendering a ProgramBoxModel's command-line
// display (box.command). Always present in outer's sections (even when
// box.command starts empty, e.g. a choice-dependent command not yet known)
// so ProgramBoxModel.SetCommand can reveal it later without needing to
// insert a new section into an already-running dialog -- its height
// collapses to 0 while box.command is empty. SetProgramBoxHeaderMsg's
// handler re-triggers layout (via outer.SetSize) when this changes, so the
// dialog actually resizes to fit rather than the row height staying stale.
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
		if box.command == "" {
			return 0
		}
		return 1
	}
	m.ContentRenderer = func(contentWidth int) string {
		if box.command == "" {
			// Must return a genuinely empty string, not a styled-but-blank
			// one -- viewWithSections only omits a section from the
			// stacked output when its rendered string is exactly "" (v !=
			// ""); lipgloss styling/padding an empty string still produces
			// a non-empty string (background color, ANSI codes), which was
			// then appended as a phantom row even though
			// SectionHeightOverride allocated 0 height for it, throwing
			// off the whole dialog's computed vs. actual line count.
			return ""
		}
		ctx := displayengine.GetActiveContext()
		renderedCmd := displayengine.RenderThemeText(box.command, ctx.Dialog)
		return lipgloss.NewStyle().
			Width(contentWidth).
			Background(ctx.Dialog.GetBackground()).
			Render(renderedCmd)
	}

	return m
}
