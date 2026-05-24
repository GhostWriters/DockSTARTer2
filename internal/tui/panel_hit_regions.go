package tui

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
	marker := "^"
	if m.expanded {
		marker = "v"
	}
	title := marker + " Console " + marker

	titleWidth := WidthWithoutZones(RenderThemeText(title, ctx.Dialog))
	titleSectionLen := 1 + 1 + titleWidth + 1 + 1
	actualWidth := m.width - 2
	var leftPad int
	if ctx.PanelTitleAlign == "left" {
		leftPad = 0
	} else {
		leftPad = (actualWidth - titleSectionLen) / 2
	}
	if leftPad < 0 {
		leftPad = 0
	}

	titleStart := 1 + leftPad
	titleEnd := titleStart + titleSectionLen

	regions = append(regions, HitRegion{
		ID:     IDPanelResize,
		X:      offsetX,
		Y:      offsetY,
		Width:  titleStart,
		Height: 1,
		ZOrder: ZPanel + 1,
		Label:  "Console Panel",
		Help:   panelHelp,
	})
	regions = append(regions, HitRegion{
		ID:     IDPanelToggle,
		X:      offsetX + titleStart,
		Y:      offsetY,
		Width:  titleSectionLen,
		Height: 1,
		ZOrder: ZPanel + 1,
		Label:  "Console Panel",
		Help:   panelHelp,
	})
	// Right side of title bar: resize widgets [▲] [▼] at far right, rest is drag-resize.
	// Widget layout: "[▲] [▼]" = 7 chars + 1 end pad before TopRight corner.
	const widgetTotalWidth = 7
	const endPad = 1
	widgetsStartX := m.width - 1 - endPad - widgetTotalWidth
	resizeUpX := widgetsStartX       // [▲] starts here
	resizeDnX := widgetsStartX + 4   // "[▲] " = 4 chars before [▼]
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
			ZOrder: ZPanel + 1,
			Label:  "Console Panel",
			Help:   panelHelp,
		})
	}
	regions = append(regions,
		HitRegion{
			ID: IDPanelResizeUp, X: offsetX + resizeUpX, Y: offsetY,
			Width: 3, Height: 1, ZOrder: ZPanel + 2,
			Label: "Grow panel",
			Help:  &HelpContext{ScreenName: "Console Panel", PageTitle: "Resize", PageText: "Click to grow the panel by one line."},
		},
		HitRegion{
			ID: IDPanelResizeDn, X: offsetX + resizeDnX, Y: offsetY,
			Width: 3, Height: 1, ZOrder: ZPanel + 2,
			Label: "Shrink panel",
			Help:  &HelpContext{ScreenName: "Console Panel", PageTitle: "Resize", PageText: "Click to shrink the panel by one line."},
		},
	)

	if m.expanded {
		vpH := m.height - 4
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

		// Input bar region (3 rows: top border + content + bottom border)
		// Text X: 1 (input box left border) + promptWidth
		m.input.SetScreenTextX(offsetX + 1 + m.input.PromptWidth())
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

		// Scrollbar hit regions
		if currentConfig.UI.Scrollbar {
			sbInfo := ComputeScrollbarInfo(m.viewport.TotalLineCount(), m.viewport.Height(), m.viewport.YOffset(), vpH)
			if sbInfo.Needed {
				sbX := offsetX + m.viewport.Width()
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
