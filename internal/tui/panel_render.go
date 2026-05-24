package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ─── View ─────────────────────────────────────────────────────────────────────

func (m PanelModel) ViewString() string {
	if m.Height() <= 0 {
		return ""
	}
	ctx := GetActiveContext()

	marker := "^"
	if m.expanded {
		marker = "v"
	}
	titleText := "Console"
	switch m.panelMode {
	case "log":
		titleText = "Log"
	case "system":
		titleText = "System Console"
	}
	title := marker + " " + titleText + " " + marker

	styles := GetStyles()
	upGlyph, dnGlyph := resizeUpWidget, resizeDnWidget
	if !ctx.LineCharacters {
		upGlyph, dnGlyph = resizeUpWidgetAscii, resizeDnWidgetAscii
	}
	activeStyle := styles.IconActive
	upInactiveStyle := styles.ResizeUpIconInactive
	dnInactiveStyle := styles.ResizeDnIconInactive
	upStyle, dnStyle := upInactiveStyle, dnInactiveStyle
	if m.titleBarFocused {
		upStyle, dnStyle = activeStyle, activeStyle
	}
	lineChar := "─"
	if !ctx.LineCharacters {
		lineChar = "-"
	}

	// Input box occupies 3 rows (top border + 1 content + bottom border).
	hasInput := m.panelMode == "console" || m.panelMode == "system"
	vpH := m.height - 1
	if hasInput {
		vpH -= 3
	}
	if vpH < 1 {
		if m.totalHeight > 0 {
			fullH := (m.totalHeight / 2) - 1
			if hasInput {
				fullH -= 3
			}
			vpH = fullH
		}
		if vpH < 1 {
			vpH = 1
		}
	}
	if m.viewport.Height() != vpH {
		m.viewport.SetHeight(vpH)
	}
	if m.viewport.Width() != m.width-ScrollbarGutterWidth {
		m.viewport.SetWidth(m.width - ScrollbarGutterWidth)
	}

	vpStyle := lipgloss.NewStyle().
		Width(m.viewport.Width()).
		Height(vpH).
		Background(ctx.Console.GetBackground()).
		Foreground(ctx.Console.GetForeground())
	m.viewport.Style = vpStyle

	vpView := MaintainBackground(m.viewport.View(), ctx.Console)
	vpView = ApplyScrollbarColumn(vpView, len(m.lines), vpH, m.viewport.YOffset(), ctx.LineCharacters, ctx)

	// Input box — bordered with submenu styling.
	// RenderTopBorderBoxCtx appends content without side borders, so full m.width is available.
	inputBoxWidth := m.width
	m.input.SetWidth(inputBoxWidth - 2)
	if m.sessionActive() {
		m.input.Placeholder = ""
		st := m.input.Styles()
		st.Focused.Placeholder = SemanticRawStyle("MarkerLocked")
		st.Blurred.Placeholder = SemanticRawStyle("MarkerLocked")
		m.input.SetStyles(st)
		marker := lockedMarker
		if !ctx.LineCharacters {
			marker = lockedMarkerAscii
		}
		// Consolidated lock marker and message into the Prompt for reliable styling
		m.input.Prompt = RenderThemeText("{{|MarkerLocked|}}"+marker+" Session active — input locked{{[-]}} ", ctx.Dialog)
	} else {
		m.input.Placeholder = ""
		st := m.input.Styles()
		st.Focused.Placeholder = lipgloss.NewStyle()
		st.Blurred.Placeholder = lipgloss.NewStyle()
		m.input.SetStyles(st)
		m.input.Prompt = "> "
	}
	inputTitleTag := "TitleSubMenu"
	if m.inputFocused {
		inputTitleTag = "TitleSubMenuFocused"
	}
	inputContent := lipgloss.NewStyle().
		Width(inputBoxWidth - 2).
		Background(ctx.Dialog.GetBackground()).
		Render(m.input.View())
	inputBox := RenderBorderedBoxCtx(
		"{{|"+inputTitleTag+"|}}Command{{[-]}}",
		inputContent,
		inputBoxWidth-2,
		3,
		m.inputFocused,
		true,
		true,
		ctx.SubmenuTitleAlign,
		inputTitleTag,
		ctx,
	)

	// Inject INS/OVR label into the bottom-left of the Command section border.
	modeLabel := "INS"
	if m.input.IsOverwrite() {
		modeLabel = "OVR"
	}
	ibLines := strings.Split(inputBox, "\n")
	if len(ibLines) > 0 {
		ibLines[len(ibLines)-1] = BuildLabeledBottomBorderCtx(inputBoxWidth, modeLabel, m.inputFocused, ctx)
		inputBox = strings.Join(ibLines, "\n")
	}

	combined := vpView
	if hasInput {
		combined += "\n" + inputBox
	}

	consoleTitleStyle := SemanticRawStyle("ConsoleTitle")
	consoleBorderStyle := SemanticRawStyle("ConsoleBorder")

	// Sep between resize widgets uses the console border color, not the dialog border color.
	lineChar = "─"
	if !ctx.LineCharacters {
		lineChar = "-"
	}
	consoleSepStyle := ctx.BorderFlags.Apply(lipgloss.NewStyle()).
		Foreground(consoleBorderStyle.GetForeground()).
		Background(consoleBorderStyle.GetBackground())
	sep := consoleSepStyle.Render(lineChar)
	icons := upInactiveStyle.Render("[") + upStyle.Render(upGlyph) + upInactiveStyle.Render("]") +
		sep +
		dnInactiveStyle.Render("[") + dnStyle.Render(dnGlyph) + dnInactiveStyle.Render("]")
	rightTitle := icons
	if m.expanded {
		pct := int(m.viewport.ScrollPercent() * 100)
		rightTitle = fmt.Sprintf(" %d%% ", pct) + icons
	}

	return RenderTopBorderBoxCtx(title, rightTitle, combined, m.width, m.titleBarFocused, consoleTitleStyle, consoleBorderStyle, ctx)
}

// Layers returns a single layer with the panel content for visual compositing.
func (m PanelModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZPanel).ID(IDPanel),
	}
}

// View renders the panel at its current height.
func (m PanelModel) View() tea.View {
	return tea.NewView(m.ViewString())
}
