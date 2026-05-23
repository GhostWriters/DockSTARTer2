package tui

import (
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ViewString renders the context menu as a string.
func (m *ContextMenuModel) ViewString() string {
	if m.menuW <= 0 {
		return ""
	}

	ctx := GetActiveContext()
	bgStyle := theme.ThemeSemanticStyle("{{|Dialog|}}")
	normalStyle := theme.ThemeSemanticStyle("{{|Item|}}")
	selectedStyle := theme.ThemeSemanticStyle("{{|ItemSelected|}}")
	subLabelStyle := theme.ThemeSemanticStyle("{{|HelpItem|}}")
	disabledStyle := normalStyle.Faint(true)

	// Compute which items are visible
	visible := m.visibleItems()

	var lines []string
	absIdx := m.offset
	for _, item := range visible {
		if item.IsSeparator {
			sepChar := "─"
			if !ctx.LineCharacters {
				sepChar = "-"
			}
			sep := bgStyle.Render(" " + strutil.Repeat(sepChar, m.menuW) + " ")
			lines = append(lines, sep)
			absIdx++
			continue
		}
		if item.IsHeader {
			headerStyle := theme.ThemeSemanticStyle("{{|EnvBuiltin|}}").Bold(true)
			lbl := item.Label
			if lipgloss.Width(lbl) > m.menuW {
				lbl = TruncateRight(lbl, m.menuW)
			}
			pad := m.menuW - lipgloss.Width(lbl)
			if pad < 0 {
				pad = 0
			}
			lines = append(lines, bgStyle.Render(" ")+headerStyle.Render(lbl)+bgStyle.Render(strutil.Repeat(" ", pad)+" "))
			absIdx++
			continue
		}

		label := item.Label
		if len(item.SubItems) > 0 {
			label += " ▶"
		}
		// Truncate if needed
		if lipgloss.Width(label) > m.menuW {
			label = TruncateRight(label, m.menuW)
		}
		// Pad to full width
		pad := m.menuW - lipgloss.Width(label)
		if pad < 0 {
			pad = 0
		}
		line := " " + label + strutil.Repeat(" ", pad) + " "

		if item.Disabled {
			lines = append(lines, MaintainBackground(disabledStyle.Render(line), bgStyle))
		} else if absIdx == m.cursor {
			lines = append(lines, MaintainBackground(selectedStyle.Render(line), selectedStyle))
			if item.SubLabel != "" {
				sl := item.SubLabel
				if lipgloss.Width(sl) > m.menuW {
					sl = TruncateRight(sl, m.menuW)
				}
				slPad := m.menuW - lipgloss.Width(sl)
				if slPad < 0 {
					slPad = 0
				}
				lines = append(lines, MaintainBackground(selectedStyle.Render(" "+sl+strutil.Repeat(" ", slPad)+" "), selectedStyle))
			}
		} else {
			lines = append(lines, MaintainBackground(normalStyle.Render(line), bgStyle))
			if item.SubLabel != "" {
				sl := item.SubLabel
				if lipgloss.Width(sl) > m.menuW {
					sl = TruncateRight(sl, m.menuW)
				}
				slPad := m.menuW - lipgloss.Width(sl)
				if slPad < 0 {
					slPad = 0
				}
				lines = append(lines, MaintainBackground(subLabelStyle.Background(bgStyle.GetBackground()).Render(" "+sl+strutil.Repeat(" ", slPad)+" "), bgStyle))
			}
		}
		absIdx++
	}

	content := strings.Join(lines, "\n")

	// Draw border using the same ctx-driven approach as other dialogs
	var border lipgloss.Border
	if ctx.LineCharacters {
		border = lipgloss.NormalBorder()
	} else {
		border = AsciiBorder
	}
	borderBG := bgStyle.GetBackground()

	boxStyle := lipgloss.NewStyle().
		Border(border).
		BorderTopForeground(ctx.BorderColor).
		BorderLeftForeground(ctx.BorderColor).
		BorderBottomForeground(ctx.Border2Color).
		BorderRightForeground(ctx.Border2Color).
		BorderTopBackground(borderBG).
		BorderLeftBackground(borderBG).
		BorderBottomBackground(borderBG).
		BorderRightBackground(borderBG).
		Background(borderBG)

	return boxStyle.Render(content)
}

// View implements tea.Model.
func (m *ContextMenuModel) View() tea.View {
	v := tea.NewView(m.ViewString())
	v.MouseMode = tea.MouseModeCellMotion
	v.AltScreen = true
	return v
}

// Layers implements LayeredView.
// model_view.go adds lx, ly (DialogMaximized offsets) to our layer X/Y.
// We compensate so the menu lands at exactly (menuX, menuY) in screen coordinates.
func (m *ContextMenuModel) Layers() []*lipgloss.Layer {
	content := m.ViewString()
	if content == "" {
		return nil
	}
	layout := GetLayout()
	lx := layout.EdgeIndent
	ly := layout.ContentStartY(1) // 1-line header: headerH(1) + separator(1) + gap(1) = 3
	layerX := m.menuX - lx
	layerY := m.menuY - ly
	return []*lipgloss.Layer{
		lipgloss.NewLayer(content).X(layerX).Y(layerY).Z(ZScreen).ID("Dialog.ContextMenu"),
	}
}

// GetHitRegions implements the hit-region interface so mouse events route correctly.
func (m *ContextMenuModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	if m.menuW <= 0 {
		return nil
	}
	var regions []HitRegion

	// The full box (border included) as a background catch-all
	totalW := m.menuW + 2 + 2 // content + 2 padding + 2 border
	totalH := m.menuH + 2     // menuH is already in display rows
	regions = append(regions, HitRegion{
		ID:     "ctxmenu.bg",
		X:      m.menuX,
		Y:      m.menuY,
		Width:  totalW,
		Height: totalH,
		ZOrder: ZDialog + 5,
		Label:  "Context Menu",
	})

	// Per-item rows (inside border, starting at menuY+1)
	visible := m.visibleItems()
	absIdx := m.offset
	rowY := m.menuY + 1
	for _, item := range visible {
		h := itemHeight(item)
		if !item.IsSeparator && !item.IsHeader {
			regions = append(regions, HitRegion{
				ID:     "ctxmenu.item-" + itoa(absIdx),
				X:      m.menuX + 1,
				Y:      rowY,
				Width:  m.menuW + 2, // content + 2 padding spaces
				Height: h,
				ZOrder: ZDialog + 10,
				Label:  item.Label,
			})
		}
		absIdx++
		rowY += h
	}
	return regions
}
