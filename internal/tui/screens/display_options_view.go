package screens

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/tui"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (s *DisplayOptionsScreen) ViewString() (result string) {
	defer func() {
		if r := recover(); r != nil {
			result = "(rendering error — theme may still be loading)"
		}
	}()
	if s.optionsMenu == nil || s.themeMenu == nil {
		return ""
	}
	layout := tui.GetLayout()

	// s.width and s.height are already the content area from layout.ContentArea()
	// which has already subtracted shadow space. Dialog body fits here,
	// shadow extends past into edge indent area.

	// If dimensions not yet set, use terminal dimensions as fallback
	// This handles the initial render before WindowSizeMsg arrives
	width, height := s.width, s.height
	if width == 0 || height == 0 {
		termW, termH, _ := console.GetTerminalSize()
		if termW > 0 && termH > 0 {
			// Apply content area calculation
			hasShadow := tui.IsShadowEnabled()
			header := tui.NewHeaderModel()
			header.SetWidth(termW - 2)
			headerH := header.Height()
			width, height = layout.ContentArea(termW, termH, hasShadow, headerH)
		}
	}

	dl := s.computePanelLayout(width)

	// 1. Render Settings Dialog
	settingsDialog := s.renderSettingsDialog(dl.settingsDialogWidth, height)

	// If preview doesn't fit, just return the settings dialog
	if !dl.previewFits {
		return settingsDialog
	}

	// 2. Render Preview Dialog
	settingsHeight := lipgloss.Height(settingsDialog)
	preview := s.renderPreviewDialog(settingsHeight)

	// 3. Compose combined view (for non-layered fallback)
	styles := tui.GetStyles()
	previewW := lipgloss.Width(preview)
	settingsW := lipgloss.Width(settingsDialog)

	// Real visual gutter calculation for ViewString
	gutterW := width - settingsW - previewW
	if gutterW < 1 {
		gutterW = 1
	}
	gutterStyle := lipgloss.NewStyle().Background(styles.Screen.GetBackground())
	gutterStr := gutterStyle.Height(settingsHeight).Width(gutterW).Render("")

	return lipgloss.JoinHorizontal(lipgloss.Top, settingsDialog, gutterStr, preview)
}

func (s *DisplayOptionsScreen) renderSettingsDialog(dialogWidth, height int) string {
	layout := tui.GetLayout()
	menuWidth := dialogWidth - layout.BorderWidth()
	if menuWidth < 40 {
		menuWidth = 40
	}

	themeView := s.themeMenu.ViewString()
	optionsView := s.optionsMenu.ViewString()

	leftColumnParts := []string{themeView, optionsView}
	for i, p := range leftColumnParts {
		leftColumnParts[i] = strings.TrimRight(p, "\n")
	}
	leftColumn := lipgloss.JoinVertical(lipgloss.Left, leftColumnParts...)

	var buttons []tui.ButtonSpec
	if s.isRoot {
		buttons = []tui.ButtonSpec{
			{Text: "Apply", Active: s.focusedPanel == FocusButtons && s.focusedButton == 0},
			{Text: "Exit", Active: s.focusedPanel == FocusButtons && s.focusedButton == 1},
		}
	} else {
		buttons = []tui.ButtonSpec{
			{Text: "Apply", Active: s.focusedPanel == FocusButtons && s.focusedButton == 0},
			{Text: "Back", Active: s.focusedPanel == FocusButtons && s.focusedButton == 1},
			{Text: "Exit", Active: s.focusedPanel == FocusButtons && s.focusedButton == 2},
		}
	}
	buttonRow := tui.RenderCenteredButtons(menuWidth, buttons...)
	settingsContent := lipgloss.JoinVertical(lipgloss.Left, leftColumn, buttonRow)

	return tui.RenderBorderedBoxCtx("Appearance Settings", settingsContent, menuWidth, s.height, s.focused, false, tui.GetActiveContext().DialogTitleAlign, "Theme_Title", tui.GetActiveContext())
}

func (s *DisplayOptionsScreen) renderPreviewDialog(targetHeight int) string {
	return s.renderMockup(targetHeight)
}

func (s *DisplayOptionsScreen) View() tea.View {
	v := tea.NewView(s.ViewString())
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// Layers implements LayeredView
func (s *DisplayOptionsScreen) Layers() []*lipgloss.Layer {
	width, height := s.width, s.height
	if width == 0 || height == 0 {
		return nil
	}

	dl := s.computePanelLayout(width)

	if dl.previewFits {
		layout := tui.GetLayout()
		gutter := layout.VisualGutter(tui.IsShadowEnabled())

		// 1. Render Preview at far right; measure actual rendered width for precise positioning.
		preview := s.renderPreviewDialog(height)
		previewW := lipgloss.Width(preview)
		previewX := width - previewW

		// 2. Settings width is derived from actual previewX so it fills the remaining space.
		settingsW := previewX - gutter
		settingsDialog := s.renderSettingsDialog(settingsW, height)
		return []*lipgloss.Layer{
			lipgloss.NewLayer(settingsDialog).X(0).Y(0).Z(tui.ZScreen),
			lipgloss.NewLayer(preview).X(previewX).Y(0).Z(tui.ZScreen),
		}
	}

	// No preview: settings fills full width (compositor handles shadow independently).
	settingsDialog := s.renderSettingsDialog(width, height)
	return []*lipgloss.Layer{
		lipgloss.NewLayer(settingsDialog).X(0).Y(0).Z(tui.ZScreen),
	}
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (s *DisplayOptionsScreen) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	var regions []tui.HitRegion

	// Content starts at (1, 1) relative to root because of the outer border
	const contentX = 1
	const contentY = 1

	// Theme menu regions
	themeRegions := s.themeMenu.GetHitRegions(offsetX+contentX, offsetY+contentY)
	regions = append(regions, themeRegions...)

	// Theme panel hit region
	regions = append(regions, tui.HitRegion{
		ID:     tui.IDThemePanel,
		X:      offsetX + contentX,
		Y:      offsetY + contentY,
		Width:  s.themeMenu.Width(),
		Height: s.themeMenu.Height(),
		ZOrder: tui.ZScreen + 1,
	})

	// Width calculations for the main dialog area
	dl := s.computePanelLayout(s.width)
	dialogContentWidth := dl.menuWidth

	// Options menu is rendered directly BELOW the Theme menu
	optionsY := contentY + s.themeMenu.Height()

	optionsRegions := s.optionsMenu.GetHitRegions(offsetX+contentX, offsetY+optionsY)
	regions = append(regions, optionsRegions...)

	// Options panel hit region
	regions = append(regions, tui.HitRegion{
		ID:     tui.IDOptionsPanel,
		X:      offsetX + contentX,
		Y:      offsetY + optionsY,
		Width:  s.optionsMenu.Width(),
		Height: s.optionsMenu.Height(),
		ZOrder: tui.ZScreen + 1,
	})

	// Button row regions
	// buttonY = 1 (top border) + themeHeight + optionsHeight
	buttonY := 1 + s.themeMenu.Height() + s.optionsMenu.Height()
	btnRowWidth := dialogContentWidth

	// Button panel background
	regions = append(regions, tui.HitRegion{
		ID:     tui.IDButtonPanel,
		X:      offsetX + contentX,
		Y:      offsetY + buttonY,
		Width:  btnRowWidth,
		Height: tui.DialogButtonHeight, // usually 3 to wrap buttons with borders
		ZOrder: tui.ZScreen + 1,
	})

	// Individual buttons
	btnSpecs := []tui.ButtonSpec{
		{Text: "Apply", ZoneID: tui.IDApplyButton},
	}
	if s.isRoot {
		btnSpecs = append(btnSpecs, tui.ButtonSpec{Text: "Exit", ZoneID: tui.IDExitButton})
	} else {
		btnSpecs = append(btnSpecs,
			tui.ButtonSpec{Text: "Back", ZoneID: tui.IDBackButton},
			tui.ButtonSpec{Text: "Exit", ZoneID: tui.IDExitButton},
		)
	}
	regions = append(regions, tui.GetButtonHitRegions(
		"", offsetX+contentX, offsetY+buttonY, btnRowWidth, tui.ZScreen+25,
		btnSpecs...,
	)...)

	// Preview Mockup Regions (when it fits)
	if dl.previewFits {
		// Calculate preview position matching Layers()
		preview := s.renderPreviewDialog(s.height)
		previewW := lipgloss.Width(preview)
		previewX := s.width - previewW

		// Pass the preview's absolute position to renderMockup for region calculation
		mockupRegions := s.getMockupHitRegions(offsetX+previewX, offsetY)
		regions = append(regions, mockupRegions...)
	}

	return regions
}

func (s *DisplayOptionsScreen) getMockupHitRegions(offsetX, offsetY int) []tui.HitRegion {
	var regions []tui.HitRegion
	// These IDs should match the ones in renderMockup or follow naming convention
	// For now, these are the regions for the "mockup" status bar and title
	regions = append(regions, tui.HitRegion{
		ID:     "mockup.header",
		X:      offsetX,
		Y:      offsetY,
		Width:  44,
		Height: 1,
		ZOrder: tui.ZScreen + 2,
	})
	// Add other mockup regions if needed for interactive preview
	return regions
}
