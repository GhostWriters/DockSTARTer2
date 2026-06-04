package tui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/help"
	keybind "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// subKeyMap implements help.KeyMap with a subset of FullHelp columns for per-page rendering.
type subKeyMap struct{ cols [][]keybind.Binding }

func (s subKeyMap) ShortHelp() []keybind.Binding  { return nil }
func (s subKeyMap) FullHelp() [][]keybind.Binding { return s.cols }

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

// HelpContext defines the two contextual help panels.
type HelpContext struct {
	ScreenName string // e.g., "Main Menu" — used in the title bar: "Help: Main Menu"
	PageTitle  string // title for the page context box (e.g. "Description")
	PageText   string // body text for the page context box
	Legend     string // multi-line legend (newline-separated); rendered centered at the bottom of each page in its own "Legend" box
	ItemTitle  string // e.g., variable name or menu item Tag
	ItemText   string

	DocMarkdown string // Markdown documentation content
	DocAppName  string // Name of the application for the documentation
}

// HelpContextProvider is implemented by models that can provide structured help content.
type HelpContextProvider interface {
	HelpContext(maxWidth int) HelpContext
}

// HelpContextWidth returns the content width the help dialog will use for word-wrapping,
// given the current terminal dimensions. Mirrors the calculation in showHelpCmd.
func HelpContextWidth(termW, termH int) int {
	availW, _ := GetAvailableDialogSize(termW, termH, true)
	w := availW - 8
	if w < 30 {
		w = 30
	}
	if w > 120 {
		w = 120
	}
	return w
}

// TitleBarRefreshMsg is dispatched when the [↺] title bar widget is activated.
// Screens that support refresh should handle this message.
type TitleBarRefreshMsg struct{}

// TriggerHelpMsg is a message that tells the app to open the help dialog.
type TriggerHelpMsg struct {
	CapturedContext *HelpContext
	// ScreenLevelOnly strips item-specific fields so help shows screen/page context only.
	// Used when [?] is activated from the title bar widget.
	ScreenLevelOnly bool
}

// HelpContext contains both page-level and item-level help information.
// It integrates with AppModel via ShowDialogMsg/CloseDialogMsg.
type HelpDialogModel struct {
	help   help.Model
	width  int
	height int

	focused bool // tracks global focus

	keyMap        help.KeyMap
	contextInfo   HelpContext // structured help info
	contextOffset int         // scroll offset for item context text

	// Paged mode: cycles through context page and/or multiple binding column pages.
	paged        bool
	contextPaged bool // true when item context overflows and occupies its own page 0
	page         int
	numPages     int // total pages; set each ViewString call, used by Update

	// Unified layout (deterministic sizing)
	layout DialogLayout

	// Markdown cache
	renderedMarkdown      string
	renderedMarkdownWidth int

	// Scrollbar component
	Scroll Scrollbar

	// Geometry cache for hit regions (set by ViewString)
	lastDocBoxX int
	lastDocBoxY int
	lastDocBoxW int
	lastDocBoxH int
}

func NewHelpDialogModel() *HelpDialogModel {
	return NewHelpDialogModelWithMap(Keys)
}

func NewHelpDialogModelWithMap(km help.KeyMap) *HelpDialogModel {
	return NewHelpDialogWithContext(km, HelpContext{})
}

// NewHelpDialogWithContext creates a help dialog that shows contextInfo
// (e.g. current variable info) above the standard key bindings.
// Pass an empty HelpContext to show only the key bindings.
func NewHelpDialogWithContext(km help.KeyMap, info HelpContext) *HelpDialogModel {
	h := help.New()
	h.ShowAll = true
	return &HelpDialogModel{help: h, focused: true, keyMap: km, contextInfo: info, Scroll: Scrollbar{ID: "help-dialog"}}
}

func (m *HelpDialogModel) Init() tea.Cmd { return nil }

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
	return lipgloss.Color("0") // Solid black halo
}

func (m *HelpDialogModel) docInfo() (total int, visible int) {
	if m.contextInfo.DocMarkdown == "" {
		return 0, 0
	}

	// Calculate target width for help content exactly matching ViewString.
	availW, availH := GetAvailableDialogSize(m.width, m.height, true)
	if availW < 30 {
		availW = 30
	}
	targetWidth := availW - 8
	if targetWidth < 20 {
		targetWidth = 20
	}
	if targetWidth > availW {
		targetWidth = availW
	}

	// Use rendered lines (wrapped) for accurate scrollbar math.
	docTextW := targetWidth - 1
	docLines := m.getDocLines(docTextW)
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

	m.layout = DialogLayout{
		Width:    0, // content-driven
		Height:   0, // content-driven
		Overhead: overhead,
	}
}
