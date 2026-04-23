package tui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"regexp"
	// "os"
	"strings"

	"DockSTARTer2/internal/graphics"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/theme"

	"github.com/charmbracelet/x/ansi"
	// "github.com/eliukblau/pixterm/pkg/ansimage"
	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"
	"github.com/pgavlin/goldmark/ast"
	kit_renderer "github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"
	_ "github.com/gen2brain/svg"

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

// TriggerHelpMsg is a message that tells the app to open the help dialog.
type TriggerHelpMsg struct {
	CapturedContext *HelpContext
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

// getRenderedMarkdown renders DocMarkdown via markdown-kit at the given column width,
// caching the result so repeated ViewString calls are cheap.
func (m *HelpDialogModel) getRenderedMarkdown(width int) string {
	if m.contextInfo.DocMarkdown == "" {
		return ""
	}
	if m.renderedMarkdown != "" && m.renderedMarkdownWidth == width {
		return m.renderedMarkdown
	}

	source := []byte(m.contextInfo.DocMarkdown)

	// Use Sixel encoder for high-fidelity web terminal support
	encoder := graphics.SixelGraphicsEncoder()

	// Initialize the terminal-optimized NodeRenderer from markdown-kit
	kitR := kit_renderer.New(
		kit_renderer.WithTheme(styles.GlamourDark),
		kit_renderer.WithWordWrap(width),
		kit_renderer.WithImages(true, width, ""),
		kit_renderer.WithImageEncoder(encoder),
		kit_renderer.WithHyperlinks(true), // Enable hyperlinks for better fallbacks
	)

	// Custom image renderer struct to fix SVG issues
	fixer := &imageFixerRenderer{
		kitR:    kitR,
		encoder: encoder,
	}

	// Create a goldmark renderer and register our terminal NodeRenderer
	mainR := renderer.NewRenderer(renderer.WithNodeRenderers(
		util.Prioritized(kitR, 100),
		util.Prioritized(fixer, 0),
	))

	// Parse the markdown into an AST
	parser := goldmark.DefaultParser()
	doc := parser.Parse(text.NewReader(source))

	var buf bytes.Buffer
	if err := mainR.Render(&buf, source, doc); err != nil {
		m.renderedMarkdown = m.contextInfo.DocMarkdown
		m.renderedMarkdownWidth = width
		return m.renderedMarkdown
	}

	m.renderedMarkdown = strings.TrimRight(buf.String(), "\n")
	m.renderedMarkdownWidth = width
	return m.renderedMarkdown
}

// getDocLines returns the rendered markdown split into individual lines.
func (m *HelpDialogModel) getDocLines(width int) []string {
	raw := m.getRenderedMarkdown(width)
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

// getDocPageIdx returns the page index for the markdown doc page, or -1 if not present.
func (m *HelpDialogModel) getDocPageIdx() int {
	if m.contextInfo.DocMarkdown == "" {
		return -1
	}
	// Doc page is always the last page.
	return m.numPages - 1
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

func (m *HelpDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	totalDocLines, visibleDocRows := m.docInfo()
	if newOff, cmd, changed := m.Scroll.Update(msg, m.contextOffset, totalDocLines, visibleDocRows); changed {
		m.contextOffset = newOff
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Help key (? / F1) cycles pages when paged, otherwise closes.
		if keybind.Matches(msg, Keys.Help) {
			if m.paged {
				n := m.numPages
				if n < 2 {
					n = 2
				}
				m.page = (m.page + 1) % n
				m.contextOffset = 0
				return m, nil
			}
			return m, func() tea.Msg { return CloseDialogMsg{} }
		}
		switch {
		case keybind.Matches(msg, Keys.Up):
			m.contextOffset--
			if m.contextOffset < 0 {
				m.contextOffset = 0
			}
			return m, nil
		case keybind.Matches(msg, Keys.Down):
			m.contextOffset++
			return m, nil
		case keybind.Matches(msg, Keys.PageUp):
			m.contextOffset -= 5
			if m.contextOffset < 0 {
				m.contextOffset = 0
			}
			return m, nil
		case keybind.Matches(msg, Keys.PageDown):
			m.contextOffset += 5
			return m, nil
		case keybind.Matches(msg, Keys.Home):
			m.contextOffset = 0
			return m, nil
		}
		// Any other key closes the help dialog (Esc also works)
		return m, func() tea.Msg { return CloseDialogMsg{} }

	case tea.MouseWheelMsg:
		// On a bindings-only page (context overflowed to its own page), scrolling does nothing.
		if m.paged && m.contextPaged && m.page != 0 {
			return m, nil
		}
		// Logic previously here is now in HandleScrollbarUpdate above.
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.MouseMotionMsg:
		// Logic previously here is now in HandleScrollbarUpdate above.
		return m, nil

	case tea.MouseClickMsg:
		// Logic handled in m.Scroll.Update at top of function.
		return m, nil

	case tea.MouseReleaseMsg:
		// Logic handled in m.Scroll.Update at top of function.
		return m, nil

	case LayerHitMsg:
		// Non-scrollbar click: cycle pages when paged, otherwise close.
		// Only handle this if the click was actually on the help dialog background.
		if msg.ID != "help_dialog" {
			return m, nil
		}
		if m.paged {
			n := m.numPages
			if n < 2 {
				n = 2
			}
			m.page = (m.page + 1) % n
			m.contextOffset = 0
			return m, nil
		}
		return m, func() tea.Msg { return CloseDialogMsg{} }

	}
	return m, nil
}



// ViewString returns the dialog content as a string for compositing
func (m *HelpDialogModel) ViewString() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	hasDocPage := m.contextInfo.DocMarkdown != ""
	isDoc := hasDocPage && m.page == m.getDocPageIdx()

	// Calculate target width for help content.
	// We want a consistent width for the help panels relative to the screen.
	availW, availH := GetAvailableDialogSize(m.width, m.height, true)

	// Ensure at least some minimal room
	if availW < 30 {
		availW = 30
	}
	if availH < 20 {
		availH = 20
	}

	targetWidth := availW - 8
	if isDoc {
		// Documentation page is allowed to fill the full usable width.
		targetWidth = availW
	}

	if targetWidth < 20 {
		targetWidth = 20
	}
	// For non-doc pages, keep a reasonable cap for readability.
	if !isDoc && targetWidth > 120 {
		targetWidth = 120
	}
	m.help.SetWidth(targetWidth)

	dialogStyle := theme.ThemeSemanticStyle("{{|Dialog|}}")
	haloColor := lipgloss.Color("0") // Solid black halo

	// Apply theme styles to the help component
	sepStyle := dialogStyle
	dimStyle := theme.ThemeSemanticStyle("{{|HelpItem|}}")
	keyStyle := theme.ThemeSemanticStyle("{{|HelpTag|}}")

	m.help.Styles.ShortKey = keyStyle
	m.help.Styles.ShortDesc = dimStyle
	m.help.Styles.ShortSeparator = sepStyle
	m.help.Styles.FullKey = keyStyle
	m.help.Styles.FullDesc = dimStyle
	m.help.Styles.FullSeparator = sepStyle
	m.help.Styles.Ellipsis = dimStyle

	bindingLines := strings.Split(m.help.View(m.keyMap), "\n")

	// Compute max line width across both context text and key bindings.
	// +1 accounts for the leading space that will be prepended when building the bindings box content.
	maxLineWidth := 0
	for i, line := range bindingLines {
		trimmed := strings.TrimRight(line, " ")
		bindingLines[i] = trimmed
		if w := lipgloss.Width(trimmed) + 1; w > maxLineWidth {
			maxLineWidth = w
		}
	}

	// Snapping width to even as before
	if (maxLineWidth+2)%2 != 0 {
		maxLineWidth++
	}

	// Build the context section (variable info) if provided.
	var pageTextBox string
	var legendBox string

	hasLegend := m.contextInfo.Legend != ""
	hasPageCtx := m.contextInfo.PageText != ""
	hasItemCtx := m.contextInfo.ItemText != ""

	var pageTextLines []string // word-wrapped PageText lines
	var itemLines []string     // word-wrapped ItemText lines
	var legendLines []string   // resolved Legend lines (multi-line, each centered at bottom)

	wrapWidth := targetWidth - 2
	if wrapWidth < 10 {
		wrapWidth = 10
	}

	if hasPageCtx {
		resolved := theme.ToThemeANSI(m.contextInfo.PageText)
		for _, line := range strings.Split(resolved, "\n") {
			wrapped := ansi.Wordwrap(line, wrapWidth, "")
			for _, wl := range strings.Split(wrapped, "\n") {
				wl = strings.TrimRight(wl, " ")
				paddedLine := " " + wl
				pageTextLines = append(pageTextLines, paddedLine)
				if w := lipgloss.Width(paddedLine); w > maxLineWidth {
					maxLineWidth = w
				}
			}
		}
	}

	if hasItemCtx {
		resolved := theme.ToThemeANSI(m.contextInfo.ItemText)
		for _, line := range strings.Split(resolved, "\n") {
			wrapped := ansi.Wordwrap(line, wrapWidth, "")
			for _, wl := range strings.Split(wrapped, "\n") {
				wl = strings.TrimRight(wl, " ")
				paddedLine := " " + wl
				itemLines = append(itemLines, paddedLine)
				if w := lipgloss.Width(paddedLine); w > maxLineWidth {
					maxLineWidth = w
				}
			}
		}
	}

	if hasLegend {
		for _, line := range strings.Split(m.contextInfo.Legend, "\n") {
			resolved := " " + strings.TrimRight(theme.ToThemeANSI(strings.TrimRight(line, " ")), " ")
			legendLines = append(legendLines, resolved)
			if w := lipgloss.Width(resolved); w > maxLineWidth {
				maxLineWidth = w
			}
		}
	}

	// Cap maxLineWidth at targetWidth and ensure minimum
	if maxLineWidth > targetWidth {
		maxLineWidth = targetWidth
	}
	if maxLineWidth < 20 {
		maxLineWidth = 20
	}

	// Build per-page binding column groups (greedy column packing).
	allCols := m.keyMap.FullHelp()
	bPages := buildBindingPages(m.help, allCols, maxLineWidth)
	m.help.SetWidth(targetWidth) // restore after measurement in buildBindingPages
	if len(bPages) == 0 {
		bPages = []subKeyMap{{cols: allCols}}
	}

	// Determine if context paging is needed: when item context + bindings together would
	// trigger a scrollbar, the context gets its own page 0.
	contextPaged := false
	if len(itemLines) > 0 {
		contextOverhead := 6
		if len(pageTextLines) > 0 {
			contextOverhead += len(pageTextLines) + 2 + 1 // pageText box (border+content) + gap line
		}
		if len(legendLines) > 0 {
			contextOverhead += len(legendLines) + 2 + 1 // legend box (border+content) + gap line
		}
		visibleWithBindings := availH - contextOverhead - (len(bindingLines) + 2) - 2
		if visibleWithBindings < 1 {
			visibleWithBindings = 1
		}
		contextPaged = len(itemLines) > visibleWithBindings
	}

	numPages := len(bPages)
	if contextPaged {
		numPages++ // page 0 = context only; pages 1..N = binding pages
	}
	if hasDocPage {
		numPages++ // last page = markdown documentation
	}
	m.contextPaged = contextPaged
	m.numPages = numPages
	m.paged = numPages > 1

	// Clamp page to valid range.
	if m.page < 0 || !m.paged || m.page >= m.numPages {
		m.page = 0
	}

	// showContext: only shown when NOT on the doc page, and only on context/binding pages.
	showContext := !hasDocPage || m.page != m.getDocPageIdx()
	showContext = showContext && (!m.paged || (contextPaged && m.page == 0) || !contextPaged)
	// showBindings: not shown on the doc page or the context-only page 0.
	showBindings := !hasDocPage || m.page != m.getDocPageIdx()
	showBindings = showBindings && (!m.paged || (!contextPaged || m.page != 0))

	// Which binding page to render.
	bindingPageIdx := m.page
	if contextPaged && m.page > 0 {
		bindingPageIdx = m.page - 1
	}
	if bindingPageIdx >= len(bPages) {
		bindingPageIdx = len(bPages) - 1
	}
	if bindingPageIdx < 0 {
		bindingPageIdx = 0
	}

	ctx := GetActiveContext()

	if showContext && (len(pageTextLines) > 0 || len(itemLines) > 0) {
		// Render Page Context box if content exists
		if len(pageTextLines) > 0 {
			pageTextBox = RenderBorderedBoxCtx(
				m.contextInfo.PageTitle,
				strings.Join(pageTextLines, "\n"),
				maxLineWidth,
				len(pageTextLines)+2,
				false, // focused: false
				true,  // showIndicators: true
				true,
				ctx.SubmenuTitleAlign,
				"TitleSubMenu",
				ctx,
			)
		}
	}

	// 2. Render Context or Documentation box
	// We handle these as mutually exclusive primary content boxes.
	var scrollBox string
	var boxTitle string
	var boxContent string
	var boxTargetW int
	var boxTargetH int
	var boxTotalLines int
	var boxOffset int

	if isDoc {
		boxTitle = "Documentation"
		if m.contextInfo.DocAppName != "" {
			boxTitle = m.contextInfo.DocAppName + " Documentation"
		}
		docTextW := targetWidth - 1
		docLines := m.getDocLines(docTextW)
		boxTotalLines = len(docLines)
		boxTargetW = targetWidth
		// Forced expansion for Docs page: Use the full available height.
		boxTargetH = availH
		if len(legendLines) > 0 {
			// Subtract space for legend: border(2) + content + gap(1)
			boxTargetH -= (len(legendLines) + 2 + 1)
		}
		if boxTargetH < 5 {
			boxTargetH = 5
		}
		
		var displayLines []string
		visibleRows := boxTargetH - 2
		if visibleRows < 1 { visibleRows = 1 }
		
		// Clamp offset
		if m.contextOffset < 0 { m.contextOffset = 0 }
		if boxTotalLines > 0 && m.contextOffset > boxTotalLines-visibleRows {
			m.contextOffset = boxTotalLines - visibleRows
		}
		if m.contextOffset < 0 { m.contextOffset = 0 }
		boxOffset = m.contextOffset

		for i := 0; i < visibleRows && (i+boxOffset) < boxTotalLines; i++ {
			line := docLines[i+boxOffset]
			if lw := lipgloss.Width(line); lw > docTextW {
				line = TruncateRight(line, docTextW)
			} else if lw < docTextW {
				line += strings.Repeat(" ", docTextW-lw)
			}
			displayLines = append(displayLines, line)
		}
		boxContent = strings.Join(displayLines, "\n")
	} else if showContext && len(itemLines) > 0 {
		boxTitle = m.contextInfo.ItemTitle
		if boxTitle == "" {
			boxTitle = "Context Sensitive Help"
		}
		boxTotalLines = len(itemLines)
		boxTargetW = targetWidth
		
		maxCtxH := availH - 4
		if maxCtxH < 5 { maxCtxH = 5 }
		boxTargetH = boxTotalLines + 2
		if boxTargetH < 3 { boxTargetH = 3 }
		if boxTargetH > maxCtxH { boxTargetH = maxCtxH }
		
		visibleRows := boxTargetH - 2
		if visibleRows < 1 { visibleRows = 1 }
		
		// Clamp offset
		if m.contextOffset < 0 { m.contextOffset = 0 }
		if boxTotalLines > 0 && m.contextOffset > boxTotalLines-visibleRows {
			m.contextOffset = boxTotalLines - visibleRows
		}
		if m.contextOffset < 0 { m.contextOffset = 0 }
		boxOffset = m.contextOffset

		var displayLines []string
		for i := 0; i < visibleRows && (i+boxOffset) < boxTotalLines; i++ {
			line := itemLines[i+boxOffset]
			// itemLines are already padded/wrapped in the loop above
			displayLines = append(displayLines, line)
		}
		boxContent = strings.Join(displayLines, "\n")
	}

	if boxContent != "" {
		// Use white-on-black ONLY for the documentation page.
		// Standard context boxes should match the outer dialog theme.
		scrollCtx := ctx
		if isDoc {
			scrollCtx.ContentBackground = lipgloss.NewStyle().
				Background(lipgloss.Color("0")).
				Foreground(lipgloss.Color("15"))
		}

		boxContent = ApplyScrollbar(
			&m.Scroll,
			boxContent,
			boxTotalLines,
			boxTargetH-2,
			boxOffset,
			ctx.LineCharacters,
			scrollCtx,
		)

		scrollBox = RenderBorderedBoxCtx(
			boxTitle,
			boxContent,
			boxTargetW,
			boxTargetH,
			true,
			true,
			true,
			ctx.SubmenuTitleAlign,
			"TitleSubMenuFocused",
			scrollCtx,
		)

		if m.Scroll.Info.Needed {
			scrollPct := 0.0
			visibleRows := boxTargetH - 2
			if boxTotalLines > visibleRows {
				scrollPct = float64(boxOffset) / float64(boxTotalLines-visibleRows)
				if scrollPct > 1.0 { scrollPct = 1.0 }
			}
			boxLines := strings.Split(scrollBox, "\n")
			if len(boxLines) > 0 {
				boxLines[len(boxLines)-1] = BuildScrollPercentBottomBorder(boxTargetW+2, scrollPct, true, ctx)
				scrollBox = strings.Join(boxLines, "\n")
			}
		}
	}

	// Render Legend box — shown at the bottom when a legend is set.
	if len(legendLines) > 0 {
		var centeredLines []string
		for _, ll := range legendLines {
			centeredLines = append(centeredLines, CenterText(ll, targetWidth))
		}
		legendBox = RenderBorderedBoxCtx(
			"Legend",
			strings.Join(centeredLines, "\n"),
			targetWidth,
			len(legendLines)+2,
			false, // focused: false
			true,  // showIndicators: true
			true,
			ctx.SubmenuTitleAlign,
			"TitleSubMenu",
			ctx,
		)
	}

	// Combine sections with "\n" as separator (not suffix) to avoid trailing blank lines.
	var parts []string
	if pageTextBox != "" {
		parts = append(parts, pageTextBox)
	}
	if scrollBox != "" {
		parts = append(parts, scrollBox)
	}
	if showBindings {
		// Render only the columns for the current binding page.
		bpLines := strings.Split(m.help.View(bPages[bindingPageIdx]), "\n")
		var bindingContent []string
		for _, line := range bpLines {
			line = strings.TrimRight(line, " ")
			paddedLine := " " + line
			if w := lipgloss.Width(paddedLine); w < targetWidth {
				paddedLine += strings.Repeat(" ", targetWidth-w)
			}
			bindingContent = append(bindingContent, paddedLine)
		}
		bindingsBox := RenderBorderedBoxCtx(
			"Keyboard & Mouse Controls",
			strings.Join(bindingContent, "\n"),
			targetWidth,
			len(bpLines)+2,
			false, // not interactive
			true,  // showIndicators: true — reserve space to match legend/context title alignment
			true,
			ctx.SubmenuTitleAlign,
			"TitleSubMenu",
			ctx,
		)
		parts = append(parts, bindingsBox)
	}
	if legendBox != "" {
		parts = append(parts, legendBox)
	}
	combinedText := strings.Join(parts, "\n")

	// Calculate relative position of the scrollBox for hit regions.
	layout := GetLayout()
	// All dialogs start inside an outer halo(2) and outer border(1).
	// But ViewString returns the content that will be wrapped by a border and halo later.
	// We calculate relative positions INSIDE the dialog content area.
	// Content is padded by layout.ContentSideMargin (1) inside the outer border.
	relativeX := layout.ContentSideMargin
	relativeY := 0 // Relative to the start of the joined parts
	for _, p := range parts {
		if scrollBox != "" && p == scrollBox {
			m.lastDocBoxX = relativeX
			m.lastDocBoxY = relativeY
			m.lastDocBoxW = lipgloss.Width(p)
			m.lastDocBoxH = lipgloss.Height(p)
		}
		h := lipgloss.Height(p)
		relativeY += h
	}

	content := combinedText
	// Per-line maintenance above is sufficient — no outer wrap needed

	// Ensure the title is visible on the black border bar.
	// Use the original themed Dialog background for the title text area.
	ctx = GetActiveContext()
	ctx.DialogTitleHelp = GetStyles().DialogTitleHelp.
		Background(GetStyles().Dialog.GetBackground()).
		Foreground(GetStyles().DialogTitleHelp.GetForeground())
	ctx.BorderColor = haloColor
	ctx.Border2Color = haloColor

	// We pass raw text so it uses the ctx.DialogTitleHelp base style without tag overrides
	title := "Help"
	if m.contextInfo.ScreenName != "" {
		title = "Help: " + m.contextInfo.ScreenName
	}
	if m.paged {
		title += fmt.Sprintf(" [%d/%d]", m.page+1, m.numPages)
	}
	// Add the title and outer dialog frame.
	// We no longer add the halo here; it is now managed by the central compositor
	// via the HaloProvider interface.
	return RenderUniformBlockDialogCtx(title, content, ctx)
}

// HasHalo implements HaloProvider
func (m *HelpDialogModel) HasHalo() bool {
	return true
}

// HaloColor implements HaloProvider
func (m *HelpDialogModel) HaloColor() color.Color {
	return lipgloss.Color("0") // Solid black halo
}

// View implements tea.Model
func (m *HelpDialogModel) View() tea.View {
	v := tea.NewView(m.ViewString())
	v.MouseMode = tea.MouseModeCellMotion
	v.AltScreen = true
	return v
}

// Layers implements LayeredView
func (m *HelpDialogModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZScreen).ID("Dialog.Help"),
	}
}

// SetSize updates the dialog dimensions (called by AppModel on window resize).
func (m *HelpDialogModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.calculateLayout()
}

func (m *HelpDialogModel) SetFocused(f bool) {
	m.focused = f
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (m *HelpDialogModel) GetHitRegions(offsetX, offsetY int) []HitRegion {
	// Make sure geometry from ViewString is fresh.
	h := lipgloss.Height(m.ViewString())

	var regions []HitRegion

	// Background catch-all for the entire dialog (lowest Z, absorbs unmatched clicks).
	regions = append(regions, HitRegion{
		ID:     "help_dialog",
		X:      offsetX,
		Y:      offsetY,
		Width:  m.width,
		Height: h,
		ZOrder: ZScreen,
		Label:  "Help",
	})
	
	layout := GetLayout()
	// Using offsetX (border) + 1 (border width) + SideMargin (1) = offsetX + 2.
	// This covers the interactive area where text is displayed.
	docBoxX := offsetX + layout.SingleBorder() + m.lastDocBoxX
	docBoxY := offsetY + layout.SingleBorder() + m.lastDocBoxY + layout.SingleBorder()

	// Restore X-axis trial adjustment to resolve reported horizontal drift.
	docBoxX -= 1

	regions = append(regions, HitRegion{
		ID:     "help_doc_viewport",
		X:      docBoxX,
		Y:      docBoxY,
		Width:  m.lastDocBoxW,
		Height: m.lastDocBoxH,
		ZOrder: ZScreen + 5, // Above dialog background, below scrollbar
		Label:  "Doc Viewport",
	})
	
	logger.Debug(context.Background(), "Help HitRegions: BorderRoot=(%d,%d) DocBox=(%d,%d) BoxW=%d BoxH=%d", 
		offsetX, offsetY, docBoxX, docBoxY, m.lastDocBoxW, m.lastDocBoxH)

	// 3. Scrollbar hit regions
	if m.Scroll.Info.Needed && IsScrollbarEnabled() {
		// The scrollbar column is the last column of the doc box content area.
		// If docBoxX is the left border, scrollbar is at docBoxX + width - 2.
		regions = append(regions, m.Scroll.HitRegions(docBoxX+m.lastDocBoxW-2, docBoxY, ZScreen+10, "Doc")...)
	}

	// 4. Hyperlink hit regions
	regions = append(regions, ScanForHyperlinks(m.ViewString(), offsetX, offsetY, ZScreen)...)

	return regions
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

type imageFixerRenderer struct {
	kitR    *kit_renderer.Renderer
	encoder kit_renderer.ImageEncoder
}

func (r *imageFixerRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, r.renderImage)
}

func (r *imageFixerRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, enter bool) (ast.WalkStatus, error) {
	if !enter {
		return ast.WalkContinue, nil
	}

	img := node.(*ast.Image)
	dest := string(img.Destination)

	// 1. Fetch the image data manually so we can clean it
	resp, err := http.Get(dest)
	if err != nil {
		return r.kitR.RenderImage(w, source, node, enter) // Fallback to standard
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return r.kitR.RenderImage(w, source, node, enter)
	}

	// 2. Fix shorthand decimal points in SVGs (e.g., .1 -> 0.1) which break the decoder
	if bytes.Contains(data, []byte("<svg")) {
		// Fix leading dots: (.1) -> (0.1),  .1 ->  0.1, etc.
		reLeading := regexp.MustCompile(`(^|[^0-9.])\.([0-9])`)
		data = reLeading.ReplaceAll(data, []byte(`${1}0.${2}`))

		// Fix stuck dots: 1.2.3 -> 1.2 0.3 (common in SVG paths)
		reStuck := regexp.MustCompile(`([0-9]+\.[0-9]+)\.([0-9])`)
		for reStuck.Match(data) {
			data = reStuck.ReplaceAll(data, []byte(`${1} 0.${2}`))
		}

		// Strip embedded icons (<image> tags) because ok-svg doesn't support nested SVGs
		reImage := regexp.MustCompile(`(?s)<image\b[^>]*/>`)
		data = reImage.ReplaceAll(data, []byte(""))

		// Strip links (<a> tags) because ok-svg doesn't support them
		reLinkOpen := regexp.MustCompile(`(?i)<a\b[^>]*>`)
		data = reLinkOpen.ReplaceAll(data, []byte(""))
		reLinkClose := regexp.MustCompile(`(?i)</a>`)
		data = reLinkClose.ReplaceAll(data, []byte(""))

		// Replace rgba(...,0) with none because ok-svg doesn't support rgba
		reRGBA := regexp.MustCompile(`rgba\([^)]*,0\)`)
		data = reRGBA.ReplaceAll(data, []byte("none"))
	}

	// 3. Decode the cleaned image
	imgObj, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return r.kitR.RenderImage(w, source, node, enter)
	}

	// 4. Encode using Kitty
	if _, err := r.encoder(w, imgObj, r.kitR); err != nil {
		return r.kitR.RenderImage(w, source, node, enter)
	}

	return ast.WalkSkipChildren, nil
}
