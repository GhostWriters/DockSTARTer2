package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ViewString delegates to outer for the dialog's own content, overlaying the
// sub-dialog (a blocking prompt shown during a running task) on top when
// present -- sub-dialog overlay compositing is orchestration-level, not a
// Content-section concept, so it stays hand-rolled here rather than moving
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
		base = Overlay(subView, base, OverlayCenter, OverlayCenter, 0, 0)
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

// Layers implements LayeredView. When a sub-dialog is active, its layers are
// returned as SIBLINGS of the base layer (not nested children) -- the caller
// (model_view.go's modal-stack loop) applies its own X/Y/Z modal offset to
// each TOP-LEVEL layer this method returns, but does NOT recurse into a
// layer's children to apply that same offset. Nesting the sub-dialog as a
// child of the base layer left it at the child's own small Z (e.g.
// ZScreen+10) while the base layer got bumped to the much larger modalZBase,
// so the sub-dialog silently rendered underneath (and was fully covered by)
// ProgramBox's own content despite compositing correctly in every other
// respect. Returning siblings lets both receive the identical modal offset,
// preserving their relative Z ordering.
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

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (m *ProgramBoxModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	regions := m.outer.GetHitRegions(offsetX, offsetY)

	if m.subDialog != nil {
		if hrp, ok := m.subDialog.(HitRegionProvider); ok {
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
