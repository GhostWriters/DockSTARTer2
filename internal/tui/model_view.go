package tui

import (
	"sort"

	"DockSTARTer2/internal/logger"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ViewStringer is an interface for models that provide string content for compositing
type ViewStringer interface {
	ViewString() string
}

// View implements tea.Model
// Uses backdrop + overlay pattern (same as dialogs)
func (m *AppModel) View() (v tea.View) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error(m.ctx, "AppModel.View Panic: %v", r)
			// Use plain ANSI only — no theme tags — to prevent a recursive panic
			// if the theme system itself was the source of the original panic.
			v = tea.NewView("\x1b[31mRendering Error\x1b[0m\n\nPress any key to continue.")
		}
	}()

	if !m.ready {
		return tea.NewView("Initializing...")
	}

	// Use Layout helpers for consistent positioning
	layout := GetLayout()
	// Query the backdrop for the actual rendered chrome height (header + bottom border).
	// This avoids hardcoding a constant and correctly handles multi-line headers.
	contentYOffset := 2 // fallback: 1-line header + 1 bottom border
	headerH := 1        // header content height, needed by layout functions
	if m.backdrop != nil {
		contentYOffset = m.backdrop.ChromeHeight()
		headerH = contentYOffset - 1
	}

	// Create native compositor for rendering
	comp := lipgloss.NewCompositor()
	maxZ := ZScreen

	// Reset hit regions for this frame
	m.hitRegions = nil

	// 1. Layer: Backdrop
	if m.backdrop != nil {
		comp.AddLayers(m.backdrop.Layers()...)
		// Collect hit regions from backdrop (header version labels)
		m.hitRegions = append(m.hitRegions, m.backdrop.GetHitRegions(0, 0)...)
	}

	// 2. Layer: Log panel
	logY := m.height - m.logPanel.Height()
	if lv, ok := interface{}(m.logPanel).(LayeredView); ok {
		for _, l := range lv.Layers() {
			comp.AddLayers(l.Y(l.GetY() + logY))
		}
	} else if vs, ok := interface{}(m.logPanel).(ViewStringer); ok {
		if logContent := vs.ViewString(); logContent != "" {
			comp.AddLayers(lipgloss.NewLayer(logContent).
				X(0).Y(logY).Z(ZLogPanel).ID(IDLogPanel))
		}
	}
	// Collect hit regions from log panel
	m.hitRegions = append(m.hitRegions, m.logPanel.GetHitRegions(0, logY)...)

	// Base coordinates for maximized elements (edge indent from left, content start from top)
	maxX := layout.EdgeIndent
	maxY := contentYOffset

	// 3. Layer: Active Screen only
	// The screen stack is navigation history — previous screens must not render through
	// the active screen's background. Dialogs overlay via the dialog stack below.
	var allScreens []ScreenModel
	if m.activeScreen != nil {
		allScreens = []ScreenModel{m.activeScreen}
	}

	for i, s := range allScreens {
		var screenContent string
		if vs, ok := s.(ViewStringer); ok {
			screenContent = vs.ViewString()
		}

		if screenContent != "" {
			// Calculate centered position for non-maximized screens
			caW, caH := m.getContentArea()
			screenW := WidthWithoutZones(screenContent)
			screenH := lipgloss.Height(screenContent)

			screenX := maxX
			screenY := maxY

			// Center if smaller than content area
			if screenW < caW {
				screenX = maxX + (caW-screenW)/2
			}
			if screenH < caH {
				screenY = maxY + (caH-screenH)/2
			}

			// Base Z for screens: each screen level is 10 units apart within ZScreen band
			screenZBase := ZScreen + (i * 10)

			// Centralized Automatic Shadowing:
			// Apply a shadow to each layer at the base Z level (relative offset == 0).
			addShadowForLayer := func(l *lipgloss.Layer) {
				// originalZ == 0 means this is the base content layer, not a sub-layer (hit regions, etc.)
				originalZ := l.GetZ() - screenZBase
				if m.config.UI.Shadow && originalZ == 0 {
					content := l.GetContent()
					shadowBox := GetShadowBoxCtx(content, GetActiveContext())
					if shadowBox != "" {
						// Shadow is placed just below the layer with standard offset
						comp.AddLayers(lipgloss.NewLayer(shadowBox).
							X(l.GetX() + 2).
							Y(l.GetY() + 1).
							Z(l.GetZ() - 1))
					}
				}
			}

			if lv, ok := s.(LayeredView); ok {
				for _, l := range lv.Layers() {
					// Translate layer relative to screen position and stack Z
					l = l.X(l.GetX() + screenX).Y(l.GetY() + screenY).Z(l.GetZ() + screenZBase - ZScreen)
					if l.GetZ() > maxZ {
						maxZ = l.GetZ()
					}
					addShadowForLayer(l)
					comp.AddLayers(l)
				}
			} else {
				l := lipgloss.NewLayer(screenContent).X(screenX).Y(screenY).Z(screenZBase)
				if l.GetZ() > maxZ {
					maxZ = l.GetZ()
				}
				addShadowForLayer(l)
				comp.AddLayers(l)
			}

			// Collect hit regions from screen with the actual position
			if hrp, ok := s.(HitRegionProvider); ok {
				m.hitRegions = append(m.hitRegions, hrp.GetHitRegions(screenX, screenY)...)
			}
		}
	}

	// 4. Layer: Modal Dialog Stack
	allDialogs := append([]tea.Model{}, m.dialogStack...)
	if m.dialog != nil {
		allDialogs = append(allDialogs, m.dialog)
	}

	for i, d := range allDialogs {
		var content string
		if vs, ok := d.(ViewStringer); ok {
			content = vs.ViewString()
		} else {
			content = d.View().Content
		}

		if content != "" {
			maximized := false
			if md, ok := d.(interface{ IsMaximized() bool }); ok {
				maximized = md.IsMaximized()
			}

			fgWidth := WidthWithoutZones(content)
			fgHeight := lipgloss.Height(content)

			mode := DialogAbsoluteCentered
			targetHeight := m.backdropHeight()

			if _, ok := d.(*HelpDialogModel); ok {
				targetHeight = m.height
			}

			if maximized {
				mode = DialogMaximized
				targetHeight = m.backdropHeight()
			}

			lx, ly := layout.DialogPosition(mode, fgWidth, fgHeight, m.width, targetHeight, m.config.UI.Shadow, headerH)

			// Modal Offset: Each dialog in the stack sits 100 Z-units above the previous one
			// Base is maxZ (highest screen layer) + 100
			modalZBase := maxZ + 100 + (i * 100)

			// Centralized Automatic Shadowing for Dialogs
			addShadowForDialogLayer := func(l *lipgloss.Layer) {
				// originalZ == 0 means this is the base content layer for this dialog slot
				originalZ := l.GetZ() - modalZBase
				if m.config.UI.Shadow && originalZ == 0 {
					content := l.GetContent()
					shadowBox := GetShadowBoxCtx(content, GetActiveContext())
					if shadowBox != "" {
						// Shadow is placed just below the layer with standard offset
						comp.AddLayers(lipgloss.NewLayer(shadowBox).
							X(l.GetX() + 2).
							Y(l.GetY() + 1).
							Z(l.GetZ() - 1))
					}
				}
			}

			if lv, ok := d.(LayeredView); ok {
				for _, l := range lv.Layers() {
					// Apply modal offset to ensure it sits above the background content
					l = l.X(l.GetX() + lx).Y(l.GetY() + ly).Z(l.GetZ() + modalZBase - ZDialog)
					addShadowForDialogLayer(l)
					comp.AddLayers(l)
				}
			} else {
				l := lipgloss.NewLayer(content).X(lx).Y(ly).Z(modalZBase)
				addShadowForDialogLayer(l)
				comp.AddLayers(l)
			}

			// Collect hit regions from dialog
			if hrp, ok := d.(HitRegionProvider); ok {
				m.hitRegions = append(m.hitRegions, hrp.GetHitRegions(lx, ly)...)
			}
		}
	}

	// Sort hit regions ascending by ZOrder so FindHit (reverse iteration) checks highest-Z first.
	// Use Stable sort to ensure specific regions added later take precedence over generic ones.
	sort.SliceStable(m.hitRegions, func(i, j int) bool {
		return m.hitRegions[i].ZOrder < m.hitRegions[j].ZOrder
	})

	// Render the compositor
	v = tea.NewView(comp.Render())
	v.MouseMode = tea.MouseModeCellMotion
	v.AltScreen = true
	return v
}

// GetActiveScreen returns the currently active screen
func (m AppModel) GetActiveScreen() ScreenModel {
	return m.activeScreen
}

// Backdrop returns the shared backdrop model
func (m *AppModel) Backdrop() *BackdropModel {
	return m.backdrop
}

// GetLogPanel returns the log panel model
func (m AppModel) GetLogPanel() LogPanelModel {
	return m.logPanel
}
