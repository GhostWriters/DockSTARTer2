package tui

import (
	"strconv"
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
	// Base vertical offset depends on whether we are in a sub-menu or a full dialog
	listY := 0
	if m.subMenuMode {
		// Sub-menus use RenderBorderedBoxCtx which puts the title IN the top border (line 0).
		// Thus content always starts at line 1 relative to the box's top-left.
		// listY = 0 logic here is correct as a base.
	} else {
		// Full dialogs start content inside the outer border (1 line)
		listY = layout.DialogBorder / 2
	}

	// Account for subtitle if present
	if m.subtitle != "" {
		// Subtitle width matches the content area (inside borders)
		contentWidth := m.width
		if m.subMenuMode {
			contentWidth -= layout.BorderWidth()
		} else {
			// Matches innerParts logic: contentWidth - 2 (left/right margin)
			contentWidth = m.GetInnerContentWidth()
		}

		subtitleStyle := styles.Dialog.Width(contentWidth).Padding(0, 1).Border(lipgloss.Border{})
		subStr := RenderThemeText(m.subtitle, subtitleStyle)
		listY += lipgloss.Height(subtitleStyle.Render(subStr))
	}

	// Account for inner border around the list (Top = 1 line)
	listY += layout.BorderWidth() / 2

	// 2. Horizontal Positioning (X)
	// Relative to dialog start: Outer Border (1) + Margin (1) + Inner Border (1) = 3
	listX := layout.BorderWidth() / 2
	if !m.subMenuMode {
		// Padding used in ViewString's marginStyle is 1
		const marginPadding = 1
		listX = (layout.DialogBorder / 2) + marginPadding + (layout.BorderWidth() / 2)
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
		if m.variableHeight {
			// Variable height list hit testing uses the cached lastHitRegions
			// which contain coordinates relative to the list's own start.
			for _, r := range m.lastHitRegions {
				// Extract itemIndex from the ID (e.g. "app-select.item-42")
				itemIndex := -1
				parts := strings.Split(r.ID, "-")
				if len(parts) > 1 {
					itemIndex, _ = strconv.Atoi(parts[len(parts)-1])
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
			// Standard height list items
			visibleItems := m.list.VisibleItems()
			startIndex := m.list.Paginator.Page * m.list.Paginator.PerPage
			itemWidth := m.list.Width()

			for i := 0; i < len(visibleItems); i++ {
				itemIndex := startIndex + i
				if itemIndex >= len(m.items) {
					break
				}
				if m.items[itemIndex].IsSeparator {
					continue
				}

				item := m.items[itemIndex]
				regions = append(regions, HitRegion{
					ID:     GetMenuItemID(m.id, itemIndex),
					X:      offsetX + listX,
					Y:      offsetY + listY + i,
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
	if currentConfig.UI.Scrollbar && m.sbInfo.Needed {
		sbX := offsetX + listX + m.list.Width()
		sbTopY := offsetY + listY
		m.sbAbsTopY = sbTopY // store for drag-to computation in scrollbarDragTo

		info := m.sbInfo

		// Up arrow (row 0)
		regions = append(regions, HitRegion{
			ID:     m.id + ".sb.up",
			X:      sbX,
			Y:      sbTopY,
			Width:  1,
			Height: 1,
			ZOrder: baseZ + 20,
			Label:  "Scroll Up",
		})

		// Track above thumb (rows 1..ThumbStart-1)
		if aboveH := info.ThumbStart - 1; aboveH > 0 {
			regions = append(regions, HitRegion{
				ID:     m.id + ".sb.above",
				X:      sbX,
				Y:      sbTopY + 1,
				Width:  1,
				Height: aboveH,
				ZOrder: baseZ + 20,
				Label:  "Page Up",
			})
		}

		// Thumb (rows ThumbStart..ThumbEnd-1)
		if thumbH := info.ThumbEnd - info.ThumbStart; thumbH > 0 {
			regions = append(regions, HitRegion{
				ID:     m.id + ".sb.thumb",
				X:      sbX,
				Y:      sbTopY + info.ThumbStart,
				Width:  1,
				Height: thumbH,
				ZOrder: baseZ + 21,
				Label:  "Scroll Thumb",
			})
		}

		// Track below thumb (rows ThumbEnd..Height-2)
		if belowH := (info.Height - 1) - info.ThumbEnd; belowH > 0 {
			regions = append(regions, HitRegion{
				ID:     m.id + ".sb.below",
				X:      sbX,
				Y:      sbTopY + info.ThumbEnd,
				Width:  1,
				Height: belowH,
				ZOrder: baseZ + 20,
				Label:  "Page Down",
			})
		}

		// Down arrow (row Height-1)
		regions = append(regions, HitRegion{
			ID:     m.id + ".sb.down",
			X:      sbX,
			Y:      sbTopY + info.Height - 1,
			Width:  1,
			Height: 1,
			ZOrder: baseZ + 20,
			Label:  "Scroll Down",
		})
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
			// and there is no gap, so buttons start at listY + listHeight + 1
			buttonY += 1
		}
		// In subMenuMode, there is no inner border around the list, so JoinVertical
		// puts buttons immediately after the last content line.

		// Calculate horizontal offset for buttons.
		// Use a precise offset that matches the padding in ViewString
		buttonX := offsetX
		contentWidth := m.GetInnerContentWidth()
		if !m.subMenuMode {
			// Matches innerBoxWidth in ViewString: contentWidth - 2
			buttonX += (layout.DialogBorder / 2) + 2 // Total 3 char offset
			contentWidth -= 2                        // Subtract margin padding
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
