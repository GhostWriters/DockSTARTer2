package screens

import (
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui/glyphs"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Layout constants for the settings/preview pairing -- single definition
// used by appearanceLayoutRow's width and collapse decisions.
const (
	displayPreviewMinWidth = 50 // minimum total width (settings + gutter + preview) to show the preview
	displayMinMenuWidth    = 40 // minimum settings column content width
	collapsedPreviewWidth  = 1  // width of the manually-hidden preview's expand-indicator column
)

// settingsPanelBGID/previewPanelBGID identify the row's own catch-all
// background regions (see GetHitRegions) -- clicking blank space inside
// either panel (not on a specific item) still switches which side is active.
// appearanceExpandPreviewID is the collapsed placeholder's own single cell
// (see collapsedPreviewView) -- click, or Ctrl+Right from the settings side,
// to restore the preview.
const (
	settingsPanelBGID         = "appearance_settings_panelbg"
	previewPanelBGID          = "appearance_preview_panelbg"
	appearanceExpandPreviewID = "appearance_expand_preview"
)

// expandPreviewGlyph renders the styled collapsed-preview indicator --
// shares the Refresh icon's theme colors rather than defining a new tag.
func expandPreviewGlyph() string {
	ctx := displayengine.GetActiveContext()
	glyph := glyphs.SubMenuCollapsed
	if !ctx.LineCharacters {
		glyph = glyphs.SubMenuCollapsedAscii
	}
	return displayengine.SemanticRawStyle("RefreshIconInactive").Render(glyph)
}

// appearanceLayoutRow pairs the settings column with the preview section.
// Unlike the generic ContentRow (even split, always shows every child), it
// gives the preview section a fixed width and hides it entirely when there
// isn't room -- same collapse behavior the screen's old hand-rolled
// composition had -- and gives both columns the same final height (the max
// of what each naturally needs), relying on each column's own expandable
// child (themeMenu on the settings side, the preview mockup's backdrop
// section) to absorb the difference, rather than centering them
// independently at different heights.
type appearanceLayoutRow struct {
	settings *displayengine.ContentColumn
	preview  *displayengine.MenuModel

	width, height int
	previewFits   bool
	settingsWidth int
	gutterWidth   int
	subFocus      int
	isDialog      bool // mirrors MenuModel's own baseZ decision -- see GetHitRegions

	// previewHidden is the user's manual close/expand choice, layered on top
	// of the width-driven auto-collapse computeWidths already does -- see
	// SetSize, which forces previewFits false whenever this is set,
	// reusing every existing collapse code path for free.
	previewHidden bool

	// showCollapsed reports whether the third (preview) content area is
	// currently rendered as a narrow expand-indicator column rather than
	// fully hidden -- true only when previewHidden and there's still room
	// for it (settingsWidth + gutter + 1). Set in SetSize. Kept separate
	// from previewHidden so a too-narrow width degrades to plain
	// settings-only (matching the pre-existing width auto-collapse) instead
	// of clipping the indicator column.
	showCollapsed bool
}

// appearancePreviewHideMsg is dispatched by the preview panel's own Close
// widget (see buildPreviewSection) to hide it -- intercepted directly in
// appearanceLayoutRow.Update rather than forwarded into the preview's own
// MenuModel, which has no idea the row can hide it.
type appearancePreviewHideMsg struct{}

var _ displayengine.Content = (*appearanceLayoutRow)(nil)

// Init satisfies tea.Model; appearanceLayoutRow has no independent init
// behavior of its own.
func (r *appearanceLayoutRow) Init() tea.Cmd { return nil }

// View satisfies tea.Model.
func (r *appearanceLayoutRow) View() tea.View { return tea.View{Content: r.ViewString()} }

func newAppearanceLayoutRow(settings *displayengine.ContentColumn, preview *displayengine.MenuModel) *appearanceLayoutRow {
	return &appearanceLayoutRow{settings: settings, preview: preview}
}

// computeWidths decides whether the preview fits and, if so, how much width
// the settings column gets after reserving the gutter and the preview's own
// fixed width.
func (r *appearanceLayoutRow) computeWidths(width int) (fits bool, settingsWidth, gutterWidth int) {
	layout := displayengine.GetLayout()
	// Plain 1-char gutter, not VisualGutter's shadow-inclusive version -- the
	// shadow allowance was for two independently-shadowed dialogs sitting
	// side by side; this gutter is an internal seam within one dialog now,
	// with no shadow of its own to clear.
	gutterWidth = layout.GutterWidth
	previewWidth := previewSectionWidth()
	fits = width >= displayMinMenuWidth+gutterWidth+previewWidth
	if fits {
		settingsWidth = width - gutterWidth - previewWidth
	} else {
		settingsWidth = width
	}
	if settingsWidth < displayMinMenuWidth {
		settingsWidth = displayMinMenuWidth
	}
	return
}

// resolveLayout is the single source of truth for how a given outer width
// splits into settings/gutter/preview, covering both the width-driven
// auto-collapse (computeWidths) and the previewHidden override -- SetSize
// and SectionHeight both call this so they can never compute a different
// settingsWidth for the same inputs, which would measure the outer dialog's
// reserved height against a different -- more or less wrapped -- settings
// layout than what SetSize actually renders.
func (r *appearanceLayoutRow) resolveLayout(width int) (fits, showCollapsed bool, settingsWidth, gutterWidth int) {
	fits, settingsWidth, gutterWidth = r.computeWidths(width)
	if !r.previewHidden {
		return fits, false, settingsWidth, gutterWidth
	}
	fits = false
	// Three areas always: [settings][gutter][preview]. When manually hidden,
	// preview's own width drops to zero rather than the gutter being removed
	// -- the gutter is the control's home (see ViewString/GetHitRegions), not
	// a fourth reserved column, so settings only gives up the preview's own
	// width, never the gutter's.
	if width >= displayMinMenuWidth+gutterWidth {
		settingsWidth = width - gutterWidth
		showCollapsed = true
	} else {
		settingsWidth = width
	}
	if settingsWidth < displayMinMenuWidth {
		settingsWidth = displayMinMenuWidth
	}
	return fits, showCollapsed, settingsWidth, gutterWidth
}

// SectionHeight returns the natural (unstretched) total height: the
// settings column's own natural height, or the preview's if taller, it fits
// at the given width, and it isn't manually hidden -- a hidden preview's
// natural height must never factor in here, or the row (and everything
// sized from it, including the outer dialog's own height) keeps reserving
// space for a panel that isn't being drawn.
func (r *appearanceLayoutRow) SectionHeight(width int) int {
	fits, _, settingsWidth, _ := r.resolveLayout(width)
	total := r.settings.SectionHeight(settingsWidth)
	if fits && !r.previewHidden {
		if pv := previewNaturalHeight(r.preview); pv > total {
			total = pv
		}
	}
	return total
}

// SectionNaturalWidth claims the full width offered -- the row always wants
// to fill available space so it can decide for itself (via computeWidths)
// whether the preview fits.
func (r *appearanceLayoutRow) SectionNaturalWidth(maxWidth int) int {
	return maxWidth
}

// SetSize decides the settings/preview split and collapse for width, then
// gives both columns the same final height so their own expandable children
// (themeMenu / the preview's backdrop section) absorb the difference.
func (r *appearanceLayoutRow) SetSize(width, height int) {
	r.width, r.height = width, height
	fits, showCollapsed, settingsWidth, gutterWidth := r.resolveLayout(width)
	r.showCollapsed = showCollapsed
	r.previewFits = fits
	r.settingsWidth = settingsWidth
	r.gutterWidth = gutterWidth

	total := r.settings.SectionHeight(settingsWidth)
	if fits {
		if pv := previewNaturalHeight(r.preview); pv > total {
			total = pv
		}
	}
	// Use the full given height whenever there is one -- not just capped
	// down to whichever side's natural size is larger. settings' own
	// ContentColumn distributes fixed vs. expandable children given
	// whatever height it receives (see ContentColumn.SetSize), so passing
	// through the natural-only total here would starve its expandable
	// child (the theme list) of any extra room beyond bare minimum, even
	// with plenty of unused space available.
	if height > 0 {
		total = height
	}

	r.settings.SetSize(settingsWidth, total)
	if fits {
		r.preview.SetSize(previewSectionWidth(), total)
	}
}

// gutterView renders the blank gutter column, or -- when the preview is
// manually collapsed -- the gutter with the expand glyph on its own first
// row instead of blank. Same column that already separates settings from a
// visible preview, just repurposed rather than adding a new one.
func (r *appearanceLayoutRow) gutterView(height int, showGlyph bool) string {
	if height <= 0 || r.gutterWidth <= 0 {
		return ""
	}
	styles := displayengine.GetStyles()
	blank := lipgloss.NewStyle().Background(styles.Dialog.GetBackground()).Width(r.gutterWidth).Render("")
	lines := make([]string, height)
	for i := range lines {
		lines[i] = blank
	}
	if showGlyph {
		lines[0] = expandPreviewGlyph()
	}
	out := lines[0]
	for _, l := range lines[1:] {
		out += "\n" + l
	}
	return out
}

// ViewString renders the settings column and joins the gutter beside it --
// with the expand glyph on the gutter's first row if manually collapsed,
// blank otherwise -- then the full preview section if it fits.
func (r *appearanceLayoutRow) ViewString() string {
	settingsView := r.settings.ViewString()
	if !r.previewFits && !r.showCollapsed {
		return settingsView
	}

	settingsHeight := lipgloss.Height(settingsView)
	if r.showCollapsed {
		return lipgloss.JoinHorizontal(lipgloss.Top, settingsView, r.gutterView(settingsHeight, true))
	}

	previewView := r.preview.ViewString()
	previewHeight := lipgloss.Height(previewView)
	gutterHeight := settingsHeight
	if previewHeight > gutterHeight {
		gutterHeight = previewHeight
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, settingsView, r.gutterView(gutterHeight, false), previewView)
}

// GetHitRegions offsets the preview's regions by the settings column's
// rendered width plus the gutter. Each side also gets a low-Z catch-all
// background region covering its full rect. model_view.go sorts all hit
// regions by ZOrder before FindHit's reverse scan, so this must sit below
// the *actual* Z every nested item uses -- mirroring MenuModel's own
// ZScreen-vs-ZDialog baseZ decision (menu_hit_regions.go) rather than a
// hardcoded value, since nested content can end up on either the ZScreen or
// ZDialog tier and a hardcoded catch-all Z could then outrank real items.
func (r *appearanceLayoutRow) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	baseZ := displayengine.ZScreen
	if r.isDialog {
		baseZ = displayengine.ZDialog
	}
	panelBGZ := baseZ - 2

	settingsView := r.settings.ViewString()
	regions := []displayengine.HitRegion{{
		ID:     settingsPanelBGID,
		X:      offsetX,
		Y:      offsetY,
		Width:  r.settingsWidth,
		Height: lipgloss.Height(settingsView),
		ZOrder: panelBGZ,
	}}
	regions = append(regions, r.settings.GetHitRegions(offsetX, offsetY)...)
	if r.showCollapsed {
		// The gutter column itself, same one that separates settings from a
		// visible preview -- its first cell is the glyph (see gutterView).
		gutterX := offsetX + lipgloss.Width(settingsView)
		return append(regions, displayengine.HitRegion{
			ID:     appearanceExpandPreviewID,
			X:      gutterX,
			Y:      offsetY,
			Width:  r.gutterWidth,
			Height: 1,
			ZOrder: panelBGZ + 1,
			Label:  "Show Preview",
			Help: &displayengine.HelpContext{
				ScreenName: "Appearance Settings",
				PageTitle:  "Show Preview",
				PageText:   "Restore the hidden preview panel.",
			},
		})
	}
	if !r.previewFits {
		return regions
	}
	previewX := offsetX + lipgloss.Width(settingsView) + r.gutterWidth
	regions = append(regions, displayengine.HitRegion{
		ID:     previewPanelBGID,
		X:      previewX,
		Y:      offsetY,
		Width:  previewSectionWidth(),
		Height: lipgloss.Height(r.preview.ViewString()),
		ZOrder: panelBGZ,
	})
	return append(regions, r.preview.GetHitRegions(previewX, offsetY)...)
}

// SubFocusable implementation, scoped to whichever group (settings or
// preview) Ctrl/Alt+Left/Right last switched to (r.subFocus) -- Tab still
// cycles within the active group only (settings' own Load Theme Defaults /
// Select Theme / Options stops, or preview's single scrollable stop), while
// Ctrl/Alt+Left/Right (handled in Update below) is the coarser control that
// switches which group Tab operates on.
func (r *appearanceLayoutRow) NumTabStops() int {
	if r.subFocus == 1 {
		return 1
	}
	return r.settings.NumTabStops()
}
func (r *appearanceLayoutRow) SubFocusIndex() int {
	if r.subFocus == 1 {
		return 0
	}
	return r.settings.SubFocusIndex()
}
func (r *appearanceLayoutRow) SetSubFocusIndex(i int) {
	if r.subFocus == 1 {
		return
	}
	r.settings.SetSubFocusIndex(i)
}
func (r *appearanceLayoutRow) NextFocusableSub(from int) (int, bool) {
	if r.subFocus == 1 {
		return -1, false
	}
	return r.settings.NextFocusableSub(from)
}
func (r *appearanceLayoutRow) PrevFocusableSub(from int) (int, bool) {
	if r.subFocus == 1 {
		return -1, false
	}
	return r.settings.PrevFocusableSub(from)
}
func (r *appearanceLayoutRow) Items() []displayengine.Content {
	if r.subFocus == 1 {
		return []displayengine.Content{r.preview}
	}
	return r.settings.Items()
}

// canFocusPreview reports whether the preview is currently a valid target to
// switch focus to -- it isn't when collapsed (no room, or manually hidden).
func (r *appearanceLayoutRow) canFocusPreview() bool {
	return r.previewFits && r.preview.Focusable()
}

// SetPreviewHidden toggles the user's manual close/expand choice and
// re-lays-out immediately (reusing r.width/r.height from the last SetSize
// call -- no external resize trigger needed). Focus always stays on
// settings after either direction -- showing it (via the expand control or
// Ctrl+Right) is a separate step from moving focus into it, so a second
// Ctrl+Right (handled by the EnvNextTab case in Update, once previewHidden
// is already false) is what actually switches focus there.
func (r *appearanceLayoutRow) SetPreviewHidden(hidden bool) tea.Cmd {
	if hidden == r.previewHidden {
		return nil
	}
	r.previewHidden = hidden
	r.subFocus = 0
	r.SetSize(r.width, r.height)
	return r.SetSubFocused(true)
}

// Update handles Ctrl/Alt+Left/Right itself (switching which group --
// settings or preview -- holds row-internal focus), retargets subFocus to
// whichever child a mouse hit/wheel message's ID actually belongs to
// (mirroring ContentRow.Update -- without this, hovering/wheeling over
// preview while settings last had subFocus keeps routing to settings,
// ignoring where the mouse actually is), then routes anything else to
// whichever group is currently active.
func (r *appearanceLayoutRow) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(appearancePreviewHideMsg); ok {
		return r, r.SetPreviewHidden(true)
	}
	if hitMsg, ok := msg.(displayengine.LayerHitMsg); ok && hitMsg.ID == appearanceExpandPreviewID && hitMsg.Button == tea.MouseLeft {
		return r, r.SetPreviewHidden(false)
	}
	// The Close widget schedules a WidgetClearPressMsg tick (see PressWidget)
	// to clear its own press-flash state a moment after the click -- but that
	// click also hides the panel, and the general routing below only reaches
	// r.preview while canFocusPreview() is true (i.e. previewFits, which
	// SetPreviewHidden just forced false). Without this, the tick never
	// reaches mockupMenu while hidden, so tbPressed/tbFocused stay stuck and
	// the close icon still shows focused/pressed whenever the panel is shown
	// again. Forward it unconditionally, regardless of fit/hide state.
	if _, ok := msg.(displayengine.WidgetClearPressMsg); ok {
		updated, cmd := r.preview.Update(msg)
		if p, ok := updated.(*displayengine.MenuModel); ok {
			r.preview = p
		}
		return r, cmd
	}
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(kp, displayengine.Keys.EnvNextTab) {
			if r.subFocus == 0 {
				if r.previewHidden {
					return r, r.SetPreviewHidden(false)
				}
				if r.canFocusPreview() {
					r.subFocus = 1
					return r, r.SetSubFocused(true)
				}
			}
			return r, nil
		}
		if key.Matches(kp, displayengine.Keys.EnvPrevTab) {
			if r.subFocus == 1 {
				r.subFocus = 0
				return r, r.SetSubFocused(true)
			}
			if r.previewFits && !r.previewHidden {
				return r, r.SetPreviewHidden(true)
			}
			return r, nil
		}
	}
	// Whether this hit/wheel resolves subFocus to a specific side (background
	// catch-all included) is tracked separately from "forward into the
	// background-matched child" -- the panel background catch-all itself has
	// no child item to forward a click into, but subFocus still must resolve
	// and get a SetSubFocused call.
	var resolved, isPanelBG bool
	switch m := msg.(type) {
	case displayengine.LayerHitMsg:
		switch {
		case m.ID == previewPanelBGID && r.previewFits:
			r.subFocus, resolved, isPanelBG = 1, true, true
		case m.ID == settingsPanelBGID:
			r.subFocus, resolved, isPanelBG = 0, true, true
		case r.previewFits && r.preview.MatchesID(m.ID):
			r.subFocus, resolved = 1, true
		case r.settings.MatchesID(m.ID):
			r.subFocus, resolved = 0, true
		}
	case displayengine.LayerWheelMsg:
		if r.previewFits && r.preview.MatchesID(m.ID) {
			r.subFocus, resolved = 1, true
		} else if r.settings.MatchesID(m.ID) {
			r.subFocus, resolved = 0, true
		}
	}
	// A resolved side must be re-focused here, after subFocus is finalized --
	// the outer dialog's own updateSectionFocus (which also calls
	// SetSubFocused) runs before this Update, still using whatever subFocus
	// was in effect before this click, so a click that switches sides would
	// otherwise leave the stale side highlighted until some later interaction.
	var focusCmd tea.Cmd
	if resolved {
		focusCmd = r.SetSubFocused(true)
	}
	if isPanelBG {
		return r, focusCmd
	}
	if r.subFocus == 1 && r.canFocusPreview() {
		updated, cmd := r.preview.Update(msg)
		if p, ok := updated.(*displayengine.MenuModel); ok {
			r.preview = p
		}
		return r, tea.Batch(focusCmd, cmd)
	}
	updated, cmd := r.settings.Update(msg)
	if s, ok := updated.(*displayengine.ContentColumn); ok {
		r.settings = s
	}
	return r, tea.Batch(focusCmd, cmd)
}

// SetSubFocused propagates focus to whichever column holds row-internal
// focus, unfocusing the other.
func (r *appearanceLayoutRow) SetSubFocused(focused bool) tea.Cmd {
	if r.subFocus == 1 && r.canFocusPreview() {
		r.settings.SetSubFocused(false)
		return r.preview.SetSubFocused(focused)
	}
	r.preview.SetSubFocused(false)
	return r.settings.SetSubFocused(focused)
}

func (r *appearanceLayoutRow) SetIsDialog(isDialog bool) {
	r.isDialog = isDialog
	r.settings.SetIsDialog(isDialog)
	r.preview.SetIsDialog(isDialog)
}

func (r *appearanceLayoutRow) SetLockedByOthers(locked bool) {
	r.settings.SetLockedByOthers(locked)
	r.preview.SetLockedByOthers(locked)
}

// IsVariableHeight reports true -- the settings side's own themeMenu section
// is expandable, so the row (which delegates height distribution down to it)
// is too.
func (r *appearanceLayoutRow) IsVariableHeight() bool { return true }

func (r *appearanceLayoutRow) Height() int {
	if !r.previewFits || r.settings.Height() > r.preview.Height() {
		return r.settings.Height()
	}
	return r.preview.Height()
}

func (r *appearanceLayoutRow) ID() string {
	return "row-" + r.settings.ID() + "-" + r.preview.ID()
}

// ScrollID returns "" -- neither column has a single scrollbar of its own
// today (the preview is a static mockup; settings' scrollable child, the
// theme list, owns its own scrollbar independently).
func (r *appearanceLayoutRow) ScrollID() string { return "" }

func (r *appearanceLayoutRow) MatchesID(msgID string) bool {
	if msgID == settingsPanelBGID || (r.previewFits && msgID == previewPanelBGID) ||
		(r.showCollapsed && msgID == appearanceExpandPreviewID) {
		return true
	}
	return r.settings.MatchesID(msgID) || (r.previewFits && r.preview.MatchesID(msgID))
}

func (r *appearanceLayoutRow) AbsorbMessage(msg tea.Msg) tea.Cmd {
	if cmd := r.settings.AbsorbMessage(msg); cmd != nil {
		return cmd
	}
	if r.previewFits {
		return r.preview.AbsorbMessage(msg)
	}
	return nil
}

func (r *appearanceLayoutRow) IsProcessing() bool {
	return r.settings.IsProcessing() || (r.previewFits && r.preview.IsProcessing())
}

// WantsHorizontalKeys always reports false -- neither column consumes
// Left/Right itself today (unlike a sinput text field).
func (r *appearanceLayoutRow) WantsHorizontalKeys() bool { return false }

func (r *appearanceLayoutRow) WantsAllMessages() bool {
	if r.subFocus == 1 {
		return r.preview.Focusable() // placeholder until Preview truly participates in focus
	}
	return false
}

// Focusable reports true -- the settings column always has focusable
// content.
func (r *appearanceLayoutRow) Focusable() bool { return true }

// ComfortableMinHeight is the row's own floor for the theme list (mirroring
// ContentColumn's minExpandableFloor) plus room for Load Theme Defaults and
// a handful of Options rows before Options needs to start scrolling --
// below this, the outer dialog flattens its button row (see
// classic.ComfortableMinHeight) rather than squeezing this row further.
func (r *appearanceLayoutRow) ComfortableMinHeight() int {
	const themeListFloor = 8 // mirrors ContentColumn's minExpandableFloor
	const loadDefaultsNatural = 3
	const optionsComfortable = 6
	return themeListFloor + loadDefaultsNatural + optionsComfortable
}

var _ displayengine.ComfortableMinHeight = (*appearanceLayoutRow)(nil)

// SuppressRightMargin reports true only while the gutter is showing the
// collapsed expand glyph -- the dialog's standard right-side content margin
// is reclaimed as that glyph's column then (see menu_render.go's
// RightMarginSuppressor). Must stay false whenever the preview is actually
// visible: that margin still belongs to the real preview panel's own right
// edge in that case, same as every other dialog.
func (r *appearanceLayoutRow) SuppressRightMargin() bool { return r.showCollapsed }

var _ displayengine.RightMarginSuppressor = (*appearanceLayoutRow)(nil)
