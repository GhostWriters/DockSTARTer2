package tui

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
	m.SetSubMenuMode(true)
	m.SetVariableHeight(false)
	m.SetIsDialog(false)
	m.SetButtons([]ButtonDef{})
	m.SetMaximized(true)
	m.SetShowLockGutter(false)
	m.SetNoLeftMargin(true)
	return m
}
