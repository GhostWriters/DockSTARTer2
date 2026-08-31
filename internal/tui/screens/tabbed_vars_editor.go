package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/tui/components/enveditor"
	"DockSTARTer2/internal/version"
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

// doubleClickWindow is how close together two clicks on the same target
// (a pane's border, or a tab strip entry) have to land to count as a
// double-click-to-maximize, matching enveditor's own multi-click timing.
const doubleClickWindow = 400 * time.Millisecond

// GutterDragState tracks a mouse drag on the split gutter between the two
// panes. Modeled on displayengine/classic's ScrollbarDragState, but without
// its PendingDragY/LastDragY/DragPending throttle fields -- that gate is a
// known bug class (see project_scrollbar_drag_cmd_throttle in memory:
// already caused a step-through-playback bug once for list scrollbars) and
// a fresh drag implementation shouldn't reintroduce it. Every motion event
// applies its resulting ratio directly.
type GutterDragState struct {
	Dragging    bool
	GutterIndex int     // which of the N-1 gutter boundaries is being dragged
	StartMouse  int     // absolute mouse X (side-by-side) or Y (stacked) when drag started
	StartShareA float64 // share of the pane left/above the dragged gutter, when drag started
	StartShareB float64 // share of the pane right/below the dragged gutter, when drag started
}

// StartDrag records the starting mouse position and the two adjacent panes'
// shares for a new drag on the given gutter boundary.
func (g *GutterDragState) StartDrag(gutterIndex, mouse int, shareA, shareB float64) {
	g.Dragging = true
	g.GutterIndex = gutterIndex
	g.StartMouse = mouse
	g.StartShareA = shareA
	g.StartShareB = shareB
}

// StopDrag ends the drag.
func (g *GutterDragState) StopDrag() {
	g.Dragging = false
}

// envSetLayoutMsg is dispatched by the Maximize/Side-by-side/Stacked title
// bar widgets to change layoutMode.
type envSetLayoutMsg struct {
	mode envLayoutMode
}

// headingLabelW is the fixed label column width, matching bash menu_heading.sh's
// LabelWidth which is the max across ALL possible labels: "Original Value: " = 16.
// Using the maximum keeps values aligned at the same column across all screens.
const headingLabelW = menuLabelW

type EnvTabSpec struct {
	Title    string
	App      string // Empty string for global vars, app name for app-specific vars
	IsGlobal bool   // Indicates if this tab edits the global .env file (potentially filtered by App)
	// FileApp is the appenv-qualified name identifying which physical
	// .env.app.* file this tab targets -- e.g. "immich-database" or
	// "immich___postgres" for a multi-service app's per-service or
	// shared/virtual files. Empty for the plain file and for global tabs,
	// where App alone already identifies the target.
	FileApp string
}

// fileApp returns the qualified name identifying which .env.app.* file this
// spec targets: FileApp when set, App otherwise (the plain-file case).
func (s EnvTabSpec) fileApp() string {
	if s.FileApp != "" {
		return s.FileApp
	}
	return s.App
}

// buildAppEditorSpecs returns the tab specs for editing appName: the
// global-vars tab (filtered to appName) plus one tab per .env.app.* file
// appName currently has on disk -- the plain file, and for a multi-service
// app, any per-service or shared/virtual files alongside it. Falls back to
// a single plain-file tab if no files are found on disk yet (e.g. an app
// whose vars haven't been created).
func buildAppEditorSpecs(appName string) []EnvTabSpec {
	specs := []EnvTabSpec{
		{Title: ".env", App: appName, IsGlobal: true},
	}

	conf := config.LoadAppConfig()
	files := appenv.AppVarFileNames(appName, conf)
	if len(files) == 0 {
		specs = append(specs, EnvTabSpec{
			Title: ".env.app." + strings.ToLower(appName),
			App:   appName,
		})
		return specs
	}

	for _, name := range files {
		fileApp := strings.TrimPrefix(name, constants.AppEnvFileNamePrefix)
		specs = append(specs, EnvTabSpec{
			Title:   name,
			App:     appName,
			FileApp: fileApp,
		})
	}
	return specs
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
	// open is whether this tab currently renders as a tiled pane. Closing
	// only hides it -- the editor buffer, cursor, and any unsaved edits
	// stay intact; Save still covers it regardless. Ignored entirely in
	// Maximized mode, which always shows exactly one tab (open or not) by
	// tab-strip/Ctrl+Left/Right switching, independent of tiling.
	open bool
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
	// paneEditorHeight/paneContentWidth are indexed by pane *slot* (position
	// within openTabIndices(), not raw tab index -- see paneSlotFor),
	// resized to openCount() in SetSize. Side-by-side splits width but
	// shares height across all panes; stacked splits height but shares
	// width -- see SetSize. In Maximized mode (or when tiling has collapsed
	// for lack of room) every entry holds the same full-content value, so
	// slot 0 is always a safe stand-in regardless of which tab is active.
	paneEditorHeight []int
	paneContentWidth []int
	fullContentWidth int // full dialog content width, spans all panes when tiled -- used by buttons/outer border
	focused          bool

	// layoutMode is the user's chosen view (Maximize/Side-by-side/Stacked);
	// splitMode is SetSize's actual per-render decision, which falls back to
	// false when the current size is too small for the chosen split, same
	// collapse-and-spring-back as the Appearance Settings preview panel.
	// "splitMode" now means "tiled" (2 or more panes), not just exactly 2.
	layoutMode envLayoutMode
	splitMode  bool

	// sideBySideShares/stackedShares hold each pane's share (0.0-1.0,
	// summing to 1.0) of the split budget -- width for side-by-side, height
	// for stacked -- one entry per tab, tracked separately per layout so
	// resizing one doesn't bleed into the other's proportions when
	// EnvCycleLayout switches between them. Resized/reset to 1/N each
	// whenever the tab count changes (not persisted -- always reset to
	// equal shares when the editor opens). SetSize clamps every pane to a
	// floor of minPaneContentWidth/minPaneEditorHeight before using these,
	// so a drag or keyboard nudge can never push a pane below its floor.
	sideBySideShares []float64
	stackedShares    []float64

	// resizingGutter is keyboard resize mode (Ctrl+S/Alt+S toggles it);
	// gutterDrag is the mouse-drag equivalent. Both feed into
	// sideBySideShares/stackedShares and both are read by ViewString to
	// show the arrow-tipped resize line on the active gutter instead of a
	// normal blank one. activeGutter is which of the N-1 boundaries
	// keyboard resize mode is currently adjusting -- Tab/Shift+Tab cycles
	// it layout-agnostically, or Ctrl/Alt/Ctrl+Alt+Left/Right (side-by-side)
	// /Up/Down (stacked) cycles it in the same direction the plain arrows
	// already nudge size in; reset to 0 whenever resizingGutter turns on.
	resizingGutter bool
	activeGutter   int
	gutterDrag     GutterDragState

	// paneOffsetXs/Ys hold each pane's top-left offset relative to pane 0's,
	// recomputed by SetSize alongside paneContentWidth/paneEditorHeight.
	// activePaneOffsetX/Y is paneOffsetFor(m.activeTab), PLUS -- when tiled
	// -- the middle "tab list" box's own left border column and top
	// border/title row (paneOffsetFor only knows about offsets between
	// sibling panes, not that outer wrapping box), so it's folded into
	// lastOffsetX/Y by GetHitRegions and every click/cursor formula that
	// combines it with layout.NestedLeftOffset()/NestedTopOffset() (a
	// constant sized for exactly two border layers: dialog -> pane) lands
	// on the right cell in both Maximized and tiled mode. See SetSize's
	// assignment for the exact computation.
	paneOffsetXs      []int
	paneOffsetYs      []int
	activePaneOffsetX int
	activePaneOffsetY int

	// tiledTabStripHeight is 1 when tiled (m.splitMode) -- the shared tab
	// strip renders as its own row above all panes in that mode (each pane
	// keeps its own small title segment too; see renderPane), rather than
	// living in a single pane's own top border like Maximized mode. 0
	// otherwise. Added at every pane-box-top calculation that already adds
	// largeTitleOverhead + subtitleHeight -- see paneBoxTop.
	tiledTabStripHeight int

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

	// lastBorderClickIdx/Time and lastTabClickIdx/Time each track
	// double-click-to-maximize for their own target kind -- a pane's own
	// border (Update's "tabbed_vars.pane-" border-click branch) and a tab
	// strip entry ("tabbed_vars.tab-") respectively -- kept separate so a
	// border double-click and a tab-strip double-click on the same tab
	// index in quick succession don't cross-pair as one. -1 when no click
	// of that kind is pending pairing. Separate from enveditor's own
	// multi-click tracking (word/value/line select), which only applies
	// inside a pane's content area.
	lastBorderClickIdx  int
	lastBorderClickTime time.Time
	lastTabClickIdx     int
	lastTabClickTime    time.Time

	// tabScrollOffset is which m.tabs index the shared tab strip's visible
	// window currently starts from -- see tabStripFit/scrollTabIntoView.
	tabScrollOffset int

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

// envButtonLabels overrides the displayed text for a button whose internal
// name (used as the lookup key everywhere else -- zone IDs, help text,
// buttonIndex) stays "Cancel". "Close" reads clearer than "Back": in the
// Full Setup wizard this same action is a stack pop that reveals the next
// wizard step rather than a previous screen, so "Back" describes the code's
// navigation mechanism but not what the user sees happen.
var envButtonLabels = map[string]string{"Cancel": "Close"}

func envButtonLabel(name string) string {
	if label, ok := envButtonLabels[name]; ok {
		return label
	}
	return name
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

// cutSelectionMsg is dispatched by the context menu's "Cut Selection" item
// (clipboard write already happened synchronously in the menu action) to
// delete the active editor's current text selection.
type cutSelectionMsg struct{}

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
		// The native/hardware cursor's color comes from the terminal
		// emulator's own theme, not DS2's -- on some terminals (e.g.
		// WezTerm) that can end up invisible against a given DS2 theme, and
		// the web frontend's browser terminal doesn't honor cursor-color
		// escapes at all. The virtual (drawn) cursor is just styled text,
		// so it always reflects the active theme correctly regardless of
		// frontend.
		editor.SetVirtualCursor(true)
		editor.ScrollbarFunc = func(content string, total, visible, offset int, lineChars bool) string {
			return displayengine.ApplyScrollbarColumn(content, total, visible, offset, lineChars, displayengine.GetActiveContext())
		}
		tabs = append(tabs, envTab{spec: spec, editor: editor, open: true})
	}

	buttons := []string{"Save", "Refresh", "Cancel", "Exit"}
	if !showBack {
		buttons = []string{"Save", "Refresh", "Exit"}
	}

	m := &TabbedVarsEditorModel{
		tabs:               tabs,
		activeTab:          0,
		title:              title,
		buttons:            buttons,
		btnIdx:             0,
		focus:              envFocusEditor,
		onClose:            onClose,
		connType:           connType,
		lastBorderClickIdx: -1,
		lastTabClickIdx:    -1,
	}
	m.resetSharesToEqual()
	zoneByName := map[string]string{
		"Save":   displayengine.IDSaveButton,
		"Cancel": displayengine.IDBackButton,
		"Exit":   displayengine.IDExitButton,
	}
	defs := make([]displayengine.ButtonDef, len(buttons))
	for i, btn := range buttons {
		defs[i] = displayengine.ButtonDef{Label: envButtonLabel(btn), ZoneID: zoneByName[btn]}
	}
	m.btnRow = displayengine.NewButtonRow(defs)
	// Maximize/Side-by-side/Stacked live on the pane's own (submenu-style)
	// border, not here -- see renderPane/paneLayoutWidgets. The main dialog
	// title bar only ever gets the standard Refresh/Help/Close set.
	m.ConfigureWidgets(displayengine.WidgetRefresh, displayengine.WidgetHelp, displayengine.WidgetClose)

	if len(tabs) >= 2 {
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
// layout mode when activated. iconName is its own theme tag prefix (e.g.
// "Maximize"); a theme need not define Maximize/SideBySide/StackedIcon* at
// all -- WidgetDef falls back to the generic Icon{Inactive,Focused,Pressed}
// tags when a widget-specific one isn't defined (see IconName's doc comment).
func envLayoutWidget(id, label, help, glyph, glyphAscii, iconName string, mode envLayoutMode) displayengine.WidgetDef {
	return displayengine.WidgetDef{
		ID:         id,
		Label:      label,
		HelpText:   help,
		Glyph:      glyph,
		GlyphAscii: glyphAscii,
		IconName:   iconName,
		Action:     envLayoutSetAction(mode),
	}
}

// maximizeWidget builds the per-pane Maximize icon -- clicking it maximizes
// to that specific pane's tab. Its ID alone doesn't say which pane; Update
// recovers that from the "tabbed_vars.paneN." hit-region ID prefix.
func maximizeWidget() displayengine.WidgetDef {
	return envLayoutWidget(displayengine.IDTitleWidgetMaximize, "Maximize", "Show only this tab, full size.", "□", "+", "Maximize", envLayoutMaximized)
}

// closeWidget builds the per-pane Close icon shown when tiled with 2+ tabs
// open -- hides that specific pane (see envTab.open); its own edits stay
// intact for reopening via the shared tab strip, Ctrl+Left/Right + Space,
// or Ctrl+Q. Reuses IDTitleWidgetClose (the outer dialog's own close
// button ID) the same way maximizeWidget reuses IDTitleWidgetMaximize --
// Update recovers which pane from the "tabbed_vars.paneN." ID prefix.
// Action is nil: closing needs the pane's own tab index, which envSetLayoutMsg's
// shape doesn't carry, so Update's widget-click handler applies it directly
// instead of dispatching a message.
func closeWidget() displayengine.WidgetDef {
	return displayengine.WidgetDef{
		ID:         displayengine.IDTitleWidgetClose,
		Label:      "Close",
		HelpText:   "Close this pane (its edits are kept -- reopen it from the tab strip, Ctrl+←/→ then Space, or Ctrl+Q).",
		Glyph:      "×",
		GlyphAscii: "x",
		IconName:   "Close",
	}
}

// paneLayoutWidgets returns the layout-control widgets shown on every pane's
// border -- only meaningful with 2+ tabs open; nil otherwise. Maximize
// is last (rightmost, matching Close's convention) and omitted in Maximized
// mode. Only offers the split direction not already active. Keyed off
// layoutMode, not splitMode, so the icons stay consistent with what's
// actually selected even while temporarily auto-collapsed (see SetSize).
func (m *TabbedVarsEditorModel) paneLayoutWidgets() []displayengine.WidgetDef {
	if len(m.tabs) < 2 {
		return nil
	}
	sideBySide := envLayoutWidget(displayengine.IDTitleWidgetSideBySide, "Side by side", "Show both open tabs side by side.", "▥", "|", "SideBySide", envLayoutSideBySide)
	stacked := envLayoutWidget(displayengine.IDTitleWidgetStacked, "Stacked", "Show both open tabs stacked vertically.", "▤", "-", "Stacked", envLayoutStacked)
	switch m.layoutMode {
	case envLayoutMaximized:
		return []displayengine.WidgetDef{sideBySide, stacked}
	case envLayoutSideBySide:
		return []displayengine.WidgetDef{stacked, maximizeWidget()}
	default: // envLayoutStacked
		return []displayengine.WidgetDef{sideBySide, maximizeWidget()}
	}
}

// paneBoxTop returns the Y coordinate where every pane's own outer box
// begins, given the dialog's screen-relative Y origin -- outer border, large
// titlebar rows, the shared subtitle, and (when tiled) the middle "tab
// list" box's own top/title row, all common to every pane regardless of
// which one.
func (m *TabbedVarsEditorModel) paneBoxTop(offsetY int) int {
	return offsetY + 1 + m.largeTitleOverhead + m.subtitleHeight + m.tiledTabStripHeight
}

// paneBoxLeft returns the X coordinate where every pane's own outer box
// begins, given the dialog's screen-relative X origin -- outer border,
// content margin, and (when tiled) the middle "tab list" box's own left
// border column.
func (m *TabbedVarsEditorModel) paneBoxLeft(offsetX int) int {
	layout := displayengine.GetLayout()
	left := offsetX + 1 + layout.ContentSideMargin
	if m.splitMode {
		left += layout.BorderWidth() / 2
	}
	return left
}

// tabStripAvailWidth returns the width available to the shared tab strip --
// the middle "tab list" box's own content width when tiled, or the single
// active pane's content width in Maximized mode. The single source every
// caller (renderTabs, GetHitRegions' tab-region block, scrollTabIntoView)
// uses, so they can never compute a different width from one another.
func (m *TabbedVarsEditorModel) tabStripAvailWidth() int {
	layout := displayengine.GetLayout()
	ctx := displayengine.GetActiveContext()
	widgets := m.paneLayoutWidgets()
	if m.splitMode {
		middleContentWidth := m.fullContentWidth - layout.BorderWidth()
		if middleContentWidth < 1 {
			middleContentWidth = 1
		}
		return displayengine.MaxRawTitleWidth(middleContentWidth, false, ctx.SubmenuTitleAlign, widgets, ctx)
	}
	if len(m.paneContentWidth) == 0 {
		return m.fullContentWidth
	}
	slot, _ := m.paneSlotFor(m.activeTab) // uniform array when !splitMode -- see SetSize
	target := m.paneContentWidth[slot] - 2
	if target < 1 {
		target = 1
	}
	return displayengine.MaxRawTitleWidth(target, false, ctx.SubmenuTitleAlign, widgets, ctx)
}

// tabStripLayout describes which tabs are currently visible in the shared
// tab strip and where. Computed once by tabStripFit, consumed identically
// by renderTabs and GetHitRegions' tab-region block so what's drawn and
// what's clickable never drift apart.
type tabStripLayout struct {
	first, last                   int // inclusive range of m.tabs indices currently shown
	showLeftArrow, showRightArrow bool
	tabX                          []int // each visible tab's X offset from the strip's own start (past any left arrow), one per index in [first, last]
	arrowWidth                    int   // column width of one arrow glyph
}

// tabStripFit computes the widest whole-tab window that fits in availWidth
// while still containing m.tabScrollOffset -- expanding both forward AND
// backward from that anchor as far as space allows, so growing availWidth
// (e.g. maximizing the terminal) always pulls more tabs into view around
// wherever we're currently scrolled/focused, rather than only growing
// forward or requiring a full reset to see the extra room. m.tabScrollOffset
// itself is normalized to the resulting window's left edge, so the next
// fit (or arrow click) continues from there. Never a partially-cut-off
// title at either edge.
func (m *TabbedVarsEditorModel) tabStripFit(availWidth int) tabStripLayout {
	const arrowWidth = 1
	n := len(m.tabs)
	if n == 0 {
		return tabStripLayout{arrowWidth: arrowWidth}
	}
	ctx := displayengine.GetActiveContext()
	widths := make([]int, n)
	for i, tab := range m.tabs {
		widths[i] = displayengine.WidthOfTitleSegment(tab.spec.Title, true, ctx)
	}

	anchor := m.tabScrollOffset
	if anchor < 0 {
		anchor = 0
	}
	if anchor >= n {
		anchor = n - 1
	}

	// rangeWidth is the strip's total rendered width for showing exactly
	// [lo, hi], including whichever arrows that range still needs.
	rangeWidth := func(lo, hi int) int {
		w := 0
		for i := lo; i <= hi; i++ {
			w += widths[i]
		}
		if lo > 0 {
			w += arrowWidth
		}
		if hi < n-1 {
			w += arrowWidth
		}
		return w
	}

	lo, hi := anchor, anchor
	for {
		progress := false
		if hi+1 < n && rangeWidth(lo, hi+1) <= availWidth {
			hi++
			progress = true
		}
		if lo-1 >= 0 && rangeWidth(lo-1, hi) <= availWidth {
			lo--
			progress = true
		}
		if !progress {
			break
		}
	}
	m.tabScrollOffset = lo

	showLeft := lo > 0
	x := 0
	if showLeft {
		x = arrowWidth
	}
	tabX := make([]int, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		tabX = append(tabX, x)
		x += widths[i]
	}

	return tabStripLayout{
		first:          lo,
		last:           hi,
		showLeftArrow:  showLeft,
		showRightArrow: hi < n-1,
		tabX:           tabX,
		arrowWidth:     arrowWidth,
	}
}

// scrollTabIntoView adjusts m.tabScrollOffset minimally (not necessarily to
// 0) so m.activeTab falls within the tab strip's currently visible window,
// at the strip's current available width. Called once at the end of
// SetSize, which already re-runs after every activeTab-changing action in
// this codebase, rather than repeating a call at each of those sites.
func (m *TabbedVarsEditorModel) scrollTabIntoView() {
	if len(m.tabs) == 0 {
		return
	}
	layout := m.tabStripFit(m.tabStripAvailWidth())
	if m.activeTab < layout.first {
		m.tabScrollOffset = m.activeTab
		return
	}
	if m.activeTab > layout.last {
		// Shift right one tab at a time until activeTab is the new last --
		// tabStripFit's own greedy width-fit (not a fixed tab count) decides
		// how many additional tabs that pulls into view on the left side.
		for m.tabScrollOffset < m.activeTab {
			m.tabScrollOffset++
			layout = m.tabStripFit(m.tabStripAvailWidth())
			if m.activeTab <= layout.last {
				break
			}
		}
	}
}

// paneOffsetFor returns pane idx's top-left offset relative to pane 0 --
// (0,0) unless tiled, from SetSize's cached paneOffsetXs/Ys.
func (m *TabbedVarsEditorModel) paneOffsetFor(idx int) (x, y int) {
	slot, ok := m.paneSlotFor(idx)
	if !m.splitMode || !ok || slot >= len(m.paneOffsetXs) {
		return 0, 0
	}
	return m.paneOffsetXs[slot], m.paneOffsetYs[slot]
}

// openTabIndices returns the real tab index of every currently open tab, in
// tab order -- the geometry arrays (paneContentWidth, paneEditorHeight,
// paneOffsetXs/Ys, sideBySideShares, stackedShares) are indexed by position
// within this slice ("pane slot"), not by raw tab index, since a closed tab
// in between two open ones would otherwise leave a gap.
func (m *TabbedVarsEditorModel) openTabIndices() []int {
	var open []int
	for i := range m.tabs {
		if m.tabs[i].open {
			open = append(open, i)
		}
	}
	return open
}

// openCount returns how many tabs are currently open (rendered as tiled
// panes, or the single Maximized pane if exactly one).
func (m *TabbedVarsEditorModel) openCount() int {
	n := 0
	for i := range m.tabs {
		if m.tabs[i].open {
			n++
		}
	}
	return n
}

// paneSlotFor resolves a real tab index to its position within
// openTabIndices() -- the index every pane-geometry array actually uses.
// ok is false if tabIdx is out of range or currently closed (nothing
// rendered to have geometry for).
func (m *TabbedVarsEditorModel) paneSlotFor(tabIdx int) (slot int, ok bool) {
	if tabIdx < 0 || tabIdx >= len(m.tabs) || !m.tabs[tabIdx].open {
		return 0, false
	}
	slot = 0
	for i := 0; i < tabIdx; i++ {
		if m.tabs[i].open {
			slot++
		}
	}
	return slot, true
}

// activeSplitShares returns whichever of sideBySideShares/stackedShares
// applies to m.layoutMode, so callers can read or write "the current
// layout's shares" without duplicating the switch at every call site.
// Defaults to sideBySideShares when maximized (there's nothing to share in
// that mode, so the choice is arbitrary).
func (m *TabbedVarsEditorModel) activeSplitShares() []float64 {
	if m.layoutMode == envLayoutStacked {
		return m.stackedShares
	}
	return m.sideBySideShares
}

// resetSharesToEqual resets every pane's share in both layouts' share
// slices to 1/N, matching the open tab count -- called whenever the open
// tab count changes (including a pane opening/closing) and by the keyboard
// resize mode's Space key.
func (m *TabbedVarsEditorModel) resetSharesToEqual() {
	n := m.openCount()
	if n == 0 {
		m.sideBySideShares, m.stackedShares = nil, nil
		return
	}
	equal := make([]float64, n)
	for i := range equal {
		equal[i] = 1.0 / float64(n)
	}
	m.sideBySideShares = append([]float64(nil), equal...)
	m.stackedShares = append([]float64(nil), equal...)
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
	boxLeft := m.paneBoxLeft(m.dialogOffsetX)
	boxTop := m.paneBoxTop(m.dialogOffsetY)
	for _, i := range m.openTabIndices() {
		slot, _ := m.paneSlotFor(i)
		offX, offY := m.paneOffsetFor(i)
		bx, by := boxLeft+offX, boxTop+offY
		boxHeight := m.paneEditorHeight[slot] + layout.BorderHeight()
		if x >= bx && x < bx+m.paneContentWidth[slot] && y >= by && y < by+boxHeight {
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
	if m.splitMode {
		// paneOffsetFor is relative to sibling panes only -- add the middle
		// "tab list" box's own left border column and top border/title row,
		// same as SetSize does for m.activePaneOffsetX/Y (see its comment).
		offX += layout.BorderWidth() / 2
		offY += m.tiledTabStripHeight
	}
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

// closePane hides tab idx (its editor buffer, cursor, and any unsaved edits
// stay intact -- see envTab.open) -- the shared core of the per-pane Close
// widget click and EnvClosePane (Ctrl+Q). A no-op if idx is already closed
// or it's the last open tab (Close isn't offered in that case, but this
// stays safe to call regardless). If idx was active, moves focus to the
// next open tab (wrapping), matching Ctrl+Right's own wrap-free linear
// order as closely as a "skip closed tabs" search can.
func (m *TabbedVarsEditorModel) closePane(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.tabs) || !m.tabs[idx].open || m.openCount() <= 1 {
		return nil
	}
	m.tabs[idx].open = false
	if idx == m.activeTab {
		m.tabs[m.activeTab].editor.Blur()
		for offset := 1; offset < len(m.tabs); offset++ {
			next := (idx + offset) % len(m.tabs)
			if m.tabs[next].open {
				m.activeTab = next
				break
			}
		}
		if m.focus == envFocusEditor {
			m.tabs[m.activeTab].editor.Focus()
		}
	}
	m.resetSharesToEqual()
	m.SetSize(m.width, m.height)
	return nil
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
			switch {
			case appenv.IsAppEnabledFromLines(appUpper, lines):
				return "active"
			case appenv.IsAppDisabledFromLines(appUpper, lines):
				return "disabled"
			default:
				return "absent"
			}
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
	// A Ctrl+Left/Right-selected-but-still-closed tab (tiled mode only --
	// Maximized always shows whichever tab is active) has no rendered pane
	// to point a cursor at until Space opens it.
	if m.splitMode && !m.tabs[m.activeTab].open {
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

// IsScrollbarDragging returns true if the current editor is dragging a line
// or a scrollbar, or the split gutter is being dragged. AppModel's mouse
// router (model_mouse.go) only forwards MouseMotionMsg/MouseReleaseMsg to
// the active screen while this is true -- without gutterDrag included here,
// a gutter drag would start on click but never receive another motion
// event, since those get dropped before reaching Update at all.
func (m *TabbedVarsEditorModel) IsScrollbarDragging() bool {
	if m.gutterDrag.Dragging {
		return true
	}
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
