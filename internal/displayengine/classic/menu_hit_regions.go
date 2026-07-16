package classic

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

	// Plain-text kind is a read-only display line with nothing to click,
	// hover, or focus -- never in Tab-cycling (Focusable() is false) and
	// never routed through the list/subtitle/section hit-region math below,
	// which assumes the standard list rendering path.
	if m.plainText != "" {
		return regions
	}

	// Borderless contentRenderer sections have no border/frame for the
	// generic list-panel math below to position against -- they're fully
	// responsible for their own hit regions via extraHitRegions, offset
	// directly from offsetX/offsetY (no border inset assumed).
	if m.borderless && m.ContentRenderer != nil {
		if m.ExtraHitRegions != nil {
			baseZ := ZScreen
			if m.isDialog {
				baseZ = ZDialog
			}
			regions = append(regions, m.ExtraHitRegions(offsetX, offsetY, baseZ)...)
		}
		return regions
	}

	// Single source of truth for all layout math
	layout := GetLayout()
	styles := GetStyles()

	// 1. Vertical Positioning (Y)
	// All menus start inside an outer Top Border (1 line).
	listY := layout.SingleBorder()
	if m.Layout.LargeTitleBar {
		listY += LargeTitleBarOverhead
	}

	// Account for subtitle if present (positioned above the list content)
	subtitleH := 0
	if m.subtitle != "" {
		contentWidth := m.width
		if m.subMenuMode {
			contentWidth -= layout.BorderWidth()
		} else {
			contentWidth = m.GetInnerContentWidth()
		}

		subtitleStyle := styles.Dialog.Width(contentWidth).Padding(0, layout.ContentSideMargin).Border(lipgloss.Border{})
		subStr := RenderThemeText(m.subtitle, subtitleStyle)
		subtitleH = lipgloss.Height(subtitleStyle.Render(subStr))
		listY += subtitleH
	}

	// Full dialogs have a NESTED inner border around the list (1 line).
	// Sub-menus only have the one outer border.
	if !m.subMenuMode {
		listY += layout.SingleBorder()
	}

	// 2. Horizontal Positioning (X)
	// Outer Border (1) + optional Margin (1) + optional Inner Border (1)
	listX := layout.SingleBorder()
	if !m.subMenuMode {
		// Full dialogs have a 1-char content margin + 1-char inner list border
		listX += layout.ContentSideMargin + layout.SingleBorder()
	}

	baseZ := ZScreen
	if m.isDialog {
		baseZ = ZDialog
	}

	// Submenu frame catch-all: covers the full bordered panel including title and border chars.
	// Lets clicking the border or empty space focus the section in the parent outer dialog.
	if m.subMenuMode {
		frameH := m.height
		if frameH > 0 && m.width > 0 {
			regions = append(regions, HitRegion{
				ID:     m.id + ".frame",
				X:      offsetX,
				Y:      offsetY,
				Width:  m.width,
				Height: frameH,
				ZOrder: baseZ - 1,
				Label:  m.title,
			})
		}
	}

	// Outer frame catch-all: covers the full dialog including border, title bar, and subtitle.
	// Placed at baseZ-1 so all specific item/button/widget regions take priority via higher Z.
	// This ensures clicks on non-item areas still register as a hit, allowing header/panel focus
	// to be cleared and the dialog's previously focused items to be restored.
	if !m.subMenuMode && m.title != "" {
		frameW := m.GetInnerContentWidth() + GetLayout().BorderWidth()
		frameH := m.Layout.Height
		if frameH <= 0 {
			frameH = m.height
		}
		if frameW > 0 && frameH > 0 {
			regions = append(regions, HitRegion{
				ID:     m.id + ".frame",
				X:      offsetX,
				Y:      offsetY,
				Width:  frameW,
				Height: frameH,
				ZOrder: baseZ - 1,
				Label:  m.title,
				Help: &HelpContext{
					ScreenName: m.title,
					PageTitle:  "Description",
					PageText:   m.subtitle,
				},
			})
		}
	}

	// Calculate inner dimensions for the background list region
	maxWidth := m.list.Width()
	maxHeight := m.Layout.ViewportHeight

	// 3. List panel region (covers only the list items, for scroll focus)
	// Use m.Layout.ViewportHeight, not m.list.Height(): flow mode menus never call
	// m.list.SetSize so m.list.Height() stays at the initial item count and would
	// extend the region far below the visible section into adjacent panels.
	// Skip for contentRenderer sections — they register their own hit regions via extraHitRegions.
	if m.ContentRenderer == nil {
		regions = append(regions, HitRegion{
			ID:     m.id + "." + IDListPanel,
			X:      offsetX + listX,
			Y:      offsetY + listY,
			Width:  m.list.Width(),
			Height: m.Layout.ViewportHeight,
			ZOrder: baseZ + 7, // Below items (+10) but above backdrop (+5)
			Label:  m.title,
		})
	}

	// Background region for the whole list — allows hover+scroll over the gaps
	regions = append(regions, HitRegion{
		ID:     m.id,
		X:      offsetX + listX,
		Y:      offsetY + listY,
		Width:  maxWidth,
		Height: maxHeight,
		ZOrder: baseZ,
		Label:  m.title,
		Help: &HelpContext{
			ScreenName: m.title,
			PageTitle:  "Description",
			PageText:   m.subtitle,
		},
	})

	// flowMaxWidth is the usable content width passed to renderFlowContent — used for
	// both item wrapping and GetFlowHeight so the two stay in sync.
	var flowMaxWidth int
	if m.subMenuMode {
		flowMaxWidth = m.width - layout.BorderWidth()
	} else {
		flowMaxWidth, _ = layout.InnerContentSize(m.width, m.height)
		if flowMaxWidth > 2 {
			flowMaxWidth -= 2
		}
	}

	// Item regions: vertical list mode vs horizontal flow mode.
	if !m.flowMode {
		// All vertical lists (uniform and variable) now use our custom layout engine.
		// We use the cached lastHitRegions generated during the render pass.
		for _, r := range m.lastHitRegions {
			// Robustly identify the item index using the shared helper.
			// This handles IDs with various prefixes and suffixes (e.g. -border, -expand).
			itemID := r.ID
			for _, sfx := range []string{"-add", "-enable", "-expand", "-border", "-parent"} {
				itemID = strings.TrimSuffix(itemID, sfx)
			}

			itemIndex, ok := ParseMenuItemIndex(itemID, m.id)
			if !ok {
				continue
			}

			var label string
			var help *HelpContext
			if itemIndex >= 0 && itemIndex < len(m.items) {
				item := m.items[itemIndex]
				label = GetPlainText(item.Tag)
				help = &HelpContext{
					ScreenName: m.title,
					PageTitle:  "Menu Item",
					ItemTitle:  GetPlainText(item.Tag),
					ItemText:   item.Desc,
				}
			}

			regions = append(regions, HitRegion{
				ID:     r.ID,
				X:      offsetX + listX + r.X,
				Y:      offsetY + listY + r.Y,
				Width:  r.Width,
				Height: r.Height,
				ZOrder: baseZ + 10,
				Label:  label,
				Help:   help,
			})
		}
	} else if m.FlowColumns >= 2 {
		// Column mode: items fill columns top-to-bottom, matching renderColumnContent exactly.
		ctx := GetActiveContext()
		numCols := m.FlowColumns
		colGap := 2
		// +1 for the 1-char left padding from lineStyle.Padding(0,1)
		lineContentX := listX + 1

		// Collect non-separator item indices.
		var itemIndices []int
		for i, item := range m.items {
			if !item.IsSeparator {
				itemIndices = append(itemIndices, i)
			}
		}
		n := len(itemIndices)
		rows := (n + numCols - 1) / numCols

		// Measure plain-text width of each item (no-selection, matches renderColumnContent).
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

		// Find widest item per column (matches renderColumnContent).
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

		// Compute X offset for the start of each column.
		colOffsets := make([]int, numCols)
		x := 0
		for col := 0; col < numCols; col++ {
			colOffsets[col] = x
			x += colWidths[col] + colGap
		}

		// Emit hit regions using (col, row) → (x, y) mapping.
		for ni, ii := range itemIndices {
			col := ni / rows
			row := ni % rows
			regions = append(regions, HitRegion{
				ID:     GetMenuItemID(m.id, ii),
				X:      offsetX + lineContentX + colOffsets[col],
				Y:      offsetY + listY + row,
				Width:  itemWidths[ni],
				Height: 1,
				ZOrder: baseZ + 10,
				Label:  GetPlainText(m.items[ii].Tag),
				Help: &HelpContext{
					ScreenName: m.title,
					PageTitle:  "Menu Item",
					ItemTitle:  GetPlainText(m.items[ii].Tag),
					ItemText:   m.items[ii].Desc,
				},
			})
		}
	} else {
		// Flow mode: items are arranged horizontally across multiple lines.
		// Uses flowMaxWidth (computed above) to match renderFlowContent wrapping exactly.
		ctx := GetActiveContext()
		itemSpacing := FlowItemSpacing
		// lineContentX: items start 1 char past listX because renderFlowContent applies
		// lineStyle.Padding(0, 1) which adds a 1-char left margin.
		lineContentX := listX + 1

		flowLine := 0
		currentLineWidth := 0

		// Matches renderFlowContent's itemGutter, which prepends a lock-gutter
		// column (lockChar, always 1 char when showLockGutter is true) before
		// every item's checkbox/tag content.
		lockMarkerWidth := 0
		if m.showLockGutter {
			lockMarkerWidth = m.StatusGutterWidth()
		}

		for i, item := range m.items {
			if item.IsSeparator {
				continue
			}

			// Compute visual width of this item
			cbWidth := 0
			if item.IsRadioButton || item.IsCheckbox {
				var glyph string
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

			itemWidth := lockMarkerWidth + cbWidth + lipgloss.Width(GetPlainText(item.Tag))
			if !item.IsCheckbox && !item.IsRadioButton && item.Desc != "" {
				itemWidth += 1 + lipgloss.Width(GetPlainText(item.Desc))
			}

			// Determine item's position within the current line
			var itemX int
			if currentLineWidth > 0 && currentLineWidth+itemSpacing+itemWidth > flowMaxWidth {
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
				Label:  GetPlainText(item.Tag),
				Help: &HelpContext{
					ScreenName: m.title,
					PageTitle:  "Menu Item",
					ItemTitle:  GetPlainText(item.Tag),
					ItemText:   item.Desc,
				},
			})
		}
	}

	// 3b. Scrollbar hit regions (when scrollbar is active)
	if currentConfig.UI.Scrollbar && m.Scroll.Info.Needed {
		var sbX int
		if m.FlowColumns >= 2 && m.MaxFlowRows > 0 {
			// Column mode: scrollbar is at the right edge of the content area.
			sbX = offsetX + listX + (m.width - layout.BorderWidth() - ScrollbarGutterWidth)
		} else {
			sbX = offsetX + listX + m.list.Width()
		}
		regions = append(regions, m.Scroll.HitRegions(sbX, offsetY+listY, baseZ, m.title)...)
	}

	// 4. Button regions
	specs := m.GetButtonSpecsForState()
	if len(specs) > 0 {
		var buttonY int
		if len(m.contentSections) > 0 {
			// Use pre-computed ButtonY from calculateSectionLayout.
			buttonY = m.Layout.ButtonY
		} else {
			listHeight := m.list.Height()
			if m.flowMode {
				listHeight = m.GetFlowHeight(flowMaxWidth)
			}
			buttonY = listY + listHeight
			if !m.subMenuMode {
				// In full dialog mode, the list has an inner border (+1 line bottom)
				// and there is no gap, so buttons start at listY + listHeight + SingleBorder
				buttonY += layout.SingleBorder()
			}
		}
		// In subMenuMode, there is no inner border around the list, so JoinVertical
		// puts buttons immediately after the last content line.

		// Calculate horizontal offset for buttons.
		// Use a precise offset that matches the padding in ViewString
		buttonX := offsetX
		contentWidth := m.GetInnerContentWidth()
		if !m.subMenuMode {
			// Matches innerBoxWidth in ViewString: outer border + margin
			buttonX += layout.LeftOffset()
			contentWidth -= layout.ContentMarginWidth() // Subtract margin padding
		}

		// Background region covering the whole button row
		regions = append(regions, HitRegion{
			ID:     m.id + "." + IDButtonPanel,
			X:      buttonX,
			Y:      offsetY + buttonY,
			Width:  contentWidth,
			Height: layout.ButtonHeight,
			ZOrder: baseZ + 15,
			Label:  "Actions",
			Help: &HelpContext{
				ScreenName: m.title,
				PageTitle:  "Description",
				PageText:   m.subtitle,
			},
		})

		// Use centralized helper for button hit regions, passing the already-decided
		// border/height choice so hit regions never disagree with what rendered
		// (m.Layout.ButtonHeight may be flat-downgraded due to height, not just width).
		regions = append(regions, GetButtonHitRegionsExplicit(
			HelpContext{ScreenName: m.title, PageTitle: "Description", PageText: m.subtitle},
			m.id, buttonX, offsetY+buttonY, contentWidth, baseZ+20,
			m.Layout.ButtonHeight == DialogButtonHeight,
			specs...,
		)...)
	}

	// 4b. Section hit regions — delegate to each content section at correct Y offset.
	if len(m.contentSections) > 0 {
		secOffsetY := offsetY + layout.SingleBorder()
		if m.Layout.LargeTitleBar {
			secOffsetY += LargeTitleBarOverhead
		}
		// Sections are inset by outer left border (1) + ContentSideMargin.
		secOffsetX := offsetX + layout.ContentInset()
		for _, sec := range m.contentSections {
			regions = append(regions, sec.GetHitRegions(secOffsetX, secOffsetY)...)
			secOffsetY += sec.Height()
		}
	}

	// 5. Hyperlink hit regions
	regions = append(regions, ScanForHyperlinks(m.ViewString(), offsetX, offsetY, baseZ)...)

	// 6. Title bar widget hit regions ([?] and [×]/[X])
	// Widget layout in title bar: "[?] [×]" = 7 chars + 1 end pad before TopRight corner.
	// Widgets appear at the right of the title bar (row 0). Sub-menus never get widgets.
	if m.title != "" && !m.subMenuMode {
		const widgetTotalWidth = 7 // "[?] [×]" or "[?] [X]"
		const endPad = 1
		// Use actual rendered dialog width, not m.width — non-maximized menus render
		// narrower than m.width based on content, so the widget X must match.
		dialogWidth := m.GetInnerContentWidth() + GetLayout().BorderWidth()
		widgetsStartX := offsetX + dialogWidth - 1 - endPad - widgetTotalWidth
		// Widget layout: "[?]" (3) + " " (1) + "[×]" (3) — help starts at 0, close at +4.
		helpWidgetX := widgetsStartX
		closeWidgetX := widgetsStartX + 4 // "[?] " = 4 chars
		widgetY := TitleBarWidgetY(offsetY, m.Layout.LargeTitleBar)
		regions = append(regions,
			HitRegion{
				ID: m.id + "." + IDTitleWidgetHelp,
				X:  helpWidgetX, Y: widgetY, Width: 3, Height: 1,
				ZOrder: baseZ + 25,
				Label:  "Help",
				Help:   &HelpContext{ScreenName: m.title, PageTitle: "Help", PageText: "Open help for this dialog."},
			},
			HitRegion{
				ID: m.id + "." + IDTitleWidgetClose,
				X:  closeWidgetX, Y: widgetY, Width: 3, Height: 1,
				ZOrder: baseZ + 25,
				Label:  "Close",
				Help:   &HelpContext{ScreenName: m.title, PageTitle: "Close", PageText: "Close this dialog."},
			},
		)
	}

	// Extra hit regions from section helpers (e.g. sinput text area).
	if m.ExtraHitRegions != nil {
		regions = append(regions, m.ExtraHitRegions(offsetX, offsetY, baseZ)...)
	}

	return regions
}
