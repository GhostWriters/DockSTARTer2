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

	// 3. Layer: Active Screen
	if m.activeScreen != nil {
		var screenContent string
		if vs, ok := m.activeScreen.(ViewStringer); ok {
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

			// Centralized Automatic Shadowing:
			// Apply a shadow to each layer that is at a main visibility level (ZScreen or ZDialog).
			// This allows complex screens like DisplayOptionsScreen to have multiple shadowed boxes
			// without manual shadow logic in the screen code.
			addShadowForLayer := func(l *lipgloss.Layer) {
				if m.config.UI.Shadow && (l.GetZ() == ZScreen || l.GetZ() == ZDialog) {
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

			if lv, ok := m.activeScreen.(LayeredView); ok {
				for _, l := range lv.Layers() {
					// Translate layer relative to screen position
					l = l.X(l.GetX() + screenX).Y(l.GetY() + screenY)
					if l.GetZ() > maxZ {
						maxZ = l.GetZ()
					}
					addShadowForLayer(l)
					comp.AddLayers(l)
				}
			} else {
				l := lipgloss.NewLayer(screenContent).X(screenX).Y(screenY).Z(ZScreen)
				maxZ = ZScreen
				addShadowForLayer(l)
				comp.AddLayers(l)
			}

			// Collect hit regions from active screen with the actual position
			if hrp, ok := m.activeScreen.(HitRegionProvider); ok {
				m.hitRegions = append(m.hitRegions, hrp.GetHitRegions(screenX, screenY)...)
			}
		}
	}

	// 4. Layer: Modal Dialog
	if m.dialog != nil {
		var content string
		if vs, ok := m.dialog.(ViewStringer); ok {
			content = vs.ViewString()
		} else {
			content = m.dialog.View().Content
		}

		if content != "" {
			maximized := false
			if md, ok := m.dialog.(interface{ IsMaximized() bool }); ok {
				maximized = md.IsMaximized()
			}

			fgWidth := WidthWithoutZones(content)
			fgHeight := lipgloss.Height(content)

			mode := DialogAbsoluteCentered
			targetHeight := m.backdropHeight()

			if _, ok := m.dialog.(*HelpDialogModel); ok {
				targetHeight = m.height
			}

			if maximized {
				mode = DialogMaximized
				targetHeight = m.backdropHeight()
			}

			lx, ly := layout.DialogPosition(mode, fgWidth, fgHeight, m.width, targetHeight, m.config.UI.Shadow, headerH)

			// Modal Offset: Ensure all dialog layers sit above the screen's maxZ
			modalZBase := maxZ + 100

			// Centralized Automatic Shadowing for Dialogs
			addShadowForDialogLayer := func(l *lipgloss.Layer) {
				// Check if the layer's Z (before modal offset) was at a main visibility level
				originalZ := l.GetZ() - modalZBase
				if m.config.UI.Shadow && (originalZ == ZDialog || originalZ == ZScreen) {
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

			if lv, ok := m.dialog.(LayeredView); ok {
				for _, l := range lv.Layers() {
					// Apply modal offset to ensure it sits above the screen content
					l = l.X(l.GetX() + lx).Y(l.GetY() + ly).Z(l.GetZ() + modalZBase)
					addShadowForDialogLayer(l)
					comp.AddLayers(l)
				}
			} else {
				l := lipgloss.NewLayer(content).X(lx).Y(ly).Z(modalZBase + ZDialog)
				addShadowForDialogLayer(l)
				comp.AddLayers(l)
			}

			// Collect hit regions from dialog
			if hrp, ok := m.dialog.(HitRegionProvider); ok {
				m.hitRegions = append(m.hitRegions, hrp.GetHitRegions(lx, ly)...)
			}
		}
	}

	// Sort hit regions ascending by ZOrder so FindHit (reverse iteration) checks highest-Z first
	sort.Slice(m.hitRegions, func(i, j int) bool {
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
