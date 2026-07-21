package classic

// GetHitRegions implements HitRegionProvider for mouse hit testing.
func (m PanelModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion

	panelHelp := &HelpContext{
		ScreenName: "Console Panel",
		PageTitle:  "Viewer",
		PageText:   "Displays live application logs and accepts ds2/shell commands.",
		ItemTitle:  "Console Panel",
		ItemText:   "Scroll with the mouse wheel or use Home/End/PgUp/PgDn when focused.",
	}
	inputHelp := &HelpContext{
		ScreenName: "Console Panel",
		PageTitle:  "Input",
		PageText:   "Type ds2 commands or shell commands and press Enter to run them.",
		ItemTitle:  "Console Input",
		ItemText:   "Enter: run | Up/Down: history | Esc: exit input",
	}

	ctx := GetActiveContext()
	title := m.Title()

	consoleTitleStyle := SemanticRawStyle("ConsoleTitle")
	titleWidth := WidthWithoutZones(RenderThemeText(title, consoleTitleStyle))
	titleLayout := ComputeTitleBarLayout(titleWidth, m.width, ctx.PanelTitleAlign)
	titleStart := titleLayout.Start
	titleEnd := titleLayout.End

	// ZPanelHeader: the panel title bar (drag handle) must beat any non-modal screen or dialog
	// content so it remains clickable when dragged up to overlap the bottom of the active screen.
	// Non-modal max is ZDialog+25=55; modals start at ZModalBaseOffset=100.
	// ZDialog+30=60 sits above all non-modal content and below modals (which should trap input).
	const ZPanelHeader = ZDialog + 30

	regions = append(regions, HitRegion{
		ID:     IDPanelResize,
		X:      offsetX,
		Y:      offsetY,
		Width:  titleStart,
		Height: 1,
		ZOrder: ZPanelHeader,
		Label:  "Console Panel",
		Help:   panelHelp,
	})
	regions = append(regions, HitRegion{
		ID:     IDPanelToggle,
		X:      offsetX + titleStart,
		Y:      offsetY,
		Width:  titleLayout.TitleSectionLen,
		Height: 1,
		ZOrder: ZPanelHeader,
		Label:  "Console Panel",
		Help:   panelHelp,
	})
	// Right side of title bar: resize widgets [▲] [▼] only when expanded.
	if m.Expanded {
		const widgetTotalWidth = 7
		const endPad = 1
		widgetsStartX := m.width - 1 - endPad - widgetTotalWidth
		resizeUpX := widgetsStartX     // [▲] starts here
		resizeDnX := widgetsStartX + 4 // "[▲] " = 4 chars before [▼]
		dragWidth := widgetsStartX - titleEnd
		if dragWidth < 0 {
			dragWidth = 0
		}
		if dragWidth > 0 {
			regions = append(regions, HitRegion{
				ID:     IDPanelResize,
				X:      offsetX + titleEnd,
				Y:      offsetY,
				Width:  dragWidth,
				Height: 1,
				ZOrder: ZPanelHeader,
				Label:  "Console Panel",
				Help:   panelHelp,
			})
		}
		regions = append(regions,
			HitRegion{
				ID: IDPanelResizeUp, X: offsetX + resizeUpX, Y: offsetY,
				Width: 3, Height: 1, ZOrder: ZPanelHeader + 1,
				Label: "Grow panel",
				Help:  &HelpContext{ScreenName: "Console Panel", PageTitle: "Resize", PageText: "Click to grow the panel by one line."},
			},
			HitRegion{
				ID: IDPanelResizeDn, X: offsetX + resizeDnX, Y: offsetY,
				Width: 3, Height: 1, ZOrder: ZPanelHeader + 1,
				Label: "Shrink panel",
				Help:  &HelpContext{ScreenName: "Console Panel", PageTitle: "Resize", PageText: "Click to shrink the panel by one line."},
			},
		)
	}

	if m.Expanded {
		vpH := m.ViewportHeight()
		regions = append(regions, HitRegion{
			ID:     IDPanelViewport,
			X:      offsetX,
			Y:      offsetY + 1,
			Width:  m.width,
			Height: vpH,
			ZOrder: ZPanel + 1,
			Label:  "Console Panel",
			Help:   panelHelp,
		})

		if m.HasInputBox() {
			// Input bar region (3 rows: top border + content + bottom border)
			// Text X: 1 (input box left border) + promptWidth
			m.Input.SetScreenTextX(offsetX + 1 + m.Input.PromptWidth())
			regions = append(regions, HitRegion{
				ID:     IDConsoleInput,
				X:      offsetX,
				Y:      offsetY + 1 + vpH,
				Width:  m.width,
				Height: 3,
				ZOrder: ZPanel + 1,
				Label:  "Console Input",
				Help:   inputHelp,
			})
			// INS/OVR hit region — bottom border of the input box.
			regions = append(regions, HitRegion{
				ID:     IDPanel + "." + IDInsOvr,
				X:      offsetX + 1, // after the left border corner
				Y:      offsetY + 1 + vpH + 2,
				Width:  3,
				Height: 1,
				ZOrder: ZPanel + 2,
				Label:  "INS/OVR",
				Help:   &HelpContext{ScreenName: "Console Panel", PageTitle: "Insert/Overwrite", PageText: "Toggle between insert and overwrite mode."},
			})
		}

		// Scrollbar hit regions
		if currentConfig.UI.Scrollbar {
			sbInfo := ComputeScrollbarInfo(m.Sv.TotalLineCount(), m.Sv.Height(), m.Sv.YOffset(), vpH)
			if sbInfo.Needed {
				sbX := offsetX + m.Sv.Width()
				sbTopY := offsetY + 1

				regions = append(regions, HitRegion{
					ID: IDPanel + ".sb.up", X: sbX, Y: sbTopY,
					Width: 1, Height: 1, ZOrder: ZPanel + 2,
				})
				if aboveH := sbInfo.ThumbStart - 1; aboveH > 0 {
					regions = append(regions, HitRegion{
						ID: IDPanel + ".sb.above", X: sbX, Y: sbTopY + 1,
						Width: 1, Height: aboveH, ZOrder: ZPanel + 2,
					})
				}
				if thumbH := sbInfo.ThumbEnd - sbInfo.ThumbStart; thumbH > 0 {
					regions = append(regions, HitRegion{
						ID: IDPanel + ".sb.thumb", X: sbX, Y: sbTopY + sbInfo.ThumbStart,
						Width: 1, Height: thumbH, ZOrder: ZPanel + 3,
					})
				}
				if belowH := (sbInfo.Height - 1) - sbInfo.ThumbEnd; belowH > 0 {
					regions = append(regions, HitRegion{
						ID: IDPanel + ".sb.below", X: sbX, Y: sbTopY + sbInfo.ThumbEnd,
						Width: 1, Height: belowH, ZOrder: ZPanel + 2,
					})
				}
				regions = append(regions, HitRegion{
					ID: IDPanel + ".sb.down", X: sbX, Y: sbTopY + sbInfo.Height - 1,
					Width: 1, Height: 1, ZOrder: ZPanel + 2,
				})
			}
		}
	}

	return regions
}
