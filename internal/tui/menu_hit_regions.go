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

	// Single source of truth for all layout math
	layout := GetLayout()
	styles := GetStyles()

	// 1. Vertical Positioning (Y)
	// All menus start inside an outer Top Border (1 line).
	listY := layout.SingleBorder()

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

	// Calculate inner dimensions for the background list region
	maxWidth := m.list.Width()
	maxHeight := m.layout.ViewportHeight

	// 3. List panel region (covers only the list items, for scroll focus)
	regions = append(regions, HitRegion{
		ID:     m.id + "." + IDListPanel,
		X:      offsetX + listX,
		Y:      offsetY + listY,
		Width:  m.list.Width(),
		Height: m.list.Height(),
		ZOrder: baseZ + 7, // Below items (+10) but above backdrop (+5)
		Label:  m.title,
	})

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
	} else {
		// Flow mode: items are arranged horizontally across multiple lines.
		// Replicate the layout logic from renderFlow() to compute per-item positions.
		// In flow mode, available width is the inner width minus 2 (for side padding)
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

			// Compute visual width of this item
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

			// Determine item's position within the current line
			var itemX int
			if currentLineWidth > 0 && currentLineWidth+itemSpacing+itemWidth > maxWidth {
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
		sbX := offsetX + listX + m.list.Width()
		regions = append(regions, m.Scroll.HitRegions(sbX, offsetY+listY, baseZ, m.title)...)
	}


	// 4. Button regions
	specs := m.getButtonSpecs()
	if len(specs) > 0 {
		listHeight := m.list.Height()
		if m.flowMode {
			listHeight = m.GetFlowHeight(m.width)
		}

		buttonY := listY + listHeight
		if !m.subMenuMode {
			// In full dialog mode, the list has an inner border (+1 line bottom)
			// and there is no gap, so buttons start at listY + listHeight + SingleBorder
			buttonY += layout.SingleBorder()
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
			contentWidth -= layout.ContentMarginWidth()                      // Subtract margin padding
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

		// Use centralized helper for button hit regions
		regions = append(regions, GetButtonHitRegions(
			HelpContext{ScreenName: m.title, PageTitle: "Description", PageText: m.subtitle},
			m.id, buttonX, offsetY+buttonY, contentWidth, baseZ+20,
			specs...,
		)...)
	}

	return regions
}
