package tui

import (
	"fmt"
	"sync/atomic"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
)

var menuInstanceCounter atomic.Uint64

// MenuItem defines an item in a menu
type MenuItem struct {
	Tag         string  // Display name (first letter used as shortcut)
	Desc        string  // Description text
	Help        string  // Help line text shown when item is selected
	Shortcut    rune    // Keyboard shortcut (usually first letter of Tag)
	Action      tea.Cmd // Command to execute when selected (Enter)
	SpaceAction tea.Cmd // Command to execute when Space is pressed

	// Checklist support
	Selectable    bool // Whether this item can be toggled
	Selected      bool // Current selection state
	IsCheckbox    bool // Whether this is a checkbox [ ] / [x]
	IsRadioButton bool // Whether this is a radio button ( ) / (*)
	Checked       bool // Current checkbox/radio state (= "Added" in app-selection)

	// Enabled state (app-selection): separate from Added (Checked)
	// Enabled means APP__ENABLED='true' in .env
	Enabled    bool // Current enabled state
	WasEnabled bool // Enabled state when the screen loaded (for gutter diff)

	// Layout support
	IsSeparator bool // Whether this is a non-selectable header/separator

	// Grouped list support (app selection with instances)
	IsGroupHeader     bool   // App name header row; checkbox shows group-enabled state (read-only)
	IsSubItem         bool   // Indented instance row under a group header
	IsAddInstance     bool   // "[+] Add instance…" action row
	IsEditing         bool   // Inline text-input row for new instance name entry
	IsNew             bool   // Newly added this session (not yet saved; used to allow rename)
	IsReferenced      bool   // Has env vars / compose reference but no __ENABLED; locked from rename
	WasAdded          bool   // Whether this item was added (present in .env) when the screen loaded (for gutter diff)
	ShowEnabledGutter bool   // Whether to show the Enabled (E/D) gutter column
	BaseApp           string // Base app name this row belongs to (sub-items / add-instance / editing)

	// Metadata
	IsUserDefined bool              // Whether this is a user-defined app (for coloring)
	IsInvalid     bool              // Whether this item is invalid (e.g. broken theme)
	Locked        bool              // Whether this item is locked by another session
	IsDestructive bool              // Whether this item leads to configuration changes
	Metadata      map[string]string // Optional extra data (e.g. internal app name)
}

// Implement list.Item interface for bubbles/list
func (i MenuItem) FilterValue() string { return i.Tag }
func (i MenuItem) Title() string       { return i.Tag }
func (i MenuItem) Description() string { return i.Desc }

// calculateMaxTagLength returns the maximum visual width of item tags
func calculateMaxTagLength(items []MenuItem) int {
	maxTagLen := 0
	for _, item := range items {
		tagWidth := lipgloss.Width(GetPlainText(item.Tag))
		if tagWidth > maxTagLen {
			maxTagLen = tagWidth
		}
	}
	return maxTagLen
}

// calculateMaxTagAndDescLength returns the maximum visual width of item tags and descriptions
func calculateMaxTagAndDescLength(items []MenuItem) (maxTagLen, maxDescLen int) {
	for _, item := range items {
		tagWidth := lipgloss.Width(GetPlainText(item.Tag))
		descWidth := lipgloss.Width(GetPlainText(item.Desc))
		if tagWidth > maxTagLen {
			maxTagLen = tagWidth
		}
		if descWidth > maxDescLen {
			maxDescLen = descWidth
		}
	}
	return
}

// MenuModel represents a selectable menu
type MenuModel struct {
	id             string // Identifier for selection persistence and zone IDs (may be shared across screens)
	instanceID     string // Globally unique per-instance ID for spinner tick and deferred action messages
	title          string // Menu title
	subtitle       string // Optional subtitle/description shown on-screen
	helpPageTitle  string // Optional title for the description box in the help dialog
	helpPageText   string // Optional description shown only in the help dialog (overrides subtitle)
	helpLegend     string // Optional legend shown in help dialog with title "Legend" (overrides helpPageText)
	helpItemPrefix string // Optional prefix for item titles in help dialog, e.g. "App", "Option", "Theme"
	items          []MenuItem
	cursor         int // Current selection
	width          int
	height         int

	// Focus state
	focused      bool
	focusedItem  FocusItem      // Which element has focus
	activeColumn CheckboxColumn // Which checkbox column has focus

	// Sub-menu mode (for consolidated screens)
	subMenuMode bool
	focusedSub  bool // If false, use normal borders. If true, use thick borders.
	disabled    bool // When true, renders title with TitleSubMenuDisabled style.

	// Bubbles list model
	list        list.Model
	maximized   bool // Whether to maximize the dialog to fill available space
	showButtons bool // Whether to show any buttons (default true)

	// Key override actions
	escAction   tea.Cmd
	enterAction tea.Cmd
	spaceAction tea.Cmd

	// Checkbox mode (for app selection)
	checkboxMode bool
	groupedMode  bool // Grouped hierarchical mode (app selection with instances)
	flowMode     bool // Whether to layout items horizontally instead of vertically
	flowColumns  int  // When > 1, render as N balanced vertical columns
	maxFlowRows  int  // When > 0, cap visible rows (enables scrolling in flow/column mode)

	// Dialog positioning
	isDialog bool // True when used as a modal dialog — raises hit-region Z priority above screen regions

	// Unified layout (deterministic sizing)
	layout DialogLayout

	dialogType DialogType

	// Variable height support (for dynamic word wrapping)
	variableHeight bool                                      // Allow list to expand naturally up to layout limits
	interceptor      func(tea.Msg, *MenuModel) (tea.Cmd, bool) // Optional custom message handler
	contentRenderer  func(contentWidth int) string              // Optional: replaces list content in viewSubMenu
	onSubFocused     func() tea.Cmd                             // Optional: called when section gains sub-focus

	// Memoization for expensive rendering
	lastView         string
	cacheValid       bool // Indicates if lastView is up-to-date with current state
	lastStateVersion int  // renderVersion snapshot when lastView was saved

	// Memoization specifically for the variable-height list (separated to avoid border recursion loops)
	lastListView    string
	lastWidth       int
	lastHeight      int
	lastIndex       int
	lastFilter      string
	lastActive      bool
	lastLineChars   bool
	lastVersion     int
	lastColumn      CheckboxColumn
	lastHitRegions  []HitRegion                                        // Cache for variable height hit regions
	extraHitRegions func(offsetX, offsetY, baseZ int) []HitRegion // Optional: extra hit regions injected by section helpers
	viewStartY      int         // Persistent scroll offset for variable height lists
	lastViewStartY  int         // Previous scroll offset for memoization check
	lastScrollTotal int         // Total content height from last renderVariableHeightList (for scrollbar)

	renderVersion       int // Incremented on item changes to invalidate list cache and top-level view cache
	showLockGutter      bool
	noLeftMargin        bool
	statusGutterWidth   int
	activityGutterWidth int
	itemPaddingWidth    int    // Optional padding after getters
	menuName            string // Name used for --menu or -M to return to this screen
	connType            string // "local", "ssh", or "web"
	externalLock        bool   // Whether destructive items are locked by an external session (persists across SetItems)
	commandLock         bool   // Whether destructive items are locked by a running panel command

	// Content sections: sub-menus rendered stacked inside the outer border.
	// When present, replaces the standard list+inner-border rendering.
	contentSections []*MenuModel
	focusedSection   int // index into contentSections; -1 = buttons focused

	// Optional hook to enrich the ItemText shown in the help dialog for a menu item.
	// If set, called by showContextMenu (right-click Help) and HelpContext.
	// Return ("", "") to keep the default item.Help text.
	itemHelpFunc func(item MenuItem) (itemTitle, itemText string)

	// Scrollbar component
	Scroll Scrollbar

	contextMenuFunc func(idx int) []ContextMenuItem // hook for screen-specific operations

	// Optional hook to provide markdown documentation for a menu item.
	// If set, called by HelpContext to populate docMarkdown and docAppName.
	itemDocFunc func(item MenuItem) (docMarkdown, docAppName string)

	// Title bar widget focus (keyboard navigation of ? and × widgets)
	TitleBarFocus

	// loadingText, when non-empty, replaces the list area with a centered spinner + message.
	// titleSpinner drives this list-item/loading spinner only — the button
	// spinner is owned by btnRow.
	loadingText  string
	titleSpinner TitleSpinner

	// processingItemIdx is the index of the menu item currently being activated (-1 = none).
	// Shows a spinner indicator while the triggered action is in flight.
	processingItemIdx int

	// Slice-based button system. Replaces any legacy configuration.
	// Use SetButtons() to set; focusedBtnIndex tracks which button in the slice is focused.
	// btnRow owns button processing/spinner state; kept in sync by SetButtons.
	buttons         []ButtonDef
	focusedBtnIndex int
	btnRow          *ButtonRow
}

// ButtonDef defines a single button in a dialog's button row.
// Use SetButtons to configure the button row.
type ButtonDef struct {
	Label  string
	ZoneID string         // used for hit regions and processingBtnID tracking
	Action func() tea.Msg // nil = no action (button is inert)
	Locked bool           // show lock marker
	Help   string         // helpline text
}

// TitleBarFocusable is implemented by models whose title bar can receive keyboard focus.
type TitleBarFocusable interface {
	FocusTitleBar()
	BlurTitleBar()
	TitleBarFocused() bool
}

// FocusTitleBar overrides the embedded TitleBarFocus method to also clear
// button-row focus (so a button doesn't stay visually "active" while the
// title bar is also focused) and invalidate the render cache — callers may
// invoke this from outside MenuModel.Update() (e.g. the global Ctrl+T
// handler), which doesn't go through Update()'s own cache invalidation.
func (m *MenuModel) FocusTitleBar() {
	m.TitleBarFocus.FocusTitleBar()
	m.focusedItem = FocusList
	m.InvalidateCache()
}

// BlurTitleBar overrides the embedded TitleBarFocus method to also
// invalidate the render cache for the same reason as FocusTitleBar.
func (m *MenuModel) BlurTitleBar() {
	m.TitleBarFocus.BlurTitleBar()
	m.InvalidateCache()
}

// applyItemLocks sets the Locked flag on all destructive items based on the
// current externalLock and commandLock states (either source locks the item).
func (m *MenuModel) applyItemLocks() {
	locked := m.externalLock || m.commandLock
	changed := false
	for i, item := range m.items {
		if item.IsDestructive && item.Locked != locked {
			item.Locked = locked
			m.items[i] = item
			changed = true
		}
	}
	items := m.list.Items()
	for i, it := range items {
		if item, ok := it.(MenuItem); ok {
			if item.IsDestructive && item.Locked != locked {
				item.Locked = locked
				items[i] = item
			}
		}
	}
	if changed {
		m.list.SetItems(items)
		m.renderVersion++
		m.cacheValid = false
	}
}

// SetLockedByOthers updates the Locked status of all destructive menu items.
func (m *MenuModel) SetLockedByOthers(locked bool) {
	m.externalLock = locked
	m.applyItemLocks()

	// Propagate to sub-menus
	for _, sub := range m.contentSections {
		sub.SetLockedByOthers(locked)
	}
}

// ScrollDoneMsg is sent after a wheel scroll is processed to clear the scrollPending flag.
// Exported so wrapper screens (e.g. DisplayOptionsScreen) can forward it to inner menus.
type ScrollDoneMsg struct{ ID string }

// scrollDoneCmd returns a zero-delay Cmd that emits ScrollDoneMsg for the given menu ID.
func scrollDoneCmd(id string) tea.Cmd {
	return func() tea.Msg { return ScrollDoneMsg{ID: id} }
}

// ScrollPending reports whether a scroll event is currently queued but not yet rendered.
func (m *MenuModel) ScrollPending() bool { return m.Scroll.Pending }

// MarkScrollPending sets the scrollPending flag and returns a Cmd that will clear it
// after the next render cycle. Call this in interceptors after processing a wheel event.
func (m *MenuModel) MarkScrollPending() tea.Cmd {
	m.Scroll.Pending = true
	return scrollDoneCmd(m.id)
}

// IsScrollbarDragging reports whether the menu is currently processing a scrollbar thumb drag.
// AppModel uses this interface to give the active screen drag priority.
func (m *MenuModel) IsScrollbarDragging() bool {
	return m.Scroll.Drag.Dragging
}

// ScrollTotal returns the total scrollable units (lines or items).
func (m *MenuModel) ScrollTotal() int {
	if m.variableHeight {
		return m.lastScrollTotal
	}
	if m.flowColumns >= 2 && m.maxFlowRows > 0 {
		// Return total rows (not items) for column scroll.
		n := 0
		for _, item := range m.items {
			if !item.IsSeparator {
				n++
			}
		}
		return (n + m.flowColumns - 1) / m.flowColumns
	}
	return len(m.items)
}

// FocusItem represents which UI element has focus
type FocusItem int

const (
	FocusList FocusItem = iota
	FocusSelectBtn
	FocusBackBtn
	FocusExitBtn
	// FocusBtn is a generic "one of the buttons[] slice has focus" alias.
	// The specific button is identified by focusedBtnIndex.
	// It is intentionally equal to FocusSelectBtn so that code that checks
	// focusedItem == FocusSelectBtn continues to detect button focus correctly.
	FocusBtn = FocusSelectBtn
)

// CheckboxColumn represents which column (Add or Enable) has focus in a row
type CheckboxColumn int

const (
	ColAdd CheckboxColumn = iota
	ColEnable
)

// SetContextMenuFunc sets the callback that provides custom context menu items for this menu
func (m *MenuModel) SetContextMenuFunc(f func(idx int) []ContextMenuItem) {
	m.contextMenuFunc = f
}

// menuSelectedIndices persists menu selection across visits
var menuSelectedIndices = make(map[string]int)

// NewMenuModel creates a new menu model
func NewMenuModel(id, title, subtitle string, items []MenuItem) *MenuModel {
	// Set default shortcuts from first letter of Tag
	for i := range items {
		if items[i].Shortcut == 0 && len(items[i].Tag) > 0 {
			items[i].Shortcut = []rune(items[i].Tag)[0]
		}
	}

	// Restore previous selection
	cursor := 0
	if idx, ok := menuSelectedIndices[id]; ok && idx >= 0 && idx < len(items) {
		cursor = idx
	} else {
		// New: Auto-focus the currently selected radio option if no persistent session
		for i, item := range items {
			if item.IsRadioButton && item.Checked {
				cursor = i
				break
			}
		}
	}

	// Convert MenuItems to list.Items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}

	// Calculate max tag and desc length for sizing
	maxTagLen, maxDescLen := calculateMaxTagAndDescLength(items)

	// Calculate initial width based on actual content
	// Width = tag + spacing(2) + desc + margins(2)
	initialWidth := maxTagLen + 2 + maxDescLen + 2 + ScrollbarGutterWidth

	// Create bubbles list

	// Size based on actual number of items for dynamic sizing
	delegate := menuItemDelegate{menuID: id, maxTagLen: maxTagLen, focused: true}

	// Calculate height
	itemHeight := delegate.Height()
	spacing := delegate.Spacing()
	totalItemHeight := len(items) * itemHeight
	if len(items) > 1 && spacing > 0 {
		totalItemHeight += (len(items) - 1) * spacing
	}
	initialHeight := totalItemHeight

	l := list.New(listItems, delegate, initialWidth, initialHeight)
	// Don't set l.Title - we render title in border instead
	l.SetShowTitle(false) // Disable list's built-in title rendering
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false) // Disable pagination indicators

	// Set list background to match dialog background (not black!)
	styles := GetStyles()
	dialogBg := styles.Dialog.GetBackground()
	l.Styles.NoItems = l.Styles.NoItems.Background(dialogBg)
	l.Styles.PaginationStyle = l.Styles.PaginationStyle.Background(dialogBg)
	l.Styles.HelpStyle = l.Styles.HelpStyle.Background(dialogBg)

	// Set initial cursor position
	if cursor > 0 && cursor < len(items) {
		l.Select(cursor)
	}

	return &MenuModel{
		id:                  id,
		instanceID:          fmt.Sprintf("%s#%d", id, menuInstanceCounter.Add(1)),
		title:               title,
		subtitle:            subtitle,
		items:               items,
		cursor:              cursor,
		connType:            "local", // default
		focused:             true,
		focusedItem:         FocusBtn,
		activeColumn:        ColAdd,
		list:                l,
		showButtons: false,
		Scroll:              Scrollbar{ID: id},
		showLockGutter:      true,
		activityGutterWidth: 0,
		itemPaddingWidth:    1, // Default 1 char padding after marker gutter
		processingItemIdx:   -1,
		btnRow:              NewButtonRow(nil),
	}
}

// SetConnType updates the connection type for the menu.
func (m *MenuModel) SetConnType(connType string) {
	m.connType = connType
}

// SetItemPaddingWidth sets the optional padding width after the gutters.
func (m *MenuModel) SetItemPaddingWidth(width int) {
	m.itemPaddingWidth = width
	m.renderVersion++
}

// SetShowLockGutter enables or disables the lock indicator gutter.
func (m *MenuModel) SetShowLockGutter(show bool) {
	m.showLockGutter = show
	m.renderVersion++
}

// SetNoLeftMargin removes the ContentSideMargin left indent from the list in subMenuMode.
// Use when the gutter should sit flush against the sub-panel's left border.
func (m *MenuModel) SetNoLeftMargin(v bool) {
	m.noLeftMargin = v
	m.renderVersion++
}

// Title returns the menu title
func (m *MenuModel) Title() string {
	return m.title
}

// Subtitle returns the menu subtitle
func (m *MenuModel) Subtitle() string {
	return m.subtitle
}

// SetTitle sets the menu title
func (m *MenuModel) SetTitle(title string) { m.title = title }

// SetHelpPageText sets a description shown only in the help dialog, overriding the subtitle there.
func (m *MenuModel) SetHelpPageText(text string) { m.helpPageText = text }

// SetHelpPageTitle sets a title for the description box shown in the help dialog.
func (m *MenuModel) SetHelpPageTitle(title string) { m.helpPageTitle = title }

// SetHelpLegend sets a legend shown in the help dialog with the title "Legend".
// When set, it takes precedence over helpPageText for both F1 and context-menu Help.
func (m *MenuModel) SetHelpLegend(text string) { m.helpLegend = text }

// SetHelpItemPrefix sets a prefix prepended to item titles in the help dialog, e.g. "App", "Option", "Theme".
func (m *MenuModel) SetHelpItemPrefix(prefix string) { m.helpItemPrefix = prefix }

// SetItemHelpFunc sets an optional callback that enriches the ItemTitle and ItemText shown in
// the help dialog for a focused menu item. Used by showContextMenu (right-click Help) and
// HelpContext. Return ("", "") to keep the default item.Help text.
func (m *MenuModel) SetItemHelpFunc(f func(item MenuItem) (itemTitle, itemText string)) {
	m.itemHelpFunc = f
}

// SetItemDocFunc sets the hook for providing markdown documentation in the help dialog.
func (m *MenuModel) SetItemDocFunc(f func(item MenuItem) (docMarkdown, docAppName string)) {
	m.itemDocFunc = f
}

// ID returns the unique identifier for this menu
func (m *MenuModel) ID() string { return m.id }

// SetDialogType sets the visual style/type of the menu dialog
func (m *MenuModel) SetDialogType(t DialogType) { m.dialogType = t }

// MenuName returns the name used for --menu or -M to return to this screen
func (m *MenuModel) MenuName() string {
	return m.menuName
}

// SetMenuName sets the persistent menu name and re-keys any saved selection
// so menus with different names don't share the same position slot.
func (m *MenuModel) SetMenuName(name string) {
	if name != "" && name != m.menuName {
		oldKey := m.persistKey()
		if idx, ok := menuSelectedIndices[oldKey]; ok {
			// Migrate saved selection from old key (id) to new key (name)
			delete(menuSelectedIndices, oldKey)
			menuSelectedIndices[name] = idx
		} else if idx, ok := menuSelectedIndices[name]; ok {
			// Restore position that was previously saved under the name key
			if idx >= 0 && idx < len(m.items) {
				m.cursor = idx
				m.list.Select(idx)
			}
		}
	}
	m.menuName = name
}

// persistKey returns the key used in menuSelectedIndices.
// Prefers menuName when set so distinct menus with the same id don't collide.
func (m *MenuModel) persistKey() string {
	if m.menuName != "" {
		return m.menuName
	}
	return m.id
}

// SetFocusedItem explicitly sets which UI element has focus (list or a button).
func (m *MenuModel) SetFocusedItem(item FocusItem) {
	m.focusedItem = item
}

// SetFocusedBtnIndex sets the focused button index within the buttons slice.
func (m *MenuModel) SetFocusedBtnIndex(idx int) {
	m.focusedBtnIndex = idx
	m.focusedItem = FocusBtn
	m.InvalidateCache()
}

// SetButtons replaces the button row with an arbitrary slice of ButtonDef entries.
// Pass an empty slice to show no buttons.
func (m *MenuModel) SetButtons(btns []ButtonDef) {
	m.buttons = btns
	m.focusedBtnIndex = 0
	m.btnRow.SetButtons(btns)
	if len(btns) > 0 {
		m.showButtons = true
	} else {
		m.showButtons = false
	}
	m.InvalidateCache()
}

// GetButtonHeight returns the current button row height (1 = flat, 3 = bordered).
func (m *MenuModel) GetButtonHeight() int {
	return m.layout.ButtonHeight
}

// HasLargeTitleBar reports whether the current layout uses a large title bar.
func (m *MenuModel) HasLargeTitleBar() bool {
	return m.layout.LargeTitleBar
}

// View implements tea.Model and ScreenModel
func (m *MenuModel) View() tea.View {
	return tea.View{Content: m.ViewString()}
}

func (m *MenuModel) SetFocused(f bool) {
	wasUnfocused := !m.focused
	m.focused = f
	// Clear processing indicators when menu regains focus after having lost it —
	// the previous action resolved (screen came back or navigated away and returned).
	if f && wasUnfocused {
		m.processingItemIdx = -1
		if m.loadingText == "" {
			m.titleSpinner.Stop()
		}
		m.btnRow.Clear()
	}
	m.updateDelegate()
	m.InvalidateCache()
}

// SetMaximized sets whether the menu should expand to fill available space
func (m *MenuModel) SetMaximized(maximized bool) {
	m.maximized = maximized
	m.calculateLayout()
}

// SetCommandLocked locks or unlocks all relevant items for a running panel command:
// the Exit button marker and all destructive menu items.
func (m *MenuModel) SetCommandLocked(locked bool) {
	for i, btn := range m.buttons {
		if btn.ZoneID == "btn-exit" {
			m.buttons[i].Locked = locked
			m.InvalidateCache()
			break
		}
	}
	m.commandLock = locked
	m.applyItemLocks()
}

// SetShowButtons sets whether to show the button row at all
func (m *MenuModel) SetShowButtons(show bool) {
	m.showButtons = show
	m.calculateLayout() // Layout needs recalculation when buttons are toggled
}

// IsMaximized returns whether the menu is maximized
func (m MenuModel) IsMaximized() bool {
	return m.maximized
}

// HasDialog returns whether the menu has an active dialog overlay
func (m MenuModel) HasDialog() bool {
	return false // Menus don't have nested dialogs
}

// SetSubMenuMode enables a compact mode for menus inside other screens/containers
func (m *MenuModel) SetSubMenuMode(v bool) {
	m.subMenuMode = v
	if v {
		m.showButtons = false
	}
	m.calculateLayout()
}

// SetDisabled marks the section as disabled, rendering its title with TitleSubMenuDisabled style.
func (m *MenuModel) SetDisabled(disabled bool) {
	m.disabled = disabled
	m.InvalidateCache()
}

// SetSubFocused sets the focus state specifically for sub-menu mode (thick vs normal border)
func (m *MenuModel) SetSubFocused(focused bool) tea.Cmd {
	m.focusedSub = focused
	var cmd tea.Cmd
	if focused && m.onSubFocused != nil {
		cmd = m.onSubFocused()
	}
	m.updateDelegate()
	return cmd
}

// SetOnSubFocused registers a callback invoked when this section gains sub-focus.
func (m *MenuModel) SetOnSubFocused(fn func() tea.Cmd) {
	m.onSubFocused = fn
}

// IsActive returns whether this menu actually has focus (accounting for subMenuMode)
func (m *MenuModel) IsActive() bool {
	if m.subMenuMode {
		return m.focusedSub
	}
	return m.focused
}

// updateDelegate refreshes the list delegate with the current focus state
// calculateMaxTagLengthForHeaders returns the max tag width among IsGroupHeader items only.
func calculateMaxTagLengthForHeaders(items []MenuItem) int {
	maxLen := 0
	for _, item := range items {
		if !item.IsGroupHeader {
			continue
		}
		w := lipgloss.Width(GetPlainText(item.Tag))
		if w > maxLen {
			maxLen = w
		}
	}
	return maxLen
}

func (m *MenuModel) updateDelegate() {
	focused := m.IsActive()
	if m.groupedMode {
		maxTagLen := calculateMaxTagLengthForHeaders(m.items)
		m.list.SetDelegate(groupedItemDelegate{
			maxTagLen:           maxTagLen,
			focused:             focused,
			activeCol:           m.activeColumn,
			showLockGutter:      m.showLockGutter,
			activityGutterWidth: m.activityGutterWidth,
		})
	} else if m.checkboxMode {
		maxTagLen := calculateMaxTagLength(m.items)
		m.list.SetDelegate(checkboxItemDelegate{
			menuID:              m.id,
			maxTagLen:           maxTagLen,
			focused:             focused,
			flowMode:            m.flowMode,
			showLockGutter:      m.showLockGutter,
			activityGutterWidth: m.activityGutterWidth,
			paddingWidth:        m.itemPaddingWidth,
		})
	} else {
		maxTagLen := calculateMaxTagLength(m.items)
		m.list.SetDelegate(menuItemDelegate{
			menuID:              m.id,
			maxTagLen:           maxTagLen,
			focused:             focused,
			flowMode:            m.flowMode,
			showLockGutter:      m.showLockGutter,
			activityGutterWidth: m.activityGutterWidth,
			paddingWidth:        m.itemPaddingWidth,
		})
	}
}

// SetCheckboxMode enables checkbox rendering for app selection
func (m *MenuModel) SetCheckboxMode(enabled bool) {
	m.checkboxMode = enabled
	m.updateDelegate()
}

// SetGroupedMode enables the hierarchical grouped delegate for the app-selection screen.
// This renders IsGroupHeader, IsSubItem, IsAddInstance, and IsEditing items correctly.
func (m *MenuModel) SetGroupedMode(enabled bool) {
	m.groupedMode = enabled
	m.updateDelegate()
}

// SetItem updates a single menu item in-place without replacing the whole list.
// Useful for live updates (e.g. refreshing the inline editing row on each keypress).
// SetStatusGutterWidth sets the number of columns to reserve for the status gutter on the left.
func (m *MenuModel) SetStatusGutterWidth(width int) {
	m.statusGutterWidth = width
	m.updateDelegate()
}

// SetActivityGutterWidth sets the number of columns to reserve for activity markers (typically 1 or 2).
func (m *MenuModel) SetActivityGutterWidth(width int) {
	m.activityGutterWidth = width
	m.updateDelegate()
}

// StatusGutterWidth returns the currently configured gutter width for this menu.
func (m *MenuModel) StatusGutterWidth() int {
	lockWidth := 0
	if m.showLockGutter {
		lockWidth = 1
	}

	activityWidth := m.activityGutterWidth
	return lockWidth + activityWidth
}

// RenderItemGutter returns the consistent gutter string (markers) for a menu item.
// It strictly reserves the width defined by m.StatusGutterWidth().
// spinnerChar, when non-empty, overrides the lock gutter slot with a spinner frame.
func (m *MenuModel) RenderItemGutter(item MenuItem, neutralStyle lipgloss.Style, spinnerChar string) string {
	return RenderMenuGutter(item, m.showLockGutter, m.activityGutterWidth, neutralStyle, spinnerChar)
}

// RenderMenuGutter is a standalone helper that returns the consistent gutter string (markers) for a menu item.
// spinnerChar, when non-empty, overrides the lock gutter slot with a spinner frame.
func RenderMenuGutter(item MenuItem, showLockGutter bool, activityGutterWidth int, neutralStyle lipgloss.Style, spinnerChar string) string {
	res := ""

	// 1. Lock Gutter (1 char)
	if showLockGutter {
		if spinnerChar != "" {
			res += neutralStyle.Render(spinnerChar)
		} else if item.IsInvalid {
			res += RenderThemeText("{{|MarkerInvalid|}}"+invalidMarker+"{{[-]}}", neutralStyle)
		} else if item.Locked {
			marker := lockedMarker
			if !GetActiveContext().LineCharacters {
				marker = lockedMarkerAscii
			}
			res += RenderThemeText("{{|MarkerLocked|}}"+marker+"{{[-]}}", neutralStyle)
		} else {
			res += neutralStyle.Render(" ")
		}
	}

	// 2. Activity Gutter 0 (1 char)
	if activityGutterWidth >= 1 {
		g0 := ""
		if item.IsReferenced && !item.IsGroupHeader {
			if item.Checked {
				g0 = RenderThemeText("{{|MarkerAdded|}}R{{[-]}}", neutralStyle)
			} else {
				g0 = RenderThemeText("{{|MarkerModified|}}r{{[-]}}", neutralStyle)
			}
		} else if item.IsCheckbox && !item.IsGroupHeader {
			if item.Checked && !item.WasAdded {
				g0 = RenderThemeText("{{|MarkerAdded|}}+{{[-]}}", neutralStyle)
			} else if !item.Checked && item.WasAdded {
				g0 = RenderThemeText("{{|MarkerDeleted|}}-{{[-]}}", neutralStyle)
			} else {
				g0 = neutralStyle.Render(" ")
			}
		} else {
			g0 = neutralStyle.Render(" ")
		}
		res += g0
	}

	// 3. Activity Gutter 1 (1 char, for Enabled state)
	if activityGutterWidth >= 2 {
		g1 := neutralStyle.Render(" ")
		if !item.IsGroupHeader {
			isRemoving := !item.Checked && item.WasAdded
			if !isRemoving {
				if item.Enabled && !item.WasEnabled {
					g1 = RenderThemeText("{{|MarkerAdded|}}E{{[-]}}", neutralStyle)
				} else if !item.Enabled && item.WasEnabled {
					g1 = RenderThemeText("{{|MarkerDeleted|}}D{{[-]}}", neutralStyle)
				}
			}
		}
		res += g1
	}

	return res
}

func (m *MenuModel) SetItem(index int, item MenuItem) {
	if index < 0 || index >= len(m.items) {
		return
	}
	m.items[index] = item
	m.list.SetItem(index, item)
	m.renderVersion++
	m.InvalidateCache()
}

// SetVariableHeight allows the list viewport to expand instead of forcing pagination
func (m *MenuModel) SetVariableHeight(variable bool) {
	m.variableHeight = variable
}

// SetUpdateInterceptor allows setting a custom handler that runs before normal message processing
func (m *MenuModel) SetUpdateInterceptor(interceptor func(tea.Msg, *MenuModel) (tea.Cmd, bool)) {
	m.interceptor = interceptor
}

// Index returns the current cursor index
func (m *MenuModel) Index() int {
	return m.cursor
}

// FocusedItem returns the currently focused UI element
func (m *MenuModel) FocusedItem() FocusItem {
	return m.focusedItem
}

// Select programmatically sets the cursor index
func (m *MenuModel) Select(index int) {
	if index >= 0 && index < len(m.items) {
		m.cursor = index
		m.list.Select(index)
		menuSelectedIndices[m.persistKey()] = index
	}
}

// GetItems returns the current list of MenuItems
func (m *MenuModel) GetItems() []MenuItem {
	return m.items
}

// GetInnerContentWidth returns the width of the space inside the outer borders
func (m *MenuModel) GetInnerContentWidth() int {
	layout := GetLayout()
	if m.subMenuMode {
		return m.width - layout.BorderWidth()
	}
	// Section-based menus always render at m.width (viewWithSections uses m.width directly).
	if len(m.contentSections) > 0 {
		return m.width - layout.BorderWidth()
	}

	var contentWidth int
	if m.maximized {
		contentWidth, _ = layout.InnerContentSize(m.width, m.height)
	} else {
		contentWidth = m.list.Width() + ScrollbarGutterWidth + layout.BorderWidth() + layout.ContentMarginWidth()
		maxWidth, _ := layout.InnerContentSize(m.width, m.height)
		if contentWidth > maxWidth {
			contentWidth = maxWidth
		}
	}
	return contentWidth
}

// SetItems updates the menu items and refreshes the bubbles list
func (m *MenuModel) SetItems(items []MenuItem) {
	m.items = items

	// Convert MenuItems to list.Items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}
	m.list.SetItems(listItems)

	// Re-apply lock state so item rebuilds don't lose lock markers.
	m.applyItemLocks()

	m.renderVersion++
	m.InvalidateCache()

	// Update delegate with new max tag length and focus
	m.updateDelegate()
}

// SetEscAction sets a custom action for the Escape key
func (m *MenuModel) SetEscAction(action tea.Cmd) {
	m.escAction = action
}

// SetEnterAction sets a custom action for the Enter key
func (m *MenuModel) SetEnterAction(action tea.Cmd) {
	m.enterAction = action
}

// SetSpaceAction sets a custom action for the Space key
func (m *MenuModel) SetSpaceAction(action tea.Cmd) {
	m.spaceAction = action
}

// ActiveColumn returns the currently focused checkbox column (Add or Enable)
func (m *MenuModel) ActiveColumn() CheckboxColumn {
	return m.activeColumn
}

// SelectedItem returns the MenuItem currently under the cursor
func (m *MenuModel) SelectedItem() MenuItem {
	idx := m.list.Index()
	if idx >= 0 && idx < len(m.items) {
		return m.items[idx]
	}
	return MenuItem{}
}

// SetActiveColumn sets the focused checkbox column (Add or Enable)
func (m *MenuModel) SetActiveColumn(col CheckboxColumn) {
	m.activeColumn = col
	m.renderVersion++
	m.updateDelegate()
}

// ToggleSelectedItem toggles the selected state of the current item (for checkbox mode)
func (m *MenuModel) ToggleSelectedItem() {
	idx := m.list.Index()
	if idx >= 0 && idx < len(m.items) && m.items[idx].Selectable {
		if m.items[idx].IsCheckbox || m.items[idx].IsRadioButton {
			if m.groupedMode && m.activeColumn == ColEnable {
				m.items[idx].Enabled = !m.items[idx].Enabled
				if m.items[idx].Enabled {
					m.items[idx].Checked = true // Auto-add if user enables
					m.items[idx].ShowEnabledGutter = true
				}
			} else {
				m.items[idx].Checked = !m.items[idx].Checked
				m.items[idx].Selected = m.items[idx].Checked
				if m.items[idx].Checked {
					m.items[idx].Enabled = true
					m.items[idx].ShowEnabledGutter = true
				} else {
					m.items[idx].Enabled = false
					m.items[idx].ShowEnabledGutter = false
				}
			}
		} else {
			m.items[idx].Selected = !m.items[idx].Selected
		}
		// Update the list item too
		m.list.SetItem(idx, m.items[idx])
		m.renderVersion++
		m.InvalidateCache()
	}
}

// helpContextForIdx builds a HelpContext for the item at the given index.
// Both HelpContext (F1) and showContextMenu (right-click Help) call this so the output is identical.
func (m *MenuModel) helpContextForIdx(idx int) HelpContext {
	itemTitle := "Help"
	itemText := ""
	if idx >= 0 && idx < len(m.items) {
		item := m.items[idx]
		if item.Tag != "" {
			itemTitle = item.Tag
		}
		itemText = item.Help
		if m.itemHelpFunc != nil {
			if t, txt := m.itemHelpFunc(item); txt != "" {
				if t != "" {
					itemTitle = t
				}
				itemText = txt
			}
		}
	}
	if m.helpItemPrefix != "" && itemTitle != "Help" {
		itemTitle = m.helpItemPrefix + ": " + itemTitle
	}

	pageTitle := m.helpPageTitle
	pageText := m.helpPageText
	if pageText == "" {
		pageText = m.subtitle
	}
	if m.helpLegend != "" {
		pageText = ""                   // legend takes precedence; suppress the description
		if pageTitle == "Description" { // Fallback cleanup if previously relied on
			pageTitle = ""
		}
	}
	h := HelpContext{
		ScreenName: m.title,
		PageTitle:  pageTitle,
		PageText:   pageText,
		Legend:     m.helpLegend,
		ItemTitle:  itemTitle,
		ItemText:   itemText,
	}

	if idx >= 0 && idx < len(m.items) && m.itemDocFunc != nil {
		h.DocMarkdown, h.DocAppName = m.itemDocFunc(m.items[idx])
	}

	return h
}

// HelpContext implements HelpContextProvider.
func (m *MenuModel) HelpContext(contentWidth int) HelpContext {
	return m.helpContextForIdx(m.list.Index())
}

// HandleContextMenuKey implements the ContextMenuKeyHandler interface.
// Called by AppModel when Keys.ContextMenu is pressed and no dialog is open.
func (m *MenuModel) HandleContextMenuKey() (tea.Model, tea.Cmd, bool) {
	cmd := m.ShowContextMenu(m.cursor, m.width/2, m.height/2)
	return m, cmd, true
}

// ShowContextMenu returns a command to show the context menu for the item at the given index.
func (m *MenuModel) ShowContextMenu(idx int, x, y int) tea.Cmd {
	var tag, desc string
	var hCtx *HelpContext

	if idx >= 0 && idx < len(m.items) {
		item := m.items[idx]
		tag = GetPlainText(item.Tag)
		desc = item.Desc
		ctx := m.helpContextForIdx(idx)
		hCtx = &ctx
	}

	var items []ContextMenuItem
	if tag != "" {
		items = append(items, ContextMenuItem{IsHeader: true, Label: tag})
		items = append(items, ContextMenuItem{IsSeparator: true})
	}

	// NEW: Inject custom operational items from the screen provider
	if m.contextMenuFunc != nil {
		customItems := m.contextMenuFunc(idx)
		if len(customItems) > 0 {
			items = append(items, customItems...)
			items = append(items, ContextMenuItem{IsSeparator: true})
		}
	}

	var clipItems []ContextMenuItem

	if tag != "" {
		t := tag
		clipItems = append(clipItems, ContextMenuItem{
			Label: "Copy Item Title",
			Help:  "Copy the item's title (tag) to clipboard.",
			Action: func() tea.Msg {
				_ = clipboard.WriteAll(t)
				return CloseDialogMsg{}
			},
		})
	}
	if desc != "" {
		d := desc
		clipItems = append(clipItems, ContextMenuItem{
			Label: "Copy Item Description",
			Help:  "Copy the item's description to clipboard.",
			Action: func() tea.Msg {
				_ = clipboard.WriteAll(d)
				return CloseDialogMsg{}
			},
		})
	}

	items = AppendContextMenuTail(items, clipItems, hCtx)

	return func() tea.Msg {
		return ShowDialogMsg{Dialog: NewContextMenuModel(x, y, m.width, m.height, items)}
	}
}

// Init implements tea.Model
func (m *MenuModel) Init() tea.Cmd {
	return nil
}

// AnyLocked returns true if any item in the menu is marked as Locked.
func (m *MenuModel) AnyLocked() bool {
	for _, item := range m.items {
		if item.Locked {
			return true
		}
	}
	return false
}

