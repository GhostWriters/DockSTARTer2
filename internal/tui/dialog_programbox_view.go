package tui

import (
	"DockSTARTer2/internal/displayengine"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ViewString delegates to outer for the dialog's own content, overlaying the
// sub-dialog (a blocking prompt shown during a running task) on top when
// present -- sub-dialog overlay compositing is orchestration-level, not a
// displayengine.Content-section concept, so it stays hand-rolled here rather than moving
// into a section.
func (m *ProgramBoxModel) ViewString() string {
	base := m.outer.ViewString()
	if m.subDialog != nil {
		var subView string
		if vs, ok := m.subDialog.(interface{ ViewString() string }); ok {
			subView = vs.ViewString()
		} else {
			subView = fmt.Sprintf("%v", m.subDialog.View())
		}
		base = displayengine.Overlay(subView, base, displayengine.OverlayCenter, displayengine.OverlayCenter, 0, 0)
	}
	return base
}

// View implements tea.Model
func (m *ProgramBoxModel) View() tea.View {
	v := tea.NewView(m.ViewString())
	v.MouseMode = tea.MouseModeCellMotion
	v.AltScreen = true
	return v
}

// Layers implements LayeredView. When a sub-dialog is active, its layers
// are returned as SIBLINGS of the base layer, not nested children -- the
// caller (model_view.go's modal-stack loop) applies its own X/Y/Z modal
// offset to each top-level layer this method returns but does not recurse
// into a layer's children. Nesting the sub-dialog as a child left it at its
// own small Z while the base layer got bumped to modalZBase, so it silently
// rendered underneath ProgramBox's content. Siblings get the same offset,
// preserving relative Z ordering.
func (m *ProgramBoxModel) Layers() []*lipgloss.Layer {
	base := m.outer.Layers()
	if m.subDialog == nil {
		return base
	}
	if len(base) == 0 {
		return base
	}

	lv, ok := m.subDialog.(LayeredView)
	if !ok {
		return base
	}
	subLayers := lv.Layers()
	if len(subLayers) == 0 {
		return base
	}

	var subStr string
	if vs, ok := m.subDialog.(interface{ ViewString() string }); ok {
		subStr = vs.ViewString()
	} else {
		subStr = fmt.Sprintf("%v", m.subDialog.View())
	}
	subW := lipgloss.Width(subStr)
	subH := lipgloss.Height(subStr)

	containerStr := m.outer.ViewString()
	containerW := lipgloss.Width(containerStr)
	containerH := lipgloss.Height(containerStr)

	offsetX := (containerW - subW) / 2
	offsetY := (containerH - subH) / 2

	// Z well above anything the base layer's own content uses, so the
	// sub-dialog draws on top after both receive the same modal-offset bump.
	const subDialogZBoost = 1000

	result := append([]*lipgloss.Layer{}, base...)
	for _, l := range subLayers {
		result = append(result, lipgloss.NewLayer(l.GetContent()).
			X(l.GetX()+offsetX).
			Y(l.GetY()+offsetY).
			Z(l.GetZ()+subDialogZBoost))
	}
	return result
}

// GetHitRegions implements displayengine.HitRegionProvider for mouse hit testing
func (m *ProgramBoxModel) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	regions := m.outer.GetHitRegions(offsetX, offsetY)

	if m.subDialog != nil {
		if hrp, ok := m.subDialog.(displayengine.HitRegionProvider); ok {
			containerStr := m.outer.ViewString()
			containerW := lipgloss.Width(containerStr)
			containerH := lipgloss.Height(containerStr)

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
