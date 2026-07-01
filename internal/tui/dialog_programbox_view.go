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
	viewportContent := MaintainBackground(m.sv.View(), ctx.Console)
	// Sections content plus scrollbar/gutter (right slot)
	viewportContent = ApplyScrollbar(&m.Scroll, viewportContent, m.sv.TotalLineCount(), m.sv.Height(), m.sv.YOffset(), GetActiveContext().LineCharacters, GetActiveContext())

	// Wrap viewport in rounded inner border with console background.
	// Disable the bottom border so we can append a custom one with the scroll indicator.
	viewportStyle := ctx.Console.
		Padding(0, 0)
	viewportStyle = ApplyInnerBorderCtx(viewportStyle, m.focused, ctx)
	viewportStyle = viewportStyle.BorderBottom(false)

	borderedViewport := InjectBorderFlags(
		viewportStyle.Height(m.sv.Height()).Render(viewportContent),
		ctx.BorderFlags, ctx.Border2Flags, false)

	// Append custom bottom border. Only show scroll indicator when content overflows.
	totalWidth := m.sv.Width() + ScrollbarGutterWidth + 2
	borderedViewport = strings.TrimSuffix(borderedViewport, "\n")
	if m.Scroll.Info.Needed {
		borderedViewport = borderedViewport + "\n" + BuildScrollPercentBottomBorder(totalWidth, m.sv.ScrollPercent(), m.focused, ctx)
	} else {
		borderedViewport = borderedViewport + "\n" + BuildPlainBottomBorder(totalWidth, m.focused, ctx)
	}

	// Build dialog content
	var contentParts []string

	// Content width for inner components: inside outer border and 1-char margin on each side.
	layout := GetLayout()
	contentWidth := m.layout.Width - layout.BorderWidth() - layout.ContentMarginWidth()

	// Build command display using theme semantic tags
	var commandDisplay string
	if m.command != "" {
		base := ctx.Dialog
		renderedCmd := RenderThemeText(m.command, base)
		commandDisplay = lipgloss.NewStyle().
			Width(contentWidth).
			Background(ctx.Dialog.GetBackground()).
			Render(renderedCmd)
	}

	// Render Header
	headerUI := m.renderHeaderUI(contentWidth)
	if headerUI != "" {
		contentParts = append(contentParts, headerUI)
	}

	// Render Command display
	if commandDisplay != "" {
		contentParts = append(contentParts, commandDisplay)
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
	// Trim trailing newlines and spaces from each part to avoid implicit extra lines in JoinVertical
	for i, part := range contentParts {
		contentParts[i] = strings.TrimRight(part, "\n ")
	}
	content := lipgloss.JoinVertical(lipgloss.Left, contentParts...)

	// Force total content height to match the calculated budget (Total - Outer Borders - Shadow)
	// only if maximized. Otherwise it should have its intrinsic height.
	if m.maximized {
		heightBudget := m.layout.Height - DialogBorderHeight
		if m.layout.LargeTitleBar {
			heightBudget -= LargeTitleBarOverhead
		}
		if heightBudget > 0 {
			content = lipgloss.NewStyle().
				Height(heightBudget).
				Background(ctx.Dialog.GetBackground()).
				Render(content)
		}
	}

	// 6. Apply 1-char side margin so content is inset from outer border (matching menu dialogs).
	content = lipgloss.NewStyle().
		Background(ctx.Dialog.GetBackground()).
		Padding(0, layout.ContentSideMargin).
		Render(content)

	// Trim trailing newline — Padding/Height Render calls can add one, which causes
	// renderDialogWithBorderCtx to split into an extra empty line and clip the button row.
	content = strings.TrimSuffix(content, "\n")

	// Wrap in border with title embedded (matching menu style)
	// targetHeight is determined by maximization state
	targetHeight := 0
	if m.maximized {
		targetHeight = m.height
	}
	tbs := m.State()
	tbs.SpinnerIndicator, tbs.SpinnerIndicatorRight = m.currentSpinnerIndicators()
	dialogWithTitle := renderDialogWithTypeAndWidgets(m.title, content, true, targetHeight, m.dialogType, GetActiveContext(), tbs)

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
	// All dialogs start inside an outer Top Border (1 line).
	currentY := layout.SingleBorder()
	if m.layout.LargeTitleBar {
		currentY += LargeTitleBarOverhead
	}

	// 1. Header Area (Subtitle + Tasks + Progress)
	headerH := m.layout.HeaderHeight
	// 2. Command Area (Command row + Gap)
	commandH := m.layout.CommandHeight

	// viewportY is where the inner border of the viewport starts
	viewportY := currentY + headerH + commandH

	if m.layout.Width > 2 && m.layout.ViewportHeight > 0 {
		regions = append(regions, HitRegion{
			ID:     m.id + ".viewport",
			X:      offsetX + layout.ContentSideMargin + 1, // outer(1) + margin + inner border(1)
			Y:      offsetY + viewportY + 1,                // +1 for inner top border
			Width:  m.sv.Width(),
			Height: m.sv.Height(),
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
		if currentConfig.UI.Scrollbar && m.Scroll.Info.Needed {
			// sbX: outer border(1) + margin + inner border(1) + viewport content width
			sbX := offsetX + layout.ContentSideMargin + 2 + m.sv.Width()
			// sbTopY: outer border(1) + header + command + spacer + inner top border(1) == viewportY+1
			sbTopY := offsetY + viewportY + 1
			regions = append(regions, m.Scroll.HitRegions(sbX, sbTopY, ZDialog+20, "Output")...)
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
		// buttonY starts after viewport + borders
		buttonY := viewportY + m.layout.ViewportHeight + layout.BorderHeight()
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

	// Titlebar widget hit regions ([?] and [×])
	contentWidth := m.layout.Width - GetLayout().BorderWidth()
	regions = append(regions, m.titleBarHitRegions(offsetX, offsetY, contentWidth, ZDialog)...)

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
