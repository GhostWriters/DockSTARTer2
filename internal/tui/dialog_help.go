package tui

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
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

// HelpContextProvider is an optional interface that screens and dialogs can
// implement to inject contextual text at the top of the help dialog.
// contentWidth is the available display width for the help dialog content area
// (used so the implementation can word-wrap text correctly at the source).
// Return an empty HelpContext to omit the context sections.
type HelpContextProvider interface {
	HelpContext(contentWidth int) HelpContext
}

// HelpDialogModel displays a keyboard shortcut reference dialog.
// It integrates with AppModel via ShowDialogMsg/CloseDialogMsg.
type HelpDialogModel struct {
	help   help.Model
	width  int
	height int

	focused bool // tracks global focus

	keyMap      help.KeyMap
	contextInfo HelpContext // structured help info
	contextOffset int // scroll offset for item context text

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
		// Any other key closes the help dialog (? toggles it off, Esc also works)
		return m, func() tea.Msg { return CloseDialogMsg{} }

	case tea.MouseWheelMsg:
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
		// Any click on the help dialog (or its components) closes it
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

	dialogStyle := SemanticStyle("{{|Theme_Dialog|}}")
	haloColor := lipgloss.Color("0") // Solid black halo
	bgStyle := dialogStyle

	// Apply theme styles to the help component
	sepStyle := dialogStyle
	dimStyle := SemanticStyle("{{|Theme_HelpItem|}}")
	keyStyle := SemanticStyle("{{|Theme_HelpTag|}}")

	m.help.Styles.ShortKey = keyStyle
	m.help.Styles.ShortDesc = dimStyle
	m.help.Styles.ShortSeparator = sepStyle
	m.help.Styles.FullKey = keyStyle
	m.help.Styles.FullDesc = dimStyle
	m.help.Styles.FullSeparator = sepStyle
	m.help.Styles.Ellipsis = dimStyle

	bindingLines := strings.Split(m.help.View(m.keyMap), "\n")

	// Compute max line width across both context text and key bindings
	maxLineWidth := 0
	for i, line := range bindingLines {
		trimmed := strings.TrimRight(line, " ")
		bindingLines[i] = trimmed
		if w := lipgloss.Width(trimmed); w > maxLineWidth {
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

	if hasPageCtx || hasItemCtx {
		// Resolve and wrap text for both panels
		var legendLines []string
		if hasPageCtx {
			resolved := console.ToANSI(m.contextInfo.PageText)
			for _, line := range strings.Split(resolved, "\n") {
				wrapped := ansi.Wordwrap(line, targetWidth, "")
				for _, wl := range strings.Split(wrapped, "\n") {
					wl = strings.TrimRight(wl, " ")
					legendLines = append(legendLines, wl)
					if w := lipgloss.Width(wl); w > maxLineWidth {
						maxLineWidth = w
					}
				}
			}
		}

		var itemLines []string
		if hasItemCtx {
			resolved := console.ToANSI(m.contextInfo.ItemText)
			for _, line := range strings.Split(resolved, "\n") {
				wrapped := ansi.Wordwrap(line, targetWidth, "")
				for _, wl := range strings.Split(wrapped, "\n") {
					wl = strings.TrimRight(wl, " ")
					itemLines = append(itemLines, wl)
					if w := lipgloss.Width(wl); w > maxLineWidth {
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

		ctx := GetActiveContext()

		// Render Page Context box (formerly Legend) if content exists
		if len(legendLines) > 0 {
			title := m.contextInfo.PageTitle
			if title == "" {
				title = "Description"
			}

			// Apply centering if specified by alignment
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
				"Theme_TitleSubMenu",
				ctx,
			)
		}

		// Render Item Context box if content exists
		if len(itemLines) > 0 {
			title := m.contextInfo.ItemTitle
			if title == "" {
				title = "Context Sensitive Help"
			}

			// Vertical budgeting: account for legend box if present
			overheadH := 6
			if legendBox != "" {
				overheadH += lipgloss.Height(legendBox) + 1
			}

			_, availH := GetAvailableDialogSize(m.width, m.height)
			maxContextHeight := availH - overheadH - len(bindingLines)
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
				displayLines = append(displayLines, itemLines[i+m.contextOffset])
			}

			contentToRender := strings.Join(displayLines, "\n")

			// Apply scrollbar
			sbInfo := ScrollbarInfo{}
			if len(itemLines) > visibleRows {
				contentToRender, sbInfo = ApplyScrollbarColumnTracked(
					contentToRender,
					len(itemLines),
					visibleRows,
					m.contextOffset,
					true,
					ctx.LineCharacters,
					ctx,
				)
			}

			contextBox = RenderBorderedBoxCtx(
				title,
				contentToRender,
				maxLineWidth,
				maxContextHeight,
				true, // focused: true
				true, // showIndicators: true
				true,
				ctx.SubmenuTitleAlign,
				"Theme_TitleSubMenuFocused",
				ctx,
			)

			// Scroll indicator
			if sbInfo.Needed {
				// Calculate scroll percentage
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

	// Combine components with original spacing
	var combinedText string
	if legendBox != "" {
		combinedText += legendBox + "\n"
	}
	if contextBox != "" {
		combinedText += contextBox + "\n"
	}

	// Use the actual width of the bordered boxes to pad the keyboard reference below.
	// This prevents the "blank column on the right" issue if a title was wider than the content.
	actualBoxWidth := maxLineWidth
	if legendBox != "" {
		actualBoxWidth = lipgloss.Width(strings.Split(legendBox, "\n")[0]) - 2
	} else if contextBox != "" {
		actualBoxWidth = lipgloss.Width(strings.Split(contextBox, "\n")[0]) - 2
	}

	// Render key bindings with original logic
	var paddedBindings []string
	for _, line := range bindingLines {
		lineWidth := lipgloss.Width(line)
		paddedLine := " " + line + strutil.Repeat(" ", actualBoxWidth-lineWidth) + " "
		paddedBindings = append(paddedBindings, MaintainBackground(bgStyle.Render(paddedLine), bgStyle))
	}
	combinedText += strings.Join(paddedBindings, "\n")

	content := combinedText
	// Per-line maintenance above is sufficient — no outer wrap needed

	// Ensure the title is visible on the black border bar.
	// Use the original themed Dialog background for the title text area.
	ctx := GetActiveContext()
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
