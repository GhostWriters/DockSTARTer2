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
) string {
	listContent, sbInfo := tui.ApplyScrollbarColumnTracked(
		strings.Join(listLines, "\n"),
		totalRows, visRows, offsetRows,
		tui.IsScrollbarEnabled(), ctx.LineCharacters, ctx,
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

	return section
}

// ListBoxHitRegions returns the two hit regions used by every scrollable list box:
//
//  1. Outer box region (boxID) at z=baseZ — covers entire bordered box including
//     title border and bottom border; clicking here focuses the list without
//     selecting an item.
//
//  2. Inner content region (contentID) at z=baseZ+5 — covers only the content
//     rows; clicking here focuses AND selects the item under the cursor.
//
// listTop is the dialog-relative Y of the first content row (title border row + 1).
// listH is the visible content row count (m.maxVis equivalent — not the box height).
// contentW = sInnerW + 2 (matches the width of the outer border).
func ListBoxHitRegions(
	boxID, contentID string,
	offsetX, offsetY int,
	contentW int,
	listTop, listH int,
	baseZ int,
	label string,
	help *tui.HelpContext,
) []tui.HitRegion {
	var regions []tui.HitRegion

	// Outer box: title border row + content rows + bottom border row = listH + 2.
	box := tui.HitRegion{
		ID:     boxID,
		X:      offsetX + 1,
		Y:      offsetY + listTop - 1, // include title border row
		Width:  contentW,
		Height: listH + 2,
		ZOrder: baseZ,
		Label:  label,
	}
	if help != nil {
		box.Help = help
	}
	regions = append(regions, box)

	// Inner content rows (excluding scrollbar column).
	// contentW = sInnerW + 2; scrollbar is the last column of sInnerW, i.e. at
	// offset (contentW - 2) from the section left border.  We cover only columns
	// [offsetX+1 .. offsetX+contentW-2] so the scrollbar column is not included.
	if listH > 0 {
		regions = append(regions, tui.HitRegion{
			ID:     contentID,
			X:      offsetX + 1,
			Y:      offsetY + listTop,
			Width:  contentW - 1, // excludes the rightmost scrollbar column
			Height: listH,
			ZOrder: baseZ + 5,
			Label:  label,
		})
		// Scrollbar column: same ID as the box so clicks here only focus, never select.
		regions = append(regions, tui.HitRegion{
			ID:     boxID,
			X:      offsetX + contentW - 1, // rightmost column of section content
			Y:      offsetY + listTop,
			Width:  1,
			Height: listH,
			ZOrder: baseZ + 8,
			Label:  label,
		})
	}

	return regions
}
