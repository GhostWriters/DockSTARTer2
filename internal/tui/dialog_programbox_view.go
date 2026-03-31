package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *ProgramBoxModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	ctx := GetActiveContext()

	// Apply background maintenance to captured output to prevent resets from bleeding
	viewportContent := MaintainBackground(m.viewport.View(), ctx.Console)
	// Sections content plus scrollbar/gutter (right slot)
	viewportContent, m.sbInfo = ApplyScrollbarColumnTracked(viewportContent, m.viewport.TotalLineCount(), m.viewport.Height(), m.viewport.YOffset(), currentConfig.UI.Scrollbar, GetActiveContext().LineCharacters, GetActiveContext())

	// Wrap viewport in rounded inner border with console background.
	// Disable the bottom border so we can append a custom one with the scroll indicator.
	viewportStyle := ctx.Console.
		Padding(0, 0)
	viewportStyle = ApplyInnerBorderCtx(viewportStyle, m.focused, ctx)
	viewportStyle = viewportStyle.BorderBottom(false)

	borderedViewport := InjectBorderFlags(
		viewportStyle.Height(m.viewport.Height()).Render(viewportContent),
		ctx.BorderFlags, ctx.Border2Flags, false)

	// Append custom bottom border. Only show scroll indicator when content overflows.
	// Calculate inner box width based on full viewport width
	totalWidth := m.viewport.Width() + ScrollbarGutterWidth + 2
	borderedViewport = strings.TrimSuffix(borderedViewport, "\n")
	if m.sbInfo.Needed {
		borderedViewport = borderedViewport + "\n" + BuildScrollPercentBottomBorder(totalWidth, m.viewport.ScrollPercent(), m.focused, ctx)
	} else {
		borderedViewport = borderedViewport + "\n" + BuildPlainBottomBorder(totalWidth, m.focused, ctx)
	}

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

	// Content width for inner components: inside outer border and 1-char margin on each side.
	layout := GetLayout()
	contentWidth = m.layout.Width - layout.BorderWidth() - layout.ContentMarginWidth()

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
		buttonRow := RenderCenteredButtonsExplicit(
			contentWidth,
			m.layout.ButtonHeight == DialogButtonHeight,
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

	// Apply 1-char side margin so content is inset from outer border (matching menu dialogs).
	content = lipgloss.NewStyle().
		Background(ctx.Dialog.GetBackground()).
		Padding(0, layout.ContentSideMargin).
		Render(content)

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
	v.MouseMode = tea.MouseModeCellMotion
	v.AltScreen = true
	return v
}

// Layers implements LayeredView
func (m *ProgramBoxModel) Layers() []*lipgloss.Layer {
	// Root dialog layer - just the rendered content
	// Hit testing is handled by GetHitRegions()
	viewStr := m.ViewString()
	root := lipgloss.NewLayer(viewStr).Z(ZScreen)

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

	layout := GetLayout()
	// Account for the 1-line spacer after the command display
	// Viewport hit region (main output area)
	viewportY := 1 + m.layout.HeaderHeight + m.layout.CommandHeight
	if m.layout.Width > 2 && m.layout.ViewportHeight > 0 {
		regions = append(regions, HitRegion{
			ID:     m.id + ".viewport",
			X:      offsetX + layout.ContentSideMargin + 1, // outer border(1) + margin + inner border(1)
			Y:      offsetY + viewportY + 1,                // +1 for inner top border
			Width:  m.viewport.Width(),
			Height: m.viewport.Height(),
			ZOrder: ZDialog + 10,
			Label:  "Output Viewport",
			Help: &HelpContext{
				ScreenName: m.title,
				PageTitle:  "Output Viewer",
				PageText:   "Displays the live output of a running command or script.",
				ItemText:   "Scroll with the mouse wheel or use Home/End/PgUp/PgDn to review output.",
			},
		})

		// Scrollbar Regions
		if currentConfig.UI.Scrollbar && m.sbInfo.Needed {
			// sbX: outer border(1) + margin + inner border(1) + viewport content width
			sbX := offsetX + layout.ContentSideMargin + 2 + m.viewport.Width()
			// sbTopY: outer border(1) + header + command + spacer + inner top border(1) == viewportY+1
			sbTopY := offsetY + viewportY + 1
			m.sbAbsTopY = sbTopY

			info := m.sbInfo
			regions = append(regions, HitRegion{
				ID: m.id + ".sb.up", X: sbX, Y: sbTopY,
				Width: 1, Height: 1, ZOrder: ZDialog + 20,
				Label: "Scroll Up",
			})
			if aboveH := info.ThumbStart - 1; aboveH > 0 {
				regions = append(regions, HitRegion{
					ID: m.id + ".sb.above", X: sbX, Y: sbTopY + 1,
					Width: 1, Height: aboveH, ZOrder: ZDialog + 20,
					Label: "Page Up",
				})
			}
			if thumbH := info.ThumbEnd - info.ThumbStart; thumbH > 0 {
				regions = append(regions, HitRegion{
					ID: m.id + ".sb.thumb", X: sbX, Y: sbTopY + info.ThumbStart,
					Width: 1, Height: thumbH, ZOrder: ZDialog + 21,
					Label: "Scroll Thumb",
				})
			}
			if belowH := (info.Height - 1) - info.ThumbEnd; belowH > 0 {
				regions = append(regions, HitRegion{
					ID: m.id + ".sb.below", X: sbX, Y: sbTopY + info.ThumbEnd,
					Width: 1, Height: belowH, ZOrder: ZDialog + 20,
					Label: "Page Down",
				})
			}
			regions = append(regions, HitRegion{
				ID: m.id + ".sb.down", X: sbX, Y: sbTopY + info.Height - 1,
				Width: 1, Height: 1, ZOrder: ZDialog + 20,
				Label: "Scroll Down",
			})
		}
	}

	// Dialog background
	regions = append(regions, HitRegion{
		ID:     m.id,
		X:      offsetX,
		Y:      offsetY,
		Width:  m.layout.Width,
		Height: m.layout.Height,
		ZOrder: ZDialog,
		Label:  m.title,
		Help: &HelpContext{
			ScreenName: m.title,
			PageTitle:  "Task Progress",
			PageText:   "This dialog shows the progress of a running task. You can view the log output in the viewport below. Click OK when done to return.",
		},
	})

	// If done, add OK button hit region using centralized helper
	if m.done {
		// Y = 1 (border) + headerH + commandH + spacerY + vpHeight + viewport border (2)
		buttonY := 1 + m.layout.HeaderHeight + m.layout.CommandHeight + m.layout.ViewportHeight + 2
		contentWidth := m.layout.Width - layout.BorderWidth() - layout.ContentMarginWidth()

		btnSpecs := []ButtonSpec{
			{Text: "OK", ZoneID: "ok", Active: true},
		}

		regions = append(regions, GetButtonHitRegions(
			HelpContext{ScreenName: m.title, PageTitle: "Task Execution", PageText: m.subtitle},
			"programbox_dialog", offsetX+layout.ContentSideMargin+1, offsetY+buttonY, contentWidth, ZDialog+20,
			btnSpecs...,
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
