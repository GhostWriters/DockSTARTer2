package classic

import (
	"fmt"
	"strings"
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

	// listFocusOverride keeps list-item/column selection highlighting visible
	// even when focused is false (e.g. a lightweight dialog like a message box
	// is on top) -- unlike focused, it does NOT affect title-bar focus
	// indicators (▸Title◂), which should still reflect that focus has moved
	// to the dialog. Set via SetListFocusOverride.
	listFocusOverride bool

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
	FlowColumns  int  // When > 1, render as N balanced vertical columns
	MaxFlowRows  int  // When > 0, cap visible rows (enables scrolling in flow/column mode)

	// Dialog positioning
	isDialog bool // True when used as a modal dialog — raises hit-region Z priority above screen regions

	// Unified layout (deterministic sizing)
	Layout DialogLayout

	dialogType DialogType

	// Variable height support (for dynamic word wrapping)
	variableHeight  bool                                      // Allow list to expand naturally up to layout limits
	Interceptor     func(tea.Msg, *MenuModel) (tea.Cmd, bool) // Optional custom message handler
	ContentRenderer func(contentWidth int) string             // Optional: replaces list content in viewSubMenu
	onSubFocused    func() tea.Cmd                            // Optional: called when section gains sub-focus

	// wantsAllMessages opts this section into updateSections' catch-all
	// fallback (see WantsAllMessages) -- unlike WantsHorizontalKeys, this is
	// NOT derived from contentRenderer != nil, since sinput sections declare
	// their message needs exhaustively via explicit case types already
	// present in updateSections and must not additionally opt in here. Set
	// only via SetWantsAllMessages.
	wantsAllMessages bool

	// SectionHeightOverride, when set, is a caller-supplied fixed-height
	// formula checked before SectionHeight's generic contentRenderer
	// default -- e.g. a header section whose height depends on dynamic
	// content (subtitle/task-list/progress-bar) rather than a single line.
	SectionHeightOverride func(width int) int

	// OnResize, when set, is called at the end of SetSize with this
	// section's own final (post-border-inset) content dimensions -- for a
	// section whose contentRenderer wraps an inner widget that needs its
	// own explicit resize (e.g. a streaming viewport recalculating wrapped
	// line layout), not just a re-render at the new width.
	OnResize func(width, height int)

	// borderless, combined with contentRenderer != nil, skips viewSubMenu's
	// outer bordered-box wrap entirely -- for a contentRenderer section that
	// renders fully self-contained content (optionally with its own inner
	// border), e.g. a header or streaming-viewport section. Generalizes the
	// bypass plainText already gets, for content beyond a single text line.
	// Set only via SetBorderless.
	borderless bool

	// nonFocusable excludes this section from Tab-cycled focus even though
	// it isn't the plain-text kind (which is non-focusable by default) --
	// e.g. a header section with no interactive content of its own. Set
	// only via SetNonFocusable.
	nonFocusable bool

	// isPlainTextKind marks this MenuModel as the "plain text" Content kind
	// (set only via NewPlainTextSection) -- a borderless, non-focusable
	// single line of theme-styled text instead of a list, used e.g. for a
	// dialog's subtitle expressed as its own content section. Kept separate
	// from plainText being non-empty so a plain-text section constructed
	// with "" (a subtitle set later, e.g. ProgramBoxModel's
	// SetProgramBoxHeaderMsg) still renders as zero-height/borderless
	// instead of falling through to viewSubMenu's bordered list rendering.
	isPlainTextKind bool
	// plainText holds the plain-text kind's current line of text.
	plainText string
	// plainTextThemeTag wraps plainText for RenderThemeText -- "{{|Subtitle|}}"
	// for menu subtitles (default), "" for dialog question/body text that
	// should inherit the surrounding Dialog style unstyled.
	plainTextThemeTag string
	// plainTextVPad adds this many blank lines above and below the text --
	// used by confirm-style dialogs to vertically center the question over
	// the button row without a border.
	plainTextVPad int

	// bottomBorderLabel, when non-empty, is injected into this section's
	// bottom border line (e.g. an INS/OVR mode indicator on a bordered
	// sinput section) via BuildLabeledBottomBorderCtx. Set via
	// SetBottomBorderLabel.
	bottomBorderLabel string

	// borderStyle overrides the corner/edge shape of this section's outer
	// bordered box independent of dialogType. Zero value (BorderStyleAuto)
	// means "let dialogType decide" (DialogTypeConfirm implies angled,
	// everything else is square) -- the pre-existing behavior. Set via
	// SetBorderStyle.
	borderStyle BorderStyle

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
	lastListActive  bool
	lastLineChars   bool
	lastVersion     int
	lastColumn      CheckboxColumn
	lastHitRegions  []HitRegion                                   // Cache for variable height hit regions
	ExtraHitRegions func(offsetX, offsetY, baseZ int) []HitRegion // Optional: extra hit regions injected by section helpers
	ViewStartY      int                                           // Persistent scroll offset for variable height lists
	lastViewStartY  int                                           // Previous scroll offset for memoization check
	lastScrollTotal int                                           // Total content height from last renderVariableHeightList (for scrollbar)

	renderVersion       int // Incremented on item changes to invalidate list cache and top-level view cache
	showLockGutter      bool
	noLeftMargin        bool
	tabEntersButtons    bool // Tab/Shift-Tab wrap into the button row at the end of the section cycle; off by default (Left/Right always reach buttons regardless)
	statusGutterWidth   int
	activityGutterWidth int
	itemPaddingWidth    int    // Optional padding after getters
	menuName            string // Name used for --menu or -M to return to this screen
	connType            string // "local", "ssh", or "web"
	externalLock        bool   // Whether destructive items are locked by an external session (persists across SetItems)
	commandLock         bool   // Whether destructive items are locked by a running panel command

	// Content sections: sub-menus (or ContentRows of sub-menus) rendered
	// stacked inside the outer border. When present, replaces the standard
	// list+inner-border rendering.
	contentSections []Content
	focusedSection  int // index into contentSections; -1 = buttons focused

	// SectionHelp is help text shown in the helpline when this section has
	// no items of its own (e.g. a sinput section) but should still describe
	// itself when focused. See HelpText.
	SectionHelp string

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

	// titleSpinnerIndicator, when set, overrides the title-bar spinner
	// indicators independent of loadingText -- e.g. a wrapper (like
	// ProgramBoxModel) that owns its own task-running spinner state and
	// wants it reflected in this outer MenuModel's title bar without
	// triggering loadingText's full content-area replacement.
	titleSpinnerIndicator func() (left, right string)

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

// IsScrollbarDragging reports whether the menu is currently processing a
// scrollbar thumb drag. AppModel uses this to give the active screen drag
// priority (model_mouse.go) -- mouse motion/release during a drag would
// otherwise never reach Update(). Recurses into content sections: a plain
// *MenuModel container has no scrollbar of its own, so the drag is
// happening on a nested section's Scroll instead.
func (m *MenuModel) IsScrollbarDragging() bool {
	if m.Scroll.Drag.Dragging {
		return true
	}
	for _, sec := range m.contentSections {
		if d, ok := sec.(interface{ IsScrollbarDragging() bool }); ok && d.IsScrollbarDragging() {
			return true
		}
	}
	return false
}

// ScrollTotal returns the total scrollable units (lines or items).
func (m *MenuModel) ScrollTotal() int {
	if m.variableHeight {
		return m.lastScrollTotal
	}
	if m.FlowColumns >= 2 && m.MaxFlowRows > 0 {
		// Return total rows (not items) for column scroll.
		n := 0
		for _, item := range m.items {
			if !item.IsSeparator {
				n++
			}
		}
		return (n + m.FlowColumns - 1) / m.FlowColumns
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

// CheckboxColumn represents which column (Add, Enable, Expand, or Name) has focus in a row
type CheckboxColumn int

const (
	ColAdd CheckboxColumn = iota
	ColEnable
	ColExpand
	ColName
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
		showButtons:         false,
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

// SetTabEntersButtons controls whether Tab/Shift-Tab wrap into the button row
// at the end of the content-section cycle. Off by default: Enter already
// activates the dual-focused button from any section, and Left/Right always
// reach every button regardless of this setting, so most dialogs don't need
// Tab as a third path there.
func (m *MenuModel) SetTabEntersButtons(v bool) {
	m.tabEntersButtons = v
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

// IsVariableHeight reports whether this menu expands to fill available space
// when used as a content section, rather than having a fixed intrinsic
// height. Part of the Content interface.
func (m *MenuModel) IsVariableHeight() bool { return m.variableHeight }

// ScrollID returns the ID of this menu's own scrollbar, for ScrollDoneMsg
// routing. Part of the Content interface.
func (m *MenuModel) ScrollID() string { return m.Scroll.ID }

// MatchesID reports whether msgID belongs to this menu (hit-region IDs like
// "item-{id}-{index}" contain the section's own id as a substring). Part of
// the Content interface.
func (m *MenuModel) MatchesID(msgID string) bool {
	return strings.Contains(msgID, m.id)
}

// WantsHorizontalKeys reports true when this menu has a contentRenderer (the
// sinput text-input kind), which consumes Left/Right itself for cursor
// movement via its own interceptor. Part of the Content interface.
func (m *MenuModel) WantsHorizontalKeys() bool {
	return m.ContentRenderer != nil
}

// WantsAllMessages reports whether updateSections' catch-all should forward
// otherwise-unhandled message types to this section. Part of the Content
// interface. See SetWantsAllMessages.
func (m *MenuModel) WantsAllMessages() bool {
	return m.wantsAllMessages
}

// SetWantsAllMessages opts this section into updateSections' catch-all
// fallback for message types none of its explicit cases match.
func (m *MenuModel) SetWantsAllMessages(v bool) {
	m.wantsAllMessages = v
}

// Focusable reports false for the plain-text kind (a read-only display line
// with nothing to interact with) and any section explicitly marked
// non-focusable via SetNonFocusable; every other kind can receive Tab focus.
// Part of the Content interface.
func (m *MenuModel) Focusable() bool {
	return !m.isPlainTextKind && !m.nonFocusable && !m.disabled
}

// SetBorderless skips viewSubMenu's outer bordered-box wrap for this
// contentRenderer section, so its render closure is responsible for its own
// (optional) framing.
func (m *MenuModel) SetBorderless(v bool) {
	m.borderless = v
}

// SetNonFocusable excludes this section from Tab-cycled focus.
func (m *MenuModel) SetNonFocusable(v bool) {
	m.nonFocusable = v
}

// SetTitleSpinnerIndicator overrides this MenuModel's title-bar spinner
// indicators, independent of loadingText.
func (m *MenuModel) SetTitleSpinnerIndicator(fn func() (left, right string)) {
	m.titleSpinnerIndicator = fn
}

// IsProcessing reports whether this menu has an in-flight item or button
// action. Part of the Content interface.
func (m *MenuModel) IsProcessing() bool {
	return m.processingItemIdx >= 0 || m.btnRow.IsProcessing()
}

// SetDialogType sets the visual style/type of the menu dialog
func (m *MenuModel) SetDialogType(t DialogType) { m.dialogType = t }

// SetBottomBorderLabel injects label into this section's bottom border line
// (e.g. an INS/OVR indicator). Pass "" to clear it. Invalidates the render
// cache so a changing label (e.g. toggling INS/OVR while typing) is picked
// up immediately.
func (m *MenuModel) SetBottomBorderLabel(label string) {
	if m.bottomBorderLabel == label {
		return
	}
	m.bottomBorderLabel = label
	m.InvalidateCache()
}

// SetBorderStyle overrides the corner/edge shape of this section's outer
// bordered box independent of dialogType. Use BorderStyleAuto to revert to
// the default (DialogTypeConfirm implies angled, everything else square).
func (m *MenuModel) SetBorderStyle(style BorderStyle) {
	m.borderStyle = style
}

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

// GetFocusedItem returns which UI element currently has focus (list or a button).
func (m *MenuModel) GetFocusedItem() FocusItem {
	return m.focusedItem
}

// SetFocusedBtnIndex sets the focused button index within the buttons slice.
func (m *MenuModel) SetFocusedBtnIndex(idx int) {
	m.focusedBtnIndex = idx
	m.focusedItem = FocusBtn
	m.InvalidateCache()
}

// GetFocusedBtnIndex returns the index of the currently focused button.
func (m *MenuModel) GetFocusedBtnIndex() int {
	return m.focusedBtnIndex
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
	return m.Layout.ButtonHeight
}

// HasLargeTitleBar reports whether the current layout uses a large title bar.
func (m *MenuModel) HasLargeTitleBar() bool {
	return m.Layout.LargeTitleBar
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
		// Screens are reused pointers across Back navigation, not
		// reconstructed, so focus must be reset explicitly here.
		m.focusedItem = FocusList
		m.focusedBtnIndex = 0
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

// SetListFocusOverride keeps list-item/column selection highlighting visible
// even while IsActive() is false, without affecting title-bar focus indicators.
// Used when a lightweight dialog (e.g. a plain message box) is on top and the
// underlying list's selection should still read clearly, but the title
// shouldn't falsely claim keyboard focus is still here.
func (m *MenuModel) SetListFocusOverride(v bool) {
	m.listFocusOverride = v
}

// IsListActive is like IsActive but also true when SetListFocusOverride(true)
// is set -- the signal list-item rendering should use instead of IsActive.
func (m *MenuModel) IsListActive() bool {
	return m.IsActive() || m.listFocusOverride
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
	m.Interceptor = interceptor
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

// focusedSectionMenu resolves the innermost *MenuModel actually holding the
// user's focus: for an outer container built with no items of its own (e.g.
// NewMenuModel(id, title, "", nil) wrapping content sections, the pattern
// used by Config Menu/Main Menu/Options Menu/etc.), m.list.Index() and
// m.cursor are meaningless -- the real focused item lives inside whichever
// content section currently has focus. Recurses through ContentRow wrappers
// to find the actual *MenuModel. Returns m itself when it has real items
// (the plain-list case) or no focusable section was found.
func (m *MenuModel) focusedSectionMenu() *MenuModel {
	if len(m.items) > 0 || len(m.contentSections) == 0 {
		return m
	}
	if m.focusedSection < 0 || m.focusedSection >= len(m.contentSections) {
		return m
	}
	c := m.contentSections[m.focusedSection]
	for {
		if row, ok := c.(*ContentRow); ok {
			items := row.Items()
			idx := row.SubFocusIndex()
			if idx < 0 || idx >= len(items) {
				return m
			}
			c = items[idx]
			continue
		}
		break
	}
	if mm, ok := c.(*MenuModel); ok {
		return mm
	}
	return m
}

// HelpContext implements HelpContextProvider.
func (m *MenuModel) HelpContext(contentWidth int) HelpContext {
	target := m.focusedSectionMenu()
	return target.helpContextForIdx(target.list.Index())
}

// HandleContextMenuKey implements the ContextMenuKeyHandler interface.
// Called by AppModel when Keys.ContextMenu is pressed and no dialog is open.
func (m *MenuModel) HandleContextMenuKey() (tea.Model, tea.Cmd, bool) {
	target := m.focusedSectionMenu()

	// m's actual screen position varies: a maximized screen is always at
	// (EdgeIndent, ContentStartY), but a centered dialog (e.g. Main Menu) is
	// positioned by DialogPosition based on its own content size, so
	// GetActiveDialogOffset reports whatever AppModel.View() computed for m
	// this frame. m.GetHitRegions already recurses into every content
	// section at the correct cumulative offset, so querying m (not target)
	// reuses that existing offset math instead of reimplementing it.
	absOffsetX, absOffsetY := GetActiveDialogOffset()

	x, y := target.width/2+absOffsetX, target.height/2+absOffsetY
	idx := target.cursor
	if idx >= 0 {
		for _, r := range m.GetHitRegions(absOffsetX, absOffsetY) {
			if suffix, ok := ParseMenuItemIndex(r.ID, target.id); ok && suffix == idx {
				x, y = r.X, r.Y
				break
			}
		}
	}
	cmd := target.ShowContextMenu(idx, x, y)
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

	// m.width/m.height are section-local when m is a content section nested
	// inside an outer sectioned container (e.g. Config Menu's item list) --
	// use the real terminal size instead so the popup clamps/positions
	// correctly regardless of which MenuModel actually owns the item.
	screenW, screenH := GetActiveScreenSize()

	return func() tea.Msg {
		return ShowDialogMsg{Dialog: NewContextMenuModel(x, y, screenW, screenH, items)}
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

// IsDestructive reports whether this menu can modify data.
// Default for MenuModel is false (read-only navigation).
func (m *MenuModel) IsDestructive() bool { return false }
