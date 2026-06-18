package tui

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"DockSTARTer2/internal/graphics"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"

	"github.com/charmbracelet/x/ansi"
	"github.com/pgavlin/goldmark"
	"github.com/pgavlin/goldmark/extension"
	goldmark_parser "github.com/pgavlin/goldmark/parser"
	"github.com/pgavlin/goldmark/renderer"
	"github.com/pgavlin/goldmark/text"
	"github.com/pgavlin/goldmark/util"

	// "golang.org/x/term"
	_ "github.com/gen2brain/svg"
	kit_renderer "github.com/pgavlin/markdown-kit/renderer"
	"github.com/pgavlin/markdown-kit/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

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

	// Use smart graphics detection for high-fidelity on Linux and clean links on Windows
	canDisplay := graphics.CanDisplayGraphics()
	// Use Sixel encoder for high-fidelity web terminal support
	encoder := graphics.SixelGraphicsEncoder()

	// Initialize the terminal-optimized NodeRenderer // Use markdown-kit renderer with auto-detected theme
	kitR := kit_renderer.New(
		kit_renderer.WithTheme(styles.GlamourDark),
		kit_renderer.WithWordWrap(width),
		kit_renderer.WithSoftBreak(width != 0),
		kit_renderer.WithImages(canDisplay, width, ""),
		kit_renderer.WithImageEncoder(encoder),
		kit_renderer.WithHyperlinks(true), // Enable hyperlinks for better fallbacks
	)

	// Create a goldmark renderer and register our terminal NodeRenderer
	mainR := renderer.NewRenderer(renderer.WithNodeRenderers(
		util.Prioritized(kitR, 100),
	))

	// Parse the markdown into an AST
	// Pre-process: convert Shields.io images to links to ensure visibility/clickability
	reBadge := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]*shields\.io[^)]*)\)`)
	source = reBadge.ReplaceAll(source, []byte(`[$1]($2)`))

	parser := goldmark.DefaultParser()
	parser.AddOptions(goldmark_parser.WithParagraphTransformers(
		util.Prioritized(extension.NewTableParagraphTransformer(), 200),
	))
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
		resolved := theme.ToANSI(m.contextInfo.PageText, "")
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
		resolved := theme.ToANSI(m.contextInfo.ItemText, "")
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
			resolved := " " + strings.TrimRight(theme.ToANSI(strings.TrimRight(line, " ")), " ")
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
		if visibleRows < 1 {
			visibleRows = 1
		}

		// Clamp offset
		if m.contextOffset < 0 {
			m.contextOffset = 0
		}
		if boxTotalLines > 0 && m.contextOffset > boxTotalLines-visibleRows {
			m.contextOffset = boxTotalLines - visibleRows
		}
		if m.contextOffset < 0 {
			m.contextOffset = 0
		}
		boxOffset = m.contextOffset

		for i := 0; i < visibleRows && (i+boxOffset) < boxTotalLines; i++ {
			line := docLines[i+boxOffset]
			if lw := lipgloss.Width(line); lw > docTextW {
				line = TruncateRight(line, docTextW)
			} else if lw < docTextW {
				line += strutil.Repeat(" ", docTextW-lw)
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
		if maxCtxH < 5 {
			maxCtxH = 5
		}
		boxTargetH = boxTotalLines + 2
		if boxTargetH < 3 {
			boxTargetH = 3
		}
		if boxTargetH > maxCtxH {
			boxTargetH = maxCtxH
		}

		visibleRows := boxTargetH - 2
		if visibleRows < 1 {
			visibleRows = 1
		}

		// Clamp offset
		if m.contextOffset < 0 {
			m.contextOffset = 0
		}
		if boxTotalLines > 0 && m.contextOffset > boxTotalLines-visibleRows {
			m.contextOffset = boxTotalLines - visibleRows
		}
		if m.contextOffset < 0 {
			m.contextOffset = 0
		}
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
				if scrollPct > 1.0 {
					scrollPct = 1.0
				}
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
				paddedLine += strutil.Repeat(" ", targetWidth-w)
			}
			bindingContent = append(bindingContent, paddedLine)
		}
		// Append modifier note at the bottom of every bindings page.
		note := theme.ToANSI("Most {{|KeyCap|}}alt+key{{[-]}} shortcuts also accept {{|KeyCap|}}ctrl{{[-]}} or {{|KeyCap|}}ctrl+alt{{[-]}}", "")
		centeredNote := CenterText(note, targetWidth)
		blankLine := strutil.Repeat(" ", targetWidth)
		bindingContent = append(bindingContent, blankLine, centeredNote)
		bindingsBox := RenderBorderedBoxCtx(
			"Keyboard & Mouse Controls",
			strings.Join(bindingContent, "\n"),
			targetWidth,
			len(bpLines)+4,
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
	// Always use small titlebar — the halo is the help dialog's visual signature
	// and the large titlebar separator conflicts with the halo border at the edges.
	ctx.LargeTitleBars = false
	return RenderUniformBlockDialogCtx(title, content, ctx)
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
