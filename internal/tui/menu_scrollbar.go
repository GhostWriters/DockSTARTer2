package tui

import (
	"fmt"
	"strings"

	"DockSTARTer2/internal/strutil"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// DragDoneMsg signals that a drag-induced render has completed, allowing
// the next throttled motion event to be processed.
type DragDoneMsg struct {
	ID string
}

// DragDoneCmd returns a command that sends a DragDoneMsg for the given ID.
func DragDoneCmd(id string) tea.Cmd {
	return func() tea.Msg {
		return DragDoneMsg{ID: id}
	}
}

// ScrollbarGutterWidth is the number of columns reserved for the right scrollbar/padding column.
// This slot is always reserved (space when scrollbar is off, track/thumb when on).
const ScrollbarGutterWidth = 1

// IsScrollbarEnabled reports whether the scrollbar is enabled in the current config.
func IsScrollbarEnabled() bool { return currentConfig.UI.Scrollbar }

// ScrollbarInfo describes the geometry of a rendered scrollbar column.
// It is returned by applyScrollbarColumnTracked so callers can compute hit regions.
type ScrollbarInfo struct {
	Needed     bool // true when total > visible and height >= 3
	Height     int  // total column height (== number of lines in content)
	ThumbStart int  // row index of thumb top (>= 1 because row 0 is the up arrow)
	ThumbEnd   int  // exclusive row index of thumb bottom (<= Height-1 because last row is down arrow)
	// TotalItems and VisibleItems are the original values passed to the scrollbar compute.
	// They are stored here so callers can compute maxOff = TotalItems - VisibleItems.
	TotalItems   int
	VisibleItems int
	// HitRegions contains granular relative hit targets for the scrollbar components,
	// anchored to the top-left of the scrollbar column itself (X=0, Y=0).
	HitRegions []HitRegion
}

// ComputeScrollbarInfo computes scrollbar geometry without rendering anything.
func ComputeScrollbarInfo(total, visible, offset, height int) ScrollbarInfo {
	if total <= visible || height < 3 {
		return ScrollbarInfo{Height: height}
	}
	trackH := height - 2 // rows 1..height-2 are the track; row 0 and height-1 are arrows
	thumbH := max(1, trackH*visible/total)
	// Map offset linearly over [0, total-visible] → thumbTrackStart over [0, trackH-thumbH]
	// so the thumb reaches the very bottom when scrolled to the end.
	thumbTrackStart := 0
	maxOff := total - visible
	if maxOff > 0 {
		thumbTrackStart = (trackH - thumbH) * offset / maxOff
		if thumbTrackStart > trackH-thumbH {
			thumbTrackStart = trackH - thumbH
		}
	}
	thumbStart := 1 + thumbTrackStart
	if thumbStart < 1 {
		thumbStart = 1
	}
	return ScrollbarInfo{
		Needed:       true,
		Height:       height,
		ThumbStart:   thumbStart,
		ThumbEnd:     thumbStart + thumbH,
		TotalItems:   total,
		VisibleItems: visible,
	}
}

// ApplyScrollbar appends one scrollbar/gutter character to the right of every line
// in content and updates the provided Scrollbar's geometry and info.
func ApplyScrollbar(sb *Scrollbar, content string, total, visible, offset int, lineChars bool, ctx StyleContext) string {
	if content == "" && visible <= 0 {
		if sb != nil {
			sb.Info = ScrollbarInfo{}
		}
		return content
	}
	// Trim trailing newline to avoid an extra blank line being treated as content.
	content = strings.TrimSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	if content == "" {
		lines = nil
	}
	contentH := len(lines)

	// Ensure the gutter column spans the full visible height even if content is shorter.
	sbHeight := contentH
	if visible > contentH {
		sbHeight = visible
	}

	info := ComputeScrollbarInfo(total, visible, offset, sbHeight)
	if sb != nil {
		sb.Info = info
	}

	if !IsScrollbarEnabled() {
		info.Needed = false
	}

	sbLines := buildScrollbarColumn(info, lineChars, ctx)

	// Append each scrollbar char to each content line
	var linesOut []string
	for i := 0; i < sbHeight; i++ {
		row := ""
		if i < contentH {
			row = lines[i]
		}
		linesOut = append(linesOut, row+sbLines[i])
	}

	return strings.Join(linesOut, "\n")
}

// ApplyScrollbarColumn is the non-tracking variant kept for callers that don't need geometry.
func ApplyScrollbarColumn(content string, total, visible, offset int, lineChars bool, ctx StyleContext) string {
	return ApplyScrollbar(nil, content, total, visible, offset, lineChars, ctx)
}

// ApplyScrollbarColumnTracked is a backward-compatibility shim for the new ApplyScrollbar helper.
func ApplyScrollbarColumnTracked(content string, total, visible, offset int, lineChars bool, ctx StyleContext) (string, ScrollbarInfo) {
	var sb Scrollbar
	res := ApplyScrollbar(&sb, content, total, visible, offset, lineChars, ctx)
	return res, sb.Info
}
// representing a vertical scrollbar column, given pre-computed geometry.
//
// When info.Needed is false the column is filled with blank styled spaces.
func buildScrollbarColumn(info ScrollbarInfo, lineChars bool, ctx StyleContext) []string {
	height := info.Height
	col := make([]string, height)

	bg := ctx.ContentBackground.GetBackground()
	trackStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(ctx.Border2Color)

	thumbStyle := lipgloss.NewStyle().
		Background(bg).
		Foreground(ctx.BorderColor)

	// No scrollbar needed — fill with spaces to hold the gutter width.
	if !info.Needed || height < 1 {
		blank := trackStyle.Render(" ")
		for i := range col {
			col[i] = blank
		}
		return col
	}

	// Choose characters based on line-art mode.
	var trackChar, thumbChar string
	var upArrow, downArrow string
	if lineChars {
		trackChar = "┃"
		thumbChar = "█"
		upArrow = "▴"
		downArrow = "▾"
	} else {
		trackChar = "|"
		thumbChar = "#"
		upArrow = "^"
		downArrow = "v"
	}

	for i := range col {
		switch {
		case i == 0:
			col[i] = thumbStyle.Render(upArrow)
		case i == height-1:
			col[i] = thumbStyle.Render(downArrow)
		case i >= info.ThumbStart && i < info.ThumbEnd:
			col[i] = thumbStyle.Render(thumbChar)
		default:
			col[i] = trackStyle.Render(trackChar)
		}
	}
	return col
}

// BuildPlainBottomBorder constructs a plain bottom border line (no label) matching the inner box style.
func BuildPlainBottomBorder(totalWidth int, focused bool, ctx StyleContext) string {
	var border lipgloss.Border
	if ctx.LineCharacters {
		if focused {
			border = ThickRoundedBorder
		} else {
			border = lipgloss.RoundedBorder()
		}
	} else {
		if focused {
			border = RoundedThickAsciiBorder
		} else {
			border = RoundedAsciiBorder
		}
	}
	borderStyle := ctx.Border2Flags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.Border2Color).
		Background(ctx.Dialog.GetBackground())
	inner := strutil.Repeat(border.Bottom, max(0, totalWidth-2))
	return borderStyle.Render(border.BottomLeft + inner + border.BottomRight)
}

// BuildLabeledBottomBorderCtx constructs a bottom border line with a short label
// on the LEFT side (e.g. "INS" or "OVR"), styled to match the box border.
// totalWidth is the full visual width of the bordered box including side border chars.
// The function selects border characters based on ctx.Type and ctx.LineCharacters.
func BuildLabeledBottomBorderCtx(totalWidth int, label string, focused bool, ctx StyleContext) string {
	var border lipgloss.Border
	var leftT, rightT string

	if ctx.Type == DialogTypeConfirm {
		// Slanted border variant used by the prompt dialog input box
		if ctx.LineCharacters {
			if focused {
				border = SlantedThickBorder
			} else {
				border = SlantedBorder
			}
		} else {
			if focused {
				border = SlantedThickAsciiBorder
			} else {
				border = SlantedAsciiBorder
			}
		}
		if ctx.LineCharacters {
			leftT = "┤"
			rightT = "├"
		} else {
			leftT = "+"
			rightT = "+"
		}
	} else {
		// Rounded border variant used by set-value / add-var input sections
		if ctx.LineCharacters {
			if focused {
				border = ThickRoundedBorder
			} else {
				border = lipgloss.RoundedBorder()
			}
		} else {
			if focused {
				border = RoundedThickAsciiBorder
			} else {
				border = RoundedAsciiBorder
			}
		}
		if ctx.LineCharacters {
			if focused {
				leftT = "┫"
				rightT = "┣"
			} else {
				leftT = "┤"
				rightT = "├"
			}
		} else {
			if focused {
				leftT = "H"
				rightT = "H"
			} else {
				leftT = "+"
				rightT = "+"
			}
		}
	}

	borderStyle := ctx.Border2Flags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.Border2Color).
		Background(ctx.Dialog.GetBackground())
	labelStyle := ctx.TagKey.Bold(true)

	labelWidth := lipgloss.Width(label)
	leftPadCnt := 1 // one border char before label connector
	totalLabelWidth := 1 + labelWidth + 1
	rightPadCnt := totalWidth - 2 - totalLabelWidth - leftPadCnt
	if rightPadCnt < 0 {
		rightPadCnt = 0
	}

	leftPart := borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, max(0, leftPadCnt)))
	leftConnector := borderStyle.Render(leftT)
	rightConnector := borderStyle.Render(rightT)
	rightPart := borderStyle.Render(strutil.Repeat(border.Bottom, max(0, rightPadCnt)) + border.BottomRight)
	return lipgloss.JoinHorizontal(lipgloss.Bottom, leftPart, leftConnector, labelStyle.Render(label), rightConnector, rightPart)
}

// BuildDualLabelBottomBorderCtx constructs a bottom border line with a label on the LEFT
// (e.g. "INS"/"OVR") and an optional label on the RIGHT (e.g. "42%").
// Pass an empty string for rightLabel when no right label is needed.
// Uses rounded borders (for editor inner boxes, not confirm-style slanted borders).
func BuildDualLabelBottomBorderCtx(totalWidth int, leftLabel, rightLabel string, focused bool, ctx StyleContext) string {
	var border lipgloss.Border
	if ctx.LineCharacters {
		if focused {
			border = ThickRoundedBorder
		} else {
			border = lipgloss.RoundedBorder()
		}
	} else {
		if focused {
			border = RoundedThickAsciiBorder
		} else {
			border = RoundedAsciiBorder
		}
	}

	var leftT, rightT string
	if ctx.LineCharacters {
		if focused {
			leftT = "┫"
			rightT = "┣"
		} else {
			leftT = "┤"
			rightT = "├"
		}
	} else {
		if focused {
			leftT = "H"
			rightT = "H"
		} else {
			leftT = "+"
			rightT = "+"
		}
	}

	borderStyle := ctx.Border2Flags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.Border2Color).
		Background(ctx.Dialog.GetBackground())
	labelStyle := ctx.TagKey.Bold(true)

	// Left segment: BottomLeft + 1×bottom + leftT + leftLabel + rightT
	leftLabelW := lipgloss.Width(leftLabel)
	leftSegW := 1 + 1 + 1 + leftLabelW + 1 // BottomLeft(1) + bottom(1) + leftT(1) + label + rightT(1)

	// Right segment (optional): leftT + rightLabel + rightT + 1×bottom + BottomRight
	rightLabelW := 0
	rightSegW := 0
	if rightLabel != "" {
		rightLabelW = lipgloss.Width(rightLabel)
		rightSegW = 1 + rightLabelW + 1 + 1 + 1 // leftT(1) + label + rightT(1) + bottom(1) + BottomRight(1)
	} else {
		rightSegW = 1 // just BottomRight
	}

	// Middle dashes fill the remaining width
	middleW := totalWidth - leftSegW - rightSegW
	if middleW < 0 {
		middleW = 0
	}

	parts := []string{
		borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, 1)),
		borderStyle.Render(leftT),
		labelStyle.Render(leftLabel),
		borderStyle.Render(rightT),
		borderStyle.Render(strutil.Repeat(border.Bottom, max(0, middleW))),
	}
	if rightLabel != "" {
		parts = append(parts,
			borderStyle.Render(leftT),
			labelStyle.Render(rightLabel),
			borderStyle.Render(rightT),
			borderStyle.Render(strutil.Repeat(border.Bottom, 1)+border.BottomRight),
		)
	} else {
		parts = append(parts, borderStyle.Render(border.BottomRight))
	}
	return lipgloss.JoinHorizontal(lipgloss.Bottom, parts...)
}

// ScrollbarDragState tracks the state needed to drag a scrollbar thumb correctly with throttling.
// Embed this in any model that has a draggable scrollbar.
type ScrollbarDragState struct {
	Dragging      bool
	StartMouseY   int // absolute mouse Y when drag started
	StartThumbTop int // absolute screen Y of thumb top when drag started

	// Throttling state
	PendingDragY int  // latest Y from motion events (always updated)
	LastDragY    int  // Y last actually applied
	DragPending  bool // true while a drag render is in-flight
}

// StartDrag records the starting positions for a new drag and resets throttling.
// sbAbsTopY is the absolute Y of the scrollbar column top (row 0 = up arrow).
// info is the ScrollbarInfo from the last render.
// clickY is the absolute mouse Y of the click.
func (s *ScrollbarDragState) StartDrag(clickY, sbAbsTopY int, info ScrollbarInfo) {
	s.Dragging = true
	s.StartMouseY = clickY
	s.StartThumbTop = sbAbsTopY + info.ThumbStart
	s.LastDragY = clickY
	s.PendingDragY = clickY
	s.DragPending = false
}

// StopDrag clears the drag state and throttling flags.
func (s *ScrollbarDragState) StopDrag() {
	s.Dragging = false
	s.DragPending = false
}

// ThumbTop returns the new clamped thumb-top position (0-based within the track)
// given the current absolute mouse Y.
// trackTopAbs is sbAbsTopY+1 (first track row, after the up arrow).
// thumbTravel is trackH - thumbH (max distance the thumb top can move).
func (s *ScrollbarDragState) ThumbTop(mouseY, trackTopAbs, thumbTravel int) int {
	newTop := s.StartThumbTop + (mouseY - s.StartMouseY)
	if newTop < trackTopAbs {
		newTop = trackTopAbs
	}
	if newTop > trackTopAbs+thumbTravel {
		newTop = trackTopAbs + thumbTravel
	}
	return newTop - trackTopAbs // 0-based within track
}

// ScrollOffset computes the new scroll offset given current mouse Y,
// the scrollbar geometry, and the content dimensions.
// Returns (newOffset, changed).
func (s *ScrollbarDragState) ScrollOffset(mouseY, sbAbsTopY, maxOff int, info ScrollbarInfo) (int, bool) {
	trackH := info.Height - 2
	if trackH < 1 {
		return 0, false
	}
	thumbH := info.ThumbEnd - info.ThumbStart
	thumbTravel := trackH - thumbH
	if thumbTravel < 1 {
		thumbTravel = 1
	}
	trackTopAbs := sbAbsTopY + 1
	thumbTrackStart := s.ThumbTop(mouseY, trackTopAbs, thumbTravel)
	newOff := thumbTrackStart * maxOff / thumbTravel
	if newOff < 0 {
		newOff = 0
	}
	if newOff > maxOff {
		newOff = maxOff
	}
	return newOff, true
}

// BuildScrollPercentBottomBorder constructs a bottom border line for an inner box
// with a scroll-percent label on the right, styled identically to the programbox indicator.
// totalWidth is the full visual width of the bordered box including side border chars.
// Only call this when a scrollbar is needed (sbInfo.Needed == true).
func BuildScrollPercentBottomBorder(totalWidth int, scrollPct float64, focused bool, ctx StyleContext) string {
	scrollIndicator := ctx.TagKey.Bold(true).Render(fmt.Sprintf("%d%%", int(scrollPct*100)))

	var border lipgloss.Border
	if ctx.LineCharacters {
		if focused {
			border = ThickRoundedBorder
		} else {
			border = lipgloss.RoundedBorder()
		}
	} else {
		if focused {
			border = RoundedThickAsciiBorder
		} else {
			border = RoundedAsciiBorder
		}
	}

	var leftT, rightT string
	if ctx.LineCharacters {
		if focused {
			leftT = "┫"
			rightT = "┣"
		} else {
			leftT = "┤"
			rightT = "├"
		}
	} else {
		if focused {
			leftT = "H"
			rightT = "H"
		} else {
			leftT = "+"
			rightT = "+"
		}
	}

	borderStyle := ctx.Border2Flags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.Border2Color).
		Background(ctx.Dialog.GetBackground())

	labelWidth := lipgloss.Width(scrollIndicator)
	rightPadCnt := 2
	totalLabelWidth := 1 + labelWidth + 1 // connector + label + connector
	if totalWidth < totalLabelWidth+rightPadCnt+2 {
		rightPadCnt = (totalWidth - totalLabelWidth) / 2
	}
	leftPadCnt := totalWidth - labelWidth - 4 - rightPadCnt
	if leftPadCnt < 0 {
		leftPadCnt = 0
		rightPadCnt = totalWidth - labelWidth - 4
		if rightPadCnt < 0 {
			rightPadCnt = 0
		}
	}

	leftPart := borderStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, max(0, leftPadCnt)))
	leftConnector := borderStyle.Render(leftT)
	rightConnector := borderStyle.Render(rightT)
	rightPart := borderStyle.Render(strutil.Repeat(border.Bottom, max(0, rightPadCnt)) + border.BottomRight)
	return lipgloss.JoinHorizontal(lipgloss.Bottom, leftPart, leftConnector, scrollIndicator, rightConnector, rightPart)
}// ScrollbarHitRegions returns a slice of granular hit regions for a scrollbar column.
// baseID is the prefix for hit IDs (e.g. "my-list").
// sbX, sbAbsTopY: absolute screen coordinates of the top-left of the scrollbar column.
// info: the ScrollbarInfo describing the geometry of the scrollbar.
func ScrollbarHitRegions(baseID string, sbX, sbAbsTopY int, info ScrollbarInfo, baseZ int, label string) []HitRegion {
	var regions []HitRegion
	if !info.Needed || info.Height < 1 {
		return regions
	}

	hitX := sbX
	hitW := 1

	// Up arrow (row 0)
	regions = append(regions, HitRegion{
		ID:     baseID + ".sb.up",
		X:      hitX,
		Y:      sbAbsTopY,
		Width:  hitW,
		Height: 1,
		ZOrder: baseZ + 10,
		Label:  label + " Scroll Up",
	})

	// Track above thumb (rows 1..ThumbStart-1)
	if aboveH := info.ThumbStart - 1; aboveH > 0 {
		regions = append(regions, HitRegion{
			ID:     baseID + ".sb.above",
			X:      hitX,
			Y:      sbAbsTopY + 1,
			Width:  hitW,
			Height: aboveH,
			ZOrder: baseZ + 10,
			Label:  label + " Page Up",
		})
	}

	// Thumb (rows ThumbStart..ThumbEnd-1)
	if thumbH := info.ThumbEnd - info.ThumbStart; thumbH > 0 {
		regions = append(regions, HitRegion{
			ID:     baseID + ".sb.thumb",
			X:      hitX,
			Y:      sbAbsTopY + info.ThumbStart,
			Width:  hitW,
			Height: thumbH,
			ZOrder: baseZ + 11,
			Label:  label + " Scroll Thumb",
		})
	}

	// Track below thumb (rows ThumbEnd..Height-2)
	if belowH := (info.Height - 1) - info.ThumbEnd; belowH > 0 {
		regions = append(regions, HitRegion{
			ID:     baseID + ".sb.below",
			X:      hitX,
			Y:      sbAbsTopY + info.ThumbEnd,
			Width:  hitW,
			Height: belowH,
			ZOrder: baseZ + 10,
			Label:  label + " Page Down",
		})
	}

	// Down arrow (row Height-1)
	regions = append(regions, HitRegion{
		ID:     baseID + ".sb.down",
		X:      hitX,
		Y:      sbAbsTopY + info.Height - 1,
		Width:  hitW,
		Height: 1,
		ZOrder: baseZ + 10,
		Label:  label + " Scroll Down",
	})

	return regions
}

// Scrollbar handles all state, rendering, and interaction logic for a vertical scrollbar.
type Scrollbar struct {
	ID string

	// Geometry and state
	Info    ScrollbarInfo
	Drag    ScrollbarDragState
	Pending bool // true while a wheel scroll is queued but not yet rendered

	// Absolute screen coordinates (set during Render/HitRegions)
	AbsTopY  int
	AbsLeftX int
}

// Update processes any message that affects the scrollbar (clicks, drags, wheel).
// Returns (newOffset, cmd, changed).
func (s *Scrollbar) Update(msg tea.Msg, currentOffset, totalItems, visibleItems int) (int, tea.Cmd, bool) {
	if !IsScrollbarEnabled() || totalItems <= visibleItems {
		return currentOffset, nil, false
	}
	maxOff := totalItems - visibleItems

	switch msg := msg.(type) {
	case ScrollDoneMsg:
		if msg.ID == s.ID {
			s.Pending = false
		}
		return currentOffset, nil, false

	case DragDoneMsg:
		if msg.ID == s.ID {
			s.Drag.DragPending = false
			// Catch up to any position skipped while the render was in flight.
			if s.Drag.PendingDragY != s.Drag.LastDragY {
				lastY := s.Drag.PendingDragY
				newOff, changed := s.Drag.ScrollOffset(lastY, s.AbsTopY, maxOff, s.Info)
				if changed {
					s.Drag.LastDragY = lastY
					s.Drag.DragPending = true
					return newOff, DragDoneCmd(s.ID), true
				}
				s.Drag.LastDragY = lastY
			}
		}
		return currentOffset, nil, false

	case LayerHitMsg:
		// Component clicks (arrows and track)
		newOff, changed := HandleScrollbarLayerHit(s.ID, msg, currentOffset, totalItems, visibleItems)
		if changed {
			return newOff, nil, true
		}
		// Drag start via hit-region
		if strings.HasSuffix(msg.ID, ".sb.thumb") && msg.Button == tea.MouseLeft {
			s.Drag.StartDrag(msg.Y, s.AbsTopY, s.Info)
			return currentOffset, nil, true
		}

	case tea.MouseWheelMsg:
		if s.Pending {
			return currentOffset, nil, false
		}
		switch msg.Button {
		case tea.MouseWheelUp:
			newOff := currentOffset - 3
			if newOff < 0 {
				newOff = 0
			}
			if newOff != currentOffset {
				s.Pending = true
				return newOff, scrollDoneCmd(s.ID), true
			}
		case tea.MouseWheelDown:
			newOff := currentOffset + 3
			if newOff > maxOff {
				newOff = maxOff
			}
			if newOff != currentOffset {
				s.Pending = true
				return newOff, scrollDoneCmd(s.ID), true
			}
		}

	case tea.MouseMotionMsg:
		if s.Drag.Dragging {
			s.Drag.PendingDragY = msg.Y
			if !s.Drag.DragPending {
				newOff, changed := s.Drag.ScrollOffset(msg.Y, s.AbsTopY, maxOff, s.Info)
				if changed {
					s.Drag.LastDragY = msg.Y
					s.Drag.DragPending = true
					return newOff, DragDoneCmd(s.ID), true
				}
			}
		}

	case tea.MouseReleaseMsg:
		if s.Drag.Dragging {
			s.Drag.StopDrag()
			return currentOffset, nil, true
		}
	}

	return currentOffset, nil, false
}

// Render returns the styled scrollbar column string after updating geometry.
func (s *Scrollbar) Render(height, total, visible, offset int, lineChars bool, ctx StyleContext) string {
	s.Info = ComputeScrollbarInfo(total, visible, offset, height)
	lines := buildScrollbarColumn(s.Info, lineChars, ctx)
	return strings.Join(lines, "\n")
}

// HitRegions returns the granular hit regions for the scrollbar.
func (s *Scrollbar) HitRegions(x, y int, baseZ int, label string) []HitRegion {
	if !IsScrollbarEnabled() {
		return nil
	}
	s.AbsLeftX = x
	s.AbsTopY = y
	return ScrollbarHitRegions(s.ID, x, y, s.Info, baseZ, label)
}

// HandleScrollbarLayerHit provides a centralized routine for updating a scroll offset
// based on a LayerHitMsg from a scrollbar component.
// It handles up/down arrows and page up/down (clicking the track above/below the thumb).
// Returns (newOffset, changed).
func HandleScrollbarLayerHit(baseID string, msg LayerHitMsg, currentOffset, totalItems, visibleItems int) (int, bool) {
	if !strings.HasPrefix(msg.ID, baseID+".sb.") {
		return currentOffset, false
	}

	maxOff := totalItems - visibleItems
	if maxOff < 0 {
		maxOff = 0
	}

	newOffset := currentOffset
	changed := false

	// Component behavior: arrows move by 1 line; track clicks move by a page.
	// (Mouse wheel is handled separately by the model using CursorUp/Down or similar).
	switch {
	case strings.HasSuffix(msg.ID, ".sb.up"):
		if msg.Button != HoverButton {
			newOffset--
			changed = true
		}
	case strings.HasSuffix(msg.ID, ".sb.down"):
		if msg.Button != HoverButton {
			newOffset++
			changed = true
		}
	case strings.HasSuffix(msg.ID, ".sb.above"):
		if msg.Button != HoverButton {
			newOffset -= visibleItems
			if newOffset < 0 {
				newOffset = 0
			}
			changed = true
		}
	case strings.HasSuffix(msg.ID, ".sb.below"):
		if msg.Button != HoverButton {
			newOffset += visibleItems
			if newOffset > maxOff {
				newOffset = maxOff
			}
			changed = true
		}
	}

	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset > maxOff {
		newOffset = maxOff
	}

	return newOffset, changed
}

