package tui

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderFlow renders items in a horizontal flow layout for compact menus
func (m *MenuModel) renderFlow() string {
	ctx := GetActiveContext()
	styles := GetStyles()
	dialogBG := styles.Dialog.GetBackground()

	// Use Layout helpers for consistent border calculations
	layout := GetLayout()
	maxWidth, _ := layout.InnerContentSize(m.width, m.height)
	// Subtract 2 for internal 1-char margin on each side (matching standard list menus)
	if maxWidth > 2 {
		maxWidth -= 2
	}

	var lines []string
	var currentLine []string
	currentLineWidth := 0
	itemSpacing := 3

	for i, item := range m.items {
		isSelected := i == m.cursor && m.IsActive()

		tagStyle := SemanticStyle("{{|Theme_Tag|}}")
		keyStyle := SemanticStyle("{{|Theme_TagKey|}}")

		if isSelected {
			tagStyle = SemanticStyle("{{|Theme_TagSelected|}}")
			keyStyle = SemanticStyle("{{|Theme_TagKeySelected|}}")
		}

		// Checkbox/Radio visual
		prefix := ""
		if item.IsRadioButton {
			var cb string
			if ctx.LineCharacters {
				cb = radioUnselected + " "
				if item.Checked {
					cb = radioSelected + " "
				}
			} else {
				cb = radioUnselectedAscii
				if item.Checked {
					cb = radioSelectedAscii
				}
			}
			prefix = tagStyle.Render(cb)
		} else if item.IsCheckbox {
			var cb string
			if ctx.LineCharacters {
				cb = checkUnselected + " "
				if item.Checked {
					cb = checkSelected + " "
				}
			} else {
				cb = checkUnselectedAscii
				if item.Checked {
					cb = checkSelectedAscii
				}
			}
			prefix = tagStyle.Render(cb)
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

		itemContent := prefix + tagStr

		// For non-checkbox/non-radio items (e.g. dropdowns), append
		// the Desc inline so the current value is visible without clicking.
		if !item.IsCheckbox && !item.IsRadioButton && item.Desc != "" {
			desc := item.Desc
			if isSelected {
				// Strip OptionValue tag so the value inherits selection colors (e.g. red background)
				desc = strings.ReplaceAll(desc, "{{|Theme_OptionValue|}}", "")
			}
			// Include leading space in RenderThemeText so it gets the correct background
			itemContent += RenderThemeText(" "+desc, tagStyle)
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

		itemWidth := cbWidth + lipgloss.Width(GetPlainText(item.Tag))

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
