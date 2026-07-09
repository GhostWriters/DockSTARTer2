package classic

// NewPlainTextSection builds a borderless, non-focusable Content section that
// renders a single line of theme-styled text with no box around it -- e.g. a
// dialog's subtitle, expressed as its own content section (the "plain text"
// Content kind) rather than special-cased subtitle layout/render logic.
// text is wrapped in the {{|Subtitle|}} theme tag, matching the styling the
// plain-list subtitle rendering already uses (menu_render.go's viewSubMenu
// and non-sectioned ViewString tail). Never call this with an empty string --
// callers should simply not add a section at all when there is no subtitle,
// so it takes zero space rather than rendering an empty line.
func NewPlainTextSection(id, text string) *MenuModel {
	m := NewMenuModel(id, "", "", nil)
	m.plainText = text
	m.plainTextThemeTag = "{{|Subtitle|}}"
	m.SetSubMenuMode(true)
	m.SetVariableHeight(false)
	m.SetIsDialog(false)
	m.SetButtons([]ButtonDef{})
	m.SetMaximized(true)
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)
	return m
}

// SetPlainText updates the plain-text kind's text after construction (e.g. a
// dialog whose subtitle text changes once some async state resolves).
// Callers must also invalidate/re-trigger layout (this alone only changes
// what gets rendered, not the section's height allocation if the new text
// wraps to a different number of lines) -- see ProgramBoxModel's
// SetProgramBoxHeaderMsg handler for the pattern (outer.SetSize to force
// calculateSectionLayout to re-measure).
func (m *MenuModel) SetPlainText(text string) *MenuModel {
	m.plainText = text
	return m
}

// SetPlainTextStyle overrides the plain-text kind's theme tag (default
// "{{|Subtitle|}}") and adds vPad blank lines above and below the text.
// Used by confirm-style dialogs to render the question as plain dialog body
// copy -- no Subtitle styling -- vertically centered over the button row,
// instead of the default menu-subtitle look.
func (m *MenuModel) SetPlainTextStyle(themeTag string, vPad int) *MenuModel {
	m.plainTextThemeTag = themeTag
	m.plainTextVPad = vPad
	return m
}
