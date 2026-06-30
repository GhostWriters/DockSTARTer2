package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/console"
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
	contentWidth       int
	focused            bool

	// Callbacks
	onClose tea.Cmd

	// Last hit region offsets for coordinate translation
	lastOffsetX int
	lastOffsetY int

	lockedByOthers bool
	connType       string

	// Title bar focus state
	tui.TitleBarFocus

	// Spinner while env data is loading from disk
	loading      bool
	spinnerFrame int
	lastSpinner  time.Time

	btnSpinner tui.ButtonSpinner
}

// AdvanceSpinners advances the loading title spinner if its interval has elapsed.
// Returns true if the frame changed. Called by the global tick via globalTickMsg.
func (m *TabbedVarsEditorModel) AdvanceSpinners(now time.Time) bool {
	if !m.loading || !console.SpinnerEnabled {
		return false
	}
	fps := time.Duration(console.SpinnerSpeed) * time.Millisecond
	if fps <= 0 || now.Sub(m.lastSpinner) < fps {
		return false
	}
	m.lastSpinner = now
	ctx := tui.GetActiveContext()
	frames := console.SpinnerFramesTitleUnicode
	if !ctx.LineCharacters {
		frames = console.SpinnerFramesTitleASCII
	}
	m.spinnerFrame = (m.spinnerFrame + 1) % len(frames)
	return true
}

func (m *TabbedVarsEditorModel) currentSpinnerIndicators() (left, right string) {
	if !m.loading || !console.SpinnerEnabled {
		return "", ""
	}
	ctx := tui.GetActiveContext()
	return console.TitleSpinnerFrames(m.spinnerFrame, ctx.LineCharacters)
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
		editor.SetLineCharacters(tui.GetActiveContext().LineCharacters)
		editor.SetVirtualCursor(false)
		editor.ScrollbarFunc = func(content string, total, visible, offset int, lineChars bool) string {
			return tui.ApplyScrollbarColumn(content, total, visible, offset, lineChars, tui.GetActiveContext())
		}
		tabs = append(tabs, envTab{spec: spec, editor: editor})
	}

	buttons := []string{"Save", "Back", "Exit"}
	if !showBack {
		buttons = []string{"Save", "Exit"}
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
	m.btnSpinner.Init()
	m.ConfigureWidgets(tui.WidgetRefresh, tui.WidgetHelp, tui.WidgetClose)
	return m
}

func (m *TabbedVarsEditorModel) Init() tea.Cmd {
	m.loading = true
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
	if tui.GetActiveContext().LargeTitleBars {
		base += tui.LargeTitleBarOverhead
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
	layout := tui.GetLayout()
	relX = 1 + layout.ContentSideMargin + 1 + c.X
	relY = 2 + m.largeTitleOverhead + m.subtitleHeight + c.Y
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
		if tui.Confirm("Unsaved Changes", "You have unsaved changes. Do you want to leave without saving?", false) {
			return onConfirm()
		}
		return nil
	}
}

// confirmExitAction returns a Cmd that exits DockSTARTer, combining the two prompts into one.
// If there are unsaved changes: "Discard changes and exit?" — one prompt, exits on yes.
// If no unsaved changes: "Exit DockSTARTer?" — standard exit prompt.
func (m *TabbedVarsEditorModel) confirmExitAction() tea.Cmd {
	hasChanges := m.hasChanges()
	return func() tea.Msg {
		if hasChanges {
			if tui.Confirm("Exit "+version.ApplicationName, "You have unsaved changes. Discard changes and exit "+version.ApplicationName+"?", false) {
				return tea.Quit()
			}
			return nil
		}
		if tui.Confirm("Exit "+version.ApplicationName, "Do you want to exit "+version.ApplicationName+"?", true) {
			return tea.Quit()
		}
		return nil
	}
}

