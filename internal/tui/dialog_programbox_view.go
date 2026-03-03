package tui

import (
	"fmt"
	"strings"

	"DockSTARTer2/internal/strutil"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *ProgramBoxModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	ctx := GetActiveContext()

	// Calculate scroll percentage
	scrollPercent := m.viewport.ScrollPercent()

	// Add scroll indicator at bottom of viewport content
	scrollIndicator := ctx.TagKey.
		Bold(true).
		Render(fmt.Sprintf("%d%%", int(scrollPercent*100)))

	// Use console background for the spacer row
	// Apply background maintenance to captured output to prevent resets from bleeding
	viewportContent := MaintainBackground(m.viewport.View(), ctx.Console)
	// viewportWithScroll := viewportContent + "\n" +
	// 	lipgloss.NewStyle().
	// 		Width(m.viewport.Width).
	// 		Align(lipgloss.Center).
	// 		Background(styles.Console.GetBackground()).
	// 		Render(scrollIndicator)

	// Wrap viewport in rounded inner border with console background
	viewportStyle := ctx.Console.
		Padding(0, 0) // Remove side padding inside inner box for a tighter look
	viewportStyle = ApplyInnerBorderCtx(viewportStyle, m.focused, ctx)

	// Apply scroll indicator manually to bottom border
	// We disable the bottom border initially to let us construct it ourselves
	viewportStyle = viewportStyle.BorderBottom(false)

	borderedViewport := viewportStyle.
		Height(m.viewport.Height()).
		Render(viewportContent)

	// Construct custom bottom border with label.
	// Use border characters matching ApplyInnerBorderCtx focus state.
	var border lipgloss.Border
	if ctx.LineCharacters {
		if m.focused {
			border = ThickRoundedBorder
		} else {
			border = lipgloss.RoundedBorder()
		}
	} else {
		if m.focused {
			border = RoundedThickAsciiBorder
		} else {
			border = RoundedAsciiBorder
		}
	}
	width := m.viewport.Width() + 2 // Add 2 for left/right padding of viewportStyle
	labelWidth := lipgloss.Width(scrollIndicator)

	// Determine T-connectors based on focus and line style
	var leftT, rightT string
	if ctx.LineCharacters {
		if m.focused {
			leftT = "┫"
			rightT = "┣"
		} else {
			leftT = "┤"
			rightT = "├"
		}
	} else {
		if m.focused {
			leftT = "H"
			rightT = "H"
		} else {
			leftT = "+"
			rightT = "+"
		}
	}

	// Calculate padding for label to place it on the right
	// We want it close to the right corner, e.g., 2 chars padding
	rightPadCnt := 2

	// Ensure we have enough space
	totalLabelWidth := 1 + labelWidth + 1 // connector + label + connector
	if width < totalLabelWidth+rightPadCnt+2 {
		// Fallback to center if too narrow
		rightPadCnt = (width - totalLabelWidth) / 2
	}

	// Correct math for bottom line length:
	// Corner(1) + LeftPad + Connector(1) + Label + Connector(1) + RightPad + Corner(1) = width
	// LeftPad + RightPad + Label + 4 = width
	leftPadCnt := width - labelWidth - 4 - rightPadCnt
	if leftPadCnt < 0 {
		leftPadCnt = 0
		rightPadCnt = width - labelWidth - 4
		if rightPadCnt < 0 {
			rightPadCnt = 0
		}
	}

	// Style for border segments (match ApplyRoundedBorder logic)
	borderStyle := lipgloss.NewStyle().
		Foreground(ctx.Border2Color).
		Background(ctx.Dialog.GetBackground())

	// Build bottom line parts
	// Left part: BottomLeftCorner + HorizontalLine...
	leftPart := borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, leftPadCnt))

	// Connectors
	leftConnector := borderStyle.Render(leftT)
	rightConnector := borderStyle.Render(rightT)

	// Right part: ...HorizontalLine + BottomRightCorner
	rightPart := borderStyle.Render(strutil.Repeat(border.Bottom, rightPadCnt) + border.BottomRight)

	// Combine parts: Left-----┤100%├--Right
	bottomLine := lipgloss.JoinHorizontal(lipgloss.Bottom, leftPart, leftConnector, scrollIndicator, rightConnector, rightPart)

	// Append custom bottom line to viewport
	// Use strings.Join to avoid extra newlines often added by lipgloss.JoinVertical
	borderedViewport = strings.TrimSuffix(borderedViewport, "\n")
	borderedViewport = borderedViewport + "\n" + bottomLine

	// Calculate content width based on viewport (matches borderedViewport width)
	// viewport.Width() + border (2) = viewport.Width() + 2
	contentWidth := m.viewport.Width() + 2

	// Build command display using theme semantic tags
	var commandDisplay string
	if m.command != "" {
		// Use RenderThemeText for robust parsing of embedded tags/colors
		// We use the console style as base, but DO NOT force the background color onto the whole bar
		// This allows the user to have unstyled spaces or mixed colors.
		// Use styles.Dialog as base so unstyled text matches the dialog background
		base := ctx.Dialog
		renderedCmd := RenderThemeText(m.command, base)

		// Use lipgloss to render the row so width and background are handled correctly
		// even with ANSI codes in renderedCmd.
		// Use lipgloss to render the row so width and background are handled correctly
		// even with ANSI codes in renderedCmd.
		commandDisplay = lipgloss.NewStyle().
			Width(contentWidth).
			Background(ctx.Dialog.GetBackground()).
			Render(renderedCmd)
	}

	// Build dialog content
	var contentParts []string

	// Calculate content width for inner components
	contentWidth = m.layout.Width - 2

	// Render Header
	headerUI := m.renderHeaderUI(contentWidth)
	if headerUI != "" {
		contentParts = append(contentParts, headerUI)
	}

	// Render Command display
	if commandDisplay != "" {
		contentParts = append(contentParts, commandDisplay)
		spacer := lipgloss.NewStyle().
			Width(contentWidth).
			Background(ctx.Dialog.GetBackground()).
			Render("")
		contentParts = append(contentParts, spacer) // Standard gap after command
	}

	contentParts = append(contentParts, borderedViewport)

	// Render OK button
	if m.done {
		buttonRow := RenderCenteredButtonsCtx(
			contentWidth,
			ctx,
			ButtonSpec{Text: "OK", Active: m.done},
		)
		contentParts = append(contentParts, buttonRow)
	}

	// Use JoinVertical to ensure all parts are correctly combined with their heights
	// Trim trailing newlines from each part to avoid implicit extra lines in JoinVertical
	for i, part := range contentParts {
		contentParts[i] = strings.TrimRight(part, "\n")
	}
	content := lipgloss.JoinVertical(lipgloss.Left, contentParts...)

	// Force total content height to match the calculated budget (Total - Outer Borders - Shadow)
	// only if maximized. Otherwise it should have its intrinsic height.
	if m.maximized {
		heightBudget := m.layout.Height - DialogBorderHeight - m.layout.ShadowHeight
		if heightBudget > 0 {
			content = lipgloss.NewStyle().
				Height(heightBudget).
				Background(ctx.Dialog.GetBackground()).
				Render(content)
		}
	}

	// Wrap in border with title embedded (matching menu style)
	dialogWithTitle := RenderDialog(m.title, content, true, 0)

	// If sub-dialog is active, overlay it
	if m.subDialog != nil {
		var subView string
		if vs, ok := m.subDialog.(interface{ ViewString() string }); ok {
			subView = vs.ViewString()
		} else {
			subView = fmt.Sprintf("%v", m.subDialog.View())
		}
		// Overlay sub-dialog on top of the program box content
		dialogWithTitle = Overlay(subView, dialogWithTitle, OverlayCenter, OverlayCenter, 0, 0)
	}

	return dialogWithTitle
}

// View implements tea.Model
func (m *ProgramBoxModel) View() tea.View {
	v := tea.NewView(m.ViewString())
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	return v
}

// Layers implements LayeredView
func (m *ProgramBoxModel) Layers() []*lipgloss.Layer {
	// Root dialog layer - just the rendered content
	// Hit testing is handled by GetHitRegions()
	viewStr := m.ViewString()
	root := lipgloss.NewLayer(viewStr).Z(ZDialog)

	// If sub-dialog is active, aggregate its layers for visual compositing
	if m.subDialog != nil {
		if lv, ok := m.subDialog.(LayeredView); ok {
			subLayers := lv.Layers()
			if len(subLayers) > 0 {
				// Center the sub-dialog layers
				var subStr string
				if vs, ok := m.subDialog.(interface{ ViewString() string }); ok {
					subStr = vs.ViewString()
				} else {
					subStr = fmt.Sprintf("%v", m.subDialog.View())
				}
				subW := lipgloss.Width(subStr)
				subH := lipgloss.Height(subStr)

				containerW := lipgloss.Width(viewStr)
				containerH := lipgloss.Height(viewStr)

				offsetX := (containerW - subW) / 2
				offsetY := (containerH - subH) / 2

				// Add sub-dialog layers with offset
				for _, l := range subLayers {
					root.AddLayers(lipgloss.NewLayer(l.GetContent()).
						X(l.GetX() + offsetX).
						Y(l.GetY() + offsetY).
						Z(l.GetZ() + 10))
				}
			}
		}
	}

	return []*lipgloss.Layer{root}
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (m *ProgramBoxModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	var regions []HitRegion

	// Viewport hit region so hover+scroll works over the output area
	if m.layout.Width > 2 && m.layout.ViewportHeight > 0 {
		viewportY := 1 + m.layout.HeaderHeight + m.layout.CommandHeight
		regions = append(regions, HitRegion{
			ID:     m.id + ".viewport",
			X:      offsetX + 1,
			Y:      offsetY + viewportY,
			Width:  m.layout.Width - 2,
			Height: m.layout.ViewportHeight + 2,
			ZOrder: ZDialog + 5,
		})
	}

	// If done, add OK button hit region using centralized helper
	if m.done {
		// Y = 1 (border) + headerH + commandH + vpHeight + viewport border (2)
		buttonY := 1 + m.layout.HeaderHeight + m.layout.CommandHeight + m.layout.ViewportHeight + 2
		contentWidth := m.layout.Width - 2

		regions = append(regions, GetButtonHitRegions(
			m.id, offsetX+1, offsetY+buttonY, contentWidth, ZDialog+20,
			ButtonSpec{Text: "OK", ZoneID: "OK"},
		)...)
	}

	// If sub-dialog is active, collect its hit regions
	if m.subDialog != nil {
		if hrp, ok := m.subDialog.(HitRegionProvider); ok {
			viewStr := m.ViewString()
			containerW := lipgloss.Width(viewStr)
			containerH := lipgloss.Height(viewStr)

			var subStr string
			if vs, ok := m.subDialog.(interface{ ViewString() string }); ok {
				subStr = vs.ViewString()
			}
			subW := lipgloss.Width(subStr)
			subH := lipgloss.Height(subStr)

			subOffsetX := (containerW - subW) / 2
			subOffsetY := (containerH - subH) / 2

			regions = append(regions, hrp.GetHitRegions(offsetX+subOffsetX, offsetY+subOffsetY)...)
		}
	}

	return regions
}
