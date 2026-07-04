package classic

import (
	"strings"

	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	semstyle "github.com/GhostWriters/semstyle/lg"

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
		m.lastListActive == m.IsListActive() &&
		m.lastLineChars == ctx.LineCharacters &&
		m.ViewStartY == m.lastViewStartY &&
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
	maxHeight := m.Layout.ViewportHeight
	if maxHeight < 1 {
		maxHeight = 1
	}

	listContentWidth := maxWidth - 1
	if listContentWidth < 1 {
		listContentWidth = 1
	}

	filter := m.list.FilterValue()
	var visibleItems []MenuItem
	var selectedVisibleIndex = -1

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
	tagStyleSel := theme.ThemeSemanticStyle("{{|TagFocused|}}")
	keyStyleSel := theme.ThemeSemanticStyle("{{|TagKeyFocused|}}")
	itemStyleSel := theme.ThemeSemanticStyle("{{|ItemFocused|}}")
	checkboxStyleBase := theme.ThemeSemanticStyle("{{|Checkbox|}}")
	checkboxStyleSel := theme.ThemeSemanticStyle("{{|CheckboxFocused|}}")
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
		isAppSelect := m.id == "app-select"
		isSelected := i == selectedVisibleIndex && m.IsListActive()

		// Highlight the parent header if a child item is selected
		isParentOfSelected := false
		paddingStr := neutralStyle.Render(strutil.Repeat(" ", m.itemPaddingWidth))
		if item.IsGroupHeader && selectedVisibleIndex != -1 {
			selItem := visibleItems[selectedVisibleIndex]
			if (selItem.IsSubItem || selItem.IsAddInstance || selItem.IsEditing) && selItem.BaseApp == item.BaseApp {
				isParentOfSelected = true
			}
		}

		tStyle := tagStyleBase
		kStyle := keyStyleBase
		dStyle := itemStyleBase
		cbStyle := checkboxStyleBase
		if isSelected {
			tStyle = tagStyleSel
			kStyle = keyStyleSel
			dStyle = itemStyleSel
			cbStyle = checkboxStyleSel
		} else if isParentOfSelected {
			// A child instance is focused, not this header row itself -- keep the
			// app name unfocused, but still highlight the description so it's
			// clear which app's description is showing below the instance list.
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
			renderedItems = append(renderedItems, neutralStyle.PaddingLeft(0).Render(line))
			itemHeights = append(itemHeights, 1)
			itemMappings = append(itemMappings, i)
			continue
		}

		if item.IsEditing && !isActuallySub {
			cbStr := ""
			if item.IsCheckbox {
				cbStr = renderCheckbox(false, item.Checked, ctx.LineCharacters, true, cbStyle) + neutralStyle.Render(" ")
			}
			editStr := RenderThemeText(item.Tag, dStyle)
			line := cbStr + editStr
			rowStyle := neutralStyle.Width(maxWidth)
			renderedItems = append(renderedItems, rowStyle.Render(line)+semstyle.CodeReset)
			itemHeights = append(itemHeights, 1)
			itemMappings = append(itemMappings, i)
			continue
		}

		checkbox := ""
		if item.IsGroupHeader {
			cb := subMenuExpanded
			if !ctx.LineCharacters {
				cb = subMenuExpandedAscii
			}
			checkbox = tStyle.Render(cb)
		} else if item.IsRadioButton || item.IsCheckbox {
			// Regular (non-flow, non-app-select) checkbox/radio rows show their
			// bracket/parens only when this row has cursor focus; the checked
			// item's bullet/checkmark still shows regardless (see renderCheckbox).
			checkbox = renderCheckbox(item.IsRadioButton, item.Checked, ctx.LineCharacters, isSelected, cbStyle)
		}

		var cbAdd3, cbEnabled3, cbExpand3 string
		if isAppSelect && (item.IsCheckbox || item.IsGroupHeader) {
			cbAStyle := checkboxStyleBase
			cbEStyle := checkboxStyleBase
			cbXStyle := neutralStyle
			if isSelected {
				switch m.activeColumn {
				case ColAdd:
					cbAStyle = checkboxStyleSel
				case ColEnable:
					cbEStyle = checkboxStyleSel
				case ColExpand:
					cbXStyle = checkboxStyleSel
				}
			}

			// Expandable rows are the plain (collapsed) base-app row and the
			// group-header (expanded) row -- never sub-items or the "+ Add
			// instance..." row, which have no arrow of their own.
			canExpand := item.IsGroupHeader || (item.IsCheckbox && !item.IsSubItem && !item.IsAddInstance)

			// Each column's brackets show only when it's the specific one with
			// keyboard focus, matching cbAStyle/cbEStyle's own per-column
			// highlighting above rather than "is this row selected at all".
			addFocused := isSelected && m.activeColumn == ColAdd
			enableFocused := isSelected && m.activeColumn == ColEnable
			expandFocused := isSelected && m.activeColumn == ColExpand

			if ctx.LineCharacters {
				ca, ce := checkOffBare, checkOffBare
				if addFocused {
					ca = checkOff
				}
				if enableFocused {
					ce = checkOff
				}
				if item.Checked {
					ca = checkOnBare
					if addFocused {
						ca = checkOn
					}
				}
				if item.Enabled {
					ce = checkOnBare
					if enableFocused {
						ce = checkOn
					}
				}
				cbAdd3 = renderCheckboxGlyph(ca, cbAStyle)
				cbEnabled3 = renderCheckboxGlyph(ce, cbEStyle)
				if canExpand {
					arrow := subMenuCollapsed
					if item.IsGroupHeader {
						arrow = subMenuExpanded
					}
					if expandFocused {
						cbExpand3 = cbXStyle.Render("[") + cbXStyle.Render(arrow) + cbXStyle.Render("]")
					} else {
						cbExpand3 = neutralStyle.Render(" ") + cbXStyle.Render(arrow) + neutralStyle.Render(" ")
					}
				} else {
					cbExpand3 = neutralStyle.Render("   ")
				}
			} else {
				caText, ceText := checkOffBareAscii, checkOffBareAscii
				if addFocused {
					caText = checkOffAscii
				}
				if enableFocused {
					ceText = checkOffAscii
				}
				if item.Checked {
					caText = checkOnBareAscii
					if addFocused {
						caText = checkOnAscii
					}
				}
				if item.Enabled {
					ceText = checkOnBareAscii
					if enableFocused {
						ceText = checkOnAscii
					}
				}
				cbAdd3 = renderCheckboxGlyph(caText, cbAStyle)
				cbEnabled3 = renderCheckboxGlyph(ceText, cbEStyle)
				if canExpand {
					arrow := subMenuCollapsedAscii
					if item.IsGroupHeader {
						arrow = subMenuExpandedAscii
					}
					if expandFocused {
						cbExpand3 = cbXStyle.Render("[") + cbXStyle.Render(arrow) + cbXStyle.Render("]")
					} else {
						cbExpand3 = neutralStyle.Render(" ") + cbXStyle.Render(arrow) + neutralStyle.Render(" ")
					}
				} else {
					cbExpand3 = neutralStyle.Render("   ")
				}
			}
		}

		tagStr := ""
		isProcessingItem := m.processingItemIdx >= 0 && i == m.processingItemIdx
		if len(item.Tag) > 0 {
			runes := []rune(item.Tag)
			letterIdx := 0
			if strings.HasPrefix(item.Tag, "[") && len(runes) > 1 {
				letterIdx = 1
			}
			// App-select rows with a docs URL render the tag as a clickable
			// hyperlink (two adjacent OSC8 spans -- hotkey-letter style and
			// rest-of-name style -- both pointing at the same URL, since a
			// single hyperlink span can't carry two different text styles).
			var linkURL string
			if isAppSelect {
				linkURL = item.Metadata["docsURL"]
			}
			if letterIdx < len(runes) {
				if linkURL != "" {
					tagStr = tStyle.Render(string(runes[:letterIdx])) +
						kStyle.Hyperlink(linkURL).Render(string(runes[letterIdx])) +
						tStyle.Hyperlink(linkURL).Render(string(runes[letterIdx+1:]))
				} else {
					tagStr = tStyle.Render(string(runes[:letterIdx])) + kStyle.Render(string(runes[letterIdx])) + RenderThemeText(string(runes[letterIdx+1:]), tStyle)
				}
			} else if linkURL != "" {
				tagStr = tStyle.Hyperlink(linkURL).Render(item.Tag)
			} else {
				tagStr = RenderThemeText(item.Tag, tStyle)
			}
			if isProcessingItem {
				spinL, spinR := m.titleSpinner.Indicators()
				if spinL != "" {
					spinStyle := GetStyles().TagSpinner
					tagStr = spinStyle.Render(spinL) + tagStr + spinStyle.Render(spinR)
				}
			}
		}

		// Prefix width calculation (Left of the Tag)
		var gutterWidth int

		hasAnyCheckboxes := false
		for _, it := range visibleItems {
			if it.IsCheckbox || it.IsRadioButton || it.IsGroupHeader {
				hasAnyCheckboxes = true
				break
			}
		}

		menuPrefixWidth := 0
		if isAppSelect {
			menuPrefixWidth = 12 // cbAdd(3) + sp(1) + cbEnabled(3) + sp(1) + cbExpand(3) + sp(1)
		} else if hasAnyCheckboxes {
			menuPrefixWidth = layout.CheckboxWidth()
		}
		minGap := 3

		// Gutter width is already lock + activity.
		// We use StatusGutterWidth() as the definitive source.
		gutterWidth = m.StatusGutterWidth()
		totalGutterWidth := gutterWidth + m.itemPaddingWidth
		availableWidth := listContentWidth - totalGutterWidth - menuPrefixWidth - (maxTagLen + minGap)
		if availableWidth < 0 {
			availableWidth = 0
		}

		var descStr string
		if (isSelected || isParentOfSelected) && item.Desc != "" {
			// item.Desc is normally pre-wrapped in its own semstyle tag (e.g.
			// "{{|ListItem|}}..."), which overrides dStyle entirely -- strip
			// tags here so dStyle (itemStyleSel) actually takes effect, same
			// as the plain isSelected case already did.
			descStr = dStyle.Render(GetPlainText(item.Desc))
		} else {
			descStr = RenderThemeText(item.Desc, dStyle)
		}
		wrapped := lipgloss.NewStyle().Width(availableWidth).Render(descStr)
		lines := strings.Split(strings.TrimSuffix(wrapped, "\n"), "\n")
		for k, l := range lines {
			lines[k] = strings.TrimRight(l, " ")
		}

		// When VariableHeight is false (Uniform mode), strictly enforce 1-line per item.
		// This keeps the scrollbar math (which uses item indices) in sync with the renderer.
		if !m.variableHeight {
			lines = lines[:1]
		}

		// Gutter: Use unified helper which respects StatusGutterWidth
		itemGutter := m.RenderItemGutter(item, neutralStyle, "")

		// Name-column focus indicator: "[AppName]", with the "[" replacing the
		// separator space before the tag (no extra width) and an unstyled "]"
		// appended right after it (accounted for via spinTagExtra-style gap
		// shrink below, since it does add a character).
		nameFocused := isAppSelect && isSelected && m.activeColumn == ColName
		nameSep := neutralStyle.Render(" ")
		nameClose := ""
		if nameFocused {
			nameSep = neutralStyle.Render("[")
			nameClose = neutralStyle.Render("]")
		}

		prefixPadding := ""
		prefixWidth := 0
		if item.IsCheckbox || item.IsRadioButton || item.IsGroupHeader {
			if isAppSelect && (item.IsCheckbox || item.IsGroupHeader) {
				// Slot1(3) + Space(1) + Slot2(3) + Space(1) + Slot3(3) + Space(1) = 12 characters.
				// This MUST match menuPrefixWidth above to align with standard app-select rows.
				prefixPadding = cbAdd3 + neutralStyle.Render(" ") + cbEnabled3 + neutralStyle.Render(" ") + cbExpand3 + nameSep
			} else {
				// Standard menus or Radio buttons: indicator followed by one space
				prefixPadding = checkbox + neutralStyle.Render(" ")
			}
		}
		prefixWidth = lipgloss.Width(GetPlainText(prefixPadding))

		// Padding spaces are AFTER the tag to reach the description column.
		// Alignment column for descriptions: menuGutterWidth(2) + menuPrefixWidth + maxTagLen + minGap(3)
		spinTagExtra := 0
		if isProcessingItem {
			// We replace the 1-char sep with spinL, and add spinR after the tag.
			// Net extra chars = +2 spinners - 1 sep = +1; shrink gap by 1 to keep desc aligned.
			spinTagExtra = 1
		}
		if nameFocused {
			// The trailing "]" adds a character beyond the tag's own width.
			spinTagExtra++
		}
		gapWidth := (maxTagLen - lipgloss.Width(GetPlainText(item.Tag))) + (menuPrefixWidth - prefixWidth) + minGap - spinTagExtra
		paddingSpaces := strutil.Repeat(" ", max(0, gapWidth))

		firstLine := prefixPadding + tagStr + nameClose + neutralStyle.Render(paddingSpaces) + lines[0]
		indent := neutralStyle.Render(strutil.Repeat(" ", menuPrefixWidth+maxTagLen+minGap))
		renderedItemLines := []string{firstLine}
		for j := 1; j < len(lines); j++ {
			renderedItemLines = append(renderedItemLines, indent+lines[j])
		}

		finalItem := ""
		// m.itemPaddingWidth is typically 1. gutterWidth(1) + 1 = 2 indent.
		// This results in the requested "|! Tag" or "|! X Tag" layout.
		rowStyle := neutralStyle.Width(maxWidth)
		gutterSpaces := neutralStyle.Render(strutil.Repeat(" ", m.StatusGutterWidth()))

		sep := paddingStr
		if isAppSelect || isProcessingItem {
			sep = ""
		}
		// Continuation lines always use the normal separator width so they align
		// with the description column on line 0 (the spinner only affects line 0).
		contSep := paddingStr
		if isAppSelect {
			contSep = ""
		}

		for j, l := range renderedItemLines {
			if j > 0 {
				finalItem += "\n"
				finalItem += rowStyle.Render(gutterSpaces+contSep+l) + semstyle.CodeReset
			} else {
				finalItem += rowStyle.Render(itemGutter+sep+l) + semstyle.CodeReset
			}
		}
		renderedItems = append(renderedItems, finalItem)
		if m.variableHeight {
			itemHeights = append(itemHeights, len(lines))
		} else {
			itemHeights = append(itemHeights, 1)
		}
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
						// Simple rows and group headers have a dedicated 3-char-wide arrow
						// column at tagX. Clicks past it (the app name / description) fall
						// through to "-border" so the name's own hyperlink hit region
						// (registered separately, higher ZOrder) and plain row-selection
						// both work. Sub-items and "+ Add instance..." rows have no arrow
						// column, so they keep the wide region for rename/add-instance clicks.
						expandX, expandWidth := tagX, 3
						if item.IsSubItem || item.IsAddInstance {
							expandX, expandWidth = tagX, listContentWidth-tagX
						}
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID + "-expand",
							X:      max(0, expandX),
							Y:      aggY,
							Width:  max(1, expandWidth),
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
			viewLines = append(viewLines, strings.Split(item, "\n")...)
		}
		// Concatenate all lines to form the final visible list view
		result := strings.Join(viewLines, "\n")
		m.lastListView = result
		m.lastHitRegions = newHitRegions
		m.lastVersion = m.renderVersion
		m.lastColumn = m.ActiveColumn()
		m.lastListActive = m.IsListActive()
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

	// When dragging the scrollbar, viewStartY is set explicitly by Scrollbar.Update —
	// skip the cursor-visibility snap so it doesn't fight the drag position.
	if !m.Scroll.Drag.Dragging {
		if currentY < m.ViewStartY {
			m.ViewStartY = currentY
		} else if currentY+selectedHeight > m.ViewStartY+maxHeight {
			m.ViewStartY = currentY + selectedHeight - maxHeight
		}
	}
	if m.ViewStartY < 0 {
		m.ViewStartY = 0
	}
	if m.ViewStartY+maxHeight > totalContentHeight {
		m.ViewStartY = totalContentHeight - maxHeight
	}

	viewStart := m.ViewStartY
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
						// See the corresponding comment in the non-scrolled render path above:
						// widen the arrow's hit region by 1 char on each side for easier clicking.
						expandX, expandWidth := tagX-1, 3
						if item.IsSubItem || item.IsAddInstance {
							expandX, expandWidth = tagX, listContentWidth-tagX
						}
						newHitRegions = append(newHitRegions, HitRegion{
							ID:     itemID + "-expand",
							X:      max(0, expandX),
							Y:      y,
							Width:  max(1, expandWidth),
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
	m.lastViewStartY = m.ViewStartY
	m.lastListActive = m.IsListActive()
	return finalResult
}

// renderSubListSequence handles a contiguous sequence of sub-items by wrapping them in a border.
func (m *MenuModel) renderSubListSequence(items []MenuItem, startVisibleIndex int, selectedVisibleIndex int, maxWidth int, hasCursor bool, ctx StyleContext) ([]string, []int, []int) {
	styles := GetStyles()
	dialogBG := styles.Dialog.GetBackground()
	neutralStyle := lipgloss.NewStyle().Background(dialogBG)
	tagStyleBase := theme.ThemeSemanticStyle("{{|Tag|}}")
	keyStyleBase := theme.ThemeSemanticStyle("{{|TagKey|}}")
	tagStyleSel := theme.ThemeSemanticStyle("{{|TagFocused|}}")
	keyStyleSel := theme.ThemeSemanticStyle("{{|TagKeyFocused|}}")

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

	subFocused := m.IsListActive() && hasCursor
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
		isSelected := visibleIdx == selectedVisibleIndex && m.IsListActive()

		tStyle := tagStyleBase
		kStyle := keyStyleBase
		if isSelected {
			tStyle = tagStyleSel
			kStyle = keyStyleSel
		}

		lockMarker := ""
		if m.showLockGutter {
			if item.IsInvalid {
				lockMarker = RenderThemeText("{{|MarkerInvalid|}}"+invalidMarker+"{{[-]}}", neutralStyle)
			} else if item.Locked {
				lockMarker = RenderThemeText("{{|MarkerLocked|}}!{{[-]}}", neutralStyle)
			} else {
				lockMarker = neutralStyle.Render(" ")
			}
		}

		var g0, g1 string
		if item.IsReferenced {
			if item.Checked {
				g0 = RenderThemeText("{{|MarkerAdded|}}R{{[-]}}", neutralStyle)
			} else {
				g0 = RenderThemeText("{{|MarkerModified|}}r{{[-]}}", neutralStyle)
			}
		} else if item.Checked && !item.WasAdded {
			g0 = RenderThemeText("{{|MarkerAdded|}}+{{[-]}}", neutralStyle)
		} else if !item.Checked && item.WasAdded {
			g0 = RenderThemeText("{{|MarkerDeleted|}}-{{[-]}}", neutralStyle)
		} else {
			g0 = neutralStyle.Render(" ")
		}

		if m.activityGutterWidth >= 2 {
			if item.Enabled && !item.WasEnabled {
				g1 = RenderThemeText("{{|MarkerAdded|}}E{{[-]}}", neutralStyle)
			} else if !item.Enabled && item.WasEnabled && (item.Checked || !item.WasAdded) {
				g1 = RenderThemeText("{{|MarkerDeleted|}}D{{[-]}}", neutralStyle)
			} else {
				g1 = neutralStyle.Render(" ")
			}
		}

		tagStr := ""
		if item.IsEditing {
			// Using the standard edit styling (red background/bold)
			editTag := GetPlainText(item.Tag)
			tagStr += theme.ThemeSemanticStyle("{{|ItemFocused|}}").Render(editTag)
		} else if len(item.Tag) > 0 {
			runes := []rune(item.Tag)
			tagStr += kStyle.Render(string(runes[0])) + tStyle.Render(string(runes[1:]))
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

		// Brackets show only on the specific column with keyboard focus, same
		// convention as the top-level app row's cbAdd3/cbEnabled3 above.
		addFocused := isSelected && subFocused && m.activeColumn == ColAdd
		enableFocused := isSelected && subFocused && m.activeColumn == ColEnable

		var checkboxA3, checkboxE3 string
		if ctx.LineCharacters {
			cA, cE := checkOffBare, checkOffBare
			if addFocused {
				cA = checkOff
			}
			if enableFocused {
				cE = checkOff
			}
			if item.Checked {
				cA = checkOnBare
				if addFocused {
					cA = checkOn
				}
			}
			if item.Enabled {
				cE = checkOnBare
				if enableFocused {
					cE = checkOn
				}
			}
			checkboxA3 = renderCheckboxGlyph(cA, cbStyleA)
			checkboxE3 = renderCheckboxGlyph(cE, cbStyleE)
		} else {
			caA, ceA := checkOffAscii, checkOffAscii
			if item.Checked {
				caA = checkOnAscii
			}
			if item.Enabled {
				ceA = checkOnAscii
			}
			if addFocused {
				checkboxA3 = neutralStyle.Render("[") + cbStyleA.Render(string(caA[1])) + neutralStyle.Render("]")
			} else {
				checkboxA3 = neutralStyle.Render(" ") + cbStyleA.Render(string(caA[1])) + neutralStyle.Render(" ")
			}
			if enableFocused {
				checkboxE3 = neutralStyle.Render("[") + cbStyleE.Render(string(ceA[1])) + neutralStyle.Render("]")
			} else {
				checkboxE3 = neutralStyle.Render(" ") + cbStyleE.Render(string(ceA[1])) + neutralStyle.Render(" ")
			}
		}

		// Sub-menus require a 10-character indent to align with the top/bottom borders.
		// Indent consists of: g0(1) + g1(1) + 8 spaces.
		indent := neutralStyle.Render(strutil.Repeat(" ", 8))

		// The rowContent starts with the left border, followed by a mandatory internal space.
		rowContent := vStyleLight.Render(vBorderChar) + neutralStyle.Render(" ") + checkboxA3 + neutralStyle.Render(" ") + checkboxE3 + neutralStyle.Render(" ") + tagStr
		rowWidth := subListWidth - 1
		pContent := rowContent + neutralStyle.Render(strutil.Repeat(" ", max(0, rowWidth-lipgloss.Width(GetPlainText(rowContent)))))

		itemGutter := lockMarker + g0
		if m.activityGutterWidth >= 2 {
			itemGutter += g1
		}
		line := itemGutter + indent + pContent + vStyleDark.Render(vBorderChar)

		resLines = append(resLines, line+semstyle.CodeReset)
		resH = append(resH, 1)
		resM = append(resM, visibleIdx)
	}

	// 3. Build Bottom Border with 1 dash.
	bottomBorder := BuildAEBottomBorder(subListWidth, 1, subFocused, m.activeColumn, -1, ctx)
	resLines = append(resLines, neutralStyle.Render(strutil.Repeat(" ", 10))+bottomBorder+semstyle.CodeReset)
	resH = append(resH, 1)
	resM = append(resM, startVisibleIndex|vIdxBorderFlag) // Flag as border

	return resLines, resH, resM
}
