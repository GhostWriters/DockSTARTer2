package classic

import (
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	semstyle "github.com/GhostWriters/semstyle/lg"
	"strings"

	"charm.land/lipgloss/v2"
)

// FlowItemSpacing is the gap between adjacent items on a flow-mode row.
// Shared by renderFlowContent, GetFlowHeight, and HitRegions' flow-mode hit
// testing -- all three must agree or wrapping/hit-boxes desync from the
// actual render.
const FlowItemSpacing = 2

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
	if m.FlowColumns >= 2 {
		return m.renderColumnContent(maxWidth, m.FlowColumns)
	}

	ctx := GetActiveContext()
	styles := GetStyles()
	dialogBG := styles.Dialog.GetBackground()

	var lines []string
	var currentLine []string
	currentLineWidth := 0
	itemSpacing := FlowItemSpacing

	for i, item := range m.items {
		if item.IsSeparator {
			continue
		}
		isSelected := i == m.cursor && m.IsListActive()
		isDisabled := m.disabled || item.Locked

		var tagStyle, keyStyle lipgloss.Style
		switch {
		case isDisabled && isSelected:
			tagStyle, _ = ResolveDisabledStyle("TagFocused")
			keyStyle, _ = ResolveDisabledStyle("TagKeyFocused")
		case isDisabled:
			tagStyle, _ = ResolveDisabledStyle("Tag")
			keyStyle, _ = ResolveDisabledStyle("TagKey")
		case isSelected:
			tagStyle = theme.ThemeSemanticStyle("{{|TagFocused|}}")
			keyStyle = theme.ThemeSemanticStyle("{{|TagKeyFocused|}}")
		default:
			tagStyle = theme.ThemeSemanticStyle("{{|Tag|}}")
			keyStyle = theme.ThemeSemanticStyle("{{|TagKey|}}")
		}

		neutralStyle := lipgloss.NewStyle().Background(dialogBG)

		// Checkbox/Radio visual
		prefix := ""
		if item.IsRadioButton || item.IsCheckbox {
			// Flow/grid lists always keep their brackets, regardless of focus,
			// but the color still follows real keyboard focus (isSelected).
			content, bracket := checkboxStylePair(item.IsRadioButton, item.Checked, isSelected, isDisabled)
			prefix = renderCheckbox(item.IsRadioButton, item.Checked, ctx.LineCharacters, true, "always", content, bracket) + neutralStyle.Render(" ")
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
			} else if item.IsNew {
				// Same transient "changed" indicator as App Select's
				// just-added/renamed marker, reusing the same field and glyph.
				arrow := subMenuCollapsed
				if !ctx.LineCharacters {
					arrow = subMenuCollapsedAscii
				}
				lockChar = RenderThemeText("{{|MarkerAdded|}}"+arrow+"{{[-]}}", neutralStyle)
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

// columnModeVisibleRows returns the [startRow, endRow) window of column-mode
// rows actually drawn, given the total row count and the menu's current
// MaxFlowRows/ViewStartY scroll state. Used by both renderColumnContent and
// its hit-region counterpart so a scrolled column-mode menu can't render one
// window while hit-testing against a different one.
func columnModeVisibleRows(rows, maxFlowRows, viewStartY int) (startRow, endRow int) {
	if maxFlowRows <= 0 {
		return 0, rows
	}
	startRow = viewStartY
	if startRow > rows-maxFlowRows {
		startRow = rows - maxFlowRows
	}
	if startRow < 0 {
		startRow = 0
	}
	endRow = startRow + maxFlowRows
	if endRow > rows {
		endRow = rows
	}
	return startRow, endRow
}

// renderColumnContent renders items in N balanced vertical columns.
// Column widths are determined by the widest item in each column.
func (m *MenuModel) renderColumnContent(maxWidth, numCols int) string {
	ctx := GetActiveContext()
	styles := GetStyles()
	dialogBG := styles.Dialog.GetBackground()
	neutralStyle := lipgloss.NewStyle().Background(dialogBG)
	colGap := 2

	// index of non-separator items in m.items
	var itemIndices []int
	for i, item := range m.items {
		if !item.IsSeparator {
			itemIndices = append(itemIndices, i)
		}
	}

	n := len(itemIndices)
	if n == 0 {
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(maxWidth + 2)
		return lineStyle.Render("")
	}

	// Distribute items into columns top-to-bottom.
	rows := (n + numCols - 1) / numCols

	// Measure plain-text width of each item (without selection styling).
	itemWidths := make([]int, n)
	for ni, ii := range itemIndices {
		item := m.items[ii]
		cbWidth := 0
		if item.IsRadioButton || item.IsCheckbox {
			glyph := radioOffAscii + " "
			if ctx.LineCharacters {
				glyph = radioOff + " "
			}
			cbWidth = lipgloss.Width(glyph)
		}
		itemWidths[ni] = cbWidth + lipgloss.Width(GetPlainText(item.Tag))
	}

	// Find the widest item in each column.
	colWidths := make([]int, numCols)
	for col := 0; col < numCols; col++ {
		for row := 0; row < rows; row++ {
			ni := col*rows + row
			if ni >= n {
				break
			}
			if itemWidths[ni] > colWidths[col] {
				colWidths[col] = itemWidths[ni]
			}
		}
	}

	// Clip to viewport when maxFlowRows is set.
	startRow, endRow := columnModeVisibleRows(rows, m.MaxFlowRows, m.ViewStartY)

	// Build each row: render each item then pad to its column width.
	var lines []string
	for row := startRow; row < endRow; row++ {
		var parts []string
		for col := 0; col < numCols; col++ {
			ni := col*rows + row
			colW := colWidths[col]
			if ni >= n {
				// Empty cell — fill with neutral background to keep columns aligned.
				parts = append(parts, neutralStyle.Width(colW).Render(""))
				continue
			}
			ii := itemIndices[ni]
			item := m.items[ii]
			isSelected := ii == m.cursor && m.IsListActive()
			isDisabled := m.disabled || item.Locked

			var tagStyle, keyStyle lipgloss.Style
			switch {
			case isDisabled && isSelected:
				tagStyle, _ = ResolveDisabledStyle("TagFocused")
				keyStyle, _ = ResolveDisabledStyle("TagKeyFocused")
			case isDisabled:
				tagStyle, _ = ResolveDisabledStyle("Tag")
				keyStyle, _ = ResolveDisabledStyle("TagKey")
			case isSelected:
				tagStyle = theme.ThemeSemanticStyle("{{|TagFocused|}}")
				keyStyle = theme.ThemeSemanticStyle("{{|TagKeyFocused|}}")
			default:
				tagStyle = theme.ThemeSemanticStyle("{{|Tag|}}")
				keyStyle = theme.ThemeSemanticStyle("{{|TagKey|}}")
			}

			prefix := ""
			if item.IsRadioButton || item.IsCheckbox {
				// Flow/grid lists always keep their brackets, regardless of focus,
				// but the color still follows real keyboard focus (isSelected).
				content, bracket := checkboxStylePair(item.IsRadioButton, item.Checked, isSelected, isDisabled)
				prefix = renderCheckbox(item.IsRadioButton, item.Checked, ctx.LineCharacters, true, "always", content, bracket) + neutralStyle.Render(" ")
			}

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

			content := prefix + tagStr + semstyle.CodeReset
			// Pad to column width with neutral background so hit zone is exact.
			pad := colW - itemWidths[ni]
			if pad > 0 {
				content += neutralStyle.Render(strutil.Repeat(" ", pad))
			}
			parts = append(parts, content)
		}
		// Join columns with a neutral gap.
		lines = append(lines, strings.Join(parts, neutralStyle.Render(strutil.Repeat(" ", colGap))))
	}

	scrolling := m.MaxFlowRows > 0 && rows > m.MaxFlowRows
	visibleRows := endRow - startRow

	// The lineStyle width must match maxWidth exactly (the outer border's content width).
	// Padding(0,1) adds 1 char on each side, so inner content = maxWidth - 2.
	// When scrolling, reserve 1 more char on the right for the scrollbar — inner = maxWidth - 3.
	lineW := maxWidth
	if scrolling {
		lineW = maxWidth - ScrollbarGutterWidth
	}

	lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(lineW)
	for i, line := range lines {
		lines[i] = lineStyle.Render(line)
	}

	if !scrolling {
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	// Render scrollbar column (1 char wide) and append to each content line.
	sbStr := m.Scroll.Render(visibleRows, rows, visibleRows, startRow, ctx.LineCharacters, ctx)
	sbLines := strings.Split(sbStr, "\n")
	blank := neutralStyle.Render(" ")
	for len(sbLines) < visibleRows {
		sbLines = append(sbLines, blank)
	}

	for i, line := range lines {
		sb := blank
		if i < len(sbLines) {
			sb = sbLines[i]
		}
		lines[i] = line + sb
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// GetFlowHeight calculates required lines for the given maxWidth.
// maxWidth must already be the usable content width (same value passed to renderFlowContent).
func (m *MenuModel) GetFlowHeight(maxWidth int) int {
	if len(m.items) == 0 {
		return 0
	}

	if m.FlowColumns >= 2 {
		// Count non-separator items and divide into columns.
		n := 0
		for _, item := range m.items {
			if !item.IsSeparator {
				n++
			}
		}
		rows := (n + m.FlowColumns - 1) / m.FlowColumns
		if m.MaxFlowRows > 0 && rows > m.MaxFlowRows {
			return m.MaxFlowRows
		}
		return rows
	}

	ctx := GetActiveContext()

	lines := 1
	currentLineWidth := 0
	itemSpacing := FlowItemSpacing

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
