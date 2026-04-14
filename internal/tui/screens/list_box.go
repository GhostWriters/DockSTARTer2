package screens

import (
	"strings"

	"DockSTARTer2/internal/tui"

	"charm.land/lipgloss/v2"
)

// TwoColumnRow is one pre-built display row for a two-column list.
// Build these with RenderTwoColumnRow, then pass the slice to RenderListInBorderedBox.
type TwoColumnRow = string

// RenderTwoColumnRow builds one row with a cursor indicator, a label column (with
// hotkey on the first character), and an optional value column.
// maxLabelW is the pre-computed max width of all label strings (for alignment).
// maxItemW is the total usable content width (excluding cursor+space and scrollbar gutter).
func RenderTwoColumnRow(label, value string, cursor, focused bool, maxLabelW, maxItemW int, ctx tui.StyleContext) string {
	bgStyle := tui.GetStyles().Dialog
	neutralStyle := lipgloss.NewStyle().Background(bgStyle.GetBackground())
	valStyle := tui.GetStyles().ItemNormal.Background(bgStyle.GetBackground())
	if focused {
		valStyle = tui.GetStyles().ItemSelected
	}

	cursorStr := " "
	if cursor {
		cursorStr = ">"
	}
	cursorRendered := neutralStyle.Render(cursorStr + " ")

	if lipgloss.Width(label) > maxLabelW {
		label = tui.TruncateRight(label, maxLabelW)
	}
	labelW := lipgloss.Width(label)
	labelStr := tui.RenderHotkeyLabelCtx(label, focused, ctx)

	if value == "" {
		// Label-only row: fill to maxItemW.
		trailW := maxItemW + 1 - 2 - labelW
		if trailW < 0 {
			trailW = 0
		}
		return cursorRendered + labelStr + neutralStyle.Render(strings.Repeat(" ", trailW))
	}

	// Two-column row.
	pad := maxLabelW - labelW
	paddingStr := neutralStyle.Render(strings.Repeat(" ", pad+2)) // align + 2-space gap

	valW := lipgloss.Width(value)
	remaining := maxItemW - maxLabelW - 2
	if remaining < 0 {
		remaining = 0
	}
	if valW > remaining {
		value = tui.TruncateRight(value, remaining)
		valW = remaining
	}
	trailW := maxItemW + 1 - 2 - maxLabelW - 2 - valW
	if trailW < 0 {
		trailW = 0
	}
	return cursorRendered + labelStr + paddingStr + valStyle.Render(value) + neutralStyle.Render(strings.Repeat(" ", trailW))
}

// RenderListInBorderedBox wraps pre-built list rows in a scrollbar column and a
// titled bordered box, replacing the bottom border with a scroll-percent indicator
// when the list overflows.
//
//   - title:      section title rendered in the box's top border
//   - listLines:  already-rendered rows (one per list item, not yet joined)
//   - totalRows:  total logical rows in the full (un-windowed) list
//   - visRows:    visible row budget (m.maxVis equivalent)
//   - offsetRows: first logical row currently visible (for scroll percent)
//   - sInnerW:    inner width of the section (contentW - 2)
//   - targetH:    total height of the bordered box including top/bottom borders
//   - focused:    whether the list has keyboard focus
//   - ctx:        style context
func RenderListInBorderedBox(
	title string,
	listLines []string,
	totalRows, visRows, offsetRows int,
	sInnerW, targetH int,
	focused bool,
	ctx tui.StyleContext,
) (string, tui.ScrollbarInfo) {
	// Use the actual physical inner height (targetH - 2) for scrollbar geometry
	// and padding, ensuring the gutter column always spans the full box.
	innerH := targetH - 2
	if innerH < 1 {
		innerH = 1
	}
	listContent, sbInfo := tui.ApplyScrollbarColumnTracked(
		strings.Join(listLines, "\n"),
		totalRows, innerH, offsetRows,
		ctx.LineCharacters, ctx,
	)

	titleTag := "TitleSubMenu"
	if focused {
		titleTag = "TitleSubMenuFocused"
	}

	section := strings.TrimRight(tui.RenderBorderedBoxCtx(
		title, listContent, sInnerW, targetH, focused, true, true,
		ctx.SubmenuTitleAlign, titleTag, ctx,
	), "\n")

	if sbInfo.Needed {
		scrollPct := 0.0
		if totalRows > visRows {
			scrollPct = float64(offsetRows) / float64(totalRows-visRows)
			if scrollPct > 1.0 {
				scrollPct = 1.0
			}
		}
		lines := strings.Split(section, "\n")
		if len(lines) > 0 {
			lines[len(lines)-1] = tui.BuildScrollPercentBottomBorder(sInnerW+2, scrollPct, focused, ctx)
			section = strings.Join(lines, "\n")
		}
	}

	return section, sbInfo
}

// ListBoxHitRegions returns the hit regions used by scrollable list boxes.
//
//  1. Outer box region (boxID) at z=baseZ — covers entire bordered box including
//     borders; clicking here focuses the list without selecting an item.
//
//  2. Inner content region (contentID) at z=baseZ+5 — covers only the content
//     rows excluding the scrollbar; clicking here focuses AND selects the item.
//
// boxX, boxY: top-left of the bordered box (at the top-left border character).
// boxW: total width of the box including borders (sInnerW + 2).
// contentH: visible content row count (m.maxVis).
func ListBoxHitRegions(
	boxID, contentID string,
	boxX, boxY, boxW, contentH int,
	baseZ int,
	label string,
	info tui.ScrollbarInfo,
	help *tui.HelpContext,
) []tui.HitRegion {
	var regions []tui.HitRegion

	// Outer box: covers borders and content. Total height = contentH + 2.
	box := tui.HitRegion{
		ID:     boxID,
		X:      boxX,
		Y:      boxY,
		Width:  boxW,
		Height: contentH + 2,
		ZOrder: baseZ,
		Label:  label,
	}
	if help != nil {
		box.Help = help
	}
	regions = append(regions, box)
	if contentH <= 0 {
		return regions
	}

	// Inner content rows (excluding left border and scrollbar column).
	// contentW = boxW - 2. Scrollbar is the last column of content.
	regions = append(regions, tui.HitRegion{
		ID:     contentID,
		X:      boxX + 1,
		Y:      boxY + 1,
		Width:  boxW - 3, // excludes borders and the 1-char scrollbar
		Height: contentH,
		ZOrder: baseZ + 5,
		Label:  label,
		Help:   help,
	})

	// Scrollbar hits (detailed targets for up/down/track/thumb)
	if info.Needed {
		sbX := boxX + boxW - 2 // column inside the right border
		regions = append(regions, tui.ScrollbarHitRegions(boxID, sbX, boxY+1, info, baseZ, label)...)
	} else {
		// Generic fallback if scrollbar is not needed or not yet computed
		regions = append(regions, tui.HitRegion{
			ID:     boxID,
			X:      boxX + boxW - 2,
			Y:      boxY + 1,
			Width:  1,
			Height: contentH,
			ZOrder: baseZ + 8,
			Label:  label,
		})
	}

	return regions
}
