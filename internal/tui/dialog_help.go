package tui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/help"
	keybind "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"DockSTARTer2/internal/displayengine"
)

// subKeyMap implements help.KeyMap with a subset of FullHelp columns for per-page rendering.
type subKeyMap struct{ cols [][]keybind.Binding }

func (s subKeyMap) ShortHelp() []keybind.Binding  { return nil }
func (s subKeyMap) FullHelp() [][]keybind.Binding { return s.cols }

// splitTallColumns splits each column taller than the first into columns of
// the first's height, so a screen's long list of extra bindings packs into
// pages side by side instead of one tall column.
func splitTallColumns(cols [][]keybind.Binding) [][]keybind.Binding {
	if len(cols) == 0 || len(cols[0]) == 0 {
		return cols
	}
	rows := len(cols[0])
	var out [][]keybind.Binding
	for _, col := range cols {
		for len(col) > rows {
			out = append(out, col[:rows])
			col = col[rows:]
		}
		out = append(out, col)
	}
	return out
}

// buildBindingPages greedily packs FullHelp columns into pages that each fit within maxW.
func buildBindingPages(h help.Model, allCols [][]keybind.Binding, maxW int) []subKeyMap {
	if len(allCols) == 0 {
		return nil
	}
	var pages []subKeyMap
	remaining := allCols
	for len(remaining) > 0 {
		fit := 0
		for n := 1; n <= len(remaining); n++ {
			h.SetWidth(9999) // no limit for measurement
			rendered := h.View(subKeyMap{cols: remaining[:n]})
			tooWide := false
			for _, line := range strings.Split(rendered, "\n") {
				if lipgloss.Width(line)+1 > maxW {
					tooWide = true
					break
				}
			}
			if tooWide && n == 1 {
				fit = 1 // single column too wide: include anyway
				break
			}
			if tooWide {
				break
			}
			fit = n
		}
		if fit == 0 {
			fit = 1
		}
		pages = append(pages, subKeyMap{cols: remaining[:fit]})
		remaining = remaining[fit:]
	}
	return pages
}

// displayengine.HelpContext contains both page-level and item-level help information.
// It integrates with AppModel via displayengine.ShowDialogMsg/displayengine.CloseDialogMsg.
type HelpDialogModel struct {
	help   help.Model
	width  int
	height int

	focused bool // tracks global focus

	keyMap            help.KeyMap
	contextInfo       displayengine.HelpContext // structured help info
	contextOffset     int                       // scroll offset for item context text
	graphicsSupported bool                      // set by the caller, not read here; the doc images are gated by images
	connType          string                    // the session's connType, for per-connType settings
	images            bool                      // the session's terminal can show Kitty placeholder images

	// Paged mode: cycles through context page and/or multiple binding column pages.
	paged        bool
	contextPaged bool // true when item context overflows and occupies its own page 0
	page         int
	numPages     int // total pages; set each ViewString call, used by Update

	// Unified layout (deterministic sizing)
	layout displayengine.DialogLayout

	// Markdown cache
	renderedMarkdown      string
	renderedMarkdownWidth int
	docImagesFor          int // column width the image render was last requested for

	// displayengine.Scrollbar component
	Scroll displayengine.Scrollbar

	// Geometry cache for hit regions (set by ViewString)
	lastDocBoxX int
	lastDocBoxY int
	lastDocBoxW int
	lastDocBoxH int
}

func NewHelpDialogModel() *HelpDialogModel {
	return NewHelpDialogModelWithMap(displayengine.Keys)
}

func NewHelpDialogModelWithMap(km help.KeyMap) *HelpDialogModel {
	return NewHelpDialogWithContext(km, displayengine.HelpContext{}, false)
}

// NewHelpDialogWithContext creates a help dialog that shows contextInfo
// (e.g. current variable info) above the standard key bindings.
// Pass an empty displayengine.HelpContext to show only the key bindings.
// graphicsSupported is stored but not read (the doc images are gated by
// SetImages); the caller resolves it from AppModel.graphicsSupported /
// connType == "web" (see showHelpCmd).
func NewHelpDialogWithContext(km help.KeyMap, info displayengine.HelpContext, graphicsSupported bool) *HelpDialogModel {
	h := help.New()
	h.ShowAll = true
	return &HelpDialogModel{help: h, focused: true, keyMap: km, contextInfo: info, graphicsSupported: graphicsSupported, Scroll: displayengine.Scrollbar{ID: "help-dialog"}}
}

// SetConnType records the owning session's connType (see AppModel.connType).
func (m *HelpDialogModel) SetConnType(connType string) { m.connType = connType }

// SetImages records whether the session's terminal can show Kitty placeholder
// images (see AppModel.kittyImages).
func (m *HelpDialogModel) SetImages(images bool) { m.images = images }

func (m *HelpDialogModel) Init() tea.Cmd { return m.ensureDocImages() }

// SetSize updates the dialog dimensions (called by AppModel on window resize).
func (m *HelpDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.calculateLayout()
}

func (m *HelpDialogModel) SetFocused(f bool) {
	m.focused = f
}

// HasHalo implements HaloProvider
func (m *HelpDialogModel) HasHalo() bool {
	return true
}

// HaloColor implements HaloProvider
func (m *HelpDialogModel) HaloColor() color.Color {
	return helpHaloColor()
}

// helpHaloColor is the colour of the help dialog's halo and rim: the theme's
// shadow colour, so that, like every themed colour, it goes through the tints
// (a raw palette index bypasses them, and shows as a different black).
func helpHaloColor() color.Color {
	if c := displayengine.GetActiveContext().ShadowColor; c != nil && c != (lipgloss.NoColor{}) {
		return c
	}
	return lipgloss.Color("0")
}

// docTextWidth is the column width the doc page's markdown is wrapped at,
// exactly matching ViewString: the doc page fills the full usable width
// (ViewString's targetWidth), less one column for the scrollbar.
func (m *HelpDialogModel) docTextWidth() int {
	availW, _ := displayengine.GetAvailableDialogSize(m.width, m.height, true)
	if availW < 30 {
		availW = 30
	}
	targetWidth := availW
	if targetWidth < 20 {
		targetWidth = 20
	}
	return targetWidth - 1
}

func (m *HelpDialogModel) docInfo() (total int, visible int) {
	if m.contextInfo.DocMarkdown == "" {
		return 0, 0
	}

	_, availH := displayengine.GetAvailableDialogSize(m.width, m.height, true)
	// Use rendered lines (wrapped) for accurate scrollbar math.
	docLines := m.getDocLines(m.docTextWidth())
	total = len(docLines)

	// Available height for the doc box matching ViewString: availH - halo/outer border overhead (4)
	maxDocBoxH := availH - 4
	if maxDocBoxH < 5 {
		maxDocBoxH = 5
	}
	visible = total
	if visible > maxDocBoxH-2 { // -2 for top/bottom borders of the doc box itself
		visible = maxDocBoxH - 2
	}
	if visible < 1 {
		visible = 1
	}
	return total, visible
}

func (m *HelpDialogModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// Overhead for Help: Halo (2) + Bordered Dialog (2) = 4
	overhead := 4

	m.layout = displayengine.DialogLayout{
		Width:    0, // content-driven
		Height:   0, // content-driven
		Overhead: overhead,
	}
}
