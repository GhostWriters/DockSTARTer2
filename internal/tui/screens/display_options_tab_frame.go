package screens

import (
	"strings"

	"DockSTARTer2/internal/displayengine"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// tabFrame is one box, with a tab strip in its top border, drawn around
// several consecutive sections of a ContentColumn. Each framed section is
// wrapped in its own tabFrameSection, which draws that section's share of
// the box, so the column keeps one item (and one Tab stop) per section.
type tabFrame struct {
	strip *displayengine.TabStrip
	// focused reports whether any framed section holds focus, so every
	// part of the box highlights together.
	focused func() bool
}

// tabFrameSection draws inner plus its share of frame's box: the side edges
// always, the top edge (with the tab strip) when top, the bottom edge when
// bottom.
type tabFrameSection struct {
	inner       displayengine.Content
	frame       *tabFrame
	top, bottom bool
	width       int
}

var (
	_ displayengine.Content        = (*tabFrameSection)(nil)
	_ displayengine.ContentWrapper = (*tabFrameSection)(nil)
)

func newTabFrameSection(inner displayengine.Content, frame *tabFrame, top, bottom bool) *tabFrameSection {
	return &tabFrameSection{inner: inner, frame: frame, top: top, bottom: bottom}
}

// Unwrap implements displayengine.ContentWrapper.
func (f *tabFrameSection) Unwrap() displayengine.Content { return f.inner }

// edgeRows returns how many rows the top and bottom edges add.
func (f *tabFrameSection) edgeRows() int {
	n := 0
	if f.top {
		n++
	}
	if f.bottom {
		n++
	}
	return n
}

// innerOffsetY is the inner section's row offset within this section.
func (f *tabFrameSection) innerOffsetY() int {
	if f.top {
		return 1
	}
	return 0
}

func (f *tabFrameSection) SectionHeight(width int) int {
	return f.inner.SectionHeight(max(width-2, 1)) + f.edgeRows()
}

func (f *tabFrameSection) SectionNaturalWidth(maxWidth int) int {
	return f.inner.SectionNaturalWidth(max(maxWidth-2, 1)) + 2
}

func (f *tabFrameSection) SetSize(width, height int) {
	f.width = width
	f.inner.SetSize(max(width-2, 1), max(height-f.edgeRows(), 1))
}

// stripAvailWidth is the width the tab strip may use in the top edge.
func (f *tabFrameSection) stripAvailWidth(ctx displayengine.StyleContext) int {
	return displayengine.MaxRawTitleWidth(max(f.width-2, 1), false, ctx.SubmenuTitleAlign, nil, ctx)
}

func (f *tabFrameSection) ViewString() string {
	ctx := displayengine.GetActiveContext()
	body := f.inner.ViewString()
	contentWidth := max(f.width-2, 1)
	focused := f.frame.focused != nil && f.frame.focused()
	title := ""
	if f.top {
		title = f.frame.strip.Render(f.stripAvailWidth(ctx), focused, ctx)
	}
	box := displayengine.RenderBorderedBoxCtx(title, body, contentWidth, lipgloss.Height(body)+2,
		focused, false, true, ctx.SubmenuTitleAlign, "RAW", ctx)
	lines := strings.Split(box, "\n")
	if !f.top && len(lines) > 0 {
		lines = lines[1:]
	}
	if !f.bottom && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func (f *tabFrameSection) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	regions := f.inner.GetHitRegions(offsetX+1, offsetY+f.innerOffsetY())
	if f.top {
		ctx := displayengine.GetActiveContext()
		avail := f.stripAvailWidth(ctx)
		titleWidth := displayengine.WidthWithoutZones(f.frame.strip.Render(avail, false, ctx))
		leftPad := 0
		if ctx.SubmenuTitleAlign != "left" {
			leftPad = max((max(f.width-2, 1)-titleWidth)/2, 0)
		}
		regions = append(regions, f.frame.strip.HitRegions(offsetX+1+leftPad, offsetY, avail, displayengine.ZDialog+10, ctx, nil)...)
	}
	return regions
}

func (f *tabFrameSection) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := f.inner.Update(msg)
	if c, ok := updated.(displayengine.Content); ok {
		f.inner = c
	}
	return f, cmd
}

func (f *tabFrameSection) View() tea.View               { return tea.View{Content: f.ViewString()} }
func (f *tabFrameSection) Init() tea.Cmd                { return nil }
func (f *tabFrameSection) SetSubFocused(v bool) tea.Cmd { return f.inner.SetSubFocused(v) }
func (f *tabFrameSection) SetIsDialog(v bool)           { f.inner.SetIsDialog(v) }
func (f *tabFrameSection) SetLockedByOthers(v bool)     { f.inner.SetLockedByOthers(v) }
func (f *tabFrameSection) IsVariableHeight() bool       { return f.inner.IsVariableHeight() }
func (f *tabFrameSection) Height() int                  { return f.inner.Height() + f.edgeRows() }
func (f *tabFrameSection) ID() string                   { return f.inner.ID() }
func (f *tabFrameSection) ScrollID() string             { return f.inner.ScrollID() }
func (f *tabFrameSection) WantsHorizontalKeys() bool    { return f.inner.WantsHorizontalKeys() }
func (f *tabFrameSection) WantsAllMessages() bool       { return f.inner.WantsAllMessages() }
func (f *tabFrameSection) Focusable() bool              { return f.inner.Focusable() }
func (f *tabFrameSection) AbsorbMessage(m tea.Msg) tea.Cmd {
	return f.inner.AbsorbMessage(m)
}
func (f *tabFrameSection) IsProcessing() bool { return f.inner.IsProcessing() }

// MatchesID also claims the tab strip's own hit regions, so a click on a tab
// label routes to this section like a click on its content.
func (f *tabFrameSection) MatchesID(msgID string) bool {
	return f.inner.MatchesID(msgID) || (f.top && f.frame.strip.OwnsID(msgID))
}

// ClearProcessingState, IsScrollbarDragging and HelpText pass through the
// optional interfaces ContentColumn/ContentRow probe their items for.
func (f *tabFrameSection) ClearProcessingState() {
	if cp, ok := f.inner.(interface{ ClearProcessingState() }); ok {
		cp.ClearProcessingState()
	}
}

func (f *tabFrameSection) IsScrollbarDragging() bool {
	d, ok := f.inner.(interface{ IsScrollbarDragging() bool })
	return ok && d.IsScrollbarDragging()
}

func (f *tabFrameSection) HelpText() string {
	if h, ok := f.inner.(interface{ HelpText() string }); ok {
		return h.HelpText()
	}
	return ""
}
