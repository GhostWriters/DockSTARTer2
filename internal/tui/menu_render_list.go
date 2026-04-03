package tui

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"strings"

	"charm.land/lipgloss/v2"
)

const vIdxBorderFlag = 0x40000000

// renderVariableHeightList renders items vertically with dynamic heights for word wrapping
func (m *MenuModel) renderVariableHeightList() string {
	ctx := GetActiveContext()
	layout := GetLayout()

	// Memoization Check
	if m.lastListView != "" &&
		m.lastWidth == m.width &&
		m.lastHeight == m.height &&
		m.lastIndex == m.list.Index() &&
		m.lastFilter == m.list.FilterValue() &&
		m.lastActive == m.IsActive() &&
		m.lastLineChars == ctx.LineCharacters &&
		m.viewStartY == m.lastViewStartY &&
		m.lastVersion == m.renderVersion &&
		m.lastColumn == m.ActiveColumn() {
		return m.lastListView
	}

	styles := GetStyles()
	dialogBG := styles.Dialog.GetBackground()

	maxWidth := m.list.Width()
	if maxWidth < 1 {
		maxWidth = 1
	}
	maxHeight := m.layout.ViewportHeight
	if maxHeight < 1 {
		maxHeight = 1
	}

	listContentWidth := maxWidth - 1
	if listContentWidth < 1 {
		listContentWidth = 1
	}

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

	tagStyleBase := theme.ThemeSemanticStyle("{{|Tag|}}")
	keyStyleBase := theme.ThemeSemanticStyle("{{|TagKey|}}")
	itemStyleBase := theme.ThemeSemanticStyle("{{|Item|}}")
	tagStyleSel := theme.ThemeSemanticStyle("{{|TagSelected|}}")
	keyStyleSel := theme.ThemeSemanticStyle("{{|TagKeySelected|}}")
	itemStyleSel := theme.ThemeSemanticStyle("{{|ItemSelected|}}")
	neutralStyle := lipgloss.NewStyle().Background(dialogBG)

	var mainItems []MenuItem
	for _, item := range visibleItems {
		if !item.IsEditing && !item.IsSeparator {
			mainItems = append(mainItems, item)
		}
	}
	maxTagLen := calculateMaxTagLength(mainItems)

	var renderedItems []string
	var itemHeights []int
	var itemMappings []int

	for i := 0; i < len(visibleItems); i++ {
		item := visibleItems[i]
		isSelected := i == selectedVisibleIndex && m.IsActive()
		
		// Highlight the parent header if a child item is selected
		isParentOfSelected := false
		if item.IsGroupHeader && selectedVisibleIndex != -1 {
			selItem := visibleItems[selectedVisibleIndex]
			if (selItem.IsSubItem || selItem.IsAddInstance || selItem.IsEditing) && selItem.BaseApp == item.BaseApp {
				isParentOfSelected = true
			}
		}

		tStyle := tagStyleBase
		kStyle := keyStyleBase
		dStyle := itemStyleBase
		if isSelected || isParentOfSelected {
			tStyle = tagStyleSel
			kStyle = keyStyleSel
			dStyle = itemStyleSel
		}

		isActuallySub := item.IsSubItem || item.IsAddInstance
		if isActuallySub {
			var subItems []MenuItem
			subGroupHasCursor := false
			j := i
			for j < len(visibleItems) && (visibleItems[j].IsSubItem || visibleItems[j].IsAddInstance) {
				subItems = append(subItems, visibleItems[j])
				if j == selectedVisibleIndex {
					subGroupHasCursor = true
				}
				j++
			}

			subLines, subH, subM := m.renderSubListSequence(subItems, i, selectedVisibleIndex, maxWidth, subGroupHasCursor, ctx)

			for k := 0; k < len(subLines); k++ {
				renderedItems = append(renderedItems, subLines[k])
				itemHeights = append(itemHeights, subH[k])
				itemMappings = append(itemMappings, subM[k])
			}

			i = j - 1
			continue
		}

		if item.IsSeparator {
			line := ""
			if item.Tag != "" {
				line = RenderThemeText(item.Tag, theme.ThemeSemanticStyle("{{|TagKey|}}"))
			} else {
				line = strutil.Repeat("─", listContentWidth)
			}
			renderedItems = append(renderedItems, neutralStyle.Padding(0, 0, 0, 1).Render(line))
			itemHeights = append(itemHeights, 1)
			itemMappings = append(itemMappings, i)
			continue
		}

		if item.IsEditing && !isActuallySub {
			cbStr := ""
			if item.IsCheckbox {
				cb := checkUnselected
				if item.Checked {
					cb = checkSelected
				}
				cbStr = tStyle.Render(cb) + neutralStyle.Render(" ")
			}
			editStr := RenderThemeText(item.Tag, dStyle)
			line := cbStr + editStr
			rowStyle := neutralStyle.Width(maxWidth)
			renderedItems = append(renderedItems, rowStyle.Render(line)+console.CodeReset)
			itemHeights = append(itemHeights, 1)
			itemMappings = append(itemMappings, i)
			continue
		}

		checkbox := ""
		if item.IsGroupHeader {
			checkbox = tStyle.Render(subMenuExpanded)
		} else if item.IsRadioButton || item.IsCheckbox {
			cb := ""
			if item.IsRadioButton {
				cb = radioUnselected
				if item.Checked {
					cb = radioSelected
				}
			} else {
				cb = checkUnselected
				if item.Checked {
					cb = checkSelected
				}
			}
			checkbox = tStyle.Render(cb)
		}

		var cbAdd3, cbEnabled3 string
		if item.IsCheckbox && !item.IsGroupHeader {
			ca, ce := checkUnselected, checkUnselected
			if item.Checked {
				ca = checkSelected
			}
			if item.Enabled {
				ce = checkSelected
			}

			cbAStyle := tagStyleBase
			cbEStyle := tagStyleBase
			if isSelected {
				if m.activeColumn == ColAdd {
					cbAStyle = tagStyleSel
				} else {
					cbEStyle = tagStyleSel
				}
			}

			if ctx.LineCharacters {
				// Line-art glyphs are 1-char wide; pad to 3 with spaces: " ▣ " to match " A " in border
				cbAdd3 = neutralStyle.Render(" ") + cbAStyle.Render(ca) + neutralStyle.Render(" ")
				cbEnabled3 = neutralStyle.Render(" ") + cbEStyle.Render(ce) + neutralStyle.Render(" ")
			} else {
				// ASCII: "[ ]" and "[x]" are already 3 chars wide — no extra padding needed
				if item.Checked {
					ca = checkSelectedAscii[:3]
				} else {
					ca = checkUnselectedAscii[:3]
				}
				if item.Enabled {
					ce = checkSelectedAscii[:3]
				} else {
					ce = checkUnselectedAscii[:3]
				}
				cbAdd3 = neutralStyle.Render("[") + cbAStyle.Render(string(ca[1])) + neutralStyle.Render("]")
				cbEnabled3 = neutralStyle.Render("[") + cbEStyle.Render(string(ce[1])) + neutralStyle.Render("]")
			}
		}

		tagStr := ""
		if len(item.Tag) > 0 {
			runes := []rune(item.Tag)
			letterIdx := 0
			if strings.HasPrefix(item.Tag, "[") && len(runes) > 1 {
				letterIdx = 1
			}
			if letterIdx < len(runes) {
				tagStr = tStyle.Render(string(runes[:letterIdx])) + kStyle.Render(string(runes[letterIdx])) + tStyle.Render(string(runes[letterIdx+1:]))
			} else {
				tagStr = RenderThemeText(item.Tag, tStyle)
			}
		}

		// Dynamically calculate prefix width based on menu type and item features
		isAppSelect := m.id == "app-select"
		var prefixWidth int
		var firstLinePrefix string

		if isAppSelect && (item.IsCheckbox || item.IsGroupHeader) {
			if item.IsGroupHeader {
				var arrowA, arrowE string
				if ctx.LineCharacters {
					arrowA = neutralStyle.Render("   ")
					arrowE = neutralStyle.Render(" ") + tStyle.Render(subMenuExpanded) + neutralStyle.Render(" ")
				} else {
					arrowA = neutralStyle.Render("   ")
					arrowE = tStyle.Render("[v]")
				}
				firstLinePrefix = arrowA + neutralStyle.Render(" ") + arrowE + neutralStyle.Render(" ")
			} else {
				firstLinePrefix = cbAdd3 + neutralStyle.Render(" ") + cbEnabled3 + neutralStyle.Render(" ")
			}
			prefixWidth = lipgloss.Width(GetPlainText(firstLinePrefix))
		} else {
			if checkbox != "" {
				firstLinePrefix = checkbox + neutralStyle.Render(" ")
				prefixWidth = lipgloss.Width(GetPlainText(firstLinePrefix))
			} else {
				firstLinePrefix = ""
				prefixWidth = 0
			}
		}

		paddingSpaces := strutil.Repeat(" ", max(0, maxTagLen-lipgloss.Width(GetPlainText(item.Tag))+layout.CheckboxWidth()))
		availableWidth := listContentWidth - (prefixWidth + layout.StatusGutterWidth()) - (maxTagLen + layout.CheckboxWidth()) // status gutter + checkbox padding
		if availableWidth < 0 {
			availableWidth = 0
		}

		descStr := RenderThemeText(item.Desc, dStyle)
		wrapped := lipgloss.NewStyle().Width(availableWidth).Render(descStr)
		lines := strings.Split(strings.TrimSuffix(wrapped, "\n"), "\n")
		for k, l := range lines {
			lines[k] = strings.TrimRight(l, " ")
		}

		var g0, g1 string
		if item.IsReferenced && !item.IsGroupHeader {
			if item.Checked {
				g0 = RenderThemeText("{{|MarkerAdded|}}R{{[-]}}", neutralStyle)
			} else {
				g0 = RenderThemeText("{{|MarkerModified|}}R{{[-]}}", neutralStyle)
			}
		} else if item.IsCheckbox && !item.IsGroupHeader {
			if item.Checked && !item.WasAdded {
				g0 = RenderThemeText("{{|MarkerAdded|}}+{{[-]}}", neutralStyle)
			} else if !item.Checked && item.WasAdded {
				g0 = RenderThemeText("{{|MarkerDeleted|}}-{{[-]}}", neutralStyle)
			} else {
				g0 = neutralStyle.Render(" ")
			}
		} else {
			g0 = neutralStyle.Render(" ")
		}

		if !item.IsGroupHeader {
			isRemoving := !item.Checked && item.WasAdded
			if !isRemoving {
				if item.Enabled && !item.WasEnabled {
					g1 = RenderThemeText("{{|MarkerAdded|}}E{{[-]}}", neutralStyle)
				} else if !item.Enabled && item.WasEnabled {
					g1 = RenderThemeText("{{|MarkerDeleted|}}D{{[-]}}", neutralStyle)
				} else {
					g1 = neutralStyle.Render(" ")
				}
			} else {
				g1 = neutralStyle.Render(" ")
			}
		} else {
			g1 = neutralStyle.Render(" ")
		}
		itemGutter := g0 + g1

		firstLine := firstLinePrefix + tagStr + neutralStyle.Render(paddingSpaces) + lines[0]
		indent := neutralStyle.Render(strutil.Repeat(" ", prefixWidth + maxTagLen + layout.CheckboxWidth()))
		renderedItemLines := []string{firstLine}
		for j := 1; j < len(lines); j++ {
			renderedItemLines = append(renderedItemLines, indent+lines[j])
		}

		finalItem := ""
		rowStyle := neutralStyle.Width(maxWidth)
		for j, l := range renderedItemLines {
			if j > 0 {
				finalItem += "\n"
				finalItem += rowStyle.Render(neutralStyle.Render(strings.Repeat(" ", layout.StatusGutterWidth()))+l) + console.CodeReset
			} else {
				finalItem += rowStyle.Render(itemGutter+l) + console.CodeReset
			}
		}
		renderedItems = append(renderedItems, finalItem)
		itemHeights = append(itemHeights, len(lines))
		itemMappings = append(itemMappings, i)
	}

	totalContentHeight := 0
	for _, h := range itemHeights {
		totalContentHeight += h
	}
	m.lastScrollTotal = totalContentHeight

	for i, item := range renderedItems {
		linesRows := strings.Split(item, "\n")
		for j, line := range linesRows {
			w := lipgloss.Width(GetPlainText(line))
			if w < maxWidth {
				linesRows[j] = line + neutralStyle.Render(strutil.Repeat(" ", maxWidth-w))
			}
		}
		renderedItems[i] = strings.Join(linesRows, "\n")
	}

	if totalContentHeight <= maxHeight {
		var newHitRegions []HitRegion
		aggY := 0
		searchFrom := 0
		for i, h := range itemHeights {
			vIdx := itemMappings[i]
			isBorder := (vIdx & vIdxBorderFlag) != 0
			if vIdx >= 0 && vIdx < len(visibleItems) && !visibleItems[vIdx].IsSeparator {
				cleanVIdx := vIdx & ^vIdxBorderFlag
				actualIndex := -1
				for actIdx := searchFrom; actIdx < len(m.items); actIdx++ {
					mi := m.items[actIdx]
					if mi.Tag == visibleItems[cleanVIdx].Tag && mi.Desc == visibleItems[cleanVIdx].Desc && mi.BaseApp == visibleItems[cleanVIdx].BaseApp {
						actualIndex = actIdx
						if !isBorder {
							searchFrom = actIdx + 1
						}
						break
					}
				}
				if actualIndex >= 0 {
					itemID := GetMenuItemID(m.id, actualIndex)
					if isBorder {
						itemID += "-parent" // Clicks on group borders jump to parent
					}
					item := m.items[actualIndex]
					if m.groupedMode && (item.IsCheckbox || item.IsSubItem || item.IsGroupHeader || item.IsAddInstance) && !isBorder {
						// Row Margin Catch-all: Registered FIRST so specific regions on top take priority
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID + "-border",
							X:      0,
							Y:      aggY,
							Width:  listContentWidth,
							Height: h,
						})

						// Sub-items and add-instance rows are indented by SubItemOffset
						baseShift := 0
						if item.IsSubItem || item.IsAddInstance || item.IsEditing {
							baseShift = layout.SubItemOffset()
						}

						// Specific Regions (Add, Enable, Expand)
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID + "-add",
							X:      baseShift + layout.SingleBorder()*2,
							Y:      aggY,
							Width:  layout.CheckboxWidth(),
							Height: h,
						})
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID + "-enable",
							X:      baseShift + layout.SingleBorder()*6,
							Y:      aggY,
							Width:  layout.CheckboxWidth(),
							Height: h,
						})
						tagX := baseShift + layout.SingleBorder()*10
						tagW := listContentWidth - tagX
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID + "-expand",
							X:      tagX,
							Y:      aggY,
							Width:  max(1, tagW),
							Height: h,
						})
					} else {
						// Standard single-hit region or border hit region
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID,
							X:      0,
							Y:      aggY,
							Width:  listContentWidth,
							Height: h,
						})
					}
				}
			}
			aggY += h
		}

		var viewLines []string
		for _, item := range renderedItems {
			for _, line := range strings.Split(item, "\n") {
				viewLines = append(viewLines, line)
			}
		}
		// Concatenate all lines to form the final visible list view
		result := strings.Join(viewLines, "\n")
		m.lastListView = result
		m.lastHitRegions = newHitRegions
		m.lastVersion = m.renderVersion
		m.lastColumn = m.ActiveColumn()
		return result
	}

	currentY := 0
	aggY_scroll := 0
	selectedY := 0
	for i, h := range itemHeights {
		if itemMappings[i] == selectedVisibleIndex {
			selectedY = aggY_scroll
			break
		}
		aggY_scroll += h
	}
	currentY = selectedY

	selectedHeight := 1
	for i, h := range itemHeights {
		if itemMappings[i] == selectedVisibleIndex {
			selectedHeight = h
			break
		}
	}

	// When dragging the scrollbar, viewStartY is set explicitly by scrollbarDragTo —
	// skip the cursor-visibility snap so it doesn't fight the drag position.
	if !m.sbDrag.Dragging {
		if currentY < m.viewStartY {
			m.viewStartY = currentY
		} else if currentY+selectedHeight > m.viewStartY+maxHeight {
			m.viewStartY = currentY + selectedHeight - maxHeight
		}
	}
	if m.viewStartY < 0 {
		m.viewStartY = 0
	}
	if m.viewStartY+maxHeight > totalContentHeight {
		m.viewStartY = totalContentHeight - maxHeight
	}

	viewStart := m.viewStartY
	var viewLines []string
	var newHitRegions []HitRegion
	aggY := 0
	searchFrom := 0
	for i, item := range renderedItems {
		h := itemHeights[i]
		vIdx := itemMappings[i]
		isBorder := (vIdx & vIdxBorderFlag) != 0
		if aggY+h > viewStart && aggY < viewStart+maxHeight {
			if vIdx >= 0 && !visibleItems[vIdx & ^vIdxBorderFlag].IsSeparator {
				cleanVIdx := vIdx & ^vIdxBorderFlag
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
				for actIdx := searchFrom; actIdx < len(m.items); actIdx++ {
					mi := m.items[actIdx]
					if mi.Tag == visibleItems[cleanVIdx].Tag && mi.Desc == visibleItems[cleanVIdx].Desc && mi.BaseApp == visibleItems[cleanVIdx].BaseApp {
						actualIndex = actIdx
						if !isBorder {
							searchFrom = actIdx + 1
						}
						break
					}
				}
				if actualIndex >= 0 {
					itemID := GetMenuItemID(m.id, actualIndex)
					if isBorder {
						itemID += "-parent" // Clicks on group borders jump to parent
					}
					item := m.items[actualIndex]
					if m.groupedMode && (item.IsCheckbox || item.IsSubItem || item.IsGroupHeader || item.IsAddInstance) && !isBorder {
						// Row Margin Catch-all: Registered FIRST so specific regions on top take priority
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID + "-border",
							X:      0,
							Y:      y,
							Width:  listContentWidth,
							Height: itemH,
						})

						// Sub-items and add-instance rows are indented by SubItemOffset
						baseShift := 0
						if item.IsSubItem || item.IsAddInstance || item.IsEditing {
							baseShift = layout.SubItemOffset()
						}

						// Specific Regions (Add, Enable, Expand)
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID + "-add",
							X:      baseShift + layout.SingleBorder()*2,
							Y:      y,
							Width:  layout.CheckboxWidth(),
							Height: itemH,
						})
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID + "-enable",
							X:      baseShift + layout.SingleBorder()*6,
							Y:      y,
							Width:  layout.CheckboxWidth(),
							Height: itemH,
						})
						tagX := baseShift + layout.SingleBorder()*10
						tagW := listContentWidth - tagX
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID + "-expand",
							X:      tagX,
							Y:      y,
							Width:  max(1, tagW),
							Height: itemH,
						})
					} else {
						// Standard single-hit region or border hit region
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID,
							X:      0,
							Y:      y,
							Width:  listContentWidth,
							Height: itemH,
						})
					}
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
	// Concatenate all lines to form the final visible list view
	finalResult := strings.Join(viewLines, "\n")
	m.lastListView = finalResult
	m.lastHitRegions = newHitRegions
	m.lastVersion = m.renderVersion
	m.lastColumn = m.ActiveColumn()
	m.lastViewStartY = m.viewStartY
	return finalResult
}

// renderSubListSequence handles a contiguous sequence of sub-items by wrapping them in a border.
func (m *MenuModel) renderSubListSequence(items []MenuItem, startVisibleIndex int, selectedVisibleIndex int, maxWidth int, hasCursor bool, ctx StyleContext) ([]string, []int, []int) {
	styles := GetStyles()
	dialogBG := styles.Dialog.GetBackground()
	neutralStyle := lipgloss.NewStyle().Background(dialogBG)
	tagStyleBase := theme.ThemeSemanticStyle("{{|Tag|}}")
	keyStyleBase := theme.ThemeSemanticStyle("{{|TagKey|}}")
	tagStyleSel := theme.ThemeSemanticStyle("{{|TagSelected|}}")
	keyStyleSel := theme.ThemeSemanticStyle("{{|TagKeySelected|}}")

	var subGroupTagMaxW int
	for _, item := range items {
		w := lipgloss.Width(GetPlainText(item.Tag))
		if w > subGroupTagMaxW {
			subGroupTagMaxW = w
		}
	}

	// Instance Grid: Indent 10, Dash 1, Left Pad 1, Right Pad 1.
	// Total width: 1(│) + 1(sp_l) + 10(prefix) + tag + 1(sp_r) + 1(│).
	// Total width: 2 + 10 + tag + 1 = 13 + tag. No, prefix is already 10.
	// Prefix = 1(sp_l) + 3(cbA) + 1(sp) + 3(cbE) + 1(sp) = 10.
	// Total width: 1(│l) + 10(prefix) + maxTag + 1(sp_r) + 1(│r) = 13 + maxTag.
	subListWidth := 12 + subGroupTagMaxW
	if subListWidth > maxWidth {
		subListWidth = maxWidth
	}

	subFocused := m.IsActive() && hasCursor
	var resLines []string
	var resH []int
	var resM []int

	// 1. Build Top Border with 1 dash.
	topBorder := BuildAETopBorder(subListWidth, 1, subFocused, m.activeColumn, ctx)
	resLines = append(resLines, neutralStyle.Render(strutil.Repeat(" ", 10))+topBorder)
	resH = append(resH, 1)
	resM = append(resM, startVisibleIndex|vIdxBorderFlag) // Flag as border

	vStyleLight := lipgloss.NewStyle().Foreground(ctx.BorderColor).Background(dialogBG)
	vStyleDark := lipgloss.NewStyle().Foreground(ctx.Border2Color).Background(dialogBG)
	var border lipgloss.Border
	if ctx.LineCharacters {
		if subFocused {
			border = ThickRoundedBorder
		} else {
			border = lipgloss.RoundedBorder()
		}
	} else {
		if subFocused {
			border = RoundedThickAsciiBorder
		} else {
			border = RoundedAsciiBorder
		}
	}
	vBorderChar := border.Left

	for i, item := range items {
		visibleIdx := startVisibleIndex + i
		isSelected := visibleIdx == selectedVisibleIndex && m.IsActive()

		tStyle := tagStyleBase
		kStyle := keyStyleBase
		if isSelected {
			tStyle = tagStyleSel
			kStyle = keyStyleSel
		}

		var g0, g1 string
		if item.IsReferenced {
			g0 = RenderThemeText("{{|MarkerAdded|}}R{{[-]}}", neutralStyle)
			if !item.Checked {
				g0 = RenderThemeText("{{|MarkerModified|}}R{{[-]}}", neutralStyle)
			}
		} else if item.Checked && !item.WasAdded {
			g0 = RenderThemeText("{{|MarkerAdded|}}+{{[-]}}", neutralStyle)
		} else if !item.Checked && item.WasAdded {
			g0 = RenderThemeText("{{|MarkerDeleted|}}-{{[-]}}", neutralStyle)
		} else {
			g0 = neutralStyle.Render(" ")
		}

		if item.Enabled && !item.WasEnabled {
			g1 = RenderThemeText("{{|MarkerAdded|}}E{{[-]}}", neutralStyle)
		} else if !item.Enabled && item.WasEnabled && !(!item.Checked && item.WasAdded) {
			g1 = RenderThemeText("{{|MarkerDeleted|}}D{{[-]}}", neutralStyle)
		} else {
			g1 = neutralStyle.Render(" ")
		}

		tagStr := ""
		if item.IsEditing {
			// Using the standard edit styling (red background/bold)
			editTag := GetPlainText(item.Tag)
			tagStr = theme.ThemeSemanticStyle("{{|ItemSelected|}}").Render(editTag)
		} else if len(item.Tag) > 0 {
			runes := []rune(item.Tag)
			tagStr = kStyle.Render(string(runes[0])) + tStyle.Render(string(runes[1:]))
		}

		// Choose checkbox styles individually
		cbStyleA := tStyle
		cbStyleE := tStyle
		if isSelected {
			if subFocused {
				if m.activeColumn == ColAdd {
					cbStyleA = tagStyleSel
					cbStyleE = tagStyleBase
				} else {
					cbStyleE = tagStyleSel
					cbStyleA = tagStyleBase
				}
			} else {
				// Sub-list not focused: use neutral style for both
				cbStyleA = tagStyleBase
				cbStyleE = tagStyleBase
			}
		}

		var checkboxA3, checkboxE3 string
		if ctx.LineCharacters {
			cA, cE := checkUnselected, checkUnselected
			if item.Checked { cA = checkSelected }
			if item.Enabled { cE = checkSelected }
			
			checkboxA3 = neutralStyle.Render(" ") + cbStyleA.Render(cA) + neutralStyle.Render(" ")
			checkboxE3 = neutralStyle.Render(" ") + cbStyleE.Render(cE) + neutralStyle.Render(" ")
		} else {
			caA, ceA := "[ ]", "[ ]"
			if item.Checked { caA = "[x]" }
			if item.Enabled { ceA = "[x]" }
			checkboxA3 = neutralStyle.Render("[") + cbStyleA.Render(string(caA[1])) + neutralStyle.Render("]")
			checkboxE3 = neutralStyle.Render("[") + cbStyleE.Render(string(ceA[1])) + neutralStyle.Render("]")
		}

		rowContent := vStyleLight.Render(vBorderChar) + neutralStyle.Render(" ") + checkboxA3 + neutralStyle.Render(" ") + checkboxE3 + neutralStyle.Render(" ") + tagStr
		rowWidth := subListWidth - 1
		pContent := rowContent + neutralStyle.Render(strutil.Repeat(" ", max(0, rowWidth-lipgloss.Width(GetPlainText(rowContent)))))
		line := g0 + g1 + neutralStyle.Render(strutil.Repeat(" ", 8)) + pContent + vStyleDark.Render(vBorderChar)

		resLines = append(resLines, line+console.CodeReset)
		resH = append(resH, 1)
		resM = append(resM, visibleIdx)
	}

	// 3. Build Bottom Border with 1 dash.
	bottomBorder := BuildAEBottomBorder(subListWidth, 1, subFocused, m.activeColumn, -1, ctx)
	resLines = append(resLines, neutralStyle.Render(strutil.Repeat(" ", 10))+bottomBorder+console.CodeReset)
	resH = append(resH, 1)
	resM = append(resM, startVisibleIndex|vIdxBorderFlag) // Flag as border

	return resLines, resH, resM
}
