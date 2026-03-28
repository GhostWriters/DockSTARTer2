package tui

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
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

			subLines, subH, _ := m.renderSubListSequence(subItems, i, selectedVisibleIndex, maxTagLen, maxWidth, listContentWidth, subGroupHasCursor, ctx)

			combined := strings.Join(subLines, "\n")
			totalH := 0
			for _, h := range subH {
				totalH += h
			}

			renderedItems = append(renderedItems, combined)
			itemHeights = append(itemHeights, totalH)
			itemMappings = append(itemMappings, i)

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

		if item.IsEditing {
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
				bgA := lipgloss.NewStyle().Background(cbAStyle.GetBackground())
				cbAdd3 = bgA.Render(" ") + cbAStyle.Render(ca) + bgA.Render(" ")

				bgE := lipgloss.NewStyle().Background(cbEStyle.GetBackground())
				cbEnabled3 = bgE.Render(" ") + cbEStyle.Render(ce) + bgE.Render(" ")
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
				cbAdd3 = cbAStyle.Render(ca)
				cbEnabled3 = cbEStyle.Render(ce)
			}
		}

		tagStr := ""
		if len(item.Tag) > 0 {
			letterIdx := 0
			if strings.HasPrefix(item.Tag, "[") && len(item.Tag) > 1 {
				letterIdx = 1
			}
			tagStr = tStyle.Render(item.Tag[:letterIdx]) + kStyle.Render(string(item.Tag[letterIdx])) + tStyle.Render(item.Tag[letterIdx+1:])
		}

		// Gutter(2) + sp(1) + cbA(3) + sp(1) + cbE(3) + sp(1) = 11 chars. Tag @ 11.
		prefixWidth := 11
		paddingSpaces := strutil.Repeat(" ", max(0, maxTagLen-lipgloss.Width(GetPlainText(item.Tag))+3))
		availableWidth := listContentWidth - prefixWidth - (maxTagLen + 3)
		if availableWidth < 0 {
			availableWidth = 0
		}

		descStr := RenderThemeText(item.Desc, dStyle)
		wrapped := lipgloss.NewStyle().Width(availableWidth).Render(descStr)
		lines := strings.Split(wrapped, "\n")
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

		var firstLine string
		if item.IsCheckbox && !item.IsGroupHeader {
			// g0(1) g1(2) cbAdd(3,4,5) cbEnabled(6,7,8) tag@9 — no space between cb and E, they're each 3 chars
			// Wait: g0(1) g1(2) cbAdd(3char) cbEnabled(3char) tag → total prefix=8.
			// Border: corner─[A(3ch)]─[E(3ch)]... with prefixDashes=1: col1=corner,col2=dash,cols3-5=A,col6=dash,cols7-9=E
			// Content inside box: g0=col1,g1=col2,cbAdd=cols3-5,cbEnabled=cols6-8... off by 1 on E.
			// Use 1 space between cbAdd and cbEnabled to get:
			// g0(1) g1(2) cbAdd(3,4,5) sp(6) cbEnabled(7,8,9) → A center=4, E center=8
			// Border prefixDashes=1: corner(1) dash(2) A(3,4,5) dash(6) E(7,8,9) → A center=4, E center=8 ✅
			firstLine = cbAdd3 + neutralStyle.Render(" ") + cbEnabled3 + neutralStyle.Render(" ") + tagStr + neutralStyle.Render(paddingSpaces) + lines[0]
			prefixWidth = 10 // 2(gutter) + 3(cbA) + 1(sp) + 3(cbE) + 1(sp) = 10
		} else if item.IsGroupHeader {
			// User requested: only show arrow in the E-position (Enabled) when expanded.
			// arrowA (Add position) becomes blank 3-char spaces.
			var arrowA, arrowE string
			if ctx.LineCharacters {
				arrowA = neutralStyle.Render("   ")
				arrowE = neutralStyle.Render(" ") + tStyle.Render(subMenuExpanded) + neutralStyle.Render(" ")
			} else {
				arrowA = neutralStyle.Render("   ")
				arrowE = tStyle.Render("[v]")
			}
			firstLine = arrowA + neutralStyle.Render(" ") + arrowE + neutralStyle.Render(" ") + tagStr + neutralStyle.Render(paddingSpaces) + lines[0]
			prefixWidth = 10
		} else {
			totalPad := 11 - (2 + lipgloss.Width(GetPlainText(checkbox)))
			firstLine = neutralStyle.Render(strutil.Repeat(" ", totalPad)) + checkbox + tagStr + neutralStyle.Render(paddingSpaces) + lines[0]
		}

		indent := neutralStyle.Render(strutil.Repeat(" ", prefixWidth))
		renderedItemLines := []string{firstLine}
		for j := 1; j < len(lines); j++ {
			renderedItemLines = append(renderedItemLines, indent+lines[j])
		}

		finalItem := ""
		rowStyle := neutralStyle.Width(maxWidth)
		for j, l := range renderedItemLines {
			if j > 0 {
				finalItem += "\n"
				finalItem += rowStyle.Render(neutralStyle.Render("  ")+l) + console.CodeReset
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

	blankLine := func() string {
		return neutralStyle.Padding(0, 0, 0, 1).Render(strutil.Repeat(" ", listContentWidth)) + console.CodeReset
	}

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
			if vIdx >= 0 && !visibleItems[vIdx].IsSeparator {
				actualIndex := -1
				for actIdx := searchFrom; actIdx < len(m.items); actIdx++ {
					mi := m.items[actIdx]
					if mi.Tag == visibleItems[vIdx].Tag && mi.Desc == visibleItems[vIdx].Desc {
						actualIndex = actIdx
						searchFrom = actIdx + 1
						break
					}
				}
				if actualIndex >= 0 {
					newHitRegions = append(newHitRegions, HitRegion{
						ID:     GetMenuItemID(m.id, actualIndex),
						X:      0,
						Y:      aggY,
						Width:  listContentWidth,
						Height: h,
					})
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
		for len(viewLines) < maxHeight {
			viewLines = append(viewLines, blankLine())
		}
		result := strings.Join(viewLines, "\n")
		m.lastListView = result
		m.lastHitRegions = newHitRegions
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
	var newHitRegions []HitRegion
	aggY := 0
	searchFrom := 0
	for i, item := range renderedItems {
		h := itemHeights[i]
		vIdx := itemMappings[i]
		if aggY+h > viewStart && aggY < viewStart+maxHeight {
			if vIdx >= 0 && !visibleItems[vIdx].IsSeparator {
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
					if mi.Tag == visibleItems[vIdx].Tag && mi.Desc == visibleItems[vIdx].Desc {
						actualIndex = actIdx
						searchFrom = actIdx + 1
						break
					}
				}
				if actualIndex >= 0 {
					newHitRegions = append(newHitRegions, HitRegion{
						ID:     GetMenuItemID(m.id, actualIndex),
						X:      0,
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
	finalResult := strings.Join(viewLines, "\n")
	m.lastListView = finalResult
	m.lastHitRegions = newHitRegions
	return finalResult
}

// renderSubListSequence handles a contiguous sequence of sub-items by wrapping them in a border.
func (m *MenuModel) renderSubListSequence(items []MenuItem, startVisibleIndex int, selectedVisibleIndex int, maxTagLen int, maxWidth int, listContentWidth int, hasCursor bool, ctx StyleContext) ([]string, []int, []int) {
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
	resM = append(resM, -1)

	vStyle := lipgloss.NewStyle().Foreground(ctx.BorderColor).Background(dialogBG)
	vBorderChar := lipgloss.RoundedBorder().Left
	if subFocused && ctx.LineCharacters {
		vBorderChar = ThickRoundedBorder.Left
	}

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
		} else if !item.Enabled && item.WasEnabled {
			g1 = RenderThemeText("{{|MarkerDeleted|}}D{{[-]}}", neutralStyle)
		} else {
			g1 = neutralStyle.Render(" ")
		}

		tagStr := ""
		if len(item.Tag) > 0 {
			tagStr = kStyle.Render(string(item.Tag[0])) + tStyle.Render(item.Tag[1:])
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
			
			bgA := lipgloss.NewStyle().Background(cbStyleA.GetBackground())
			checkboxA3 = bgA.Render(" ") + cbStyleA.Render(cA) + bgA.Render(" ")
			
			bgE := lipgloss.NewStyle().Background(cbStyleE.GetBackground())
			checkboxE3 = bgE.Render(" ") + cbStyleE.Render(cE) + bgE.Render(" ")
		} else {
			caA, ceA := "[ ]", "[ ]"
			if item.Checked { caA = "[x]" }
			if item.Enabled { ceA = "[x]" }
			checkboxA3 = cbStyleA.Render(caA)
			checkboxE3 = cbStyleE.Render(ceA)
		}

		rowContent := vStyle.Render(vBorderChar) + neutralStyle.Render(" ") + checkboxA3 + neutralStyle.Render(" ") + checkboxE3 + neutralStyle.Render(" ") + tagStr
		rowWidth := subListWidth - 1
		pContent := rowContent + neutralStyle.Render(strutil.Repeat(" ", max(0, rowWidth-lipgloss.Width(GetPlainText(rowContent)))))
		line := g0 + g1 + neutralStyle.Render(strutil.Repeat(" ", 8)) + pContent + vStyle.Render(vBorderChar)

		resLines = append(resLines, line+console.CodeReset)
		resH = append(resH, 1)
		resM = append(resM, visibleIdx)
	}

	// 3. Build Bottom Border with 1 dash.
	bottomBorder := BuildAEBottomBorder(subListWidth, 1, subFocused, m.activeColumn, ctx)
	resLines = append(resLines, neutralStyle.Render(strutil.Repeat(" ", 10))+bottomBorder)
	resH = append(resH, 1)
	resM = append(resM, -1)

	return resLines, resH, resM
}
