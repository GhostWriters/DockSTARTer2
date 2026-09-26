package classic

import (
	"DockSTARTer2/internal/strutil"
	"fmt"
	"strconv"

	"charm.land/lipgloss/v2"
)

// FooterBar is controls at the right of a submenu's bottom border, below
// its footer (see SetFooter) when there is one. Every field is read each time
// the menu draws.
type FooterBar struct {
	Controls func() []TitleControl
}

// SetFooterBar draws bar in this submenu's bottom border; nil removes it.
func (m *MenuModel) SetFooterBar(bar *FooterBar) {
	m.footerBar = bar
	m.InvalidateCache()
}

// FooterBarControlID is the hit region ID of footer bar control i.
func (m *MenuModel) FooterBarControlID(i int) string {
	return m.id + ".footerbar.ctl" + strconv.Itoa(i)
}

// SetDarkBorder draws every edge of this submenu's border in the Border2
// style, as for a footer (see SetFooter) set apart from the list above.
func (m *MenuModel) SetDarkBorder(v bool) {
	if m.darkBorder != v {
		m.darkBorder = v
		m.InvalidateCache()
	}
}

// footerBarControls returns the bar's controls.
func (m *MenuModel) footerBarControls() []TitleControl {
	if m.footerBar.Controls == nil {
		return nil
	}
	return m.footerBar.Controls()
}

// footerBarRegion is a hit region of the last drawn footer bar, in columns
// from the menu's left edge.
type footerBarRegion struct {
	id, label string
	x, width  int
}

// footerBarLine draws a line width wide in the Border2 style: the bottom
// border when bottom, else the footer's top edge, joining the side edges.
// pct, when not empty, is the scroll percent drawn at the right as on a
// plain bottom border. With controls it draws the footer bar's controls,
// recording their hit regions.
func (m *MenuModel) footerBarLine(width int, bottom, focused bool, pct string, controls bool, ctx StyleContext) string {
	var border lipgloss.Border
	switch {
	case !ctx.DrawBorders:
		border = lipgloss.HiddenBorder()
	case ctx.LineCharacters && focused:
		border = ThickRoundedBorder
	case ctx.LineCharacters:
		border = lipgloss.RoundedBorder()
	case focused:
		border = RoundedThickAsciiBorder
	default:
		border = RoundedAsciiBorder
	}
	style := ctx.Border2Flags.Apply(lipgloss.NewStyle()).Foreground(ctx.Border2Color).Background(ctx.Dialog.GetBackground())
	h := border.Bottom
	left, right := border.BottomLeft, border.BottomRight
	if !bottom {
		switch {
		case !ctx.DrawBorders:
		case ctx.LineCharacters:
			left = footerJunction(true, focused, []rune(h)[0])
			right = footerJunction(false, focused, []rune(h)[0])
		default:
			left, right = "+", "+"
		}
	}

	tail := style.Render(right)
	if pct != "" {
		leftT, rightT := "┤", "├"
		switch {
		case !ctx.LineCharacters && focused:
			leftT, rightT = "H", "H"
		case !ctx.LineCharacters:
			leftT, rightT = "|", "|"
		case focused:
			leftT, rightT = "┫", "┣"
		}
		tail = style.Render(leftT) + ctx.TagKey.Bold(true).Render(pct) + style.Render(rightT+strutil.Repeat(h, 2)+right)
	}
	head := style.Render(left + h)
	tailW, headW := WidthWithoutZones(tail), WidthWithoutZones(head)

	if !controls {
		return head + style.Render(strutil.Repeat(h, max(width-headW-tailW, 0))) + tail
	}

	// The controls that fit, separated by the border line, right-aligned
	// before the scroll percent's room, so they stay put when it moves (see
	// joinFooter).
	reserved := max(tailW, WidthWithoutZones(style.Render("┤100%├"+strutil.Repeat(h, 2)+right)))
	bar := m.footerBarControls()
	pieces := titleControlPiecesFor(bar, ctx, false)
	group, n := 0, 0
	for _, p := range pieces {
		w := WidthWithoutZones(p)
		if n > 0 {
			w++
		}
		if headW+group+w+1+reserved > width {
			break
		}
		group += w
		n++
	}
	x := max(width-reserved-1-group, headW)
	line := head + style.Render(strutil.Repeat(h, x-headW))
	regions := make([]footerBarRegion, 0, n)
	for i := range n {
		if i > 0 {
			line += style.Render(h)
			x++
		}
		w := WidthWithoutZones(pieces[i])
		regions = append(regions, footerBarRegion{id: m.FooterBarControlID(i), label: bar[i].Help, x: x, width: w})
		line += pieces[i]
		x += w
	}
	m.footerBarRegions = regions
	return line + style.Render(strutil.Repeat(h, max(width-x-tailW, 0))) + tail
}

// footerBarHitRegions returns the last drawn footer bar's hit regions for a
// menu at (offsetX, offsetY).
func (m *MenuModel) footerBarHitRegions(offsetX, offsetY, zOrder int) []HitRegion {
	regions := make([]HitRegion, 0, len(m.footerBarRegions))
	for _, r := range m.footerBarRegions {
		regions = append(regions, HitRegion{ID: r.id, X: offsetX + r.x, Y: offsetY + m.footerBarY,
			Width: r.width, Height: 1, ZOrder: zOrder, Label: r.label})
	}
	return regions
}

// footerBarStamp describes the footer bar's current drawing, for the view
// cache (see viewStamp).
func (m *MenuModel) footerBarStamp() string {
	if m.footerBar == nil {
		return ""
	}
	s := ""
	for _, c := range m.footerBarControls() {
		if c.Checked != nil {
			s += fmt.Sprint("|", c.Checked())
		}
		if c.Value != nil {
			s += "|" + c.Value()
		}
		if c.Changed != nil {
			s += fmt.Sprint("|", c.Changed())
		}
	}
	return s
}
