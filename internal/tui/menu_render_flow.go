package tui

import (
	"DockSTARTer2/internal/console"
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

		if isSelected {
			tagStyle = theme.ThemeSemanticStyle("{{|TagSelected|}}")
			keyStyle = theme.ThemeSemanticStyle("{{|TagKeySelected|}}")
		}

		neutralStyle := lipgloss.NewStyle().Background(dialogBG)

		// Checkbox/Radio visual
		prefix := ""
		if item.IsRadioButton {
			var cb string
			if ctx.LineCharacters {
				cb = radioUnselected
				if item.Checked {
					cb = radioSelected
				}
			} else {
				cb = strings.TrimRight(radioUnselectedAscii, " ")
				if item.Checked {
					cb = strings.TrimRight(radioSelectedAscii, " ")
				}
			}
			prefix = tagStyle.Render(cb) + neutralStyle.Render(" ")
		} else if item.IsCheckbox {
			var cb string
			if ctx.LineCharacters {
				cb = checkUnselected
				if item.Checked {
					cb = checkSelected
				}
			} else {
				cb = strings.TrimRight(checkUnselectedAscii, " ")
				if item.Checked {
					cb = strings.TrimRight(checkSelectedAscii, " ")
				}
			}
			prefix = tagStyle.Render(cb) + neutralStyle.Render(" ")
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

		lockMarker := ""
		gutterWidth := m.StatusGutterWidth()
		if m.showLockGutter {
			if item.Locked {
				lockMarker = RenderThemeText("{{|MarkerLocked|}}!{{[-]}}", lipgloss.NewStyle().Background(dialogBG))
			} else {
				lockMarker = lipgloss.NewStyle().Background(dialogBG).Render(strings.Repeat(" ", gutterWidth))
			}
		}

		itemContent := lockMarker + prefix + tagStr

		// For non-checkbox/non-radio items (e.g. dropdowns), append the value inline.
		// Neutral space (dialogBG) breaks the selection background color in the gap only.
		if !item.IsCheckbox && !item.IsRadioButton && item.Desc != "" {
			if isSelected {
				// Strip theme tags so OptionValue color doesn't override tagStyle (selection).
				itemContent += neutralStyle.Render(" ") + tagStyle.Render(GetPlainText(item.Desc))
			} else {
				itemContent += RenderThemeText(" "+item.Desc, tagStyle)
			}
		}

		// Hard reset after each element to ensure background colors (like selection)
		// don't bleed into the itemSpacing gaps.
		itemContent += console.CodeReset

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

// GetFlowHeight calculates required lines for horizontal layout given the available width
func (m *MenuModel) GetFlowHeight(width int) int {
	if len(m.items) == 0 {
		return 0
	}

	ctx := GetActiveContext()

	maxWidth := width
	// Subtract 2 for borders and 2 for internal 1-char margins (matching standard list menus)
	if maxWidth > 4 {
		maxWidth -= 4
	}

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
					glyph = radioUnselected + " "
				} else {
					glyph = checkUnselected + " "
				}
			} else {
				if item.IsRadioButton {
					glyph = radioUnselectedAscii
				} else {
					glyph = checkUnselectedAscii
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
