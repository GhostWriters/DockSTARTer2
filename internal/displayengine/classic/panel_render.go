package classic

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ─── View ─────────────────────────────────────────────────────────────────────

// SyncInputPrompt recomputes and persists the console input's Prompt string
// (see the switch in ViewString, which this duplicates -- same
// hit-region/render duplication tradeoff already accepted elsewhere in this
// package). Needs a pointer receiver: ViewString/GetInputCursor are both
// value-receiver methods, so a mutation made inside either only affects its
// own throwaway copy, never AppModel.panel itself -- GetInputCursor's cursor
// math then reads a stale Prompt that doesn't match what's actually
// rendered. Call this (via a real *PanelModel, i.e. &AppModel.panel) once
// per frame before either of those run.
func (m *PanelModel) SyncInputPrompt() {
	// Resolved each frame, so the input follows the active theme.
	m.applyInputStyles()
	if m.SessionActive() {
		return
	}
	// The prompt sits outside the field, which has a column each side:
	// plain, or the focused-row brackets while focused but not editing.
	outside, _, idle := m.inputStyles()
	typed := strings.TrimSpace(m.Input.Value())
	switch {
	case m.PanelMode == "system" && strings.HasPrefix(typed, "!!"):
		m.Input.Prompt = RenderThemeText("{{|PromptSudo|}}!!>{{[-]}}", outside)
	case m.PanelMode == "system" && strings.HasPrefix(typed, "!"):
		m.Input.Prompt = RenderThemeText("{{|PromptShell|}} !>{{[-]}}", outside)
	default:
		m.Input.Prompt = RenderThemeText("{{|Prompt|}}  >{{[-]}}", outside)
	}
	m.Input.Prompt += m.inputSlots(outside, idle)[0]
	// Matches the width ViewString draws the field at.
	m.Input.SetWidth(max(m.width-2-m.Input.PromptWidth()-2, 1))
}

// inputSlots returns the columns either side of the input bar's field:
// plain, or the focused-row brackets while focused but not editing.
func (m PanelModel) inputSlots(outside lipgloss.Style, idle bool) [2]string {
	ctx := GetActiveContext()
	if idle && ctx.MenuBrackets {
		open, closeCh := bracketGlyphs(ctx)
		return [2]string{
			RenderThemeText("{{[-]}}{{|TagBrackets|}}"+open+"{{[-]}}", outside),
			RenderThemeText("{{[-]}}{{|TagBrackets|}}"+closeCh+"{{[-]}}", outside),
		}
	}
	return [2]string{outside.Render(" "), outside.Render(" ")}
}

func (m PanelModel) ViewString() string {
	if m.Height() <= 0 {
		return ""
	}
	ctx := GetActiveContext()
	title := m.Title()

	upGlyph, dnGlyph := resizeUpWidget, resizeDnWidget
	if !ctx.LineCharacters {
		upGlyph, dnGlyph = resizeUpWidgetAscii, resizeDnWidgetAscii
	}
	panelBorderStyle := SemanticRawStyle("PanelBorder")
	baseStyle := ctx.BorderFlags.Apply(panelBorderStyle)

	// Input box occupies 3 rows (top border + 1 content + bottom border).
	hasInput := m.HasInputBox()
	vpH := m.ViewportHeight()
	if m.Sv.Height() != vpH {
		m.Sv.SetSize(m.Sv.Width(), vpH)
	}
	if m.Sv.Width() != m.width-ScrollbarGutterWidth {
		m.Sv.SetSize(m.width-ScrollbarGutterWidth, vpH)
	}

	// The content area uses the output tint; the borders and scrollbar keep
	// the session's own.
	output := OutputConsoleStyle()
	m.Sv.SetStyle(lipgloss.NewStyle().
		Background(output.GetBackground()).
		Foreground(output.GetForeground()))

	vpView := MaintainBackground(m.Sv.View(), output)
	vpView = ApplyScrollbarColumn(vpView, m.Sv.TotalLineCount(), vpH, m.Sv.YOffset(), ctx.LineCharacters, ctx)

	// Input box — bordered with submenu styling.
	// RenderTopBorderBoxCtx appends content without side borders, so full m.width is available.
	inputBoxWidth := m.width
	if m.SessionActive() {
		m.Input.Placeholder = ""
		st := m.Input.Styles()
		st.Focused.Placeholder = SemanticRawStyle("MarkerLocked")
		st.Blurred.Placeholder = SemanticRawStyle("MarkerLocked")
		m.Input.SetStyles(st)
		marker := lockedMarker
		if !ctx.LineCharacters {
			marker = lockedMarkerAscii
		}
		// Consolidated lock marker and message into the Prompt for reliable styling
		m.Input.Prompt = RenderThemeText("{{|MarkerLocked|}}"+marker+" Session active — input locked{{[-]}} ", inputFieldStyle(false, m.InputFocused && !m.SessionActive()))
	} else {
		m.Input.Placeholder = ""
		st := m.Input.Styles()
		st.Focused.Placeholder = lipgloss.NewStyle()
		st.Blurred.Placeholder = lipgloss.NewStyle()
		m.Input.SetStyles(st)
		// Prompt is already set by SyncInputPrompt, called once per frame
		// before ViewString (see its doc comment for why it can't be set
		// here instead).
	}
	inputTitleTag := "TitleSubMenu"
	if m.InputFocused {
		inputTitleTag = "TitleSubMenuFocused"
	}
	inputTitle := "Command"
	if m.PanelMode == "system" {
		inputTitle = "Command (! = System command, !! = Elevated system command)"
	}
	// The field is drawn without the prompt, so its style stays off the
	// prompt and the columns either side. The text scrolls within the
	// field; the right column and the cursor take one each.
	outside, field, idle := m.inputStyles()
	prefix := m.Input.Prompt
	prefixWidth := m.Input.PromptWidth()
	m.Input.SetWidth(max(inputBoxWidth-2-prefixWidth-2, 1))
	m.Input.Prompt = ""
	// The cell under the terminal cursor has no style of its own.
	view := MaintainBackground(m.Input.View(), field)
	m.Input.Prompt = prefix
	fieldWidth := max(inputBoxWidth-2-prefixWidth-1, 1)
	inputContent := prefix + field.Width(fieldWidth).MaxWidth(fieldWidth).Render(view) + m.inputSlots(outside, idle)[1]
	inputBox := RenderBorderedBoxCtx(
		"{{|"+inputTitleTag+"|}}"+inputTitle+"{{[-]}}",
		inputContent,
		inputBoxWidth-2,
		3,
		m.InputFocused,
		true,
		true,
		ctx.SubmenuTitleAlign,
		inputTitleTag,
		ctx,
	)

	// Inject INS/OVR label into the bottom-left of the Command section border.
	modeLabel := "INS"
	if m.Input.IsOverwrite() {
		modeLabel = "OVR"
	}
	ibLines := strings.Split(inputBox, "\n")
	if len(ibLines) > 0 {
		ibLines[len(ibLines)-1] = BuildLabeledBottomBorderCtx(inputBoxWidth, modeLabel, m.InputFocused, ctx)
		inputBox = strings.Join(ibLines, "\n")
	}

	combined := vpView
	if hasInput {
		combined += "\n" + inputBox
	}

	panelTitleStyle := SemanticRawStyle("PanelTitle")

	// Sep between resize widgets uses the console border color, not the dialog border color.
	lineChar := "─"
	if !ctx.LineCharacters {
		lineChar = "-"
	}
	rightTitle := ""
	rightSuffix := ""
	if m.Expanded {
		upTag, dnTag := "IconResizeUpInactive", "IconResizeDnInactive"
		if m.PressedWidget() == PanelWidgetUp {
			upTag = "IconPressed"
		} else if m.PressedWidget() == PanelWidgetDn {
			dnTag = "IconPressed"
		} else if m.TitleBarFocused() {
			if m.ActiveWidget() == PanelWidgetUp {
				upTag = "IconFocused"
			} else {
				dnTag = "IconFocused"
			}
		}
		iconStr := lineChar +
			"{{|" + upTag + "|}}[" + upGlyph + "]{{[-]}}" +
			lineChar +
			"{{|" + dnTag + "|}}[" + dnGlyph + "]{{[-]}}"
		pct := int(m.Sv.ScrollPercent() * 100)
		rightTitle = fmt.Sprintf(" %3d%% ", pct)
		rightSuffix = RenderThemeText(iconStr, baseStyle)
	}

	spinIndL, spinIndR, isChanged := m.currentSpinnerMarker()
	changedFlag := ""
	if isChanged {
		changedFlag = "1"
	}
	return RenderTopBorderBoxCtx(title, rightTitle, rightSuffix, combined, m.width, m.Focused || m.TitleBarFocused(), panelTitleStyle, panelBorderStyle, ctx, spinIndL, changedFlag, spinIndR)
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
