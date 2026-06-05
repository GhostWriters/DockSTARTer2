package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// widgetClearPressMsg is sent after the press-flash duration to clear the pressed state.
type widgetClearPressMsg struct{ id string }

const widgetPressDuration = 500 * time.Millisecond

// TitleBarWidget identifies a specific widget in the title bar.
type TitleBarWidget int

const (
	TitleBarWidgetNone    TitleBarWidget = 0
	TitleBarWidgetClose   TitleBarWidget = 1
	TitleBarWidgetHelp    TitleBarWidget = 2
	TitleBarWidgetRefresh TitleBarWidget = 3
)

// defaultWidgets is the widget set used when none is configured: [?]─[×].
var defaultWidgets = []TitleBarWidget{TitleBarWidgetHelp, TitleBarWidgetClose}

// TitleBarFocus is an embeddable struct that provides the TitleBarFocusable interface
// and key/hit handling for any dialog or screen with [?] and [×] title widgets.
// Embed this instead of duplicating the fields and methods.
// Call ConfigureWidgets to add optional widgets (e.g. TitleBarWidgetRefresh).
type TitleBarFocus struct {
	tbFocused bool
	tbWidget  TitleBarWidget
	tbPressed TitleBarWidget   // widget currently showing press flash
	tbWidgets []TitleBarWidget // ordered left-to-right; nil means defaultWidgets
}

// ConfigureWidgets sets the ordered list of widgets shown in the title bar (left to right).
// Must be called before FocusTitleBar is first called.
func (t *TitleBarFocus) ConfigureWidgets(widgets ...TitleBarWidget) {
	t.tbWidgets = widgets
}

func (t *TitleBarFocus) ActiveWidgets() []TitleBarWidget {
	if len(t.tbWidgets) > 0 {
		return t.tbWidgets
	}
	return defaultWidgets
}

func (t *TitleBarFocus) FocusTitleBar() {
	t.tbFocused = true
	w := t.ActiveWidgets()
	// Default focus: rightmost widget (Close).
	t.tbWidget = w[len(w)-1]
}

func (t *TitleBarFocus) BlurTitleBar() {
	t.tbFocused = false
	t.tbWidget = TitleBarWidgetNone
}

func (t *TitleBarFocus) TitleBarFocused() bool        { return t.tbFocused }
func (t *TitleBarFocus) ActiveWidget() TitleBarWidget { return t.tbWidget }
func (t *TitleBarFocus) SetWidget(w TitleBarWidget)   { t.tbWidget = w }
func (t *TitleBarFocus) PressedWidget() TitleBarWidget { return t.tbPressed }

// PressWidget sets the pressed flash state and returns a tea.Cmd that clears it after the duration.
func (t *TitleBarFocus) PressWidget(w TitleBarWidget, id string) tea.Cmd {
	t.tbPressed = w
	return tea.Tick(widgetPressDuration, func(time.Time) tea.Msg { return widgetClearPressMsg{id: id} })
}

// ClearPress clears the pressed flash state. Call when widgetClearPressMsg is received.
func (t *TitleBarFocus) ClearPress() { t.tbPressed = TitleBarWidgetNone }

// HandleWidgetClearPress checks if msg is a widgetClearPressMsg for this instance
// and clears the pressed state if so. Returns true if handled.
func (t *TitleBarFocus) HandleWidgetClearPress(msg tea.Msg) bool {
	if _, ok := msg.(widgetClearPressMsg); ok {
		t.ClearPress()
		return true
	}
	return false
}

func (t *TitleBarFocus) CycleWidget(dir int) {
	widgets := t.ActiveWidgets()
	for i, w := range widgets {
		if w == t.tbWidget {
			next := (i + dir + len(widgets)) % len(widgets)
			t.tbWidget = widgets[next]
			return
		}
	}
	t.tbWidget = widgets[len(widgets)-1]
}

// HandleTitleBarKey intercepts key events when the title bar has focus.
// closeCmd is the dialog-specific command to run when [×] is activated.
// Returns (true, cmd) if the key was consumed, (false, nil) otherwise.
func (t *TitleBarFocus) HandleTitleBarKey(msg tea.KeyPressMsg, closeCmd tea.Cmd) (handled bool, cmd tea.Cmd) {
	if !t.tbFocused {
		return false, nil
	}
	switch {
	case key.Matches(msg, Keys.Esc):
		t.BlurTitleBar()
	case key.Matches(msg, Keys.Left):
		t.CycleWidget(-1)
	case key.Matches(msg, Keys.Right):
		t.CycleWidget(+1)
	case key.Matches(msg, Keys.Enter), key.Matches(msg, Keys.Space):
		switch t.tbWidget {
		case TitleBarWidgetHelp:
			pressCmd := t.PressWidget(TitleBarWidgetHelp, "key")
			t.BlurTitleBar()
			cmd = tea.Batch(pressCmd, func() tea.Msg { return TriggerHelpMsg{ScreenLevelOnly: true} })
		case TitleBarWidgetClose:
			pressCmd := t.PressWidget(TitleBarWidgetClose, "key")
			t.BlurTitleBar()
			cmd = tea.Batch(pressCmd, closeCmd)
		case TitleBarWidgetRefresh:
			pressCmd := t.PressWidget(TitleBarWidgetRefresh, "key")
			t.BlurTitleBar()
			cmd = tea.Batch(pressCmd, func() tea.Msg { return TitleBarRefreshMsg{} })
		}
	}
	return true, cmd
}

// HandleTitleBarHit handles LayerHitMsg for the [?] and [×] widgets.
// closeCmd is the dialog-specific command to run when [×] is clicked.
// Returns (true, cmd) if the hit was consumed, (false, nil) otherwise.
func (t *TitleBarFocus) HandleTitleBarHit(msg LayerHitMsg, closeCmd tea.Cmd) (handled bool, cmd tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return false, nil
	}
	if strings.HasSuffix(msg.ID, "."+IDTitleWidgetClose) {
		pressCmd := t.PressWidget(TitleBarWidgetClose, msg.ID)
		t.BlurTitleBar()
		return true, tea.Batch(pressCmd, closeCmd)
	}
	if strings.HasSuffix(msg.ID, "."+IDTitleWidgetHelp) {
		pressCmd := t.PressWidget(TitleBarWidgetHelp, msg.ID)
		t.BlurTitleBar()
		return true, tea.Batch(pressCmd, func() tea.Msg { return TriggerHelpMsg{ScreenLevelOnly: true} })
	}
	if strings.HasSuffix(msg.ID, "."+IDTitleWidgetRefresh) {
		pressCmd := t.PressWidget(TitleBarWidgetRefresh, msg.ID)
		t.BlurTitleBar()
		return true, tea.Batch(pressCmd, func() tea.Msg { return TitleBarRefreshMsg{} })
	}
	return false, nil
}

// buildTitleBarWidgets returns the rendered [?]─[×] widget string for this dialog,
// with the correct active/inactive styling based on current title bar focus state.
// Uses large or small widget styles depending on ctx.LargeTitleBars.
func (b *baseDialogModel) buildTitleBarWidgets(ctx StyleContext) string {
	if ctx.LargeTitleBars {
		return buildLargeTitleBarWidgets(b.tbFocused, b.tbWidget, b.tbPressed, b.ActiveWidgets(), ctx)
	}
	return buildDialogTitleWidgets(b.tbFocused, b.tbWidget, b.tbPressed, b.ActiveWidgets(), ctx)
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

// hasWidget reports whether the given widget is in the list.
func hasWidget(widgets []TitleBarWidget, w TitleBarWidget) bool {
	for _, v := range widgets {
		if v == w {
			return true
		}
	}
	return false
}

// BuildInactiveTitleWidgets builds the title widget string using inactive styles only.
// Automatically selects large or small widget style based on ctx.LargeTitleBars.
func BuildInactiveTitleWidgets(ctx StyleContext) string {
	return BuildInactiveTitleWidgetsFor(defaultWidgets, ctx)
}

// BuildInactiveTitleWidgetsFor builds the title widget string for a specific widget set.
func BuildInactiveTitleWidgetsFor(widgets []TitleBarWidget, ctx StyleContext) string {
	if ctx.LargeTitleBars {
		return buildLargeTitleBarWidgets(false, TitleBarWidgetNone, TitleBarWidgetNone, widgets, ctx)
	}
	return buildDialogTitleWidgets(false, TitleBarWidgetNone, TitleBarWidgetNone, widgets, ctx)
}

// BuildInactiveLargeTitleWidgets builds the widget string styled for the large titlebar row.
func BuildInactiveLargeTitleWidgets(ctx StyleContext) string {
	return buildLargeTitleBarWidgets(false, TitleBarWidgetNone, TitleBarWidgetNone, defaultWidgets, ctx)
}

// buildLargeTitleBarWidgets returns the raw theme-tag string for the title bar widgets.
// It is NOT pre-rendered — callers must render it via RenderThemeTextCtx in the correct
// area context so it picks up the type-specific LargeTitleArea* background.
func buildLargeTitleBarWidgets(focused bool, activeWidget, pressedWidget TitleBarWidget, widgets []TitleBarWidget, ctx StyleContext) string {
	helpGlyph := helpWidget
	closeGlyph := closeWidget
	refreshGlyph := refreshWidget
	if !ctx.LineCharacters {
		closeGlyph = closeWidgetAscii
		refreshGlyph = refreshWidgetAscii
	}
	isActive  := func(w TitleBarWidget) bool { return pressedWidget == w || (focused && activeWidget == w) }
	isPressed := func(w TitleBarWidget) bool { return pressedWidget == w }

	var parts []string
	if hasWidget(widgets, TitleBarWidgetRefresh) {
		prefix := "{{|LargeRefreshIconInactive|}}"
		if isPressed(TitleBarWidgetRefresh) {
			prefix += "{{|LargeIconPressed|}}"
		} else if isActive(TitleBarWidgetRefresh) {
			prefix += "{{|LargeIconFocused|}}"
		}
		parts = append(parts, prefix+"["+refreshGlyph+"]{{[-]}}")
	}
	if hasWidget(widgets, TitleBarWidgetHelp) {
		prefix := "{{|LargeHelpIconInactive|}}"
		if isPressed(TitleBarWidgetHelp) {
			prefix += "{{|LargeIconPressed|}}"
		} else if isActive(TitleBarWidgetHelp) {
			prefix += "{{|LargeIconFocused|}}"
		}
		parts = append(parts, prefix+"["+helpGlyph+"]{{[-]}}")
	}
	if hasWidget(widgets, TitleBarWidgetClose) {
		prefix := "{{|LargeExitIconInactive|}}"
		if isPressed(TitleBarWidgetClose) {
			prefix += "{{|LargeIconPressed|}}"
		} else if isActive(TitleBarWidgetClose) {
			prefix += "{{|LargeIconFocused|}}"
		}
		parts = append(parts, prefix+"["+closeGlyph+"]{{[-]}}")
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}

// buildDialogTitleWidgets is the shared renderer for the title bar widgets.
// focused/activeWidget are the title bar state; use TitleBarWidgetNone for always-inactive output.
func buildDialogTitleWidgets(focused bool, activeWidget, pressedWidget TitleBarWidget, widgets []TitleBarWidget, ctx StyleContext) string {
	helpGlyph := helpWidget
	closeGlyph := closeWidget
	refreshGlyph := refreshWidget
	lineChar := "─"
	if !ctx.LineCharacters {
		closeGlyph = closeWidgetAscii
		refreshGlyph = refreshWidgetAscii
		lineChar = "-"
	}
	borderBase := ctx.BorderFlags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.BorderColor).
		Background(ctx.Dialog.GetBackground())
	isPressed := func(w TitleBarWidget) bool { return pressedWidget == w }
	isActive  := func(w TitleBarWidget) bool { return pressedWidget == w || (focused && activeWidget == w) }

	var parts []string
	if hasWidget(widgets, TitleBarWidgetRefresh) {
		prefix := "{{|RefreshIconInactive|}}"
		if isPressed(TitleBarWidgetRefresh) {
			prefix += "{{|IconPressed|}}"
		} else if isActive(TitleBarWidgetRefresh) {
			prefix += "{{|IconFocused|}}"
		}
		parts = append(parts, prefix+"["+refreshGlyph+"]{{[-]}}")
	}
	if hasWidget(widgets, TitleBarWidgetHelp) {
		prefix := "{{|HelpIconInactive|}}"
		if isPressed(TitleBarWidgetHelp) {
			prefix += "{{|IconPressed|}}"
		} else if isActive(TitleBarWidgetHelp) {
			prefix += "{{|IconFocused|}}"
		}
		parts = append(parts, prefix+"["+helpGlyph+"]{{[-]}}")
	}
	if hasWidget(widgets, TitleBarWidgetClose) {
		prefix := "{{|ExitIconInactive|}}"
		if isPressed(TitleBarWidgetClose) {
			prefix += "{{|IconPressed|}}"
		} else if isActive(TitleBarWidgetClose) {
			prefix += "{{|IconFocused|}}"
		}
		parts = append(parts, prefix+"["+closeGlyph+"]{{[-]}}")
	}

	iconStr := ""
	for i, p := range parts {
		if i > 0 {
			iconStr += lineChar
		}
		iconStr += p
	}
	ctx.Dialog = borderBase
	return RenderThemeTextCtx(iconStr, ctx)
}

// handleTitleBarHit is a convenience alias so existing baseDialogModel callers compile unchanged.
func (b *baseDialogModel) handleTitleBarHit(msg LayerHitMsg, closeCmd tea.Cmd) (bool, tea.Cmd) {
	return b.HandleTitleBarHit(msg, closeCmd)
}

// handleTitleBarKey is a convenience alias so existing baseDialogModel callers compile unchanged.
func (b *baseDialogModel) handleTitleBarKey(msg tea.KeyPressMsg, closeCmd tea.Cmd) (bool, tea.Cmd) {
	return b.HandleTitleBarKey(msg, closeCmd)
}

// titleBarHitRegions returns hit regions for the title bar widgets.
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
	return TitleBarWidgetRegions(b.id, b.ActiveWidgets(), widgetsStartX, TitleBarWidgetY(offsetY, b.layout.LargeTitleBar), baseZ)
}

// TitleBarWidgetY returns the screen Y coordinate for title bar widgets.
// For small titlebars the widgets are on the top border row (offsetY).
// For large titlebars they are on the title content row (offsetY+1).
func TitleBarWidgetY(offsetY int, largeTitleBar bool) int {
	if largeTitleBar {
		return offsetY + 1
	}
	return offsetY
}

// TitleBarWidgetHelper is implemented by models that can report the ID suffix of their
// currently focused title bar widget. Used by F1 to find the matching HitRegion help.
type TitleBarWidgetHelper interface {
	FocusedWidgetID() string
}

// FocusedWidgetID returns the hit-region ID suffix for the currently focused widget
// (e.g. IDTitleWidgetClose), or "" if no widget is focused.
func (t *TitleBarFocus) FocusedWidgetID() string {
	if !t.tbFocused || t.tbWidget == TitleBarWidgetNone {
		return ""
	}
	switch t.tbWidget {
	case TitleBarWidgetRefresh:
		return IDTitleWidgetRefresh
	case TitleBarWidgetHelp:
		return IDTitleWidgetHelp
	case TitleBarWidgetClose:
		return IDTitleWidgetClose
	}
	return ""
}

// IsTitleWidgetID reports whether the hit region ID suffix matches any title bar widget.
func IsTitleWidgetID(id string) bool {
	return strings.HasSuffix(id, "."+IDTitleWidgetHelp) ||
		strings.HasSuffix(id, "."+IDTitleWidgetClose) ||
		strings.HasSuffix(id, "."+IDTitleWidgetRefresh)
}

// TitleBarWidgetRegions builds HitRegions for a set of widgets starting at x, y.
// Each widget is 3 chars wide; widgets are separated by 1 char (separator or space).
func TitleBarWidgetRegions(id string, widgets []TitleBarWidget, startX, y, baseZ int) []HitRegion {
	var regions []HitRegion
	x := startX
	for _, w := range widgets {
		var hitID, label, helpText string
		switch w {
		case TitleBarWidgetRefresh:
			hitID, label, helpText = IDTitleWidgetRefresh, "Refresh", "Refresh the current view."
		case TitleBarWidgetHelp:
			hitID, label, helpText = IDTitleWidgetHelp, "Help", "Open help for this dialog."
		case TitleBarWidgetClose:
			hitID, label, helpText = IDTitleWidgetClose, "Close", "Close this dialog."
		default:
			x += 4
			continue
		}
		regions = append(regions, HitRegion{
			ID:     id + "." + hitID,
			X:      x, Y: y, Width: 3, Height: 1,
			ZOrder: baseZ + 25,
			Label:  label,
			Help:   &HelpContext{PageTitle: label, PageText: helpText},
		})
		x += 4 // [X] = 3 chars + 1 separator/space
	}
	return regions
}
