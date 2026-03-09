package tui

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderVariableHeightList renders items vertically with dynamic heights for word wrapping
func (m *MenuModel) renderVariableHeightList() string {
	ctx := GetActiveContext()

	// Memoization Check
	if m.lastListView != "" &&
		m.lastWidth == m.width &&
		m.lastHeight == m.height &&
		m.lastIndex == m.list.Index() &&
		m.lastFilter == m.list.FilterValue() &&
		m.lastActive == m.IsActive() &&
		m.lastLineChars == ctx.LineCharacters {
		return m.lastListView
	}

	styles := GetStyles()
	dialogBG := styles.Dialog.GetBackground()

	// Available width for content
	layout := GetLayout()
	maxWidth, _ := layout.InnerContentSize(m.width, m.height)
	if maxWidth > 2 {
		maxWidth -= 2
	}
	maxHeight := m.layout.ViewportHeight
	if maxHeight < 1 {
		maxHeight = 1
	}

	// Always reserve 1 char on the right for the scrollbar/padding column.
	// This slot is a space when scrollbar is disabled, and track/thumb chars when enabled.
	// Keeping it constant prevents layout jumps and fixes description overflow.
	scrollbar := currentConfig.UI.Scrollbar
	listContentWidth := maxWidth
	if listContentWidth > scrollbarGutterWidth+2 {
		listContentWidth -= scrollbarGutterWidth
	}

	// Filter items manually to match list state
	filter := m.list.FilterValue()
	var visibleItems []MenuItem
	var selectedVisibleIndex int = -1

	filteredCount := 0
	for _, item := range m.items {
		if filter != "" && !strings.Contains(strings.ToLower(item.Tag), strings.ToLower(filter)) {
			continue
		}
		visibleItems = append(visibleItems, item)
		if filteredCount == m.list.Index() {
			selectedVisibleIndex = len(visibleItems) - 1
		}
		filteredCount++
	}

	if len(visibleItems) == 0 {
		return lipgloss.NewStyle().
			Background(dialogBG).
			Height(maxHeight).
			Width(listContentWidth).
			Padding(0, 1).
			Render("No results found.")
	}

	// Styles for items
	neutralStyle := lipgloss.NewStyle().Background(dialogBG)
	tagStyleBase := SemanticStyle("{{|Theme_Tag|}}")
	keyStyleBase := SemanticStyle("{{|Theme_TagKey|}}")
	itemStyleBase := SemanticStyle("{{|Theme_Item|}}")

	tagStyleSel := SemanticStyle("{{|Theme_TagSelected|}}")
	keyStyleSel := SemanticStyle("{{|Theme_TagKeySelected|}}")
	itemStyleSel := SemanticStyle("{{|Theme_ItemSelected|}}")

	var renderedItems []string
	var itemHeights []int

	maxTagLen := calculateMaxTagLength(visibleItems)

	for i, item := range visibleItems {
		isSelected := i == selectedVisibleIndex && m.IsActive()

		tStyle := tagStyleBase
		kStyle := keyStyleBase
		dStyle := itemStyleBase
		if isSelected {
			tStyle = tagStyleSel
			kStyle = keyStyleSel
			dStyle = itemStyleSel
		}

		if item.IsSeparator {
			line := ""
			if item.Tag != "" {
				line = SemanticStyle("{{|Theme_TagKey|}}").Render(item.Tag)
			} else {
				line = strutil.Repeat("─", listContentWidth)
			}
			renderedItems = append(renderedItems, neutralStyle.Padding(0, 1).Render(line))
			itemHeights = append(itemHeights, 1)
			continue
		}

		checkbox := ""
		if item.IsRadioButton || item.IsCheckbox {
			cb := ""
			if item.IsRadioButton {
				if ctx.LineCharacters {
					cb = radioUnselected
					if item.Checked {
						cb = radioSelected
					}
				} else {
					cb = radioUnselectedAscii
					if item.Checked {
						cb = radioSelectedAscii
					}
				}
			} else {
				if ctx.LineCharacters {
					cb = checkUnselected
					if item.Checked {
						cb = checkSelected
					}
				} else {
					cb = checkUnselectedAscii
					if item.Checked {
						cb = checkSelectedAscii
					}
				}
			}
			checkbox = tStyle.Render(cb) + neutralStyle.Render(" ")
		}

		tagStr := ""
		tag := item.Tag
		if len(tag) > 0 {
			letterIdx := 0
			if strings.HasPrefix(tag, "[") && len(tag) > 1 {
				letterIdx = 1
			}
			p := tag[:letterIdx]
			f := string(tag[letterIdx])
			r := tag[letterIdx+1:]
			tagStr = tStyle.Render(p) + kStyle.Render(f) + tStyle.Render(r)
		}

		cbWidth := lipgloss.Width(GetPlainText(checkbox))
		paddingSpaces := strutil.Repeat(" ", max(0, maxTagLen-lipgloss.Width(GetPlainText(tag))+3))
		prefixWidth := cbWidth + maxTagLen + 3
		availableWidth := listContentWidth - prefixWidth

		// The key here is that RenderThemeText must process the raw string *first* so
		// lipgloss gets real ANSI codes instead of pseudo-brackets `{{ }}` which falsely
		// inflate the measured width.
		descStr := RenderThemeText(item.Desc, dStyle)
		wrapped := lipgloss.NewStyle().Width(availableWidth).Render(descStr)
		lines := strings.Split(wrapped, "\n")

		// Trim right spaces from trailing wrapping fills so we don't highlight the background
		for k, l := range lines {
			lines[k] = strings.TrimRight(l, " ")
		}

		firstLine := checkbox + tagStr + neutralStyle.Render(paddingSpaces) + lines[0]

		indent := neutralStyle.Render(strutil.Repeat(" ", prefixWidth))
		renderedItemLines := []string{firstLine}
		for j := 1; j < len(lines); j++ {
			renderedItemLines = append(renderedItemLines, indent+lines[j])
		}

		finalItem := ""
		// listContentWidth is the inner content width. The outer Lipgloss Width with left/right padding
		// must be listContentWidth + 2, otherwise Lipgloss compresses the box and double-wraps lines.
		rowStyle := neutralStyle.Width(listContentWidth+2).Padding(0, 1)
		for j, l := range renderedItemLines {
			if j > 0 {
				finalItem += "\n"
			}
			finalItem += rowStyle.Render(l) + console.CodeReset
		}

		renderedItems = append(renderedItems, finalItem)
		itemHeights = append(itemHeights, len(lines))
	}

	totalContentHeight := 0
	for _, h := range itemHeights {
		totalContentHeight += h
	}

	// Helper: build the blank padding line (used in both branches).
	blankLine := func() string {
		return neutralStyle.Padding(0, 1).Render(strutil.Repeat(" ", listContentWidth)) + console.CodeReset
	}

	// appendRightColumn appends the right-edge column to each view line.
	// When scrollbar is enabled: track/thumb chars (blank when content fits, active when scrolling).
	// When scrollbar is disabled: a neutral space (right padding).
	appendRightColumn := func(lines []string, total, offset int) []string {
		var col []string
		if scrollbar {
			col = buildScrollbarColumn(total, maxHeight, offset, len(lines), ctx.LineCharacters, ctx)
		} else {
			rightPad := neutralStyle.Render(" ")
			col = make([]string, len(lines))
			for i := range col {
				col[i] = rightPad
			}
		}
		for i, line := range lines {
			if i < len(col) {
				lines[i] = line + col[i]
			}
		}
		return lines
	}

	if totalContentHeight <= maxHeight {
		var newHitRegions []HitRegion
		aggY := 0
		for i, h := range itemHeights {
			if !visibleItems[i].IsSeparator {
				actualIndex := -1
				for actIdx, mi := range m.items {
					if mi.Tag == visibleItems[i].Tag && mi.Desc == visibleItems[i].Desc {
						actualIndex = actIdx
						break
					}
				}
				if actualIndex >= 0 {
					newHitRegions = append(newHitRegions, HitRegion{
						ID:     GetMenuItemID(m.id, actualIndex),
						X:      0, // Relative to start of list-box inner content
						Y:      aggY,
						Width:  listContentWidth,
						Height: h,
					})
				}
			}
			aggY += h
		}

		// Build viewLines so we can attach the scrollbar column uniformly.
		var viewLines []string
		for _, item := range renderedItems {
			for _, line := range strings.Split(item, "\n") {
				viewLines = append(viewLines, line)
			}
		}
		for len(viewLines) < maxHeight {
			viewLines = append(viewLines, blankLine())
		}
		// No scrolling — scrollbar column is all blanks/spaces (stable gutter).
		viewLines = appendRightColumn(viewLines, totalContentHeight, 0)

		result := strings.Join(viewLines, "\n")

		// Save for memoization
		m.lastListView = result
		m.lastWidth = m.width
		m.lastHeight = m.height
		m.lastIndex = m.list.Index()
		m.lastFilter = m.list.FilterValue()
		m.lastActive = m.IsActive()
		m.lastLineChars = ctx.LineCharacters
		m.lastHitRegions = newHitRegions

		return result
	}

	currentY := 0
	for i := 0; i < selectedVisibleIndex && i < len(itemHeights); i++ {
		currentY += itemHeights[i]
	}

	selectedHeight := 1
	if selectedVisibleIndex >= 0 && selectedVisibleIndex < len(itemHeights) {
		selectedHeight = itemHeights[selectedVisibleIndex]
	}

	// Bounding box scroll logic: only move viewStart if the selected item is out of bounds
	if currentY < m.viewStartY {
		m.viewStartY = currentY
	} else if currentY+selectedHeight > m.viewStartY+maxHeight {
		m.viewStartY = currentY + selectedHeight - maxHeight
	}

	if m.viewStartY < 0 {
		m.viewStartY = 0
	}
	if m.viewStartY+maxHeight > totalContentHeight {
		m.viewStartY = totalContentHeight - maxHeight
	}

	viewStart := m.viewStartY

	var viewLines []string
	var newHitRegions []HitRegion // Build a new cache corresponding to actual visual lines
	aggY := 0
	for i, item := range renderedItems {
		h := itemHeights[i]
		if aggY+h > viewStart && aggY < viewStart+maxHeight {
			// Save the hit region exactly corresponding to the rendered lines
			if !visibleItems[i].IsSeparator {
				y := aggY - viewStart
				itemH := h
				if aggY < viewStart {
					itemH -= (viewStart - aggY)
					y = 0
				}
				if aggY+h > viewStart+maxHeight {
					itemH -= (aggY + h - (viewStart + maxHeight))
				}

				actualIndex := -1
				for actIdx, mi := range m.items {
					if mi.Tag == visibleItems[i].Tag && mi.Desc == visibleItems[i].Desc {
						actualIndex = actIdx
						break
					}
				}
				if actualIndex >= 0 {
					newHitRegions = append(newHitRegions, HitRegion{
						ID:     GetMenuItemID(m.id, actualIndex),
						X:      0, // Relative to list content
						Y:      y,
						Width:  listContentWidth,
						Height: itemH,
					})
				}
			}

			parts := strings.Split(item, "\n")
			for j, p := range parts {
				lineY := aggY + j
				if lineY >= viewStart && lineY < viewStart+maxHeight {
					viewLines = append(viewLines, p)
				}
			}
		}
		aggY += h
	}

	for len(viewLines) < maxHeight {
		viewLines = append(viewLines, blankLine())
	}

	// Attach scrollbar column on the right.
	viewLines = appendRightColumn(viewLines, totalContentHeight, viewStart)

	finalResult := strings.Join(viewLines, "\n")

	// Save for memoization
	m.lastListView = finalResult
	m.lastWidth = m.width
	m.lastHeight = m.height
	m.lastIndex = m.list.Index()
	m.lastFilter = m.list.FilterValue()
	m.lastActive = m.IsActive()
	m.lastLineChars = ctx.LineCharacters
	m.lastHitRegions = newHitRegions

	return finalResult
}
