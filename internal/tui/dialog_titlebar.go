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

// WidgetDef describes a single title bar widget (button).
type WidgetDef struct {
	ID                 string         // hit region ID suffix, e.g. IDTitleWidgetClose
	Label              string         // human label for hit region
	HelpText           string         // help tooltip
	Glyph              string         // unicode glyph e.g. "x"
	GlyphAscii         string         // ASCII fallback e.g. "X"
	ThemeInactive      string         // small titlebar inactive theme tag
	ThemeActive        string         // small titlebar active/focused theme tag
	ThemePressed       string         // small titlebar pressed theme tag
	LargeThemeInactive string         // large titlebar inactive theme tag
	LargeThemeActive   string         // large titlebar active theme tag
	LargeThemePressed  string         // large titlebar pressed theme tag
	Action             func() tea.Cmd // nil = no action; caller supplies closeCmd for Close widget
}

// Pre-built widget vars -- replace the old TitleBarWidget enum constants.
var (
	WidgetClose = WidgetDef{
		ID:                 IDTitleWidgetClose,
		Label:              "Close",
		HelpText:           "Close this dialog.",
		Glyph:              closeWidget,
		GlyphAscii:         closeWidgetAscii,
		ThemeInactive:      "{{|ExitIconInactive|}}",
		ThemeActive:        "{{|IconFocused|}}",
		ThemePressed:       "{{|IconPressed|}}",
		LargeThemeInactive: "{{|LargeExitIconInactive|}}",
		LargeThemeActive:   "{{|LargeIconFocused|}}",
		LargeThemePressed:  "{{|LargeIconPressed|}}",
		Action:             nil, // caller supplies closeCmd
	}

	WidgetHelp = WidgetDef{
		ID:                 IDTitleWidgetHelp,
		Label:              "Help",
		HelpText:           "Open help for this dialog.",
		Glyph:              helpWidget,
		GlyphAscii:         helpWidget, // no ASCII fallback in original
		ThemeInactive:      "{{|HelpIconInactive|}}",
		ThemeActive:        "{{|IconFocused|}}",
		ThemePressed:       "{{|IconPressed|}}",
		LargeThemeInactive: "{{|LargeHelpIconInactive|}}",
		LargeThemeActive:   "{{|LargeIconFocused|}}",
		LargeThemePressed:  "{{|LargeIconPressed|}}",
		Action: func() tea.Cmd {
			return func() tea.Msg { return TriggerHelpMsg{ScreenLevelOnly: true} }
		},
	}

	WidgetRefresh = WidgetDef{
		ID:                 IDTitleWidgetRefresh,
		Label:              "Refresh",
		HelpText:           "Refresh the current view.",
		Glyph:              refreshWidget,
		GlyphAscii:         refreshWidgetAscii,
		ThemeInactive:      "{{|RefreshIconInactive|}}",
		ThemeActive:        "{{|IconFocused|}}",
		ThemePressed:       "{{|IconPressed|}}",
		LargeThemeInactive: "{{|LargeRefreshIconInactive|}}",
		LargeThemeActive:   "{{|LargeIconFocused|}}",
		LargeThemePressed:  "{{|LargeIconPressed|}}",
		Action: func() tea.Cmd {
			return func() tea.Msg { return TitleBarRefreshMsg{} }
		},
	}
)

// defaultWidgets is the widget set used when none is configured: [?]-[x].
var defaultWidgets = []WidgetDef{WidgetHelp, WidgetClose}

// findWidget returns a pointer to the matching widget in the list, or nil.
func findWidget(widgets []WidgetDef, id string) *WidgetDef {
	for i := range widgets {
		if widgets[i].ID == id {
			return &widgets[i]
		}
	}
	return nil
}

// TitleBarFocus is an embeddable struct that provides the TitleBarFocusable interface
// and key/hit handling for any dialog or screen with [?] and [x] title widgets.
// Embed this instead of duplicating the fields and methods.
// Call ConfigureWidgets to add optional widgets (e.g. WidgetRefresh).
// Convention: Close always last (rightmost).
type TitleBarFocus struct {
	tbFocused bool
	tbWidget  string      // focused widget ID, "" = none
	tbPressed string      // widget currently showing press flash (widget ID)
	tbWidgets []WidgetDef // ordered left-to-right; nil means defaultWidgets
}

// ConfigureWidgets sets the ordered list of widgets shown in the title bar (left to right).
// Must be called before FocusTitleBar is first called.
func (t *TitleBarFocus) ConfigureWidgets(widgets ...WidgetDef) {
	t.tbWidgets = widgets
}

func (t *TitleBarFocus) ActiveWidgets() []WidgetDef {
	if len(t.tbWidgets) > 0 {
		return t.tbWidgets
	}
	return defaultWidgets
}

func (t *TitleBarFocus) FocusTitleBar() {
	t.tbFocused = true
	w := t.ActiveWidgets()
	// Default focus: rightmost widget (Close).
	t.tbWidget = w[len(w)-1].ID
}

func (t *TitleBarFocus) BlurTitleBar() {
	t.tbFocused = false
	t.tbWidget = ""
}

func (t *TitleBarFocus) TitleBarFocused() bool { return t.tbFocused }
func (t *TitleBarFocus) ActiveWidget() string   { return t.tbWidget }
func (t *TitleBarFocus) SetWidget(id string)    { t.tbWidget = id }
func (t *TitleBarFocus) PressedWidget() string  { return t.tbPressed }

// State returns the TitleBarState for rendering, populated from this
// TitleBarFocus's current focus/widget state. Show defaults to true;
// callers needing Show:false should set it after: tbs := t.State(); tbs.Show = false.
// Widgets is left nil (render code falls back to defaultWidgets) unless a
// custom widget set was configured via ConfigureWidgets.
func (t *TitleBarFocus) State() TitleBarState {
	return TitleBarState{
		Show:          true,
		Focused:       t.tbFocused,
		ActiveWidget:  t.tbWidget,
		PressedWidget: t.tbPressed,
		Widgets:       t.tbWidgets,
	}
}

// PressWidget sets the pressed flash state and returns a tea.Cmd that clears it after the duration.
func (t *TitleBarFocus) PressWidget(w WidgetDef, id string) tea.Cmd {
	t.tbPressed = w.ID
	return tea.Tick(widgetPressDuration, func(time.Time) tea.Msg { return widgetClearPressMsg{id: id} })
}

// ClearPress clears the pressed flash state. Call when widgetClearPressMsg is received.
func (t *TitleBarFocus) ClearPress() { t.tbPressed = "" }

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
		if w.ID == t.tbWidget {
			next := (i + dir + len(widgets)) % len(widgets)
			t.tbWidget = widgets[next].ID
			return
		}
	}
	t.tbWidget = widgets[len(widgets)-1].ID
}

// HandleTitleBarKey intercepts key events when the title bar has focus.
// closeCmd is the dialog-specific command to run when [x] is activated.
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
		w := findWidget(t.ActiveWidgets(), t.tbWidget)
		if w != nil {
			pressCmd := t.PressWidget(*w, "key")
			t.BlurTitleBar()
			if w.Action != nil {
				cmd = tea.Batch(pressCmd, w.Action())
			} else if w.ID == IDTitleWidgetClose {
				cmd = tea.Batch(pressCmd, closeCmd)
			} else {
				cmd = pressCmd
			}
		}
	}
	return true, cmd
}

// HandleTitleBarHit handles LayerHitMsg for the [?] and [x] widgets.
// closeCmd is the dialog-specific command to run when [x] is clicked.
// Returns (true, cmd) if the hit was consumed, (false, nil) otherwise.
func (t *TitleBarFocus) HandleTitleBarHit(msg LayerHitMsg, closeCmd tea.Cmd) (handled bool, cmd tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return false, nil
	}
	for _, w := range t.ActiveWidgets() {
		if strings.HasSuffix(msg.ID, "."+w.ID) {
			pressCmd := t.PressWidget(w, msg.ID)
			t.BlurTitleBar()
			if w.Action != nil {
				return true, tea.Batch(pressCmd, w.Action())
			} else if w.ID == IDTitleWidgetClose {
				return true, tea.Batch(pressCmd, closeCmd)
			}
			return true, pressCmd
		}
	}
	return false, nil
}

// buildTitleBarWidgets returns the rendered [?]-[x] widget string for this dialog,
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

// BuildInactiveTitleWidgets builds the title widget string using inactive styles only.
// Automatically selects large or small widget style based on ctx.LargeTitleBars.
func BuildInactiveTitleWidgets(ctx StyleContext) string {
	return BuildInactiveTitleWidgetsFor(defaultWidgets, ctx)
}

// BuildInactiveTitleWidgetsFor builds the title widget string for a specific widget set.
func BuildInactiveTitleWidgetsFor(widgets []WidgetDef, ctx StyleContext) string {
	if ctx.LargeTitleBars {
		return buildLargeTitleBarWidgets(false, "", "", widgets, ctx)
	}
	return buildDialogTitleWidgets(false, "", "", widgets, ctx)
}

// BuildInactiveLargeTitleWidgets builds the widget string styled for the large titlebar row.
func BuildInactiveLargeTitleWidgets(ctx StyleContext) string {
	return buildLargeTitleBarWidgets(false, "", "", defaultWidgets, ctx)
}

// buildLargeTitleBarWidgets returns the raw theme-tag string for the title bar widgets.
// It is NOT pre-rendered -- callers must render it via RenderThemeTextCtx in the correct
// area context so it picks up the type-specific LargeTitleArea* background.
func buildLargeTitleBarWidgets(focused bool, activeWidget, pressedWidget string, widgets []WidgetDef, ctx StyleContext) string {
	isActive  := func(id string) bool { return pressedWidget == id || (focused && activeWidget == id) }
	isPressed := func(id string) bool { return pressedWidget == id }

	var parts []string
	for _, w := range widgets {
		glyph := w.Glyph
		if !ctx.LineCharacters && w.GlyphAscii != "" {
			glyph = w.GlyphAscii
		}
		prefix := w.LargeThemeInactive
		if isPressed(w.ID) {
			prefix += w.LargeThemePressed
		} else if isActive(w.ID) {
			prefix += w.LargeThemeActive
		}
		parts = append(parts, prefix+"["+glyph+"]{{[-]}}")
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
// focused/activeWidget are the title bar state; use "" for always-inactive output.
func buildDialogTitleWidgets(focused bool, activeWidget, pressedWidget string, widgets []WidgetDef, ctx StyleContext) string {
	lineChar := "─"
	if !ctx.LineCharacters {
		lineChar = "-"
	}
	borderBase := ctx.BorderFlags.Apply(lipgloss.NewStyle()).
		Foreground(ctx.BorderColor).
		Background(ctx.Dialog.GetBackground())
	isPressed := func(id string) bool { return pressedWidget == id }
	isActive  := func(id string) bool { return pressedWidget == id || (focused && activeWidget == id) }

	var parts []string
	for _, w := range widgets {
		glyph := w.Glyph
		if !ctx.LineCharacters && w.GlyphAscii != "" {
			glyph = w.GlyphAscii
		}
		prefix := w.ThemeInactive
		if isPressed(w.ID) {
			prefix += w.ThemePressed
		} else if isActive(w.ID) {
			prefix += w.ThemeActive
		}
		parts = append(parts, prefix+"["+glyph+"]{{[-]}}")
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
	if !t.tbFocused || t.tbWidget == "" {
		return ""
	}
	return t.tbWidget
}

// IsTitleWidgetID reports whether the hit region ID suffix matches any title bar widget.
func IsTitleWidgetID(id string) bool {
	return strings.HasSuffix(id, "."+IDTitleWidgetHelp) ||
		strings.HasSuffix(id, "."+IDTitleWidgetClose) ||
		strings.HasSuffix(id, "."+IDTitleWidgetRefresh)
}

// TitleBarWidgetRegions builds HitRegions for a set of widgets starting at x, y.
// Each widget is 3 chars wide; widgets are separated by 1 char (separator or space).
func TitleBarWidgetRegions(id string, widgets []WidgetDef, startX, y, baseZ int) []HitRegion {
	var regions []HitRegion
	x := startX
	for _, w := range widgets {
		regions = append(regions, HitRegion{
			ID:     id + "." + w.ID,
			X:      x, Y: y, Width: 3, Height: 1,
			ZOrder: baseZ + 25,
			Label:  w.Label,
			Help:   &HelpContext{PageTitle: w.Label, PageText: w.HelpText},
		})
		x += 4 // [X] = 3 chars + 1 separator/space
	}
	return regions
}

// PressWidgetID sets the pressed flash state using a raw widget ID (for non-WidgetDef callers)
// and returns a tea.Cmd that clears it after the duration.
func (t *TitleBarFocus) PressWidgetID(widgetID, hitID string) tea.Cmd {
	t.tbPressed = widgetID
	return tea.Tick(widgetPressDuration, func(time.Time) tea.Msg { return widgetClearPressMsg{id: hitID} })
}
