package tui

import (
	"DockSTARTer2/internal/semstyle"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderFlow renders items in a horizontal flow layout for compact menus
func (m *MenuModel) renderFlow() string {
	layout := GetLayout()
	maxWidth, _ := layout.InnerContentSize(m.width, m.height)
	// Subtract 2 for internal 1-char margin on each side (matching standard list menus)
	if maxWidth > 2 {
		maxWidth -= 2
	}
	return m.renderFlowContent(maxWidth)
}

// renderFlowContent renders the flow items at the given content width.
// Used by both renderFlow (standalone) and viewSubMenu (subMenu+flow combination).
func (m *MenuModel) renderFlowContent(maxWidth int) string {
	ctx := GetActiveContext()
	styles := GetStyles()
	dialogBG := styles.Dialog.GetBackground()

	var lines []string
	var currentLine []string
	currentLineWidth := 0
	itemSpacing := 3

	for i, item := range m.items {
		if item.IsSeparator {
			continue
		}
		isSelected := i == m.cursor && m.IsActive()

		tagStyle := theme.ThemeSemanticStyle("{{|Tag|}}")
		keyStyle := theme.ThemeSemanticStyle("{{|TagKey|}}")
		checkboxStyle := theme.ThemeSemanticStyle("{{|Checkbox|}}")

		if isSelected {
			tagStyle = theme.ThemeSemanticStyle("{{|TagFocused|}}")
			keyStyle = theme.ThemeSemanticStyle("{{|TagKeyFocused|}}")
			checkboxStyle = theme.ThemeSemanticStyle("{{|CheckboxFocused|}}")
		}

		neutralStyle := lipgloss.NewStyle().Background(dialogBG)

		// Checkbox/Radio visual
		prefix := ""
		if item.IsRadioButton || item.IsCheckbox {
			prefix = renderCheckbox(item.IsRadioButton, item.Checked, ctx.LineCharacters, checkboxStyle) + neutralStyle.Render(" ")
		}

		// Tag with first-letter shortcut
		tag := item.Tag
		tagStr := ""
		if len(tag) > 0 {
			letterIdx := 0
			if strings.HasPrefix(tag, "[") && len(tag) > 1 {
				letterIdx = 1
			}
			p := tag[:letterIdx]
			f := string(tag[letterIdx])
			r := tag[letterIdx+1:]
			tagStr = tagStyle.Render(p) + keyStyle.Render(f) + tagStyle.Render(r)
		}

		itemGutter := ""
		if m.showLockGutter {
			lockChar := ""
			if item.IsInvalid {
				lockChar = RenderThemeText("{{|MarkerInvalid|}}"+invalidMarker+"{{[-]}}", neutralStyle)
			} else if item.Locked {
				lockChar = RenderThemeText("{{|MarkerLocked|}}!{{[-]}}", neutralStyle)
			} else {
				lockChar = neutralStyle.Render(" ")
			}
			itemGutter = lockChar
		}

		if m.activityGutterWidth >= 1 {
			// For flow menus, we reserve space for activity but dont typically show it
			itemGutter += neutralStyle.Render(strutil.Repeat(" ", m.activityGutterWidth))
		}

		itemContent := itemGutter + prefix + tagStr

		// For non-checkbox/non-radio items (e.g. dropdowns), append the value inline.
		// Neutral space (dialogBG) breaks the selection background color in the gap only.
		if !item.IsCheckbox && !item.IsRadioButton && item.Desc != "" {
			if isSelected {
				itemContent += neutralStyle.Render(" ") + ctx.OptionValueFocused.Render(GetPlainText(item.Desc))
			} else {
				// Neutral space breaks the tag background before the value color starts.
				itemContent += neutralStyle.Render(" ") + RenderThemeText(item.Desc, neutralStyle)
			}
		}

		// Hard reset after each element to ensure background colors (like selection)
		// don't bleed into the itemSpacing gaps.
		itemContent += semstyle.CodeReset

		itemWidth := lipgloss.Width(GetPlainText(itemContent))

		// Check if we need to wrap
		if currentLineWidth > 0 && currentLineWidth+itemSpacing+itemWidth > maxWidth {
			lines = append(lines, strings.Join(currentLine, strutil.Repeat(" ", itemSpacing)))
			currentLine = []string{itemContent}
			currentLineWidth = itemWidth
		} else {
			currentLine = append(currentLine, itemContent)
			if currentLineWidth > 0 {
				currentLineWidth += itemSpacing
			}
			currentLineWidth += itemWidth
		}
	}

	// Add final line
	if len(currentLine) > 0 {
		lines = append(lines, strings.Join(currentLine, strutil.Repeat(" ", itemSpacing)))
	}

	// Apply 1-char side margins to match MenuItemDelegate.Render
	lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(maxWidth + 2)
	for i, line := range lines {
		lines[i] = lineStyle.Render(line)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// GetFlowHeight calculates required lines for the given maxWidth.
// maxWidth must already be the usable content width (same value passed to renderFlowContent).
func (m *MenuModel) GetFlowHeight(maxWidth int) int {
	if len(m.items) == 0 {
		return 0
	}

	ctx := GetActiveContext()

	lines := 1
	currentLineWidth := 0
	itemSpacing := 3

	for _, item := range m.items {
		if item.IsSeparator {
			continue
		}
		// Dynamic width calculation
		cbWidth := 0
		if item.IsRadioButton || item.IsCheckbox {
			glyph := ""
			if ctx.LineCharacters {
				if item.IsRadioButton {
					glyph = radioOff + " "
				} else {
					glyph = checkOff + " "
				}
			} else {
				// ASCII glyphs also get a trailing space (matching renderFlowContent's
				// `renderCheckbox(...) + neutralStyle.Render(" ")` which adds the space).
				if item.IsRadioButton {
					glyph = radioOffAscii + " "
				} else {
					glyph = checkOffAscii + " "
				}
			}
			cbWidth = lipgloss.Width(glyph)
		}

		lockMarkerWidth := 0
		if m.showLockGutter {
			lockMarkerWidth = m.StatusGutterWidth()
		}

		itemWidth := lockMarkerWidth + cbWidth + lipgloss.Width(GetPlainText(item.Tag))

		// For non-checkbox/non-radio items, include the Desc width
		// to match renderFlow which appends Desc inline
		if !item.IsCheckbox && !item.IsRadioButton && item.Desc != "" {
			itemWidth += 1 + lipgloss.Width(GetPlainText(item.Desc))
		}

		if currentLineWidth > 0 && currentLineWidth+itemSpacing+itemWidth > maxWidth {
			lines++
			currentLineWidth = itemWidth
		} else {
			if currentLineWidth > 0 {
				currentLineWidth += itemSpacing
			}
			currentLineWidth += itemWidth
		}
	}

	return lines
}
