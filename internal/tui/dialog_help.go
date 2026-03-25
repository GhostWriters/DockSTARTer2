package tui

import (
	"DockSTARTer2/internal/theme"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"charm.land/bubbles/v2/help"
	keybind "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// HelpContext defines the two contextual help panels.
type HelpContext struct {
	ScreenName string // e.g., "Main Menu" — used in the title bar: "Help: Main Menu"
	PageTitle  string // e.g., "Legend" or "Description"
	PageText   string
	ItemTitle  string // e.g., variable name or menu item Tag
	ItemText   string
}

// HelpContextProvider is implemented by models that can provide structured help content.
type HelpContextProvider interface {
	HelpContext(maxWidth int) HelpContext
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

	// Paged mode: when context + bindings don't fit together, cycle between pages.
	// page 0 = context info, page 1 = keyboard bindings.
	paged bool
	page  int

	// Unified layout (deterministic sizing)
	layout DialogLayout
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
	return &HelpDialogModel{help: h, focused: true, keyMap: km, contextInfo: info}
}

func (m *HelpDialogModel) Init() tea.Cmd { return nil }

func (m *HelpDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Help key (? / F1) cycles pages when paged, otherwise closes.
		if keybind.Matches(msg, Keys.Help) {
			if m.paged {
				m.page = (m.page + 1) % 2
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
		if m.paged && m.page == 1 {
			// On bindings page, scrolling does nothing meaningful
			return m, nil
		}
		if msg.Button == tea.MouseWheelUp {
			m.contextOffset--
			if m.contextOffset < 0 {
				m.contextOffset = 0
			}
		} else if msg.Button == tea.MouseWheelDown {
			m.contextOffset++
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case LayerHitMsg:
		// When paged, clicking cycles to the next page instead of closing.
		if m.paged {
			m.page = (m.page + 1) % 2
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

	// Calculate target width for help content.
	// We want a consistent width for the help panels relative to the screen.
	availW, availH := GetAvailableDialogSize(m.width, m.height)
	
	// Ensure at least some minimal room
	if availW < 30 {
		availW = 30
	}
	if availH < 20 {
		availH = 20
	}

	targetWidth := availW - 8
	if targetWidth < 20 {
		targetWidth = 20
	}
	if targetWidth > 120 {
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
	var legendBox string
	var contextBox string

	hasPageCtx := m.contextInfo.PageText != ""
	hasItemCtx := m.contextInfo.ItemText != ""

	var legendLines []string
	var itemLines []string

	if hasPageCtx || hasItemCtx {
		// Resolve and wrap text for both panels
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
					// Prepend a space for left padding. The 1-char right gutter is added by ApplyScrollbarColumn.
					paddedLine := " " + wl
					legendLines = append(legendLines, paddedLine)
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

		// Cap maxLineWidth at targetWidth and ensure minimum
		if maxLineWidth > targetWidth {
			maxLineWidth = targetWidth
		}
		if maxLineWidth < 20 {
			maxLineWidth = 20
		}
	}

	// Determine if paging is needed: when item context + bindings together would trigger a scrollbar.
	// Overhead: outer dialog (6) + legend box height if present.
	if len(itemLines) > 0 {
		contextOverhead := 6
		if len(legendLines) > 0 {
			contextOverhead += len(legendLines) + 2 + 1 // legend box (border+content) + gap line
		}
		visibleWithBindings := availH - contextOverhead - (len(bindingLines) + 2) - 2
		if visibleWithBindings < 1 {
			visibleWithBindings = 1
		}
		m.paged = len(itemLines) > visibleWithBindings
	} else {
		m.paged = false
	}

	// Clamp page to valid range.
	if m.page < 0 || !m.paged {
		m.page = 0
	}

	showContext := !m.paged || m.page == 0
	showBindings := !m.paged || m.page == 1

	ctx := GetActiveContext()

	if showContext && (len(legendLines) > 0 || len(itemLines) > 0) {
		// Render Page Context box (formerly Legend) if content exists
		if len(legendLines) > 0 {
			title := m.contextInfo.PageTitle
			if title == "" {
				title = "Description"
			}

			legendToRender := strings.Join(legendLines, "\n")
			if ctx.SubmenuTitleAlign == "center" {
				var centeredLegend []string
				for _, ll := range legendLines {
					centeredLegend = append(centeredLegend, CenterText(ll, maxLineWidth))
				}
				legendToRender = strings.Join(centeredLegend, "\n")
			}

			legendBox = RenderBorderedBoxCtx(
				title,
				legendToRender,
				maxLineWidth,
				len(legendLines)+2,
				false, // focused: false
				true,  // showIndicators: true
				true,
				ctx.SubmenuTitleAlign,
				"TitleSubMenu",
				ctx,
			)
		}

		// Render Item Context box if content exists
		if len(itemLines) > 0 {
			title := m.contextInfo.ItemTitle
			if title == "" {
				title = "Context Sensitive Help"
			}

			// When paged (page 0): use the full available height minus legend overhead only.
			// When not paged: subtract bindings from available height too.
			overheadH := 6
			if legendBox != "" {
				overheadH += lipgloss.Height(legendBox) + 1
			}
			bindingsH := 0
			if !m.paged {
				bindingsH = len(bindingLines) + 2 // +2 for the bindings box border
			}
			maxContextHeight := availH - overheadH - bindingsH
			if maxContextHeight < 5 {
				maxContextHeight = 5
			}

			// Size box to content
			if len(itemLines)+2 < maxContextHeight {
				maxContextHeight = len(itemLines) + 2
			}

			visibleRows := maxContextHeight - 2
			if visibleRows < 1 {
				visibleRows = 1
			}

			// Clamp offset
			if m.contextOffset < 0 {
				m.contextOffset = 0
			}
			if m.contextOffset > len(itemLines)-visibleRows {
				m.contextOffset = len(itemLines) - visibleRows
			}
			if m.contextOffset < 0 {
				m.contextOffset = 0
			}

			var displayLines []string
			for i := 0; i < visibleRows && (i+m.contextOffset) < len(itemLines); i++ {
				line := itemLines[i+m.contextOffset]
				// Pad to uniform width so the scrollbar column always appears at the right edge.
				if w := lipgloss.Width(line); w < maxLineWidth {
					line += strings.Repeat(" ", maxLineWidth-w)
				}
				displayLines = append(displayLines, line)
			}

			contentToRender := strings.Join(displayLines, "\n")

			// Apply scrollbar — always reserve the gutter column for consistent width.
			contentToRender, sbInfo := ApplyScrollbarColumnTracked(
				contentToRender,
				len(itemLines),
				visibleRows,
				m.contextOffset,
				IsScrollbarEnabled(),
				ctx.LineCharacters,
				ctx,
			)

			contextBox = RenderBorderedBoxCtx(
				title,
				contentToRender,
				maxLineWidth,
				maxContextHeight,
				true, // focused: true
				true, // showIndicators: true
				true,
				ctx.SubmenuTitleAlign,
				"TitleSubMenuFocused",
				ctx,
			)

			// Scroll indicator
			if sbInfo.Needed {
				scrollPct := 1.0
				if len(itemLines) > visibleRows {
					scrollPct = float64(m.contextOffset) / float64(len(itemLines)-visibleRows)
				}
				if scrollPct > 1.0 {
					scrollPct = 1.0
				}

				boxLines := strings.Split(contextBox, "\n")
				if len(boxLines) > 0 {
					boxLines[len(boxLines)-1] = BuildScrollPercentBottomBorder(maxLineWidth+2, scrollPct, true, ctx)
					contextBox = strings.Join(boxLines, "\n")
				}
			}
		}
	}

	// Combine sections with "\n" as separator (not suffix) to avoid trailing blank lines.
	var parts []string
	if legendBox != "" {
		parts = append(parts, legendBox)
	}
	if contextBox != "" {
		parts = append(parts, contextBox)
	}
	if showBindings {
		var bindingContent []string
		for _, line := range bindingLines {
			paddedLine := " " + line
			if w := lipgloss.Width(paddedLine); w < maxLineWidth {
				paddedLine += strings.Repeat(" ", maxLineWidth-w)
			}
			bindingContent = append(bindingContent, paddedLine)
		}
		bindingsBox := RenderBorderedBoxCtx(
			"Keyboard & Mouse Controls",
			strings.Join(bindingContent, "\n"),
			maxLineWidth,
			len(bindingLines)+2,
			false, // not interactive
			false, // no scroll indicators
			true,
			ctx.SubmenuTitleAlign,
			"TitleSubMenu",
			ctx,
		)
		parts = append(parts, bindingsBox)
	}
	combinedText := strings.Join(parts, "\n")

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
	dialogStr := RenderUniformBlockDialogCtx(title, content, ctx)

	// Add the solid black halo
	return AddPatternHalo(dialogStr, haloColor)
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
		lipgloss.NewLayer(m.ViewString()).Z(ZScreen + 1).ID("Dialog.Help"),
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
	// Help dialog has a halo (2) and a border (2).
	// Content area starts at offsetX + 2, offsetY + 2.
	// We'll use the full width and height for hit testing.
	
	// Re-calculate height since HelpDialog is content-driven
	h := lipgloss.Height(m.ViewString())

	var regions []HitRegion
	
	// Close button (anywhere in the dialog for now, or maybe specifically at the bottom)
	// For help dialog, we usually close on any click, but let's be more specific.
	// Let's add an "OK" or "Close" label hit region at the bottom.
	
	regions = append(regions, HitRegion{
		ID:     "help_dialog",
		X:      offsetX,
		Y:      offsetY,
		Width:  m.width,
		Height: h,
		ZOrder: ZScreen + 1,
		Label:  "Help",
	})
	
	return regions
}

func (m *HelpDialogModel) calculateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// Overhead for Help: Halo (2) + Bordered Dialog (2) = 4
	overhead := 4

	m.layout = DialogLayout{
		Width:    m.width,
		Height:   0, // height is content-driven, not forced
		Overhead: overhead,
	}
}
