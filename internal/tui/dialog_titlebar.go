package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Title bar widget constants.
const (
	titleBarWidgetClose = 1
	titleBarWidgetHelp  = 2
)

// Title bar widget state fields shared by all simple dialog types.
// Embedded in baseDialogModel so dialogs get TitleBarFocusable for free.

func (b *baseDialogModel) FocusTitleBar() {
	b.titleBarFocused = true
	b.titleBarWidget = titleBarWidgetClose
}

func (b *baseDialogModel) BlurTitleBar() {
	b.titleBarFocused = false
	b.titleBarWidget = 0
}

func (b *baseDialogModel) TitleBarFocused() bool { return b.titleBarFocused }

// buildTitleBarWidgets returns the rendered [?]─[×] widget string for this dialog,
// with the correct active/inactive styling based on current title bar focus state.
// Uses large or small widget styles depending on ctx.LargeTitleBars.
func (b *baseDialogModel) buildTitleBarWidgets(ctx StyleContext) string {
	if ctx.LargeTitleBars {
		return buildLargeTitleBarWidgets(b.titleBarFocused, b.titleBarWidget, ctx)
	}
	return buildDialogTitleWidgets(b.titleBarFocused, b.titleBarWidget, ctx)
}

// minWidthForWidgets returns the minimum content width required so that right-side
// widgets fit after the title is positioned (accounting for centering).
// w is the starting content width candidate; the return value is >= w.
func minWidthForWidgets(w int, titleText, titleAlign string, widgets string) int {
	if widgets == "" {
		return w
	}
	widgetWidth := lipgloss.Width(GetPlainText(widgets))
	if widgetWidth == 0 {
		return w
	}
	// titleSectionLen: leftT + indicator + title + indicator + rightT
	titleSection := lipgloss.Width(titleText) + 4
	needed := widgetWidth + 1 // widget + 1 trailing dash minimum
	for {
		lp := 0
		if titleAlign != "left" {
			lp = (w - titleSection) / 2
		}
		if w-titleSection-lp >= needed {
			break
		}
		w++
	}
	return w
}

// BuildInactiveTitleWidgets builds the [?]─[×] widget string using inactive styles only.
// Automatically selects large or small widget style based on ctx.LargeTitleBars.
func BuildInactiveTitleWidgets(ctx StyleContext) string {
	if ctx.LargeTitleBars {
		return buildLargeTitleBarWidgets(false, 0, ctx)
	}
	return buildDialogTitleWidgets(false, 0, ctx)
}

// BuildInactiveLargeTitleWidgets builds the [?]─[×] widget string styled for the large titlebar row.
func BuildInactiveLargeTitleWidgets(ctx StyleContext) string {
	return buildLargeTitleBarWidgets(false, 0, ctx)
}

// buildLargeTitleBarWidgets returns the raw theme-tag string for the [?] [×] widgets.
// It is NOT pre-rendered — callers must render it via RenderThemeTextCtx in the correct
// area context so it picks up the type-specific LargeTitleArea* background.
func buildLargeTitleBarWidgets(focused bool, activeWidget int, ctx StyleContext) string {
	helpGlyph := helpWidget
	closeGlyph := closeWidget
	if !ctx.LineCharacters {
		closeGlyph = closeWidgetAscii
	}
	helpActive, closeActive := false, false
	if focused {
		switch activeWidget {
		case titleBarWidgetHelp:
			helpActive = true
		case titleBarWidgetClose:
			closeActive = true
		}
	}
	helpPrefix := "{{|LargeHelpIconInactive|}}"
	if helpActive {
		helpPrefix += "{{|LargeIconActive|}}"
	}
	closePrefix := "{{|LargeExitIconInactive|}}"
	if closeActive {
		closePrefix += "{{|LargeIconActive|}}"
	}
	return helpPrefix + "[" + helpGlyph + "]{{[-]}} " +
		closePrefix + "[" + closeGlyph + "]{{[-]}}"
}

// buildDialogTitleWidgets is the shared renderer for the [?]─[×] title bar widgets.
// focused/activeWidget are the title bar state; use false/0 for always-inactive output.
func buildDialogTitleWidgets(focused bool, activeWidget int, ctx StyleContext) string {
	helpGlyph := helpWidget
	closeGlyph := closeWidget
	lineChar := "─"
	if !ctx.LineCharacters {
		closeGlyph = closeWidgetAscii
		lineChar = "-"
	}
	borderBase := ctx.BorderFlags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.BorderColor).
		Background(ctx.Dialog.GetBackground())
	helpActive, closeActive := false, false
	if focused {
		switch activeWidget {
		case titleBarWidgetHelp:
			helpActive = true
		case titleBarWidgetClose:
			closeActive = true
		}
	}
	helpPrefix := "{{|HelpIconInactive|}}"
	if helpActive {
		helpPrefix += "{{|IconActive|}}"
	}
	closePrefix := "{{|ExitIconInactive|}}"
	if closeActive {
		closePrefix += "{{|IconActive|}}"
	}
	iconStr := helpPrefix + "[" + helpGlyph + "]{{[-]}}" +
		lineChar +
		closePrefix + "[" + closeGlyph + "]{{[-]}}"
	ctx.Dialog = borderBase
	return RenderThemeTextCtx(iconStr, ctx)
}

// handleTitleBarHit handles LayerHitMsg for the [?] and [×] widgets.
// closeCmd is the dialog-specific command to run when [×] is clicked.
// Returns (true, cmd) if the hit was consumed, (false, nil) otherwise.
func (b *baseDialogModel) handleTitleBarHit(msg LayerHitMsg, closeCmd tea.Cmd) (handled bool, cmd tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return false, nil
	}
	if strings.HasSuffix(msg.ID, "."+IDTitleWidgetClose) {
		b.BlurTitleBar()
		return true, closeCmd
	}
	if strings.HasSuffix(msg.ID, "."+IDTitleWidgetHelp) {
		b.BlurTitleBar()
		return true, func() tea.Msg { return TriggerHelpMsg{ScreenLevelOnly: true} }
	}
	return false, nil
}

// handleTitleBarKey intercepts key events when the title bar has focus.
// closeCmd is the dialog-specific command to run when [×] is activated.
// Returns (true, cmd) if the key was consumed, (false, nil) otherwise.
func (b *baseDialogModel) handleTitleBarKey(msg tea.KeyPressMsg, closeCmd tea.Cmd) (handled bool, cmd tea.Cmd) {
	if !b.titleBarFocused {
		return false, nil
	}
	switch {
	case key.Matches(msg, Keys.Esc):
		b.BlurTitleBar()
	case key.Matches(msg, Keys.Left):
		b.titleBarWidget = titleBarWidgetHelp
	case key.Matches(msg, Keys.Right):
		b.titleBarWidget = titleBarWidgetClose
	case key.Matches(msg, Keys.Enter), key.Matches(msg, Keys.Space):
		switch b.titleBarWidget {
		case titleBarWidgetHelp:
			b.BlurTitleBar()
			cmd = func() tea.Msg { return TriggerHelpMsg{ScreenLevelOnly: true} }
		case titleBarWidgetClose:
			b.BlurTitleBar()
			cmd = closeCmd
		}
	}
	return true, cmd
}

// titleBarHitRegions returns hit regions for the [?] and [×] widgets in the title bar.
func (b *baseDialogModel) titleBarHitRegions(offsetX, offsetY, contentWidth, baseZ int) []HitRegion {
	ctx := GetActiveContext()
	// Use GetPlainText to strip theme tags before measuring (large titlebar widgets are raw tags).
	widgetWidth := lipgloss.Width(GetPlainText(b.buildTitleBarWidgets(ctx)))
	if widgetWidth == 0 {
		return nil
	}
	dialogWidth := contentWidth + 2
	endPad := 1
	widgetsStartX := offsetX + dialogWidth - 1 - endPad - widgetWidth
	helpWidgetX := widgetsStartX
	// Small titlebar: "[?]─" = 4 chars; large titlebar: "[?] " = 4 chars. Both are +4.
	closeWidgetX := widgetsStartX + 4
	// Large titlebar: widgets are on the title row (row 1), not the top border (row 0).
	widgetY := offsetY
	if b.layout.LargeTitleBar {
		widgetY++
	}
	return []HitRegion{
		{
			ID:     b.id + "." + IDTitleWidgetHelp,
			X:      helpWidgetX, Y: widgetY, Width: 3, Height: 1,
			ZOrder: baseZ + 25,
			Label:  "Help",
			Help:   &HelpContext{PageTitle: "Help", PageText: "Open help for this dialog."},
		},
		{
			ID:     b.id + "." + IDTitleWidgetClose,
			X:      closeWidgetX, Y: widgetY, Width: 3, Height: 1,
			ZOrder: baseZ + 25,
			Label:  "Close",
			Help:   &HelpContext{PageTitle: "Close", PageText: "Close this dialog."},
		},
	}
}
