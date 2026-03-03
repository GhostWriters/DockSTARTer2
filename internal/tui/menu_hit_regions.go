package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (m *MenuModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion

	if m.width == 0 {
		return regions
	}

	// Calculate list position by measuring rendered components
	styles := GetStyles()
	listY := 0
	listX := 1 // default: inside outer border
	subtitleHeight := 0

	listY = 1

	// Subtitle adds its rendered height.
	// Use outerContentWidth (list.Width + 4) to match actual rendering — the subtitle
	// is rendered at that width in ViewString, not at the narrower list.Width(), so
	// that text wraps the same number of times as in the real output.
	// Crucially, expand theme tags via RenderThemeText first so the measured height
	// matches what ViewString actually renders (unexpanded tags add spurious characters
	// that cause extra wrapping and push all item hit regions down by 1+ rows).
	if m.subtitle != "" {
		subtitleWidth := m.list.Width() + 4
		subtitleStyle := styles.Dialog.Width(subtitleWidth).Padding(0, 1).Border(lipgloss.Border{})
		subStr := RenderThemeText(m.subtitle, subtitleStyle)
		subtitleRendered := subtitleStyle.Render(subStr)
		subtitleHeight = lipgloss.Height(subtitleRendered)
		listY += subtitleHeight
	}

	// In standard mode the list is wrapped in an inner border with a 1-char margin on
	// each side: outer(1) + margin(1) + inner border(1) = listX 3, listY +1.
	// In subMenuMode items render directly inside the outer border with no inner border
	// and no margin, so both adjustments are skipped.
	if !m.subMenuMode {
		listX = 3 // outer border (1) + margin (1) + inner border (1)
		listY += 1
	}

	baseZ := ZScreen
	if m.isDialog {
		baseZ = ZDialog
	}

	// Item regions: vertical list mode vs horizontal flow mode.
	if !m.flowMode {
		if m.variableHeight {
			// Variable height list: replicate renderVariableHeightList logic
			layout := GetLayout()
			maxWidth, maxHeight := layout.InnerContentSize(m.width, m.height)
			if maxWidth > 2 {
				maxWidth -= 2
			}

			filter := m.list.FilterValue()
			var visibleItems []MenuItem
			for _, item := range m.items {
				if filter != "" && !strings.Contains(strings.ToLower(item.Tag), strings.ToLower(filter)) {
					continue
				}
				visibleItems = append(visibleItems, item)
			}

			if len(visibleItems) > 0 {
				ctx := GetActiveContext()
				var itemHeights []int
				maxTagLen := calculateMaxTagLength(visibleItems)
				totalHeight := 0
				for _, item := range visibleItems {
					if item.IsSeparator {
						itemHeights = append(itemHeights, 1)
						totalHeight += 1
						continue
					}
					cbWidth := 0
					if item.IsCheckbox || item.IsRadioButton {
						cbWidth = 2 // standard [ ] or ( )
						if !ctx.LineCharacters {
							cbWidth = 4 // standard [ ] or (*)
						}
					}
					prefixWidth := cbWidth + maxTagLen + 3
					availableWidth := maxWidth - prefixWidth
					descStr := RenderThemeText(item.Desc, styles.Dialog)
					wrapped := lipgloss.NewStyle().Width(availableWidth).Render(descStr)
					h := lipgloss.Height(wrapped)
					itemHeights = append(itemHeights, h)
					totalHeight += h
				}

				actualSelectedVisibleIndex := m.list.Index()

				currentY := 0
				for i := 0; i < actualSelectedVisibleIndex && i < len(itemHeights); i++ {
					currentY += itemHeights[i]
				}

				viewStart := currentY - maxHeight/2
				if viewStart < 0 {
					viewStart = 0
				}
				if viewStart+maxHeight > totalHeight {
					viewStart = totalHeight - maxHeight
				}
				if totalHeight <= maxHeight {
					viewStart = 0
				}

				for _, r := range m.lastHitRegions {
					r.X = offsetX + listX
					r.Y = offsetY + listY + r.Y
					r.Width = maxWidth
					r.ZOrder = baseZ + 10
					regions = append(regions, r)
				}
			}

		} else {
			// Calculate visible items
			visibleItems := m.list.VisibleItems()
			startIndex := m.list.Paginator.Page * m.list.Paginator.PerPage

			// Item regions
			itemWidth := m.list.Width()
			for i := 0; i < len(visibleItems); i++ {
				itemIndex := startIndex + i
				if itemIndex >= len(m.items) {
					break
				}
				item := m.items[itemIndex]
				if item.IsSeparator {
					continue
				}

				regions = append(regions, HitRegion{
					ID:     GetMenuItemID(m.id, itemIndex),
					X:      offsetX + listX,
					Y:      offsetY + listY + i,
					Width:  itemWidth,
					Height: 1,
					ZOrder: baseZ + 10,
				})
			}
		}
	} else {
		// Flow mode: items are arranged horizontally across multiple lines.
		// Replicate the layout logic from renderFlow() to compute per-item positions.
		layout := GetLayout()
		maxWidth, _ := layout.InnerContentSize(m.width, m.height)
		if maxWidth > 2 {
			maxWidth -= 2
		}

		ctx := GetActiveContext()
		const itemSpacing = 3
		// lineContentX: items start 1 char past listX because renderFlow applies
		// lineStyle.Padding(0, 1) which adds a 1-char left margin.
		lineContentX := listX + 1

		flowLine := 0
		currentLineWidth := 0

		for i, item := range m.items {
			if item.IsSeparator {
				continue
			}

			// Compute visual width of this item, matching renderFlow's logic.
			cbWidth := 0
			if item.IsRadioButton || item.IsCheckbox {
				var glyph string
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
			if !item.IsCheckbox && !item.IsRadioButton && item.Desc != "" {
				itemWidth += 1 + lipgloss.Width(GetPlainText(item.Desc))
			}

			// Determine item's position within the current line.
			var itemX int
			if currentLineWidth > 0 && currentLineWidth+itemSpacing+itemWidth > maxWidth {
				// This item wraps to the next line.
				flowLine++
				itemX = 0
				currentLineWidth = itemWidth
			} else {
				if currentLineWidth > 0 {
					itemX = currentLineWidth + itemSpacing
					currentLineWidth += itemSpacing + itemWidth
				} else {
					itemX = 0
					currentLineWidth = itemWidth
				}
			}

			regions = append(regions, HitRegion{
				ID:     GetMenuItemID(m.id, i),
				X:      offsetX + lineContentX + itemX,
				Y:      offsetY + listY + flowLine,
				Width:  itemWidth,
				Height: 1,
				ZOrder: baseZ + 10,
			})
		}
	}

	// Button regions are only valid for full (non-subMenu) menus. SubMenu panels don't
	// render their own button row, so generating regions for them would create spurious
	// hit targets at incorrect positions.
	if !m.subMenuMode {
		specs := m.getButtonSpecs()
		if len(specs) > 0 {
			// Get dialog height without shadow
			content := m.ViewString()
			dialogH := lipgloss.Height(content)

			// Account for shadow (1 line at bottom)
			hasShadow := currentConfig.UI.Shadow
			if hasShadow {
				dialogH -= 1
			}

			// Button box starts 3 lines from bottom: bottom border (1) + button border (1) + button text (1)
			// We want to cover all 3 lines of the button box (border + text + border)
			buttonY := dialogH - 4 // Start at top of button box

			// Calculate actual dialog width (not m.width which is available space)
			dialogW := WidthWithoutZones(content)
			if hasShadow {
				dialogW -= 2 // shadow adds 2 chars on right
			}

			contentWidth := dialogW - 2 // inside borders

			// Background region covering the whole button row — lets hover+scroll cycle buttons
			// even when the mouse is in the gap between individual buttons.
			// ZDialog+15 sits below individual button regions (ZDialog+20).
			regions = append(regions, HitRegion{
				ID:     IDButtonPanel,
				X:      offsetX + 1,
				Y:      offsetY + buttonY,
				Width:  contentWidth,
				Height: DialogButtonHeight,
				ZOrder: baseZ + 15,
			})

			// Use centralized helper for button hit regions (empty dialogID since menus don't need prefixes)
			regions = append(regions, GetButtonHitRegions(
				"", offsetX+1, offsetY+buttonY, contentWidth, baseZ+20,
				specs...,
			)...)
		}
	}

	return regions
}
