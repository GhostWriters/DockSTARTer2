package screens

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/strutil"
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
	if s.outerMenu == nil {
		return ""
	}
	layout := displayengine.GetLayout()

	// If dimensions not yet set, use terminal dimensions as fallback.
	width, height := s.width, s.height
	if width == 0 || height == 0 {
		termW, termH, _ := console.GetTerminalSize()
		if termW > 0 && termH > 0 {
			hasShadow := tui.IsShadowEnabled()
			header := displayengine.NewHeaderModel()
			header.SetWidth(termW - 2)
			headerH := header.Height()
			width, height = layout.ContentArea(termW, termH, hasShadow, false, headerH, layout.HelplineHeight)
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

	styles := displayengine.GetStyles()
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
	for _, t := range s.themes {
		if t.ConfigValue == s.previewTheme && t.IsInvalid {
			const contentWidth = 44 // matches renderMockup width
			contentHeight := targetHeight - 2
			if contentHeight < 1 {
				contentHeight = 1
			}
			label := "Invalid theme"
			leftPad := (contentWidth - len(label)) / 2
			rightPad := contentWidth - len(label) - leftPad
			centeredLine := strutil.Repeat(" ", leftPad) + label + strutil.Repeat(" ", rightPad)
			topBlanks := (contentHeight - 1) / 2
			lines := make([]string, contentHeight)
			for i := range lines {
				lines[i] = strutil.Repeat(" ", contentWidth)
			}
			lines[topBlanks] = centeredLine
			ctx := displayengine.GetActiveContext()
			return displayengine.RenderBorderedBoxCtx("Preview", strings.Join(lines, "\n"), contentWidth, targetHeight, false, true, false, ctx.DialogTitleAlign, "Title", ctx)
		}
	}
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
		layout := displayengine.GetLayout()
		gutter := layout.VisualGutter(tui.IsShadowEnabled())

		// 1. Render the settings dialog first, at the full available height --
		// it shrinks to its own natural content height internally (outerMenu
		// isn't maximized), same as ViewString() does. The preview must be
		// sized to that ACTUAL rendered height, not the raw available height,
		// or the two disagree and the shared centering offset (computed from
		// ViewString()'s measurement) no longer matches what Layers() draws.
		s.outerMenu.SetSize(dl.settingsDialogWidth, height)
		settingsDialog := s.outerMenu.ViewString()
		settingsHeight := lipgloss.Height(settingsDialog)

		// 2. Render Preview at far right, matched to the settings dialog's
		// actual height; measure actual rendered width for precise positioning.
		preview := s.renderPreviewDialog(settingsHeight)
		previewW := lipgloss.Width(preview)
		previewX := width - previewW

		// 3. Re-size the settings dialog to fill up to the gutter edge at the
		// exact available width, matching ViewString()'s final sizing pass.
		settingsW := previewX - gutter
		s.outerMenu.SetSize(settingsW, height)
		settingsDialog = s.outerMenu.ViewString()

		return []*lipgloss.Layer{
			lipgloss.NewLayer(settingsDialog).X(0).Y(0).Z(displayengine.ZScreen),
			lipgloss.NewLayer(preview).X(previewX).Y(0).Z(displayengine.ZScreen),
		}
	}

	// No preview: settings fills available width.
	settingsDialog := s.outerMenu.ViewString()
	return []*lipgloss.Layer{
		lipgloss.NewLayer(settingsDialog).X(0).Y(0).Z(displayengine.ZScreen),
	}
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (s *DisplayOptionsScreen) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	var regions []displayengine.HitRegion

	// Content starts at outer border (1) + content side margin (1) from left; 1 from top (outer border).
	layout := displayengine.GetLayout()
	contentX := (layout.BorderWidth() / 2) + layout.ContentSideMargin

	// Account for large title bar overhead when the outerMenu computed a large title bar.
	largeTitleOffset := 0
	if s.outerMenu != nil && s.outerMenu.HasLargeTitleBar() {
		largeTitleOffset = displayengine.LargeTitleBarOverhead
	}
	contentY := 1 + largeTitleOffset

	// Load Theme Defaults menu regions (rendered first, above the theme list)
	loadDefaultsRegions := s.loadDefaultsMenu.GetHitRegions(offsetX+contentX, offsetY+contentY)
	regions = append(regions, loadDefaultsRegions...)

	// Load Theme Defaults panel hit region
	regions = append(regions, displayengine.HitRegion{
		ID:     displayengine.IDLoadDefaultsPanel,
		X:      offsetX + contentX,
		Y:      offsetY + contentY,
		Width:  s.loadDefaultsMenu.Width(),
		Height: s.loadDefaultsMenu.Height(),
		ZOrder: displayengine.ZScreen + 1,
		Label:  "Load Theme Defaults",
	})

	// Theme menu is rendered directly BELOW the Load Theme Defaults menu
	themeY := contentY + s.loadDefaultsMenu.Height()

	// Theme menu regions
	themeRegions := s.themeMenu.GetHitRegions(offsetX+contentX, offsetY+themeY)
	regions = append(regions, themeRegions...)

	// Theme panel hit region
	regions = append(regions, displayengine.HitRegion{
		ID:     displayengine.IDThemePanel,
		X:      offsetX + contentX,
		Y:      offsetY + themeY,
		Width:  s.themeMenu.Width(),
		Height: s.themeMenu.Height(),
		ZOrder: displayengine.ZScreen + 1,
		Label:  "Theme Selection",
	})

	// Width calculations for the main dialog area
	dl := s.computePanelLayout(s.width)
	dialogContentWidth := dl.menuWidth

	// Options menu is rendered directly BELOW the Theme menu
	optionsY := themeY + s.themeMenu.Height()

	optionsRegions := s.optionsMenu.GetHitRegions(offsetX+contentX, offsetY+optionsY)
	regions = append(regions, optionsRegions...)

	// Options panel hit region
	regions = append(regions, displayengine.HitRegion{
		ID:     displayengine.IDOptionsPanel,
		X:      offsetX + contentX,
		Y:      offsetY + optionsY,
		Width:  s.optionsMenu.Width(),
		Height: s.optionsMenu.Height(),
		ZOrder: displayengine.ZScreen + 1,
		Label:  "Display Options",
		Help: &displayengine.HelpContext{
			ScreenName: s.outerMenu.Title(),
			PageTitle:  "Description",
			PageText:   "Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.",
		},
	})

	// Button row regions
	// buttonY = 1 (top border) + largeTitleOffset + loadDefaultsHeight + themeHeight + optionsHeight
	buttonY := 1 + largeTitleOffset + s.loadDefaultsMenu.Height() + s.themeMenu.Height() + s.optionsMenu.Height()
	btnRowWidth := dialogContentWidth - layout.ContentMarginWidth()

	// Button panel background — height matches flat (1) vs bordered (3) from outerMenu layout.
	regions = append(regions, displayengine.HitRegion{
		ID:     displayengine.IDButtonPanel,
		X:      offsetX + contentX,
		Y:      offsetY + buttonY,
		Width:  btnRowWidth,
		Height: s.outerMenu.GetButtonHeight(),
		ZOrder: displayengine.ZScreen + 1,
		Label:  "Actions",
		Help: &displayengine.HelpContext{
			ScreenName: s.outerMenu.Title(),
			PageTitle:  "Description",
			PageText:   "Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.",
		},
	})

	// Individual buttons
	btnSpecs := []displayengine.ButtonSpec{
		{Text: "Apply", ZoneID: displayengine.IDApplyButton, Help: "Save all changed display options to your configuration file."},
	}
	if s.isRoot {
		btnSpecs = append(btnSpecs, displayengine.ButtonSpec{
			Text:   "Exit",
			ZoneID: displayengine.IDExitButton,
			Help:   "Exit the application.",
		})
	} else {
		btnSpecs = append(btnSpecs,
			displayengine.ButtonSpec{
				Text:   "Back",
				ZoneID: displayengine.IDBackButton,
				Help:   "Return to the previous screen.",
			},
			displayengine.ButtonSpec{
				Text:   "Exit",
				ZoneID: displayengine.IDExitButton,
				Help:   "Exit the application.",
			},
		)
	}
	regions = append(regions, displayengine.GetButtonHitRegions(
		displayengine.HelpContext{
			ScreenName: s.outerMenu.Title(),
			PageTitle:  "Description",
			PageText:   "Configure the visual appearance of the application, including theme selection, borders, shadows, and other display options.",
		},
		s.outerMenu.ID(), offsetX+contentX, offsetY+buttonY, btnRowWidth, displayengine.ZScreen+25,
		btnSpecs...,
	)...)

	// Title widget regions — delegate to outerMenu and filter for title widget IDs.
	if s.outerMenu != nil {
		for _, r := range s.outerMenu.GetHitRegions(offsetX, offsetY) {
			if displayengine.IsTitleWidgetID(r.ID) {
				regions = append(regions, r)
			}
		}
	}

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

func (s *DisplayOptionsScreen) getMockupHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	var regions []displayengine.HitRegion
	// These IDs should match the ones in renderMockup or follow naming convention
	// For now, these are the regions for the "mockup" status bar and title
	regions = append(regions, displayengine.HitRegion{
		ID:     "mockup.header",
		X:      offsetX,
		Y:      offsetY,
		Width:  44,
		Height: 1,
		ZOrder: displayengine.ZScreen + 2,
		Label:  "Preview",
	})
	// Add other mockup regions if needed for interactive preview
	return regions
}
