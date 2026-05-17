package tui

import (
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// menuItemDelegate implements list.ItemDelegate for standard navigation menus
type menuItemDelegate struct {
	menuID              string
	maxTagLen           int
	focused             bool
	flowMode            bool
	showLockGutter      bool
	activityGutterWidth int
	paddingWidth        int
}

func (d menuItemDelegate) Height() int                             { return 1 }
func (d menuItemDelegate) Spacing() int                            { return 0 }
func (d menuItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d menuItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	ctx := GetActiveContext()
	dialogBG := ctx.Dialog.GetBackground()
	isSelected := index == m.Index()

	// Handle separator items
	if menuItem.IsSeparator {
		// Set width to exactly m.Width() so inner text of m.Width()-1 plus 1 char right padding fits perfectly
		lineStyle := lipgloss.NewStyle().Background(dialogBG).PaddingLeft(0).PaddingRight(1).Width(m.Width())
		var content string
		if menuItem.Tag != "" {
			content = RenderThemeText(menuItem.Tag, theme.ThemeSemanticStyle("{{|TagKey|}}"))
		} else {
			// -1 for right padding
			content = strutil.Repeat("─", m.Width()-1)
		}
		fmt.Fprint(w, lineStyle.Render(content))
		return
	}

	neutralStyle := lipgloss.NewStyle().Background(dialogBG)
	itemStyle := theme.ThemeSemanticStyle("{{|Item|}}")
	tagStyle := theme.ThemeSemanticStyle("{{|Tag|}}")
	keyStyle := theme.ThemeSemanticStyle("{{|TagKey|}}")

	if isSelected {
		itemStyle = theme.ThemeSemanticStyle("{{|ItemSelected|}}")
		tagStyle = theme.ThemeSemanticStyle("{{|TagSelected|}}")
		keyStyle = theme.ThemeSemanticStyle("{{|TagKeySelected|}}")
	}

	// Render tag with first-letter highlighting (if no semantic tags present)
	tag := menuItem.Tag
	var tagStr string
	if len(tag) > 0 {
		// If tag already contains theme tags, render it normally (highlights might be ruined)
		if strings.Contains(tag, "{{") {
			tagStr = RenderThemeText(tag, tagStyle)
		} else {
			runes := []rune(tag)
			letterIdx := 0
			if strings.HasPrefix(tag, "[") && len(runes) > 1 {
				letterIdx = 1
			}
			if letterIdx < len(runes) {
				tagStr = tagStyle.Render(string(runes[:letterIdx])) + keyStyle.Render(string(runes[letterIdx])) + tagStyle.Render(string(runes[letterIdx+1:]))
			} else {
				tagStr = RenderThemeText(tag, tagStyle)
			}
		}
	}

	// Whitespace (gaps and trailing) should always use neutral background
	gapStyle := neutralStyle

	// Construct fixed-width gutter
	itemGutter := RenderMenuGutter(menuItem, d.showLockGutter, d.activityGutterWidth, neutralStyle)
	paddingStr := neutralStyle.Render(strutil.Repeat(" ", d.paddingWidth))
	checkbox := ""
	if menuItem.Locked {
		var cb string
		if ctx.LineCharacters {
			cb = invalidMarker
		} else {
			cb = invalidMarkerAscii
		}
		// Render with tagStyle for consistency with other markers, and add a neutral space after it
		checkbox = tagStyle.Render(cb) + neutralStyle.Render(" ")
	} else if menuItem.IsInvalid {
		var cb string
		if ctx.LineCharacters {
			cb = invalidMarker
		} else {
			cb = invalidMarkerAscii
		}
		checkbox = theme.ThemeSemanticStyle("{{|MarkerInvalid|}}").Render(cb) + neutralStyle.Render(" ")
	} else if menuItem.IsRadioButton {
		var cb string
		if ctx.LineCharacters {
			cb = radioUnselected
			if menuItem.Checked {
				cb = radioSelected
			}
		} else {
			cb = radioUnselectedAscii
			if menuItem.Checked {
				cb = radioSelectedAscii
			}
		}
		// Render just the glyph with tagStyle, and add a neutral space after it
		checkbox = tagStyle.Render(cb) + neutralStyle.Render(" ")
	} else if menuItem.IsCheckbox {
		var cb string
		if ctx.LineCharacters {
			cb = checkUnselected
			if menuItem.Checked {
				cb = checkSelected
			}
		} else {
			cb = checkUnselectedAscii
			if menuItem.Checked {
				cb = checkSelectedAscii
			}
		}
		// Render just the glyph with tagStyle, and add a neutral space after it
		checkbox = tagStyle.Render(cb) + neutralStyle.Render(" ")
	}

	// Whitespace (gaps and trailing) should always use neutral background
	gapStyle = neutralStyle

	paddingSpaces := strutil.Repeat(" ", max(0, d.maxTagLen-lipgloss.Width(GetPlainText(tag))+3))

	// Calculate checkbox/radio width dynamically
	cbWidth := lipgloss.Width(GetPlainText(checkbox)) + lipgloss.Width(GetPlainText(itemGutter)) + d.paddingWidth

	// Available width: list width - right padding(1) - (cbWidth + maxTagLen + 3)
	availableWidth := m.Width() - 1 - (cbWidth + d.maxTagLen + 3)
	if availableWidth < 0 {
		availableWidth = 0
	}

	descStr := RenderThemeText(menuItem.Desc, itemStyle)
	// Use TruncateRight for proper truncation instead of MaxWidth which wraps
	descLine := TruncateRight(descStr, availableWidth)

	// Build the line
	line := itemGutter + paddingStr + checkbox + tagStr + gapStyle.Render(paddingSpaces) + descLine

	actualWidth := lipgloss.Width(line)
	// -1 for right padding
	if actualWidth < m.Width()-1 {
		line += gapStyle.Render(strutil.Repeat(" ", m.Width()-1-actualWidth))
	}

	lineStyle := lipgloss.NewStyle().Background(dialogBG).PaddingLeft(0).PaddingRight(1).Width(m.Width())
	line = lineStyle.Render(line)
	fmt.Fprint(w, line)

}

// checkboxItemDelegate implements specialized styling for app selection screens
type checkboxItemDelegate struct {
	menuID              string
	maxTagLen           int
	focused             bool
	flowMode            bool
	showLockGutter      bool
	activityGutterWidth int
	paddingWidth        int
}

func (d checkboxItemDelegate) Height() int                             { return 1 }
func (d checkboxItemDelegate) Spacing() int                            { return 0 }
func (d checkboxItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d checkboxItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	ctx := GetActiveContext()
	dialogBG := ctx.Dialog.GetBackground()
	isSelected := index == m.Index()

	if menuItem.IsSeparator {
		lineStyle := lipgloss.NewStyle().Background(dialogBG).PaddingLeft(0).PaddingRight(1).Width(m.Width())
		var content string
		if menuItem.Tag != "" {
			content = RenderThemeText(menuItem.Tag, theme.ThemeSemanticStyle("{{|TagKey|}}"))
		} else {
			content = strutil.Repeat("─", max(0, m.Width()-1))
		}
		fmt.Fprint(w, lineStyle.Render(content))
		return
	}

	neutralStyle := lipgloss.NewStyle().Background(dialogBG)

	// Construct fixed-width gutter (standard columns: [LockChar][ActivityChar1][ActivityChar2])
	itemGutter := RenderMenuGutter(menuItem, d.showLockGutter, d.activityGutterWidth, neutralStyle)
	paddingStr := neutralStyle.Render(strutil.Repeat(" ", d.paddingWidth))

	itemStyle := theme.ThemeSemanticStyle("{{|Item|}}")
	tagStyle := theme.ThemeSemanticStyle("{{|Tag|}}")
	keyStyle := theme.ThemeSemanticStyle("{{|TagKey|}}")

	if isSelected {
		itemStyle = theme.ThemeSemanticStyle("{{|ItemSelected|}}")
		tagStyle = theme.ThemeSemanticStyle("{{|TagSelected|}}")
		keyStyle = theme.ThemeSemanticStyle("{{|TagKeySelected|}}")
	}

	// Render checkbox for selectable items
	var checkbox string
	if menuItem.Locked {
		var cb string
		if ctx.LineCharacters {
			cb = invalidMarker
		} else {
			cb = invalidMarkerAscii
		}
		checkbox = tagStyle.Render(cb) + neutralStyle.Render(" ")
	} else if menuItem.IsInvalid {
		var cb string
		if ctx.LineCharacters {
			cb = invalidMarker
		} else {
			cb = invalidMarkerAscii
		}
		checkbox = theme.ThemeSemanticStyle("{{|MarkerInvalid|}}").Render(cb) + neutralStyle.Render(" ")
	} else if menuItem.IsCheckbox {
		if ctx.LineCharacters {
			cbGlyph := checkUnselected
			if menuItem.Checked {
				cbGlyph = checkSelected
			}
			// Use tag style for checkbox to match user request
			// Render just the glyph with tagStyle, and add a neutral space after it
			checkbox = tagStyle.Render(cbGlyph) + neutralStyle.Render(" ")
		} else {
			cbContent := checkUnselectedAscii
			if menuItem.Checked {
				cbContent = checkSelectedAscii
			}
			// Use tag style for checkbox to match user request
			checkbox = tagStyle.Render(cbContent) + neutralStyle.Render(" ")
		}
	} else if menuItem.IsRadioButton {
		if ctx.LineCharacters {
			cbGlyph := radioUnselected
			if menuItem.Checked {
				cbGlyph = radioSelected
			}
			checkbox = tagStyle.Render(cbGlyph) + neutralStyle.Render(" ")
		} else {
			cbContent := radioUnselectedAscii
			if menuItem.Checked {
				cbContent = radioSelectedAscii
			}
			checkbox = tagStyle.Render(cbContent) + neutralStyle.Render(" ")
		}
	}

	var tagStr string
	tag := menuItem.Tag
	if len(tag) > 0 {
		if isSelected {
			tagStr = RenderThemeText(tag, tagStyle)
		} else {
			if strings.Contains(tag, "{{") {
				tagStr = RenderThemeText(tag, tagStyle)
			} else {
				runes := []rune(tag)
				letterIdx := 0
				if strings.HasPrefix(tag, "[") && len(runes) > 1 {
					letterIdx = 1
				}
				if letterIdx < len(runes) {
					tagStr = tagStyle.Render(string(runes[:letterIdx])) + keyStyle.Render(string(runes[letterIdx])) + tagStyle.Render(string(runes[letterIdx+1:]))
				} else {
					tagStr = RenderThemeText(tag, tagStyle)
				}
			}
		}
	}

	// tagWidth removed as it was unused

	// Highlighting for gap and description
	// Use itemStyle as base for description so highlight applies, or dialogBG if not selected
	descStyle := lipgloss.NewStyle().Background(dialogBG)
	if isSelected {
		descStyle = itemStyle
	}
	// Whitespace (gaps and trailing) should always use neutral background
	gapStyle := neutralStyle

	paddingSpaces := strutil.Repeat(" ", max(0, d.maxTagLen-lipgloss.Width(GetPlainText(tag))+3))
	// Calculate checkbox/radio width dynamically
	cbWidth := lipgloss.Width(GetPlainText(checkbox)) + lipgloss.Width(GetPlainText(itemGutter)) + d.paddingWidth

	// Available width: list width - right padding(1) - (cbWidth + maxTagLen + 3)
	availableWidth := m.Width() - 1 - (cbWidth + d.maxTagLen + 3)
	if availableWidth < 0 {
		availableWidth = 0
	}

	descStr := RenderThemeText(menuItem.Desc, descStyle)
	// Use TruncateRight for proper truncation instead of MaxWidth which wraps
	descLine := TruncateRight(descStr, availableWidth)

	// Build the line
	line := itemGutter + paddingStr + checkbox + tagStr + gapStyle.Render(paddingSpaces) + descLine

	actualWidth := lipgloss.Width(line)
	// -1 for right padding
	if actualWidth < m.Width()-1 {
		line += gapStyle.Render(strutil.Repeat(" ", m.Width()-1-actualWidth))
	}

	lineStyle := lipgloss.NewStyle().Background(dialogBG).PaddingLeft(0).PaddingRight(1).Width(m.Width())
	line = lineStyle.Render(line)
	fmt.Fprint(w, line)

}

// groupedItemDelegate renders the hierarchical app-selection list:
//   - IsGroupHeader rows: app label with read-only group-enabled checkbox + description
//   - IsSubItem rows:     indented instance checkbox rows
//   - IsAddInstance rows: indented "[+] Add instance…" action label
//   - IsEditing rows:     indented inline text-input display (Tag holds current text + cursor)
//   - IsSeparator rows:   unchanged (letter headers / blank spacers)
type groupedItemDelegate struct {
	maxTagLen           int // max tag width of header rows only
	focused             bool
	activeCol           CheckboxColumn
	showLockGutter      bool
	activityGutterWidth int
}

func (d groupedItemDelegate) Height() int                             { return 1 }
func (d groupedItemDelegate) Spacing() int                            { return 0 }
func (d groupedItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d groupedItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	ctx := GetActiveContext()
	dialogBG := ctx.Dialog.GetBackground()
	isSelected := index == m.Index()

	// Highlight the parent header if a child item is selected
	isParentOfSelected := false
	if menuItem.IsGroupHeader {
		if selItemRaw := m.SelectedItem(); selItemRaw != nil {
			if selItem, ok := selItemRaw.(MenuItem); ok {
				if (selItem.IsSubItem || selItem.IsAddInstance || selItem.IsEditing) && selItem.BaseApp == menuItem.BaseApp {
					isParentOfSelected = true
				}
			}
		}
	}

	// Separator rows (letter headers and blank spacers)
	if menuItem.IsSeparator {
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		var content string
		if menuItem.Tag != "" {
			content = RenderThemeText(menuItem.Tag, theme.ThemeSemanticStyle("{{|TagKey|}}"))
		} else {
			content = strutil.Repeat("─", max(0, m.Width()-2))
		}
		fmt.Fprint(w, lineStyle.Render(content))
		return
	}

	neutralStyle := lipgloss.NewStyle().Background(dialogBG)
	tagStyle := theme.ThemeSemanticStyle("{{|Tag|}}")
	itemStyle := theme.ThemeSemanticStyle("{{|Item|}}")
	keyStyle := theme.ThemeSemanticStyle("{{|TagKey|}}")
	if isSelected || isParentOfSelected {
		tagStyle = theme.ThemeSemanticStyle("{{|TagSelected|}}")
		itemStyle = theme.ThemeSemanticStyle("{{|ItemSelected|}}")
		keyStyle = theme.ThemeSemanticStyle("{{|TagKeySelected|}}")
	}
	descStyle := lipgloss.NewStyle().Background(dialogBG)
	if isSelected || isParentOfSelected {
		descStyle = itemStyle
	}

	subIndent := "    " // 4 spaces for sub-items

	// "[+] Add instance…" row — rendered like a standard unchecked sub-item checkbox
	if menuItem.IsAddInstance {
		var cb string
		if ctx.LineCharacters {
			cb = checkUnselected
		} else {
			cb = checkUnselectedAscii
		}
		cbStr := tagStyle.Render(cb) + neutralStyle.Render(" ")
		subTagStr := ""
		if len(menuItem.Tag) > 0 {
			runes := []rune(menuItem.Tag)
			subTagStr = keyStyle.Render(string(runes[0])) + tagStyle.Render(string(runes[1:]))
		}
		line := neutralStyle.Render(" ") + neutralStyle.Render(subIndent) + cbStr + subTagStr
		actualWidth := lipgloss.Width(line)
		if actualWidth < m.Width()-2 {
			line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
		}
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		fmt.Fprint(w, lineStyle.Render(line))
		return
	}

	// Inline editing row — Tag holds "SUFFIX▌" or "SUFFIX▌ {{|StatusError|}}msg{{[-]}}"
	if menuItem.IsEditing {
		cbStr := ""
		if menuItem.IsCheckbox {
			var cb string
			if ctx.LineCharacters {
				if menuItem.Checked {
					cb = checkSelected
				} else {
					cb = checkUnselected
				}
			} else {
				if menuItem.Checked {
					cb = checkSelectedAscii
				} else {
					cb = checkUnselectedAscii
				}
			}
			cbStr = tagStyle.Render(cb) + neutralStyle.Render(" ")
		}
		rendered := RenderThemeText(menuItem.Tag, descStyle)
		line := neutralStyle.Render(" ") + neutralStyle.Render(subIndent) + cbStr + rendered
		actualWidth := lipgloss.Width(line)
		if actualWidth < m.Width()-2 {
			line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
		}
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		fmt.Fprint(w, lineStyle.Render(line))
		return
	}

	// Gutter: 2 chars on left edge.
	// Render using the unified helper which respects StatusGutterWidth
	itemGutter := RenderMenuGutter(menuItem, d.showLockGutter, d.activityGutterWidth, neutralStyle)

	// buildCb3 renders a 3-character wide checkbox block with a fixed style.
	buildCb3 := func(checked bool, cbStyle lipgloss.Style) string {
		if ctx.LineCharacters {
			g := checkUnselected
			if checked {
				g = checkSelected
			}
			// Draw with the chosen style (Red for focused, Blue for neutral)
			// Each block is exactly 3 chars: [space][glyph][space]
			inner := cbStyle.Render(g)
			// Apply the checkbox's background color to the surrounding spaces
			bgStyle := lipgloss.NewStyle().Background(cbStyle.GetBackground())
			return bgStyle.Render(" ") + inner + bgStyle.Render(" ")
		}

		content := "[ ]"
		if checked {
			content = "[x]"
		}
		return cbStyle.Render(content)
	}

	// Group headers never show checkboxes — just the disclosure glyph.
	// IsSubItem and IsCheckbox rows use the full two-checkbox layout.
	if menuItem.IsSubItem {
		addStyle := neutralStyle
		enableStyle := neutralStyle
		if isSelected {
			switch d.activeCol {
			case ColAdd:
				addStyle = tagStyle
			case ColEnable:
				enableStyle = tagStyle
			}
		}

		cbAdd := buildCb3(menuItem.Checked, addStyle)
		cbEnabled := buildCb3(menuItem.Enabled, enableStyle)
		tagStr := RenderThemeText(menuItem.Tag, tagStyle)
		// Layout matches border: Gutter(2) + cbAdd(3) + Spacer(1) + cbEnabled(3) + Spacer(1) + Tag.
		line := itemGutter + cbAdd + neutralStyle.Render(" ") + cbEnabled + neutralStyle.Render(" ") + tagStr
		actualWidth := lipgloss.Width(line)
		if actualWidth < m.Width()-2 {
			line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
		}
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Width(m.Width())
		fmt.Fprint(w, lineStyle.Render(line))
		return
	}

	// IsGroupHeader: expansion arrows + name.
	if menuItem.IsGroupHeader {
		tag := menuItem.Tag
		var tagStr string
		if strings.Contains(tag, "{{") {
			tagStr = RenderThemeText(tag, tagStyle)
		} else {
			firstLetter := string([]rune(tag)[0])
			rest := string([]rune(tag)[1:])
			tagStr = keyStyle.Render(firstLetter) + tagStyle.Render(rest)
		}

		// Disclosure arrow glyph (▼ or [v])
		var disclosureGlyph string
		if ctx.LineCharacters {
			disclosureGlyph = subMenuExpanded
		} else {
			disclosureGlyph = subMenuExpandedAscii
		}

		// Styled arrows for Add and Enable columns
		// As per previous Turn, only the arrow in the 'E' column is shown.
		// However, to support 'Add' toggle on groups if needed, we define both.
		cbStyle := theme.ThemeSemanticStyle("{{|TitleCheckbox|}}")
		cbAdd := neutralStyle.Render("   ")
		if menuItem.IsCheckbox {
			s := cbStyle
			if isSelected && d.activeCol == ColAdd {
				s = tagStyle
			}
			if ctx.LineCharacters {
				cbAdd = neutralStyle.Render(" ") + s.Render(disclosureGlyph) + neutralStyle.Render(" ")
			} else {
				cbAdd = s.Render(disclosureGlyph)
			}
		}

		cbEnabled := neutralStyle.Render("   ")
		if menuItem.ShowEnabledGutter {
			s := cbStyle
			if isSelected && d.activeCol == ColEnable {
				s = tagStyle
			}
			if ctx.LineCharacters {
				cbEnabled = neutralStyle.Render(" ") + s.Render(disclosureGlyph) + neutralStyle.Render(" ")
			} else {
				cbEnabled = s.Render(disclosureGlyph)
			}
		}

		paddingSpaces := strutil.Repeat(" ", max(0, d.maxTagLen-lipgloss.Width(GetPlainText(tag))+3))
		prefixW := 11
		availableWidth := m.Width() - 2 - prefixW - (d.maxTagLen + 3)
		if availableWidth < 0 {
			availableWidth = 0
		}
		descStr := RenderThemeText(menuItem.Desc, descStyle)
		descLine := TruncateRight(descStr, availableWidth)

		// Layout restores the dual or single arrow look as it was before.
		// Layout matches border: Gutter(2) + cbAdd(3) + Spacer(1) + cbEnabled(3) + Spacer(1) + Tag.
		line := itemGutter + cbAdd + neutralStyle.Render(" ") + cbEnabled + neutralStyle.Render(" ") + tagStr + neutralStyle.Render(paddingSpaces) + descLine

		actualWidth := lipgloss.Width(line)
		if actualWidth < m.Width()-2 {
			line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
		}
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Width(m.Width())
		fmt.Fprint(w, lineStyle.Render(line))
		return
	}

	// IsCheckbox simple row: g0 g1  [cb_add]  [cb_enabled]  AppName  Desc
	tag := menuItem.Tag
	var tagStr string
	if strings.Contains(tag, "{{") {
		tagStr = RenderThemeText(tag, tagStyle)
	} else {
		firstLetter := string([]rune(tag)[0])
		rest := string([]rune(tag)[1:])
		tagStr = keyStyle.Render(firstLetter) + tagStyle.Render(rest)
	}
	addStyle := neutralStyle
	enableStyle := neutralStyle
	if isSelected {
		switch d.activeCol {
		case ColAdd:
			addStyle = tagStyle
		case ColEnable:
			enableStyle = tagStyle
		}
	}

	cbAdd := buildCb3(menuItem.Checked, addStyle)
	cbEnabled := buildCb3(menuItem.Enabled, enableStyle)

	// Layout matches border: Gutter(2) + cbAdd(3) + Spacer(1) + cbEnabled(3) + Spacer(1) + Tag.
	line := itemGutter + cbAdd + neutralStyle.Render(" ") + cbEnabled + neutralStyle.Render(" ") + tagStr

	const rowPrefixW = 11
	paddingSpaces := strutil.Repeat(" ", max(0, d.maxTagLen-lipgloss.Width(GetPlainText(tag))+3))
	availableWidth := m.Width() - 2 - rowPrefixW - (d.maxTagLen + 3)
	if availableWidth < 0 {
		availableWidth = 0
	}
	descStr := RenderThemeText(menuItem.Desc, descStyle)
	descLine := TruncateRight(descStr, availableWidth)

	line = line + neutralStyle.Render(paddingSpaces) + descLine
	actualWidth := lipgloss.Width(line)
	if actualWidth < m.Width()-2 {
		line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
	}
	lineStyle := lipgloss.NewStyle().Background(dialogBG).Width(m.Width())
	fmt.Fprint(w, lineStyle.Render(line))
}
