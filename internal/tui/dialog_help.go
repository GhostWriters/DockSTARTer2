package tui

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// HelpContextProvider is an optional interface that screens and dialogs can
// implement to inject contextual text at the top of the help dialog.
// contentWidth is the available display width for the help dialog content area
// (used so the implementation can word-wrap text correctly at the source).
// Return an empty string to omit the context section entirely.
type HelpContextProvider interface {
	HelpContext(contentWidth int) string
}

// HelpDialogModel displays a keyboard shortcut reference dialog.
// It integrates with AppModel via ShowDialogMsg/CloseDialogMsg.
type HelpDialogModel struct {
	help   help.Model
	width  int
	height int

	focused bool // tracks global focus

	keyMap      help.KeyMap
	contextText string // optional text shown above the key bindings

	// Unified layout (deterministic sizing)
	layout DialogLayout
}

func NewHelpDialogModel() *HelpDialogModel {
	return NewHelpDialogModelWithMap(Keys)
}

func NewHelpDialogModelWithMap(km help.KeyMap) *HelpDialogModel {
	return NewHelpDialogWithContext(km, "")
}

// NewHelpDialogWithContext creates a help dialog that shows contextText
// (e.g. current variable info) above the standard key bindings.
// Pass an empty string to show only the key bindings.
func NewHelpDialogWithContext(km help.KeyMap, contextText string) *HelpDialogModel {
	h := help.New()
	h.ShowAll = true
	return &HelpDialogModel{help: h, focused: true, keyMap: km, contextText: contextText}
}

func (m *HelpDialogModel) Init() tea.Cmd { return nil }

func (m *HelpDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Any key closes the help dialog (? toggles it off, Esc also works)
		_ = msg
		return m, func() tea.Msg { return CloseDialogMsg{} }

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
	if m.layout.Width == 0 {
		return ""
	}

	// Calculate target width for help content.
	// Overhead: halo(4) + border(2) + per-line padding(2) = 8.
	// Base on the maximized-dialog available width so we never exceed that bound.
	availW, _ := GetAvailableDialogSize(m.width, m.height)
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

	// Build the context section (variable info) if provided.
	// The contextText is already word-wrapped to targetWidth by the HelpContextProvider.
	var contextLines []string
	if m.contextText != "" {
		for _, line := range strings.Split(m.contextText, "\n") {
			trimmed := strings.TrimRight(line, " ")
			// Resolve semantic tags to ANSI once, then word-wrap the ANSI string
			// so width measurement is accurate (no invisible tag characters).
			resolved := console.ToANSI(trimmed)
			wrapped := ansi.Wordwrap(resolved, targetWidth, "")
			for _, wl := range strings.Split(wrapped, "\n") {
				wl = strings.TrimRight(wl, " ")
				contextLines = append(contextLines, wl)
				if w := lipgloss.Width(wl); w > maxLineWidth {
					maxLineWidth = w
				}
			}
		}
		// Cap at targetWidth — long values (e.g. file paths) may still exceed it.
		if maxLineWidth > targetWidth {
			maxLineWidth = targetWidth
		}
		// Center the first context line (e.g. a legend) now that maxLineWidth is known.
		if len(contextLines) > 0 {
			if pad := (maxLineWidth - lipgloss.Width(contextLines[0])) / 2; pad > 0 {
				contextLines[0] = strings.Repeat(" ", pad) + contextLines[0]
			}
		}
		sepChar := "─"
		if !GetActiveContext().LineCharacters {
			sepChar = "-"
		}
		sep := sepStyle.Render(strings.Repeat(sepChar, maxLineWidth))

		// Only add separators (above and below context) when vertical space allows.
		// Available height: screen height minus halo(2) + border(2) + title(1) = 5 overhead.
		_, availH := GetAvailableDialogSize(m.width, m.height)
		totalWithSeps := len(contextLines) + 2 + len(bindingLines)
		if totalWithSeps <= availH-5 {
			contextLines = append([]string{sep}, contextLines...)
		}
		contextLines = append(contextLines, sep)
	}

	// Combine: context (if any) then key bindings
	var allLines []string
	allLines = append(allLines, contextLines...)
	allLines = append(allLines, bindingLines...)

	// Apply dialog background with uniform padding on all lines
	for i, line := range allLines {
		lineWidth := lipgloss.Width(line)
		paddedLine := " " + line + strutil.Repeat(" ", maxLineWidth-lineWidth) + " "
		allLines[i] = MaintainBackground(bgStyle.Render(paddedLine), bgStyle)
	}
	content := strings.Join(allLines, "\n")
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
	dialogStr := RenderUniformBlockDialogCtx(" Keyboard & Mouse Controls ", content, ctx)

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
