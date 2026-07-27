package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/tui/components/enveditor"
	"DockSTARTer2/internal/version"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type envFocusArea int

const (
	envFocusEditor envFocusArea = iota
	envFocusButtons
)

// envLayoutMode controls how open tabs are displayed. With 0 or 1 tabs
// there's nothing to split, so it's always effectively maximized.
type envLayoutMode int

const (
	envLayoutMaximized envLayoutMode = iota
	envLayoutSideBySide
	envLayoutStacked
)

// minPaneContentWidth/minPaneEditorHeight are the smallest a split pane can
// get before SetSize auto-collapses to Maximized rendering for this size,
// without changing the user's chosen layoutMode.
const (
	minPaneContentWidth = 30
	minPaneEditorHeight = 4
)

// envSetLayoutMsg is dispatched by the Maximize/Side-by-side/Stacked title
// bar widgets to change layoutMode.
type envSetLayoutMsg struct {
	mode envLayoutMode
}

// headingLabelW is the fixed label column width, matching bash menu_heading.sh's
// LabelWidth which is the max across ALL possible labels: "Original Value: " = 16.
// Using the maximum keeps values aligned at the same column across all screens.
const headingLabelW = menuLabelW

// headingLabel right-aligns label to headingLabelW (e.g. "    Variable: ").
func headingLabel(label string) string {
	return fmt.Sprintf("%*s", headingLabelW, label)
}

type EnvTabSpec struct {
	Title    string
	App      string // Empty string for global vars, app name for app-specific vars
	IsGlobal bool   // Indicates if this tab edits the global .env file (potentially filtered by App)
}

type envTab struct {
	spec            EnvTabSpec
	editor          enveditor.Model
	initialVars     map[string]string // Captured when loaded, used for scoped syncing on save
	defaultFilePath string            // Cached for Refresh
	defaultLines    []string          // Template lines cached in memory (avoids re-reading on F5)
	composeEnvPath  string            // Path to the main .env file (for metadata lookups on F5)
	readOnlyVars    []string          // Cached for Refresh
	// Cached heading display info (populated during loadEnv)
	envFilePath      string          // Actual .env file being edited
	niceName         string          // App nicename (empty for global tabs)
	description      string          // App description (empty for global tabs or if unavailable)
	appMeta          *appenv.AppMeta // Optional metadata from appname.meta.toml (nil if not present)
	lastEnabledState string          // "active", "disabled", or "absent" — triggers auto-refresh on change
}

// defaultVal returns the computed default for any variable name via DefaultValueFunc
// (which calls VarDefaultValue — the single source of truth, same as the bash version).
func (t *envTab) defaultVal(key string) string {
	if t.editor.DefaultValueFunc != nil {
		return t.editor.DefaultValueFunc(key)
	}
	return ""
}

type TabbedVarsEditorModel struct {
	tabs      []envTab
	activeTab int

	width  int
	height int
	title  string

	focus envFocusArea

	// Action buttons
	buttons            []string
	btnIdx             int
	buttonHeight       int
	subtitleHeight     int
	largeTitleOverhead int
	editorHeight       int
	contentWidth       int // per-pane editor box width -- see SetSize
	fullContentWidth   int // full dialog content width, spans both panes when split -- used by buttons/outer border
	focused            bool

	// layoutMode is the user's chosen view (Maximize/Side-by-side/Stacked);
	// splitMode is SetSize's actual per-render decision, which falls back to
	// false when the current size is too small for the chosen split, same
	// collapse-and-spring-back as the Appearance Settings preview panel.
	layoutMode envLayoutMode
	splitMode  bool

	// pane1OffsetX/Y is where tabs[1]'s pane starts, relative to tabs[0]'s.
	// activePaneOffsetX/Y is that offset when tab 1 is active, else (0,0) --
	// folded into lastOffsetX/Y by GetHitRegions so existing single-pane
	// cursor/click/context-menu math keeps working unmodified.
	pane1OffsetX      int
	pane1OffsetY      int
	activePaneOffsetX int
	activePaneOffsetY int

	// dialogOffsetX/Y is the dialog's raw screen origin (unshifted by any
	// pane offset), for raw mouse events (drag clicks, wheel) that carry
	// absolute coordinates but no hit-region ID. See paneBoxAt/editorRelCoords.
	dialogOffsetX int
	dialogOffsetY int

	// paneTitleFocused/paneActiveWidget track keyboard focus on the active
	// pane's own border widgets, a level below the dialog's own title bar,
	// cycled by CyclePaneTitleFocus (F9/Ctrl+T).
	paneTitleFocused bool
	paneActiveWidget string

	// Callbacks
	onClose tea.Cmd

	// Last hit region offsets for coordinate translation
	lastOffsetX int
	lastOffsetY int

	lockedByOthers bool
	connType       string

	// Title bar focus state
	displayengine.TitleBarFocus

	// Spinner while env data is loading from disk
	loading      bool
	titleSpinner displayengine.TitleSpinner

	btnRow *displayengine.ButtonRow
}

// ClearProcessingState clears any in-flight button spinner. Called by
// AppModel when a dialog closes and returns focus to this screen (e.g. the
// Exit or Back confirm dialog resolving) -- without this, a button's
// spinner started via btnRow.SetProcessing keeps running forever once its
// action resolves without navigating away.
func (m *TabbedVarsEditorModel) ClearProcessingState() {
	if m.btnRow != nil {
		m.btnRow.Clear()
	}
}

// AdvanceSpinners advances the loading title spinner and the button-row
// spinner if their intervals have elapsed. Returns true if either frame
// changed. Called by the global tick via globalTickMsg.
func (m *TabbedVarsEditorModel) AdvanceSpinners(now time.Time) bool {
	changed := m.btnRow != nil && m.btnRow.AdvanceSpinner(now)
	if m.titleSpinner.AdvanceSpinner(now) {
		changed = true
	}
	return changed
}

func (m *TabbedVarsEditorModel) currentSpinnerIndicators() (left, right string) {
	return m.titleSpinner.Indicators()
}

type envAddVarMsg struct {
	key string
}

type envAddVarTemplateMsg struct {
	prefix string
}

type envAddAllStockMsg struct {
	vars     []string
	defaults map[string]string
}

type envSaveSuccessMsg struct{}

// ApplyVarValueMsg is dispatched by the context menu to set a variable's value in the active editor.
type ApplyVarValueMsg struct {
	VarName string
	Value   string
}

// deleteVarMsg is dispatched by the context menu to delete a variable line from the active editor.
type deleteVarMsg struct {
	VarName string
}

// restoreVarMsg is dispatched by the context menu or keyboard shortcut to undelete a pending-delete line.
type restoreVarMsg struct {
	VarName string
}

// envRefreshMsg triggers the same staged reformat as F5.
// preservePendingDeletes keeps staged deletions intact — used for auto-refresh
// triggered by ENABLED changes so the user doesn't lose staged deletions silently.
// Manual F5 and context-menu Refresh set it false (explicit re-sync).
type envRefreshMsg struct {
	preservePendingDeletes bool
}

func NewEnvEditorGlobal(onClose tea.Cmd, showBack bool, connType string) *TabbedVarsEditorModel {
	return NewTabbedVarsEditorScreen(onClose, "Global Variables", []EnvTabSpec{
		{Title: ".env", App: "", IsGlobal: true},
	}, showBack, connType)
}

func NewTabbedVarsEditorScreen(onClose tea.Cmd, title string, specs []EnvTabSpec, showBack bool, connType string) *TabbedVarsEditorModel {
	var tabs []envTab
	for _, spec := range specs {
		editor := enveditor.New()
		editor.ShowLineNumbers = true
		editor.SetLineCharacters(displayengine.GetActiveContext().LineCharacters)
		editor.LineNumberBrackets = displayengine.GetActiveContext().LineNumberBrackets
		editor.LineNumberBracketOpen, editor.LineNumberBracketClose = displayengine.TagBracketGlyphs()
		editor.SetVirtualCursor(false)
		editor.ScrollbarFunc = func(content string, total, visible, offset int, lineChars bool) string {
			return displayengine.ApplyScrollbarColumn(content, total, visible, offset, lineChars, displayengine.GetActiveContext())
		}
		tabs = append(tabs, envTab{spec: spec, editor: editor})
	}

	buttons := []string{"Save", "Refresh", "Back", "Exit"}
	if !showBack {
		buttons = []string{"Save", "Refresh", "Exit"}
	}

	m := &TabbedVarsEditorModel{
		tabs:      tabs,
		activeTab: 0,
		title:     title,
		buttons:   buttons,
		btnIdx:    0,
		focus:     envFocusEditor,
		onClose:   onClose,
		connType:  connType,
	}
	zoneByName := map[string]string{
		"Save": displayengine.IDSaveButton,
		"Back": displayengine.IDBackButton,
		"Exit": displayengine.IDExitButton,
	}
	defs := make([]displayengine.ButtonDef, len(buttons))
	for i, btn := range buttons {
		defs[i] = displayengine.ButtonDef{Label: btn, ZoneID: zoneByName[btn]}
	}
	m.btnRow = displayengine.NewButtonRow(defs)
	// Maximize/Side-by-side/Stacked live on the pane's own (submenu-style)
	// border, not here -- see renderPane/paneLayoutWidgets. The main dialog
	// title bar only ever gets the standard Refresh/Help/Close set.
	m.ConfigureWidgets(displayengine.WidgetRefresh, displayengine.WidgetHelp, displayengine.WidgetClose)

	if len(tabs) == 2 {
		switch config.LoadAppConfig().UI.TabLayout {
		case "sidebyside":
			m.layoutMode = envLayoutSideBySide
		case "stacked":
			m.layoutMode = envLayoutStacked
		default:
			m.layoutMode = envLayoutMaximized
		}
	}
	return m
}

// envLayoutSetAction returns a Cmd that dispatches envSetLayoutMsg to switch
// to the given layout mode.
func envLayoutSetAction(mode envLayoutMode) func() tea.Cmd {
	return func() tea.Cmd {
		return func() tea.Msg { return envSetLayoutMsg{mode: mode} }
	}
}

// envLayoutWidget builds a title-bar widget that switches to the given
// layout mode when activated. Shares the Refresh icon's theme tags rather
// than defining new ones across every theme file.
func envLayoutWidget(id, label, help, glyph, glyphAscii string, mode envLayoutMode) displayengine.WidgetDef {
	return displayengine.WidgetDef{
		ID:                 id,
		Label:              label,
		HelpText:           help,
		Glyph:              glyph,
		GlyphAscii:         glyphAscii,
		ThemeInactive:      "{{|RefreshIconInactive|}}",
		ThemeActive:        "{{|IconFocused|}}",
		ThemePressed:       "{{|IconPressed|}}",
		LargeThemeInactive: "{{|LargeRefreshIconInactive|}}",
		LargeThemeActive:   "{{|LargeIconFocused|}}",
		LargeThemePressed:  "{{|LargeIconPressed|}}",
		Action:             envLayoutSetAction(mode),
	}
}

// maximizeWidget builds the per-pane Maximize icon -- clicking it maximizes
// to that specific pane's tab. Its ID alone doesn't say which pane; Update
// recovers that from the "tabbed_vars.paneN." hit-region ID prefix.
func maximizeWidget() displayengine.WidgetDef {
	return envLayoutWidget(displayengine.IDTitleWidgetMaximize, "Maximize", "Show only this tab, full size.", "□", "+", envLayoutMaximized)
}

// paneLayoutWidgets returns the layout-control widgets shown on both panes'
// borders -- only meaningful with exactly 2 tabs open; nil otherwise. Maximize
// is last (rightmost, matching Close's convention) and omitted in Maximized
// mode. Only offers the split direction not already active. Keyed off
// layoutMode, not splitMode, so the icons stay consistent with what's
// actually selected even while temporarily auto-collapsed (see SetSize).
func (m *TabbedVarsEditorModel) paneLayoutWidgets() []displayengine.WidgetDef {
	if len(m.tabs) != 2 {
		return nil
	}
	sideBySide := envLayoutWidget(displayengine.IDTitleWidgetSideBySide, "Side by side", "Show both open tabs side by side.", "▥", "|", envLayoutSideBySide)
	stacked := envLayoutWidget(displayengine.IDTitleWidgetStacked, "Stacked", "Show both open tabs stacked vertically.", "▤", "-", envLayoutStacked)
	switch m.layoutMode {
	case envLayoutMaximized:
		return []displayengine.WidgetDef{sideBySide, stacked}
	case envLayoutSideBySide:
		return []displayengine.WidgetDef{stacked, maximizeWidget()}
	default: // envLayoutStacked
		return []displayengine.WidgetDef{sideBySide, maximizeWidget()}
	}
}

// paneOffsetFor returns pane idx's top-left offset relative to pane 0 --
// (0,0) unless split and idx is the second pane.
func (m *TabbedVarsEditorModel) paneOffsetFor(idx int) (x, y int) {
	if idx == 1 && m.splitMode {
		return m.pane1OffsetX, m.pane1OffsetY
	}
	return 0, 0
}

// paneBoxAt returns which pane's box contains the given absolute screen
// coordinates -- for raw mouse events with coordinates but no hit-region ID
// (scrollbar drag clicks, raw tea.MouseWheelMsg). Always m.activeTab when
// not split.
func (m *TabbedVarsEditorModel) paneBoxAt(x, y int) (idx int, ok bool) {
	if !m.splitMode {
		return m.activeTab, true
	}
	layout := displayengine.GetLayout()
	boxTop := m.dialogOffsetY + 1 + m.largeTitleOverhead + m.subtitleHeight
	boxHeight := m.editorHeight + layout.BorderHeight()
	for i := 0; i < 2; i++ {
		offX, offY := m.paneOffsetFor(i)
		bx, by := m.dialogOffsetX+offX, boxTop+offY
		if x >= bx && x < bx+m.contentWidth && y >= by && y < by+boxHeight {
			return i, true
		}
	}
	return m.activeTab, false
}

// editorRelCoords translates absolute screen coordinates into pane idx's
// editor-relative coordinates -- works immediately after switching which
// pane is active, without waiting for the next GetHitRegions call to
// refresh lastOffsetX/Y.
func (m *TabbedVarsEditorModel) editorRelCoords(idx, x, y int) (relX, relY int) {
	layout := displayengine.GetLayout()
	offX, offY := m.paneOffsetFor(idx)
	relX = x - (m.dialogOffsetX + offX + layout.NestedLeftOffset())
	relY = y - (m.dialogOffsetY + offY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight)
	return
}

// focusPane switches the active tab to idx (if different), blurring the old
// editor and focusing/resizing for the new one -- the shared core of every
// "click/wheel landed on the other pane" handler.
func (m *TabbedVarsEditorModel) focusPane(idx int) {
	if idx == m.activeTab {
		return
	}
	m.tabs[m.activeTab].editor.Blur()
	m.activeTab = idx
	m.SetSize(m.width, m.height)
}

// CyclePaneTitleFocus implements displayengine.PaneTitleBarFocusable --
// F9/Ctrl+T tries this before the plain dialog-level title bar toggle.
// Cycles editor content -> active pane's border widgets (only if it has any)
// -> dialog border (handled by the fallback toggle, not here) -> content.
func (m *TabbedVarsEditorModel) CyclePaneTitleFocus() bool {
	if m.TitleBarFocused() {
		// Dialog border already focused -- this press is the plain toggle's
		// to blur, not the pane level's.
		return false
	}
	if m.paneTitleFocused {
		// Pane border focused -- advance up to the dialog border instead of
		// blurring straight back to editor content.
		m.paneTitleFocused = false
		m.FocusTitleBar()
		return true
	}
	if m.focus != envFocusEditor || len(m.tabs) == 0 {
		return false // buttons focused, or nothing loaded -- nothing pane-specific to do
	}
	widgets := m.paneLayoutWidgets()
	if len(widgets) == 0 {
		return false // this pane has no border widgets -- skip straight to the dialog border
	}
	m.tabs[m.activeTab].editor.Blur()
	m.paneTitleFocused = true
	m.paneActiveWidget = widgets[len(widgets)-1].ID // default to rightmost, matching FocusTitleBar's convention
	return true
}

// cyclePaneWidget moves paneActiveWidget left/right among the active pane's
// current border widgets, wrapping at the ends.
func (m *TabbedVarsEditorModel) cyclePaneWidget(dir int) {
	widgets := m.paneLayoutWidgets()
	for i, w := range widgets {
		if w.ID == m.paneActiveWidget {
			next := (i + dir + len(widgets)) % len(widgets)
			m.paneActiveWidget = widgets[next].ID
			return
		}
	}
	if len(widgets) > 0 {
		m.paneActiveWidget = widgets[len(widgets)-1].ID
	}
}

// activatePaneWidget dispatches the action for whichever pane widget is
// currently focused -- Maximize goes to this specific pane's tab (matching
// the mouse click handler), Side-by-side/Stacked switch layoutMode.
func (m *TabbedVarsEditorModel) activatePaneWidget() tea.Cmd {
	id := m.paneActiveWidget
	m.paneTitleFocused = false
	m.tabs[m.activeTab].editor.Focus()
	switch id {
	case displayengine.IDTitleWidgetMaximize:
		m.layoutMode = envLayoutMaximized
		m.SetSize(m.width, m.height)
	case displayengine.IDTitleWidgetSideBySide:
		m.layoutMode = envLayoutSideBySide
		m.SetSize(m.width, m.height)
	case displayengine.IDTitleWidgetStacked:
		m.layoutMode = envLayoutStacked
		m.SetSize(m.width, m.height)
	}
	return nil
}

func (m *TabbedVarsEditorModel) Init() tea.Cmd {
	m.loading = true
	m.titleSpinner.Start()
	return m.loadEnv
}

// EscapeAction implements tui.EscapeActioner: prompts for unsaved changes if needed.
func (m *TabbedVarsEditorModel) EscapeAction() tea.Cmd {
	if m.hasChanges() {
		return m.promptUnsavedChanges(m.onClose)
	}
	return m.onClose
}

func (m *TabbedVarsEditorModel) hasErrors() bool {
	for _, tab := range m.tabs {
		if tab.editor.HasValidationErrors() {
			return true
		}
	}
	return false
}

// enabledStateForApp computes the tri-state enabled status for the given app
// from the global tab's active (non-pending-delete) lines.
// Returns "active", "disabled", or "absent".
func (m *TabbedVarsEditorModel) enabledStateForApp(appUpper string) string {
	for i := range m.tabs {
		if m.tabs[i].spec.IsGlobal && strings.ToUpper(m.tabs[i].spec.App) == appUpper {
			lines := m.tabs[i].editor.ActiveLines()
			_, exists := appenv.GetFromLines(appUpper+"__ENABLED", lines)
			if !exists {
				return "absent"
			}
			if appenv.IsAppEnabledFromLines(appUpper, lines) {
				return "active"
			}
			return "disabled"
		}
	}
	return "absent"
}

// checkEnabledChangedForKey finds the global tab whose APPNAME__ENABLED key
// matches the given varName and calls checkEnabledChanged on it. Used by
// ApplyVarValueMsg, deleteVarMsg, and restoreVarMsg to trigger an immediate
// refresh when an ENABLED variable is set, deleted, or restored.
func (m *TabbedVarsEditorModel) checkEnabledChangedForKey(varName string) tea.Cmd {
	upper := strings.ToUpper(varName)
	for i := range m.tabs {
		if !m.tabs[i].spec.IsGlobal || m.tabs[i].spec.App == "" {
			continue
		}
		// Exact match only: APPNAME__ENABLED — vars like APPNAME__FOO__ENABLED are unrelated.
		if upper == strings.ToUpper(m.tabs[i].spec.App)+"__ENABLED" {
			return m.checkEnabledChanged(i)
		}
	}
	return nil
}

// checkEnabledChanged computes the current enabled state for the app on the
// given global tab and, if it differs from lastEnabledState, updates it and
// returns a cmd that dispatches envRefreshMsg{}.
// No-ops for non-global tabs, apps with no name, or apps that are not built-in
// (user-defined apps have no template sections to reorganize).
func (m *TabbedVarsEditorModel) checkEnabledChanged(tabIdx int) tea.Cmd {
	tab := &m.tabs[tabIdx]
	if !tab.spec.IsGlobal || tab.spec.App == "" {
		return nil
	}
	appUpper := strings.ToUpper(tab.spec.App)
	if !appenv.IsAppBuiltIn(appUpper) {
		return nil
	}
	newState := m.enabledStateForApp(appUpper)
	if newState == tab.lastEnabledState {
		return nil
	}
	tab.lastEnabledState = newState
	return func() tea.Msg { return envRefreshMsg{preservePendingDeletes: true} }
}

func (m *TabbedVarsEditorModel) buttonIndex(name string) int {
	for i, btn := range m.buttons {
		if btn == name {
			return i
		}
	}
	return 0
}

func (m *TabbedVarsEditorModel) Title() string {
	return m.title
}

func (m *TabbedVarsEditorModel) SetFocused(f bool) {
	m.focused = f
	if f {
		if m.focus == envFocusEditor && len(m.tabs) > 0 {
			m.tabs[m.activeTab].editor.Focus()
		}
	} else {
		if len(m.tabs) > 0 {
			m.tabs[m.activeTab].editor.Blur()
		}
	}
}

func (m *TabbedVarsEditorModel) IsMaximized() bool {
	return true
}

// MinHeight returns the minimum content-area height for the tabbed vars editor.
// Breakdown: outer border(2) + subtitle min(1) + inner editor border(2) + editor min(3) + flat buttons(1) = 9.
// Increases by LargeTitleBarOverhead when large titlebars are enabled.
func (m *TabbedVarsEditorModel) MinHeight() int {
	base := 9
	if displayengine.GetActiveContext().LargeTitleBars {
		base += displayengine.LargeTitleBarOverhead
	}
	return base
}

func (m *TabbedVarsEditorModel) MenuName() string {
	return "tabbed_vars"
}

func (m *TabbedVarsEditorModel) IsDestructive() bool { return true }
func (m *TabbedVarsEditorModel) IsLoading() bool     { return m.loading }

func (m *TabbedVarsEditorModel) HasDialog() bool {
	return false
}

// GetInputCursor implements tui.InputCursorProvider.
// It returns the hardware cursor position relative to the screen's top-left corner,
// allowing AppModel.View() to position the terminal cursor over the active editor.
func (m *TabbedVarsEditorModel) GetInputCursor() (relX, relY int, shape tea.CursorShape, ok bool) {
	if m.focus != envFocusEditor || len(m.tabs) == 0 {
		return 0, 0, tea.CursorBar, false
	}
	editor := m.tabs[m.activeTab].editor
	c := editor.Cursor()
	if c == nil {
		return 0, 0, tea.CursorBar, false
	}
	// Editor content starts at:
	//   outer_border(1) + ContentSideMargin(1) + inner_border(1) = 3 cols
	//   outer_border(1) + inner_border/tab_row(1) = 2 rows
	// plus subtitle rows stacked above the inner border.
	layout := displayengine.GetLayout()
	relX = 1 + layout.ContentSideMargin + 1 + c.X + m.activePaneOffsetX
	relY = 2 + m.largeTitleOverhead + m.subtitleHeight + c.Y + m.activePaneOffsetY
	switch {
	case !editor.IsEditableAtCursor():
		shape = tea.CursorUnderline
	case editor.IsOverwrite():
		shape = tea.CursorBlock
	default:
		shape = tea.CursorBar
	}
	return relX, relY, shape, true
}

// IsScrollbarDragging returns true if the current editor is dragging a line or a scrollbar.
func (m *TabbedVarsEditorModel) IsScrollbarDragging() bool {
	if len(m.tabs) > 0 {
		return m.tabs[m.activeTab].editor.IsDragging()
	}
	return false
}

func (m *TabbedVarsEditorModel) hasChanges() bool {
	for _, tab := range m.tabs {
		// Value-keyed map comparison below can't see position -- a swapped
		// pair of vars produces the identical map -- so a pure reorder needs
		// its own check.
		if tab.editor.HasMovedLines() {
			return true
		}
		currentVars, _ := appenv.ListVarsLiteralsData(tab.editor.GetContent())
		if len(currentVars) != len(tab.initialVars) {
			return true
		}
		for k, v := range currentVars {
			if initialV, ok := tab.initialVars[k]; !ok || initialV != v {
				return true
			}
		}
	}
	return false
}

func (m *TabbedVarsEditorModel) promptUnsavedChanges(onConfirm tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		return tui.ShowConfirmDialogMsg{
			Title:      "Unsaved Changes",
			Question:   "You have unsaved changes. Do you want to leave without saving?",
			DefaultYes: false,
			OnResult: func(confirmed bool) tea.Cmd {
				if !confirmed {
					return nil
				}
				return onConfirm
			},
		}
	}
}

// confirmExitAction returns a Cmd that exits DockSTARTer, combining the two prompts into one.
// If there are unsaved changes: "Discard changes and exit?" — one prompt, exits on yes.
// If no unsaved changes: "Exit DockSTARTer?" — standard exit prompt.
func (m *TabbedVarsEditorModel) confirmExitAction() tea.Cmd {
	hasChanges := m.hasChanges()
	return func() tea.Msg {
		if hasChanges {
			return tui.ShowConfirmDialogMsg{
				Title:      "Exit " + version.ApplicationName,
				Question:   "You have unsaved changes. Discard changes and exit " + version.ApplicationName + "?",
				DefaultYes: false,
				OnResult: func(confirmed bool) tea.Cmd {
					if !confirmed {
						return nil
					}
					return tea.Quit
				},
			}
		}
		return tui.ShowConfirmDialogMsg{
			Title:      "Exit " + version.ApplicationName,
			Question:   "Do you want to exit " + version.ApplicationName + "?",
			DefaultYes: true,
			OnResult: func(confirmed bool) tea.Cmd {
				if !confirmed {
					return nil
				}
				return tea.Quit
			},
		}
	}
}
