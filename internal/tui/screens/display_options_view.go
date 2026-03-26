package screens

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (s *DisplayOptionsScreen) ViewString() (result string) {
	defer func() {
		if r := recover(); r != nil {
			result = "(rendering error — theme may still be loading)"
		}
	}()
	if s.outerMenu == nil {
		return ""
	}
	layout := tui.GetLayout()

	// If dimensions not yet set, use terminal dimensions as fallback.
	width, height := s.width, s.height
	if width == 0 || height == 0 {
		termW, termH, _ := console.GetTerminalSize()
		if termW > 0 && termH > 0 {
			hasShadow := tui.IsShadowEnabled()
			header := tui.NewHeaderModel()
			header.SetWidth(termW - 2)
			headerH := header.Height()
			width, height = layout.ContentArea(termW, termH, hasShadow, headerH, layout.HelplineHeight)
		}
	}

	dl := s.computePanelLayout(width)
	s.outerMenu.SetSize(dl.settingsDialogWidth, height)
	settingsDialog := s.outerMenu.ViewString()

	if !dl.previewFits {
		return settingsDialog
	}

	settingsHeight := lipgloss.Height(settingsDialog)
	preview := s.renderPreviewDialog(settingsHeight)

	styles := tui.GetStyles()
	previewW := lipgloss.Width(preview)
	settingsW := lipgloss.Width(settingsDialog)

	gutterW := width - settingsW - previewW
	if gutterW < 1 {
		gutterW = 1
	}
	gutterStyle := lipgloss.NewStyle().Background(styles.Screen.GetBackground())
	gutterStr := gutterStyle.Height(settingsHeight).Width(gutterW).Render("")

	return lipgloss.JoinHorizontal(lipgloss.Top, settingsDialog, gutterStr, preview)
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

		// 2. Settings dialog uses the width computed by SetSize (settingsDialogWidth).
		// Re-size to the exact available space so it fills up to the gutter edge.
		settingsW := previewX - gutter
		s.outerMenu.SetSize(settingsW, height)
		settingsDialog := s.outerMenu.ViewString()
		return []*lipgloss.Layer{
			lipgloss.NewLayer(settingsDialog).X(0).Y(0).Z(tui.ZScreen),
			lipgloss.NewLayer(preview).X(previewX).Y(0).Z(tui.ZScreen),
		}
	}

	// No preview: settings fills available width.
	settingsDialog := s.outerMenu.ViewString()
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
		Label:  "Theme Selection",
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
		Label:  "Display Options",
		Help: &tui.HelpContext{
			ScreenName: s.outerMenu.Title(),
			PageTitle:  "Description",
			PageText:   "Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.",
		},
	})

	// Button row regions
	// buttonY = 1 (top border) + themeHeight + optionsHeight
	buttonY := 1 + s.themeMenu.Height() + s.optionsMenu.Height()
	btnRowWidth := dialogContentWidth

	// Button panel background — height matches flat (1) vs bordered (3) from outerMenu layout.
	regions = append(regions, tui.HitRegion{
		ID:     tui.IDButtonPanel,
		X:      offsetX + contentX,
		Y:      offsetY + buttonY,
		Width:  btnRowWidth,
		Height: s.outerMenu.GetButtonHeight(),
		ZOrder: tui.ZScreen + 1,
		Label:  "Actions",
		Help: &tui.HelpContext{
			ScreenName: s.outerMenu.Title(),
			PageTitle:  "Description",
			PageText:   "Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.",
		},
	})

	// Individual buttons
	btnSpecs := []tui.ButtonSpec{
		{Text: "Apply", ZoneID: tui.IDApplyButton, Help: "Save all changed display options to your configuration file."},
	}
	if s.isRoot {
		btnSpecs = append(btnSpecs, tui.ButtonSpec{
			Text:   "Exit",
			ZoneID: tui.IDExitButton,
			Help:   "Exit the application.",
		})
	} else {
		btnSpecs = append(btnSpecs,
			tui.ButtonSpec{
				Text:   "Back",
				ZoneID: tui.IDBackButton,
				Help:   "Return to the previous screen.",
			},
			tui.ButtonSpec{
				Text:   "Exit",
				ZoneID: tui.IDExitButton,
				Help:   "Exit the application.",
			},
		)
	}
	regions = append(regions, tui.GetButtonHitRegions(
		tui.HelpContext{
			ScreenName: s.outerMenu.Title(),
			PageTitle:  "Description",
			PageText:   "Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.",
		},
		s.outerMenu.ID(), offsetX+contentX, offsetY+buttonY, btnRowWidth, tui.ZScreen+25,
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
		Label:  "Preview",
	})
	// Add other mockup regions if needed for interactive preview
	return regions
}
