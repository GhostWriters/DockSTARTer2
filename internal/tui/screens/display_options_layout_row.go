package screens

import (
	"DockSTARTer2/internal/displayengine"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Layout constants for the settings/preview pairing -- single definition
// used by appearanceLayoutRow's width and collapse decisions.
const (
	displayPreviewMinWidth = 50 // minimum total width (settings + gutter + preview) to show the preview
	displayMinMenuWidth    = 40 // minimum settings column content width
)

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
}

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

// SectionHeight returns the natural (unstretched) total height: the
// settings column's own natural height, or the preview's if taller and it
// fits at the given width.
func (r *appearanceLayoutRow) SectionHeight(width int) int {
	fits, settingsWidth, _ := r.computeWidths(width)
	total := r.settings.SectionHeight(settingsWidth)
	if fits {
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
	fits, settingsWidth, gutterWidth := r.computeWidths(width)
	r.previewFits = fits
	r.settingsWidth = settingsWidth
	r.gutterWidth = gutterWidth

	total := r.settings.SectionHeight(settingsWidth)
	if fits {
		if pv := previewNaturalHeight(r.preview); pv > total {
			total = pv
		}
	}
	// Never claim more than what's actually available.
	if height > 0 && total > height {
		total = height
	}

	r.settings.SetSize(settingsWidth, total)
	if fits {
		r.preview.SetSize(previewSectionWidth(), total)
	}
}

// ViewString joins the settings column and (if it fits) the preview section
// left-to-right, with a blank gutter between them.
func (r *appearanceLayoutRow) ViewString() string {
	settingsView := r.settings.ViewString()
	if !r.previewFits {
		return settingsView
	}
	previewView := r.preview.ViewString()

	settingsHeight := lipgloss.Height(settingsView)
	previewHeight := lipgloss.Height(previewView)
	gutterHeight := settingsHeight
	if previewHeight > gutterHeight {
		gutterHeight = previewHeight
	}
	// Dialog background, not Screen -- the gutter now sits inside the outer
	// Appearance Settings dialog's own interior, not on the raw terminal
	// backdrop the way it did when settings and preview were two separate
	// top-level dialogs.
	styles := displayengine.GetStyles()
	gutterStyle := lipgloss.NewStyle().Background(styles.Dialog.GetBackground())
	gutterStr := gutterStyle.Height(gutterHeight).Width(r.gutterWidth).Render("")

	return lipgloss.JoinHorizontal(lipgloss.Top, settingsView, gutterStr, previewView)
}

// GetHitRegions offsets the preview's regions by the settings column's
// rendered width plus the gutter.
func (r *appearanceLayoutRow) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	regions := r.settings.GetHitRegions(offsetX, offsetY)
	if !r.previewFits {
		return regions
	}
	previewX := offsetX + lipgloss.Width(r.settings.ViewString()) + r.gutterWidth
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
// switch focus to -- it isn't when collapsed (no room for it at all).
func (r *appearanceLayoutRow) canFocusPreview() bool {
	return r.previewFits && r.preview.Focusable()
}

// Update handles Ctrl/Alt+Left/Right itself (switching which group --
// settings or preview -- holds row-internal focus), retargets subFocus to
// whichever child a mouse hit/wheel message's ID actually belongs to
// (mirroring ContentRow.Update -- without this, hovering/wheeling over
// preview while settings last had subFocus keeps routing to settings,
// ignoring where the mouse actually is), then routes anything else to
// whichever group is currently active.
func (r *appearanceLayoutRow) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok && r.canFocusPreview() {
		if key.Matches(kp, displayengine.Keys.EnvNextTab) {
			if r.subFocus == 0 {
				r.subFocus = 1
				return r, r.SetSubFocused(true)
			}
			return r, nil
		}
		if key.Matches(kp, displayengine.Keys.EnvPrevTab) {
			if r.subFocus == 1 {
				r.subFocus = 0
				return r, r.SetSubFocused(true)
			}
			return r, nil
		}
	}
	switch m := msg.(type) {
	case displayengine.LayerHitMsg:
		if r.previewFits && r.preview.MatchesID(m.ID) {
			r.subFocus = 1
		} else if r.settings.MatchesID(m.ID) {
			r.subFocus = 0
		}
	case displayengine.LayerWheelMsg:
		if r.previewFits && r.preview.MatchesID(m.ID) {
			r.subFocus = 1
		} else if r.settings.MatchesID(m.ID) {
			r.subFocus = 0
		}
	}
	if r.subFocus == 1 && r.canFocusPreview() {
		updated, cmd := r.preview.Update(msg)
		if p, ok := updated.(*displayengine.MenuModel); ok {
			r.preview = p
		}
		return r, cmd
	}
	updated, cmd := r.settings.Update(msg)
	if s, ok := updated.(*displayengine.ContentColumn); ok {
		r.settings = s
	}
	return r, cmd
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
	if r.settings.Height() > r.preview.Height() {
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
