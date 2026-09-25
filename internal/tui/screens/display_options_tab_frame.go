package screens

import (
	"strings"

	"DockSTARTer2/internal/displayengine"

	"charm.land/bubbles/v2/key"
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

func (f *tabFrameSection) ViewString() string {
	ctx := displayengine.GetActiveContext()
	body := f.inner.ViewString()
	contentWidth := max(f.width-2, 1)
	focused := f.frame.focused != nil && f.frame.focused()
	title := ""
	if f.top {
		_, avail := f.frame.strip.TitlePlacement(contentWidth, ctx.SubmenuTitleAlign, nil, ctx)
		title = f.frame.strip.Render(avail, focused, ctx)
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
		x, avail := f.frame.strip.TitlePlacement(max(f.width-2, 1), ctx.SubmenuTitleAlign, nil, ctx)
		regions = append(regions, f.frame.strip.HitRegions(offsetX+x, offsetY, avail, displayengine.ZDialog+10, ctx, nil)...)
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

// tabStripSection is a tab frame's top edge as its own section and Tab stop:
// it draws the frame's top border with the tab strip, and while focused
// Left/Right move to the previous/next tab. A click on a tab does the same.
// onSwitch is called with the tab to show.
type tabStripSection struct {
	frame    *tabFrame
	width    int
	focused  bool
	onSwitch func(tab int) tea.Cmd
}

var _ displayengine.Content = (*tabStripSection)(nil)

func newTabStripSection(frame *tabFrame, onSwitch func(tab int) tea.Cmd) *tabStripSection {
	s := &tabStripSection{frame: frame, onSwitch: onSwitch}
	frame.strip.ActiveFocus = func() bool { return s.focused }
	return s
}

func (s *tabStripSection) SectionHeight(int) int                { return 1 }
func (s *tabStripSection) SectionNaturalWidth(maxWidth int) int { return maxWidth }
func (s *tabStripSection) SetSize(width, _ int)                 { s.width = width }
func (s *tabStripSection) Height() int                          { return 1 }

func (s *tabStripSection) ViewString() string {
	ctx := displayengine.GetActiveContext()
	contentWidth := max(s.width-2, 1)
	focused := s.focused || (s.frame.focused != nil && s.frame.focused())
	_, avail := s.frame.strip.TitlePlacement(contentWidth, ctx.SubmenuTitleAlign, nil, ctx)
	box := displayengine.RenderBorderedBoxCtx(s.frame.strip.Render(avail, focused, ctx), "", contentWidth, 2,
		focused, false, true, ctx.SubmenuTitleAlign, "RAW", ctx)
	top, _, _ := strings.Cut(box, "\n")
	return top
}

func (s *tabStripSection) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	ctx := displayengine.GetActiveContext()
	x, avail := s.frame.strip.TitlePlacement(max(s.width-2, 1), ctx.SubmenuTitleAlign, nil, ctx)
	return s.frame.strip.HitRegions(offsetX+x, offsetY, avail, displayengine.ZDialog+10, ctx, nil)
}

func (s *tabStripSection) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	strip := s.frame.strip
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		n := len(strip.Labels)
		switch {
		case key.Matches(msg, displayengine.Keys.Left) && n > 0:
			return s, s.onSwitch((strip.Active - 1 + n) % n)
		case key.Matches(msg, displayengine.Keys.Right) && n > 0:
			return s, s.onSwitch((strip.Active + 1) % n)
		}
	case displayengine.LayerHitMsg:
		if i, ok := strip.TabFromID(msg.ID); ok {
			return s, s.onSwitch(i)
		}
		if d, ok := strip.ScrollFromID(msg.ID); ok {
			strip.ScrollBy(d)
		}
	}
	return s, nil
}

func (s *tabStripSection) View() tea.View                { return tea.View{Content: s.ViewString()} }
func (s *tabStripSection) Init() tea.Cmd                 { return nil }
func (s *tabStripSection) SetSubFocused(v bool) tea.Cmd  { s.focused = v; return nil }
func (s *tabStripSection) SetIsDialog(bool)              {}
func (s *tabStripSection) SetLockedByOthers(bool)        {}
func (s *tabStripSection) IsVariableHeight() bool        { return false }
func (s *tabStripSection) ID() string                    { return s.frame.strip.ID }
func (s *tabStripSection) ScrollID() string              { return "" }
func (s *tabStripSection) WantsHorizontalKeys() bool     { return true }
func (s *tabStripSection) WantsAllMessages() bool        { return false }
func (s *tabStripSection) Focusable() bool               { return true }
func (s *tabStripSection) AbsorbMessage(tea.Msg) tea.Cmd { return nil }
func (s *tabStripSection) IsProcessing() bool            { return false }
func (s *tabStripSection) MatchesID(msgID string) bool   { return s.frame.strip.OwnsID(msgID) }
