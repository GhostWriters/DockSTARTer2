// Package enveditor provides a multi-line text input component for Bubble Tea
// applications, augmented to support locked lines and formatting for .env files.
package enveditor

import (
	"crypto/sha256"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/cursor"
	"DockSTARTer2/internal/tui/components/enveditor/memoization"
	"DockSTARTer2/internal/tui/components/enveditor/runeutil"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	rw "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
	"github.com/sergi/go-diff/diffmatchpatch"
)

const (
	minHeight        = 1
	defaultHeight    = 6
	defaultWidth     = 40
	defaultCharLimit = 0 // no limit
	defaultMaxHeight = 99
	defaultMaxWidth  = 500

	// XXX: in v2, make max lines dynamic and default max lines configurable.
	maxLines = 10000
)

// Internal messages for clipboard operations.
type (
	pasteMsg    string
	pasteErrMsg struct{ error }
)

// undoSnapshot captures editor state before a modifying operation.
type undoSnapshot struct {
	value    [][]rune
	lineMeta []Line
	row, col int
}

// KeyMap is the key bindings for different actions within the textarea.
type KeyMap struct {
	CharacterBackward       key.Binding
	CharacterForward        key.Binding
	DeleteAfterCursor       key.Binding
	DeleteBeforeCursor      key.Binding
	DeleteCharacterBackward key.Binding
	DeleteCharacterForward  key.Binding
	DeleteWordBackward      key.Binding
	DeleteWordForward       key.Binding
	InsertNewline           key.Binding
	SplitLine               key.Binding
	LineEnd                 key.Binding
	LineNext                key.Binding
	LinePrevious            key.Binding
	LineStart               key.Binding
	PageUp                  key.Binding
	PageDown                key.Binding
	Paste                   key.Binding
	WordBackward            key.Binding
	WordForward             key.Binding
	InputBegin              key.Binding
	InputEnd                key.Binding

	UppercaseWordForward  key.Binding
	LowercaseWordForward  key.Binding
	CapitalizeWordForward key.Binding

	TransposeCharacterBackward key.Binding
	InsertLine                 key.Binding
	Undo                       key.Binding
	Redo                       key.Binding

	// Copy selection or value to clipboard
	Copy key.Binding

	// Keyboard text selection (shift+arrow)
	SelectLeft  key.Binding
	SelectRight key.Binding
	SelectHome  key.Binding
	SelectEnd   key.Binding

	// ToggleInsert switches between insert and overwrite mode.
	ToggleInsert key.Binding
}

// DefaultKeyMap returns the default set of key bindings for navigating and acting
// upon the textarea.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		CharacterForward:        key.NewBinding(key.WithKeys("right", "ctrl+f"), key.WithHelp("right", "character forward")),
		CharacterBackward:       key.NewBinding(key.WithKeys("left", "ctrl+b"), key.WithHelp("left", "character backward")),
		WordForward:             key.NewBinding(key.WithKeys("alt+right", "alt+f"), key.WithHelp("alt+right", "word forward")),
		WordBackward:            key.NewBinding(key.WithKeys("alt+left", "alt+b"), key.WithHelp("alt+left", "word backward")),
		LineNext:                key.NewBinding(key.WithKeys("down"), key.WithHelp("down", "next line")),
		LinePrevious:            key.NewBinding(key.WithKeys("up"), key.WithHelp("up", "previous line")),
		DeleteWordBackward:      key.NewBinding(key.WithKeys("alt+backspace", "ctrl+w"), key.WithHelp("alt+backspace", "delete word backward")),
		DeleteWordForward:       key.NewBinding(key.WithKeys("alt+delete", "alt+d"), key.WithHelp("alt+delete", "delete word forward")),
		DeleteAfterCursor:       key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("ctrl+k", "delete after cursor")),
		DeleteBeforeCursor:      key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "delete before cursor")),
		InsertNewline:           key.NewBinding(key.WithKeys("enter", "ctrl+m"), key.WithHelp("enter", "insert newline")),
		SplitLine:               key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "split line at cursor")),
		DeleteCharacterBackward: key.NewBinding(key.WithKeys("backspace"), key.WithHelp("backspace", "delete character backward")),
		DeleteCharacterForward:  key.NewBinding(key.WithKeys("delete", "ctrl+d"), key.WithHelp("delete", "delete character forward")),
		LineStart:               key.NewBinding(key.WithKeys("home", "ctrl+a"), key.WithHelp("home", "line start")),
		LineEnd:                 key.NewBinding(key.WithKeys("end", "ctrl+e"), key.WithHelp("end", "line end")),
		PageUp:                  key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown:                key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "page down")),
		Paste:                   key.NewBinding(key.WithKeys("ctrl+v"), key.WithHelp("ctrl+v", "paste")),
		InputBegin:              key.NewBinding(key.WithKeys("alt+<", "ctrl+home"), key.WithHelp("alt+<", "input begin")),
		InputEnd:                key.NewBinding(key.WithKeys("alt+>", "ctrl+end"), key.WithHelp("alt+>", "input end")),

		CapitalizeWordForward: key.NewBinding(key.WithKeys("alt+c"), key.WithHelp("alt+c", "capitalize word forward")),
		LowercaseWordForward:  key.NewBinding(key.WithKeys("alt+l"), key.WithHelp("alt+l", "lowercase word forward")),
		UppercaseWordForward:  key.NewBinding(key.WithKeys("alt+u"), key.WithHelp("alt+u", "uppercase word forward")),

		TransposeCharacterBackward: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "transpose character backward")),
		InsertLine:                 key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "insert line")),
		Undo:                       key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("ctrl+z", "undo")),
		Redo:                       key.NewBinding(key.WithKeys("ctrl+y", "ctrl+shift+z"), key.WithHelp("ctrl+y", "redo")),
		Copy:                       key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "copy")),
		SelectLeft:                 key.NewBinding(key.WithKeys("shift+left"), key.WithHelp("shift+left", "select left")),
		SelectRight:                key.NewBinding(key.WithKeys("shift+right"), key.WithHelp("shift+right", "select right")),
		SelectHome:                 key.NewBinding(key.WithKeys("shift+home"), key.WithHelp("shift+home", "select to start")),
		SelectEnd:                  key.NewBinding(key.WithKeys("shift+end"), key.WithHelp("shift+end", "select to end")),
		ToggleInsert:               key.NewBinding(key.WithKeys("insert"), key.WithHelp("insert", "toggle insert/overwrite")),
	}
}

// LineInfo is a helper for keeping track of line information regarding
// soft-wrapped lines.
type LineInfo struct {
	// Width is the number of columns in the line.
	Width int

	// CharWidth is the number of characters in the line to account for
	// double-width runes.
	CharWidth int

	// Height is the number of rows in the line.
	Height int

	// StartColumn is the index of the first column of the line.
	StartColumn int

	// ColumnOffset is the number of columns that the cursor is offset from the
	// start of the line.
	ColumnOffset int

	// RowOffset is the number of rows that the cursor is offset from the start
	// of the line.
	RowOffset int

	// CharOffset is the number of characters that the cursor is offset
	// from the start of the line. This will generally be equivalent to
	// ColumnOffset, but will be different there are double-width runes before
	// the cursor.
	CharOffset int
}

// PromptInfo is a struct that can be used to store information about the
// prompt.
type PromptInfo struct {
	LineNumber int
	Focused    bool
}

// Line represents a single line in our environment variable editor.
type Line struct {
	Text             string
	ReadOnly         bool
	IsVariable       bool // specific formatting for KEY=VALUE
	IsUserDefined    bool // can be reordered
	EditableStartCol int
	DefaultValue     string
	PendingDelete    bool   // marked for deletion on next save; shown with strikethrough
	InitialLine      string // full line text at load time, used for changed (C) gutter marker
	IsNewLine        bool   // added by the user after load; shows + in gutter
	IsInvalid        bool   // in user-defined section but key is in readOnlyVars; shows ! in gutter
}

// CursorStyle is the style for real and virtual cursors.
type CursorStyle struct {
	// Style styles the cursor block.
	//
	// For real cursors, the foreground color set here will be used as the
	// cursor color.
	Color color.Color

	// Shape is the cursor shape. The following shapes are available:
	//
	// - tea.CursorBlock
	// - tea.CursorUnderline
	// - tea.CursorBar
	//
	// This is only used for real cursors.
	Shape tea.CursorShape

	// CursorBlink determines whether or not the cursor should blink.
	Blink bool

	// BlinkSpeed is the speed at which the virtual cursor blinks. This has no
	// effect on real cursors as well as no effect if the cursor is set not to
	// [CursorBlink].
	//
	// By default, the blink speed is set to about 500ms.
	BlinkSpeed time.Duration
}

// Styles are the styles for the textarea, separated into focused and blurred
// states. The appropriate styles will be chosen based on the focus state of
// the textarea.
type Styles struct {
	Focused StyleState
	Blurred StyleState
	Cursor  CursorStyle
}

// StyleState that will be applied to the text area.
//
// StyleState can be applied to focused and unfocused states to change the styles
// depending on the focus state.
//
// For an introduction to styling with Lip Gloss see:
// https://github.com/charmbracelet/lipgloss
type StyleState struct {
	Base                     lipgloss.Style
	Text                     lipgloss.Style
	LineNumber               lipgloss.Style
	LineNumberSelected       lipgloss.Style // cursor line
	LineNumberModified       lipgloss.Style // line differs from default
	LineNumberModifiedSelected lipgloss.Style // cursor line + differs from default
	CursorLine               lipgloss.Style
	EndOfBuffer      lipgloss.Style
	Placeholder      lipgloss.Style
	Prompt           lipgloss.Style
	ModifiedText      lipgloss.Style
	ReadOnlyText      lipgloss.Style
	InvalidText       lipgloss.Style
	DuplicateText     lipgloss.Style
	BuiltinText       lipgloss.Style
	UserDefinedText   lipgloss.Style
	PendingDeleteText lipgloss.Style
	GutterAdded       lipgloss.Style // + marker for new lines
	GutterDeleted     lipgloss.Style // - marker for pending-delete lines
	GutterModified    lipgloss.Style // ~ marker for changed lines
	GutterInvalid     lipgloss.Style // ! marker for protected vars entered in user-defined section
	ScrollbarTrack    lipgloss.Style
	ScrollbarThumb    lipgloss.Style
	SelectionText     lipgloss.Style
}

func (s StyleState) computedCursorLine() lipgloss.Style {
	return s.CursorLine.Inherit(s.Base).Inline(true)
}

func (s StyleState) computedLineNumberSelected() lipgloss.Style {
	return s.LineNumberSelected.
		Inherit(s.CursorLine).
		Inherit(s.Base).
		Inline(true)
}

func (s StyleState) computedLineNumberModified() lipgloss.Style {
	return s.LineNumberModified.Inherit(s.Base).Inline(true)
}

func (s StyleState) computedLineNumberModifiedSelected() lipgloss.Style {
	return s.LineNumberModifiedSelected.
		Inherit(s.CursorLine).
		Inherit(s.Base).
		Inline(true)
}

func (s StyleState) computedEndOfBuffer() lipgloss.Style {
	return s.EndOfBuffer.Inherit(s.Base).Inline(true)
}

func (s StyleState) computedLineNumber() lipgloss.Style {
	return s.LineNumber.Inherit(s.Base).Inline(true)
}

func (s StyleState) computedPlaceholder() lipgloss.Style {
	return s.Placeholder.Inherit(s.Base).Inline(true)
}

func (s StyleState) computedPrompt() lipgloss.Style {
	return s.Prompt.Inherit(s.Base).Inline(true)
}

func (s StyleState) computedText() lipgloss.Style {
	return s.Text.Inherit(s.Base).Inline(true)
}

// line is the input to the text wrapping function. This is stored in a struct
// so that it can be hashed and memoized.
type line struct {
	runes []rune
	width int
}

// Hash returns a hash of the line.
func (w line) Hash() string {
	v := fmt.Sprintf("%s:%d", string(w.runes), w.width)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(v)))
}

// Model is the Bubble Tea model for this text area element.
type Model struct {
	Err error

	// General settings.
	cache *memoization.MemoCache[line, [][]rune]

	// Prompt is printed at the beginning of each line.
	//
	// When changing the value of Prompt after the model has been
	// initialized, ensure that SetWidth() gets called afterwards.
	//
	// See also [SetPromptFunc] for a dynamic prompt.
	Prompt string

	// Placeholder is the text displayed when the user
	// hasn't entered anything yet.
	Placeholder string

	// ShowLineNumbers, if enabled, causes line numbers to be printed
	// after the prompt.
	ShowLineNumbers bool

	// EndOfBufferCharacter is displayed at the end of the input.
	EndOfBufferCharacter rune

	// KeyMap encodes the keybindings recognized by the widget.
	KeyMap KeyMap

	// virtualCursor manages the virtual cursor.
	virtualCursor cursor.Model

	// CharLimit is the maximum number of characters this input element will
	// accept. If 0 or less, there's no limit.
	CharLimit int

	// MaxHeight is the maximum height of the text area in rows. If 0 or less,
	// there's no limit.
	MaxHeight int

	// MaxWidth is the maximum width of the text area in columns. If 0 or less,
	// there's no limit.
	MaxWidth int

	// LineCharacters determines whether to use stylized line-art characters for
	// scrollbars and other UI elements.
	LineCharacters bool

	// ScrollbarFunc, when non-nil, is called to append a scrollbar/gutter column
	// to the rendered viewport text. It has the same signature as
	// tui.ApplyScrollbarColumn. When nil the textarea falls back to its built-in
	// scrollbar renderer.
	ScrollbarFunc func(content string, total, visible, offset int, enabled bool, lineChars bool) string

	// Styling. Styles are defined in [Styles]. Use [SetStyles] and [GetStyles]
	// to work with this value publicly.
	styles Styles

	// useVirtualCursor determines whether or not to use the virtual cursor.
	// Use [SetVirtualCursor] and [VirtualCursor] to work with this this
	// value publicly.
	useVirtualCursor bool

	// If promptFunc is set, it replaces Prompt as a generator for
	// prompt strings at the beginning of each line.
	promptFunc func(PromptInfo) string

	// promptWidth is the width of the prompt.
	promptWidth int

	// width is the maximum number of characters that can be displayed at once.
	// If 0 or less this setting is ignored.
	width int

	// height is the maximum number of lines that can be displayed at once. It
	// essentially treats the text field like a vertically scrolling viewport
	// if there are more lines than the permitted height.
	height int

	// Underlying text value.
	value [][]rune

	// line properties tracking readonly and editable regions
	lineMeta []Line

	// Overwrite determines whether typing replaces existing characters (overwrite mode)
	// rather than inserting before the cursor. Toggled by the Insert key.
	Overwrite bool

	// focus indicates whether user input focus should be on this input
	// component. When false, ignore keyboard input and hide the cursor.
	focus bool

	// Cursor column.
	col int

	// Cursor row.
	row int

	// Last character offset, used to maintain state when the cursor is moved
	// vertically such that we can maintain the same navigating position.
	lastCharOffset int

	// viewport is the vertically-scrollable viewport of the multi-line text
	// input.
	viewport *viewport.Model

	// rune sanitizer for input.
	rsan runeutil.Sanitizer

	// Dragging state for reordering
	isDragging bool
	draggedRow int

	// Scrollbar dragging state
	isScrollbarDragging  bool
	sbDragMouseOffsetY   int // relative offset of mouse within thumb when drag started
	// sbScrolled is set to true whenever a scrollbar action directly sets the
	// viewport offset (drag, track click, arrow click). It suppresses the
	// repositionView() snap at the end of Update() so the user can scroll the
	// view to see non-editable lines (e.g. comments) without the cursor
	// snapping the view back.
	sbScrolled bool

	// Undo/redo history
	undoStack []undoSnapshot
	redoStack []undoSnapshot

	// DefaultValueFunc, if set, is called with the variable name when the user
	// types '=' at the end of a new variable line. If it returns a non-empty
	// value (not just ''), that value is automatically inserted after '='.
	DefaultValueFunc func(varName string) string

	// Text selection state (single-row, left-click drag)
	isSelecting  bool // mouse button held, tracking drag selection
	selActive    bool // a selection region is active
	selRow       int  // logical row of selection
	selAnchorCol int  // col where mouse-down occurred
	selStartCol  int  // inclusive start col (always <= selEndCol)
	selEndCol    int  // exclusive end col

	// Multi-click tracking (double/triple/quad click selection)
	lastClickTime time.Time
	lastClickRow  int
	lastClickCol  int
	clickCount    int

	// Total visual width set by SetWidth
	totalWidth int

	// Memoization for expensive rendering
	lastView   string
	cacheValid bool // Indicates if lastView is up-to-date with current state
	dmp        *diffmatchpatch.DiffMatchPatch
	diffCache  map[int][]bool // row index -> modified mask (true = modified)

	// Intelligent variable addition settings.
	AddPrefix         string
	ValidationType    string // _GLOBAL_, _BARE_, or APPNAME (actual app name)
	ValidationAppName string // Actual app name if ValidationType is APPNAME
	ValidateFunc      func(string, string) bool

	// Theme integration for duplicates
	duplicateKeys map[string]int
}

// New creates a new model with default settings.
func New() Model {
	vp := viewport.New()
	vp.KeyMap = viewport.KeyMap{}
	cur := cursor.New()

	styles := DefaultDarkStyles()

	m := Model{
		CharLimit:            defaultCharLimit,
		MaxHeight:            defaultMaxHeight,
		MaxWidth:             defaultMaxWidth,
		Prompt:               " ",
		styles:               styles,
		cache:                memoization.NewMemoCache[line, [][]rune](maxLines),
		EndOfBufferCharacter: ' ',
		ShowLineNumbers:      true,
		useVirtualCursor:     true,
		virtualCursor:        cur,
		KeyMap:               DefaultKeyMap(),

		value:    make([][]rune, minHeight, maxLines),
		lineMeta: make([]Line, minHeight, maxLines),
		focus:    false,
		col:      0,
		row:      0,

		viewport: &vp,
		dmp:      diffmatchpatch.New(),
		diffCache: make(map[int][]bool),
	}

	m.SetHeight(defaultHeight)
	m.SetWidth(defaultWidth)

	return m
}

// DefaultStyles returns the default styles for focused and blurred states for
// the textarea.
func DefaultStyles(isDark bool) Styles {
	lightDark := lipgloss.LightDark(isDark)

	var s Styles
	s.Focused = StyleState{
		Base:                       lipgloss.NewStyle(),
		CursorLine:                 lipgloss.NewStyle(),
		LineNumber:                 lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("249"), lipgloss.Color("7"))),
		LineNumberSelected:         lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("240"), lipgloss.Color("240"))),
		LineNumberModified:         lipgloss.NewStyle().Foreground(lipgloss.Color("3")), // Yellow
		LineNumberModifiedSelected: lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
		EndOfBuffer:                lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("254"), lipgloss.Color("0"))),
		Placeholder:      lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Prompt:           lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		Text:             lipgloss.NewStyle(),
		ModifiedText:     lipgloss.NewStyle().Foreground(lipgloss.Color("3")),   // Yellow
		ReadOnlyText:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")), // Dark Grey
		InvalidText:      lipgloss.NewStyle().Foreground(lipgloss.Color("9")),   // Red
		DuplicateText:    lipgloss.NewStyle().Foreground(lipgloss.Color("13")),  // Magenta
		BuiltinText:       lipgloss.NewStyle(),                                                                    // Inherit from text by default
		UserDefinedText:   lipgloss.NewStyle(),                                                                    // Inherit from text by default
		PendingDeleteText: lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("240")),
		GutterAdded:       lipgloss.NewStyle().Foreground(lipgloss.Color("2")),  // Green
		GutterDeleted:     lipgloss.NewStyle().Foreground(lipgloss.Color("1")),  // Red
		GutterModified:    lipgloss.NewStyle().Foreground(lipgloss.Color("3")),  // Yellow
		GutterInvalid:     lipgloss.NewStyle().Foreground(lipgloss.Color("9")),  // Bright red
		ScrollbarTrack:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		ScrollbarThumb:    lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		SelectionText:     lipgloss.NewStyle().Reverse(true),
	}
	s.Blurred = StyleState{
		Base:                       lipgloss.NewStyle(),
		CursorLine:                 lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("245"), lipgloss.Color("7"))),
		LineNumber:                 lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("249"), lipgloss.Color("7"))),
		LineNumberSelected:         lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("249"), lipgloss.Color("7"))),
		LineNumberModified:         lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		LineNumberModifiedSelected: lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
		EndOfBuffer:                lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("254"), lipgloss.Color("0"))),
		Placeholder:      lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Prompt:           lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		Text:             lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("245"), lipgloss.Color("7"))),
		ModifiedText:     lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		ReadOnlyText:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		InvalidText:      lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		DuplicateText:    lipgloss.NewStyle().Foreground(lipgloss.Color("13")),
		BuiltinText:       lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		UserDefinedText:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		PendingDeleteText: lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("240")),
		GutterAdded:       lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		GutterDeleted:     lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		GutterModified:    lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		GutterInvalid:     lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		ScrollbarTrack:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		ScrollbarThumb:    lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		SelectionText:     lipgloss.NewStyle().Reverse(true),
	}
	s.Cursor = CursorStyle{
		Color: lipgloss.Color("7"),
		Shape: tea.CursorBlock,
		Blink: true,
	}
	return s
}

// DefaultLightStyles returns the default styles for a light background.
func DefaultLightStyles() Styles {
	return DefaultStyles(false)
}

// DefaultDarkStyles returns the default styles for a dark background.
func DefaultDarkStyles() Styles {
	return DefaultStyles(true)
}

// Styles returns the current styles for the textarea.
func (m Model) Styles() Styles {
	return m.styles
}

// SetStyles updates styling for the textarea.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.updateVirtualCursorStyle()
	m.cacheValid = false
}

// IsOverwrite returns true when the textarea is in overwrite (replace) mode.
func (m Model) IsOverwrite() bool { return m.Overwrite }

// IsEditableAtCursor returns true when the cursor is on an editable position.
func (m Model) IsEditableAtCursor() bool { return m.isEditableAtCursor() }

// VirtualCursor returns whether or not the virtual cursor is enabled.
func (m Model) VirtualCursor() bool {
	return m.useVirtualCursor
}

// SetVirtualCursor sets whether or not to use the virtual cursor.
func (m *Model) SetVirtualCursor(v bool) {
	m.useVirtualCursor = v
	m.updateVirtualCursorStyle()
}

// updateVirtualCursorStyle sets styling on the virtual cursor based on the
// textarea's style settings.
func (m *Model) updateVirtualCursorStyle() {
	if !m.useVirtualCursor {
		m.virtualCursor.SetMode(cursor.CursorHide)
		return
	}

	m.virtualCursor.Style = lipgloss.NewStyle().Foreground(m.styles.Cursor.Color)

	// By default, the blink speed of the cursor is set to a default
	// internally.
	if m.styles.Cursor.Blink {
		if m.styles.Cursor.BlinkSpeed > 0 {
			m.virtualCursor.BlinkSpeed = m.styles.Cursor.BlinkSpeed
		}
		m.virtualCursor.SetMode(cursor.CursorBlink)
		return
	}
	m.virtualCursor.SetMode(cursor.CursorStatic)
}

// SetValue sets the value of the text input.
func (m *Model) SetValue(s string) {
	m.Reset()
	m.InsertString(s)
}

// InsertString inserts a string at the cursor position.
func (m *Model) InsertString(s string) {
	m.insertRunesFromUserInput([]rune(s))
}

// InsertRune inserts a rune at the cursor position.
func (m *Model) InsertRune(r rune) {
	m.insertRunesFromUserInput([]rune{r})
}

// insertRunesFromUserInput inserts runes at the current cursor position.
func (m *Model) insertRunesFromUserInput(runes []rune) {
	m.insertRunes(runes, false)
}

func (m *Model) insertRunes(runes []rune, literal bool) {
	m.invalidateDiffCache(m.row)
	if !literal {
		// Intelligent Prefix Handling
		if m.AddPrefix != "" && m.row < len(m.lineMeta) && !m.lineMeta[m.row].ReadOnly && len(m.value[m.row]) == 0 && len(runes) > 0 {
			// Prepend the app prefix on any blank editable line — works even when
			// no User Defined section exists yet (IsUserDefined is false in that case).
			prefixRunes := []rune(strings.ReplaceAll(m.AddPrefix, "APPNAME", m.ValidationAppName))
			runes = append(prefixRunes, runes...)

			// Adjust EditableStartCol if we just inserted a prefix
			m.lineMeta[m.row].EditableStartCol = len(prefixRunes)
		}

		// Strict Key Validation & = Handling — applies to any editable line, not just
		// those in the user-defined section (a built-in var typed anywhere is still built-in).
		if m.ValidationType != "" && m.row < len(m.lineMeta) && !m.lineMeta[m.row].ReadOnly {
			meta := &m.lineMeta[m.row]

			// Find if line already has an '='
			eqIdx := -1
			for i, cr := range m.value[m.row] {
				if cr == '=' {
					eqIdx = i
					break
				}
			}

			// If we are still in the key part (no "=" yet, or cursor is before/at existing "=")
			if eqIdx == -1 || m.col <= eqIdx {
				filtered := make([]rune, 0, len(runes))
				for _, r := range runes {
					if r == '=' {
						// Validate the key before allowing "="
						key := string(m.value[m.row][:m.col])
						vType := m.ValidationType
						if vType == "APPNAME" {
							vType = m.ValidationAppName
						}
						if m.ValidateFunc != nil && !m.ValidateFunc(key, vType) {
							// Block "=" if key is invalid
							continue
						}
						// Valid key, allow "=" and lock it as the prefix point.
						// Capture locked-builtin state before we modify IsVariable so
						// that reclassifyCurrentLine's guard doesn't fire on user-typed lines.
						wasLockedBuiltin := meta.IsVariable && !meta.IsUserDefined && !meta.IsNewLine
						meta.EditableStartCol = m.col + 1
						meta.IsVariable = true
						if !wasLockedBuiltin {
							meta.IsUserDefined = true
						}
					} else if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
						// Block invalid characters in key
						continue
					}
					filtered = append(filtered, r)
				}
				runes = filtered
			}
		}
	}

	// Clean up any special characters in the input provided by the
	// clipboard. This avoids bugs due to e.g. tab characters and
	// whatnot.
	runes = m.san().Sanitize(runes)

	if len(runes) == 0 {
		return
	}

	if m.CharLimit > 0 {
		availSpace := m.CharLimit - m.Length()
		// If the char limit's been reached, cancel.
		if availSpace <= 0 {
			return
		}
		// If there's not enough space to paste the whole thing cut the pasted
		// runes down so they'll fit.
		if availSpace < len(runes) {
			runes = runes[:availSpace]
		}
	}

	// Split the input into lines.
	var lines [][]rune
	lstart := 0
	for i := range runes {
		if runes[i] == '\n' {
			// Queue a line to become a new row in the text area below.
			// Beware to clamp the max capacity of the slice, to ensure no
			// data from different rows get overwritten when later edits
			// will modify this line.
			lines = append(lines, runes[lstart:i:i])
			lstart = i + 1
		}
	}
	if lstart <= len(runes) {
		// The last line did not end with a newline character.
		// Take it now.
		lines = append(lines, runes[lstart:])
	}

	// Obey the maximum line limit.
	if maxLines > 0 && len(m.value)+len(lines)-1 > maxLines {
		allowedHeight := max(0, maxLines-len(m.value)+1)
		lines = lines[:allowedHeight]
	}

	if len(lines) == 0 {
		// Nothing left to insert.
		return
	}

	// Save the remainder of the original line at the current
	// cursor position.
	tail := make([]rune, len(m.value[m.row][m.col:]))
	copy(tail, m.value[m.row][m.col:])

	// Paste the first line at the current cursor position.
	m.value[m.row] = append(m.value[m.row][:m.col], lines[0]...)
	m.col += len(lines[0])

	if numExtraLines := len(lines) - 1; numExtraLines > 0 {
		// Add the new lines.
		// We try to reuse the slice if there's already space.
		var newGrid [][]rune
		if cap(m.value) >= len(m.value)+numExtraLines {
			// Can reuse the extra space.
			newGrid = m.value[:len(m.value)+numExtraLines]
		} else {
			// No space left; need a new slice.
			newGrid = make([][]rune, len(m.value)+numExtraLines)
			copy(newGrid, m.value[:m.row+1])
		}
		// Add all the rows that were after the cursor in the original
		// grid at the end of the new grid.
		copy(newGrid[m.row+1+numExtraLines:], m.value[m.row+1:])
		m.value = newGrid
		// Insert all the new lines in the middle.
		for _, l := range lines[1:] {
			m.row++
			m.value[m.row] = l
			m.col = len(l)
		}
	}

	// Finally add the tail at the end of the last line inserted.
	m.value[m.row] = append(m.value[m.row], tail...)

	m.SetCursorColumn(m.col)
}

// Value returns the value of the text input.
func (m Model) Value() string {
	if m.value == nil {
		return ""
	}

	var v strings.Builder
	for _, l := range m.value {
		v.WriteString(string(l))
		v.WriteByte('\n')
	}

	return strings.TrimSuffix(v.String(), "\n")
}

// Length returns the number of characters currently in the text input.
func (m *Model) Length() int {
	var l int
	for _, row := range m.value {
		l += uniseg.StringWidth(string(row))
	}
	// We add len(m.value) to include the newline characters.
	return l + len(m.value) - 1
}

// LineCount returns the number of lines that are currently in the text input.
func (m *Model) LineCount() int {
	return len(m.value)
}

// SetLineMeta updates the metadata for a specific line.
func (m *Model) SetLineMeta(row int, l Line) {
	if row >= 0 && row < len(m.lineMeta) {
		m.lineMeta[row] = l
		m.cacheValid = false
	}
}

// Line returns the 0-indexed row position of the cursor.
func (m Model) Line() int {
	return m.row
}

// Column returns the 0-indexed column position of the cursor.
func (m Model) Column() int {
	return m.col
}

// ScrollYOffset returns the Y offset (top row) index of the current view, which
// can be used to calculate the current scroll position.
func (m Model) ScrollYOffset() int {
	return m.viewport.YOffset()
}

// ScrollPercent returns the amount of the textarea that is currently scrolled
// through, clamped between 0 and 1.
func (m Model) ScrollPercent() float64 {
	return m.viewport.ScrollPercent()
}

// setCursorLineRelative moves the cursor by the given number of lines. Negative
// values move the cursor up, positive values move the cursor down.
func (m *Model) setCursorLineRelative(delta int) {
	if delta == 0 {
		return
	}

	li := m.LineInfo()
	charOffset := max(m.lastCharOffset, li.CharOffset)
	m.lastCharOffset = charOffset

	// 2 columns to account for the trailing space wrapping.
	const trailingSpace = 2

	if delta > 0 { //nolint:nestif
		// Moving down.
		for range delta {
			if li.RowOffset+1 >= li.Height && m.row < len(m.value)-1 {
				m.row++
				m.col = 0
			} else {
				// Move the cursor to the start of the next virtual line.
				m.col = min(li.StartColumn+li.Width+trailingSpace, len(m.value[m.row])-1)
			}
			li = m.LineInfo()
		}
	} else {
		// Moving up.
		for range -delta {
			if li.RowOffset <= 0 && m.row > 0 {
				m.row--
				m.col = len(m.value[m.row])
			} else {
				// Move the cursor to the end of the previous line.
				m.col = li.StartColumn - trailingSpace
			}
			li = m.LineInfo()
		}
	}

	nli := m.LineInfo()
	m.col = nli.StartColumn

	if nli.Width <= 0 {
		m.repositionView()
		return
	}

	offset := 0
	for offset < charOffset {
		if m.row >= len(m.value) || m.col >= len(m.value[m.row]) || offset >= nli.CharWidth-1 {
			break
		}
		offset += rw.RuneWidth(m.value[m.row][m.col])
		m.col++
	}
	m.repositionView()
}

// CursorDown moves the cursor down by one line.
func (m *Model) CursorDown() {
	m.setCursorLineRelative(1)
}

// CursorUp moves the cursor up by one line.
func (m *Model) CursorUp() {
	m.setCursorLineRelative(-1)
}

// SetCursorColumn moves the cursor to the given position. If the position is
// out of bounds the cursor will be moved to the start or end accordingly.
func (m *Model) SetCursorColumn(col int) {
	m.col = clamp(col, 0, len(m.value[m.row]))
	// Any time that we move the cursor horizontally we need to reset the last
	// offset so that the horizontal position when navigating is adjusted.
	m.lastCharOffset = 0
}

// CursorStart moves the cursor to the start of the input field.
func (m *Model) CursorStart() {
	m.SetCursorColumn(0)
}

// CursorEnd moves the cursor to the end of the input field.
func (m *Model) CursorEnd() {
	m.SetCursorColumn(len(m.value[m.row]))
}

// GotoFirstEditable moves the cursor to the first editable position in the file
// (first non-ReadOnly, non-PendingDelete row, at its EditableStartCol).
// If no such row exists the cursor stays at (0, 0).
func (m *Model) GotoFirstEditable() {
	for row, meta := range m.lineMeta {
		if meta.ReadOnly || meta.PendingDelete {
			continue
		}
		m.row = row
		m.col = meta.EditableStartCol
		m.repositionView()
		return
	}
	m.row = 0
	m.col = 0
}

// Focused returns the focus state on the model.
func (m Model) Focused() bool {
	return m.focus
}

// activeStyle returns the appropriate set of styles to use depending on
// whether the textarea is focused or blurred.
func (m Model) activeStyle() *StyleState {
	// Always return focused styles so syntax highlighting doesn't disappear when tabbing away.
	return &m.styles.Focused
}

// Focus sets the focus state on the model. When the model is in focus it can
// receive keyboard input and the cursor will be hidden.
func (m *Model) Focus() tea.Cmd {
	m.focus = true
	return m.virtualCursor.Focus()
}

// Blur removes the focus state on the model. When the model is blurred it can
// not receive keyboard input and the cursor will be hidden.
func (m *Model) Blur() {
	m.focus = false
	m.virtualCursor.Blur()
}

// Reset sets the input to its default state with no input.
func (m *Model) Reset() {
	m.diffCache = make(map[int][]bool)
	m.value = make([][]rune, minHeight, maxLines)
	m.lineMeta = make([]Line, minHeight, maxLines)
	m.col = 0
	m.row = 0
	m.viewport.GotoTop()
	m.SetCursorColumn(0)
	m.InvalidateCache()
}

// GetContent returns the reconstituted .env file content, excluding any lines
// marked as PendingDelete (those will be removed when the file is saved).
func (m *Model) GetContent() string {
	var sb strings.Builder
	needNewline := false
	for i, l := range m.value {
		if i < len(m.lineMeta) && m.lineMeta[i].PendingDelete {
			continue
		}
		if needNewline {
			sb.WriteString("\n")
		}
		sb.WriteString(string(l))
		needNewline = true
	}
	return sb.String()
}

// CurrentLineMeta returns the meta information of the current line
func (m *Model) CurrentLineMeta() (Line, bool) {
	if m.row < len(m.lineMeta) {
		return m.lineMeta[m.row], true
	}
	return Line{}, false
}

// AddVariable appends a new variable line to the editor
func (m *Model) AddVariable(key string, value string) {
	m.diffCache = make(map[int][]bool)
	m.insertVariableAt(len(m.value), key, value)
}

func (m *Model) insertVariableAt(row int, key string, value string) {
	m.diffCache = make(map[int][]bool)
	newLine := key + "=" + value
	if key == "" && value == "" {
		newLine = ""
	}
	l := Line{
		Text:             newLine,
		IsVariable:       key != "",
		IsUserDefined:    true,
		EditableStartCol: 0,
		IsNewLine:        true, // added by user after load
	}
	if key != "" {
		l.EditableStartCol = len(key) + 1
	}

	if row >= len(m.value) {
		m.value = append(m.value, []rune(newLine))
		m.lineMeta = append(m.lineMeta, l)
		m.row = len(m.value) - 1
	} else {
		m.value = slices.Insert(m.value, row, []rune(newLine))
		m.lineMeta = slices.Insert(m.lineMeta, row, l)
		m.row = row
	}
	m.col = 0
	if key != "" {
		m.col = len(newLine)
	}
	m.repositionView()
}

// MoveVariableUp swaps the current row with the row above it if both are not read-only.
func (m *Model) MoveVariableUp() {
	if m.row <= 0 || m.row >= len(m.value) {
		return
	}
	if m.lineMeta[m.row].ReadOnly || m.lineMeta[m.row-1].ReadOnly {
		return
	}
	if !m.lineMeta[m.row].IsUserDefined || !m.lineMeta[m.row-1].IsUserDefined {
		return
	}

	// Swap value
	m.value[m.row], m.value[m.row-1] = m.value[m.row-1], m.value[m.row]
	// Swap meta
	m.lineMeta[m.row], m.lineMeta[m.row-1] = m.lineMeta[m.row-1], m.lineMeta[m.row]

	m.row--
	m.repositionView()
	m.InvalidateCache()
}

// MoveVariableDown swaps the current row with the row below it if both are not read-only.
func (m *Model) MoveVariableDown() {
	if m.row >= len(m.value)-1 {
		return
	}
	if m.lineMeta[m.row].ReadOnly || m.lineMeta[m.row+1].ReadOnly {
		return
	}
	if !m.lineMeta[m.row].IsUserDefined || !m.lineMeta[m.row+1].IsUserDefined {
		return
	}

	// Swap value
	m.value[m.row], m.value[m.row+1] = m.value[m.row+1], m.value[m.row]
	// Swap meta
	m.lineMeta[m.row], m.lineMeta[m.row+1] = m.lineMeta[m.row+1], m.lineMeta[m.row]

	m.row++
	m.repositionView()
	m.InvalidateCache()
}

// DeleteCurrentVariable marks the row under the cursor as pending deletion.
// The line stays visible with strikethrough styling until the file is saved.
// Ctrl+Z (undo) can restore it. Refresh from disk also clears all pending deletes.
func (m *Model) DeleteCurrentVariable() bool {
	if m.row >= len(m.lineMeta) || m.lineMeta[m.row].ReadOnly {
		return false
	}
	m.pushUndoSnapshot()
	m.lineMeta[m.row].PendingDelete = true
	m.lineMeta[m.row].ReadOnly = true // prevent editing while pending
	m.InvalidateCache()
	return true
}

// DeleteVariableByName finds the first row containing varName= and deletes it.
func (m *Model) DeleteVariableByName(varName string) bool {
	prefix := varName + "="
	for row, meta := range m.lineMeta {
		if meta.ReadOnly {
			continue
		}
		line := strings.TrimSpace(string(m.value[row]))
		if strings.HasPrefix(line, prefix) {
			saved := m.row
			m.row = row
			ok := m.DeleteCurrentVariable()
			if !ok {
				m.row = saved
			}
			return ok
		}
	}
	return false
}

// ResetCurrentVariable restores the DefaultValue if the line is a built-in variable
func (m *Model) ResetCurrentVariable() bool {
	if m.row >= len(m.lineMeta) || m.lineMeta[m.row].ReadOnly || m.lineMeta[m.row].DefaultValue == "" {
		return false
	}
	meta := &m.lineMeta[m.row]
	prefix := string(m.value[m.row][:meta.EditableStartCol])
	m.value[m.row] = []rune(prefix + meta.DefaultValue)
	m.repositionView()
	m.InvalidateCache()
	return true
}

// LineMetaAt returns the metadata for any row by absolute index.
// Unlike CurrentLineMeta, this does not require the cursor to be on that row.
// Returns (Line{}, false) if row is out of range.
func (m *Model) LineMetaAt(row int) (Line, bool) {
	if row >= 0 && row < len(m.lineMeta) {
		return m.lineMeta[row], true
	}
	return Line{}, false
}

// YOffset returns the viewport's current vertical scroll offset (first visible line index).
func (m *Model) YOffset() int {
	return m.viewport.YOffset()
}

// VisualRowToLogical converts a visual (screen) row index to the corresponding
// logical line index, accounting for wrapped lines. Returns -1 if out of range.
func (m *Model) VisualRowToLogical(visualRow int) int {
	curr := 0
	for l, lineRunes := range m.value {
		wrapped := m.memoizedWrap(lineRunes, m.width)
		n := len(wrapped)
		if visualRow >= curr && visualRow < curr+n {
			return l
		}
		curr += n
	}
	return -1
}

// CursorVisualRow returns the visual (screen) row index of the cursor, accounting
// for wrapped lines above it.
func (m *Model) CursorVisualRow() int {
	curr := 0
	for l, lineRunes := range m.value {
		if l == m.row {
			return curr
		}
		curr += len(m.memoizedWrap(lineRunes, m.width))
	}
	return curr
}

// SetVariableValue finds the row for varName, replaces its value, and invalidates the cache.
// The new value is written as-is after the '=' sign (include quoting if needed by the caller).
// Returns true if the variable was found and updated. Read-only rows are skipped.
func (m *Model) SetVariableValue(varName, newValue string) bool {
	for row, meta := range m.lineMeta {
		if !meta.IsVariable || meta.ReadOnly {
			continue
		}
		// The editable start column is len(varName)+1 (position after '=').
		// Verify this row's key matches varName.
		startCol := meta.EditableStartCol
		if startCol < 1 || startCol > len(m.value[row]) {
			continue
		}
		rowKey := string(m.value[row][:startCol-1]) // everything before '='
		if rowKey != varName {
			continue
		}
		m.pushUndoSnapshot()
		// Update the value portion of the line.
		prefix := string(m.value[row][:startCol]) // includes '='
		m.value[row] = []rune(prefix + newValue)
		m.lineMeta[row].Text = string(m.value[row])
		m.InvalidateCache()
		return true
	}
	return false
}

// snapshot returns a deep copy of the current editor state.
func (m *Model) snapshot() undoSnapshot {
	valueCopy := make([][]rune, len(m.value))
	for i, line := range m.value {
		lc := make([]rune, len(line))
		copy(lc, line)
		valueCopy[i] = lc
	}
	metaCopy := make([]Line, len(m.lineMeta))
	copy(metaCopy, m.lineMeta)
	return undoSnapshot{value: valueCopy, lineMeta: metaCopy, row: m.row, col: m.col}
}

// restoreSnapshot applies a snapshot and refreshes caches.
func (m *Model) restoreSnapshot(s undoSnapshot) {
	m.value = s.value
	m.lineMeta = s.lineMeta
	m.row = clamp(s.row, 0, max(0, len(s.value)-1))
	m.col = clamp(s.col, 0, len(m.value[m.row]))
	m.cache = memoization.NewMemoCache[line, [][]rune](m.cache.Capacity())
	m.InvalidateCache()
}

// pushUndoSnapshot saves a deep copy of the current editor state onto the undo stack.
// Any new edit clears the redo stack. The stack is capped at 100 entries.
func (m *Model) pushUndoSnapshot() {
	m.redoStack = nil // new edit invalidates redo history
	m.undoStack = append(m.undoStack, m.snapshot())
	const maxUndoDepth = 100
	if len(m.undoStack) > maxUndoDepth {
		m.undoStack = m.undoStack[1:]
	}
}

// Undo restores the most recent snapshot from the undo stack.
// The current state is pushed onto the redo stack so it can be redone.
// Returns true if a snapshot was available and restored.
func (m *Model) Undo() bool {
	if len(m.undoStack) == 0 {
		return false
	}
	m.redoStack = append(m.redoStack, m.snapshot())
	entry := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.restoreSnapshot(entry)
	return true
}

// Redo reapplies the most recently undone edit.
// Returns true if a redo snapshot was available and restored.
func (m *Model) Redo() bool {
	if len(m.redoStack) == 0 {
		return false
	}
	m.undoStack = append(m.undoStack, m.snapshot())
	entry := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	m.restoreSnapshot(entry)
	return true
}

// ClearUndo discards all undo and redo history. Call this after a full content
// reload (e.g. refresh) to prevent undoing across the reload boundary.
func (m *Model) ClearUndo() {
	m.undoStack = nil
	m.redoStack = nil
}

// GetSelectedText returns the currently selected text, or "" if no selection is active.
func (m *Model) GetSelectedText() string {
	if !m.selActive || m.selRow < 0 || m.selRow >= len(m.value) {
		return ""
	}
	line := m.value[m.selRow]
	s := clamp(m.selStartCol, 0, len(line))
	e := clamp(m.selEndCol, 0, len(line))
	if s >= e {
		return ""
	}
	return string(line[s:e])
}

// GetVariableValue returns everything after '=' for varName (raw, including any surrounding quotes), or "" if not found.
func (m *Model) GetVariableValue(varName string) string {
	for row, meta := range m.lineMeta {
		if !meta.IsVariable {
			continue
		}
		lineStr := string(m.value[row])
		if eqIdx := strings.Index(lineStr, "="); eqIdx > 0 {
			if strings.TrimSpace(lineStr[:eqIdx]) == varName {
				return lineStr[eqIdx+1:]
			}
		}
	}
	return ""
}

// GetVariableMeta returns the Line metadata for varName, or false if not found.
func (m *Model) GetVariableMeta(varName string) (Line, bool) {
	for row, meta := range m.lineMeta {
		if !meta.IsVariable {
			continue
		}
		lineStr := string(m.value[row])
		if eqIdx := strings.Index(lineStr, "="); eqIdx > 0 {
			if strings.TrimSpace(lineStr[:eqIdx]) == varName {
				return meta, true
			}
		}
	}
	return Line{}, false
}

// GetVariableInitialValue returns everything after '=' in InitialLine for varName, or "" if not found.
func (m *Model) GetVariableInitialValue(varName string) string {
	for _, meta := range m.lineMeta {
		if !meta.IsVariable {
			continue
		}
		if eqIdx := strings.Index(meta.InitialLine, "="); eqIdx > 0 {
			if strings.TrimSpace(meta.InitialLine[:eqIdx]) == varName {
				return meta.InitialLine[eqIdx+1:]
			}
		}
	}
	return ""
}

// HasVariable returns true if varName exists in the editor (regardless of its value).
func (m *Model) HasVariable(varName string) bool {
	for row, meta := range m.lineMeta {
		if !meta.IsVariable {
			continue
		}
		lineStr := string(m.value[row])
		if eqIdx := strings.Index(lineStr, "="); eqIdx > 0 {
			if strings.TrimSpace(lineStr[:eqIdx]) == varName {
				return true
			}
		}
	}
	return false
}

// Word returns the word at the cursor position.
// A word is delimited by spaces or line-breaks.
func (m *Model) Word() string {
	line := m.value[m.row]
	col := m.col - 1

	if col < 0 {
		return ""
	}

	// If cursor is beyond the line, return empty string
	if col >= len(line) {
		return ""
	}

	// If cursor is on a space, return empty string
	if unicode.IsSpace(line[col]) {
		return ""
	}

	// Find the start of the word by moving left
	start := col
	for start > 0 && !unicode.IsSpace(line[start-1]) {
		start--
	}

	// Find the end of the word by moving right
	end := col
	for end < len(line) && !unicode.IsSpace(line[end]) {
		end++
	}

	return string(line[start:end])
}

// san initializes or retrieves the rune sanitizer.
func (m *Model) san() runeutil.Sanitizer {
	if m.rsan == nil {
		// Textinput has all its input on a single line so collapse
		// newlines/tabs to single spaces.
		m.rsan = runeutil.NewSanitizer()
	}
	return m.rsan
}

// deleteBeforeCursor deletes all text before the cursor. Returns whether or
// not the cursor blink should be reset.
func (m *Model) deleteBeforeCursor() {
	m.invalidateDiffCache(m.row)
	startCol := 0
	if m.row < len(m.lineMeta) {
		startCol = m.lineMeta[m.row].EditableStartCol
	}
	if startCol >= m.col {
		return
	}
	m.value[m.row] = append(m.value[m.row][:startCol], m.value[m.row][m.col:]...)
	m.SetCursorColumn(startCol)
}

// deleteAfterCursor deletes all text after the cursor. Returns whether or not
// the cursor blink should be reset. If input is masked delete everything after
// the cursor so as not to reveal word breaks in the masked input.
func (m *Model) deleteAfterCursor() {
	m.invalidateDiffCache(m.row)
	m.value[m.row] = m.value[m.row][:m.col]
	m.SetCursorColumn(len(m.value[m.row]))
}

// transposeLeft exchanges the runes at the cursor and immediately
// before. No-op if the cursor is at the beginning of the line.  If
// the cursor is not at the end of the line yet, moves the cursor to
// the right.
func (m *Model) transposeLeft() {
	if m.col == 0 || len(m.value[m.row]) < 2 {
		return
	}
	m.invalidateDiffCache(m.row)
	if m.col >= len(m.value[m.row]) {
		m.SetCursorColumn(m.col - 1)
	}
	m.value[m.row][m.col-1], m.value[m.row][m.col] = m.value[m.row][m.col], m.value[m.row][m.col-1]
	if m.col < len(m.value[m.row]) {
		m.SetCursorColumn(m.col + 1)
	}
}

// deleteWordLeft deletes the word left to the cursor. Returns whether or not
// the cursor blink should be reset.
func (m *Model) deleteWordLeft() {
	m.invalidateDiffCache(m.row)
	startCol := 0
	if m.row < len(m.lineMeta) {
		startCol = m.lineMeta[m.row].EditableStartCol
	}
	if m.col <= startCol || len(m.value[m.row]) == 0 {
		return
	}

	// Linter note: it's critical that we acquire the initial cursor position
	// here prior to altering it via SetCursor() below. As such, moving this
	// call into the corresponding if clause does not apply here.
	oldCol := m.col

	m.SetCursorColumn(m.col - 1)
	for unicode.IsSpace(m.value[m.row][m.col]) {
		if m.col <= startCol {
			break
		}
		// ignore series of whitespace before cursor
		m.SetCursorColumn(m.col - 1)
	}

	for m.col > startCol {
		if !unicode.IsSpace(m.value[m.row][m.col]) {
			m.SetCursorColumn(m.col - 1)
		} else {
			if m.col > startCol {
				// keep the previous space
				m.SetCursorColumn(m.col + 1)
			}
			break
		}
	}
	if m.col < startCol {
		m.col = startCol
	}

	if oldCol > len(m.value[m.row]) {
		m.value[m.row] = m.value[m.row][:m.col]
	} else {
		m.value[m.row] = append(m.value[m.row][:m.col], m.value[m.row][oldCol:]...)
	}
}

// deleteWordRight deletes the word right to the cursor.
func (m *Model) deleteWordRight() {
	m.invalidateDiffCache(m.row)
	if m.col >= len(m.value[m.row]) || len(m.value[m.row]) == 0 {
		return
	}

	oldCol := m.col

	for m.col < len(m.value[m.row]) && unicode.IsSpace(m.value[m.row][m.col]) {
		// ignore series of whitespace after cursor
		m.SetCursorColumn(m.col + 1)
	}

	for m.col < len(m.value[m.row]) {
		if !unicode.IsSpace(m.value[m.row][m.col]) {
			m.SetCursorColumn(m.col + 1)
		} else {
			break
		}
	}

	if m.col > len(m.value[m.row]) {
		m.value[m.row] = m.value[m.row][:oldCol]
	} else {
		m.value[m.row] = append(m.value[m.row][:oldCol], m.value[m.row][m.col:]...)
	}

	m.SetCursorColumn(oldCol)
}

// characterRight moves the cursor one character to the right.
func (m *Model) characterRight() {
	if m.col < len(m.value[m.row]) {
		m.SetCursorColumn(m.col + 1)
	} else {
		if m.row < len(m.value)-1 {
			m.row++
			m.CursorStart()
		}
	}
}

// characterLeft moves the cursor one character to the left.
// If insideLine is set, the cursor is moved to the last
// character in the previous line, instead of one past that.
func (m *Model) characterLeft(insideLine bool) {
	if m.col == 0 && m.row != 0 {
		m.row--
		m.CursorEnd()
		if !insideLine {
			return
		}
	}
	if m.col > 0 {
		m.SetCursorColumn(m.col - 1)
	}
}

// wordLeft moves the cursor one word to the left. Returns whether or not the
// cursor blink should be reset. If input is masked, move input to the start
// so as not to reveal word breaks in the masked input.
func (m *Model) wordLeft() {
	for {
		m.characterLeft(true /* insideLine */)
		if m.col < len(m.value[m.row]) && !unicode.IsSpace(m.value[m.row][m.col]) {
			break
		}
	}

	for m.col > 0 {
		if unicode.IsSpace(m.value[m.row][m.col-1]) {
			break
		}
		m.SetCursorColumn(m.col - 1)
	}
}

// wordRight moves the cursor one word to the right. Returns whether or not the
// cursor blink should be reset. If the input is masked, move input to the end
// so as not to reveal word breaks in the masked input.
func (m *Model) wordRight() {
	m.doWordRight(func(int, int) { /* nothing */ })
}

func (m *Model) doWordRight(fn func(charIdx int, pos int)) {
	// Skip spaces forward.
	for m.col >= len(m.value[m.row]) || unicode.IsSpace(m.value[m.row][m.col]) {
		if m.row == len(m.value)-1 && m.col == len(m.value[m.row]) {
			// End of text.
			break
		}
		m.characterRight()
	}

	charIdx := 0
	for m.col < len(m.value[m.row]) {
		if unicode.IsSpace(m.value[m.row][m.col]) {
			break
		}
		fn(charIdx, m.col)
		m.SetCursorColumn(m.col + 1)
		charIdx++
	}
}

// uppercaseRight changes the word to the right to uppercase.
func (m *Model) uppercaseRight() {
	m.invalidateDiffCache(m.row)
	m.doWordRight(func(_ int, i int) {
		m.value[m.row][i] = unicode.ToUpper(m.value[m.row][i])
	})
}

// lowercaseRight changes the word to the right to lowercase.
func (m *Model) lowercaseRight() {
	m.invalidateDiffCache(m.row)
	m.doWordRight(func(_ int, i int) {
		m.value[m.row][i] = unicode.ToLower(m.value[m.row][i])
	})
}

// capitalizeRight changes the word to the right to title case.
func (m *Model) capitalizeRight() {
	m.invalidateDiffCache(m.row)
	m.doWordRight(func(charIdx int, i int) {
		if charIdx == 0 {
			m.value[m.row][i] = unicode.ToTitle(m.value[m.row][i])
		}
	})
}

// LineInfo returns the number of characters from the start of the
// (soft-wrapped) line and the (soft-wrapped) line width.
func (m Model) LineInfo() LineInfo {
	grid := m.memoizedWrap(m.value[m.row], m.width)

	// Find out which line we are currently on. This can be determined by the
	// m.col and counting the number of runes that we need to skip.
	var counter int
	for i, line := range grid {
		// We've found the line that we are on
		if counter+len(line) == m.col && i+1 < len(grid) {
			// We wrap around to the next line if we are at the end of the
			// previous line so that we can be at the very beginning of the row
			return LineInfo{
				CharOffset:   0,
				ColumnOffset: 0,
				Height:       len(grid),
				RowOffset:    i + 1,
				StartColumn:  m.col,
				Width:        len(grid[i+1]),
				CharWidth:    uniseg.StringWidth(string(line)),
			}
		}

		if counter+len(line) >= m.col {
			return LineInfo{
				CharOffset:   uniseg.StringWidth(string(line[:max(0, m.col-counter)])),
				ColumnOffset: m.col - counter,
				Height:       len(grid),
				RowOffset:    i,
				StartColumn:  counter,
				Width:        len(line),
				CharWidth:    uniseg.StringWidth(string(line)),
			}
		}

		counter += len(line)
	}
	return LineInfo{}
}

// repositionView repositions the view of the viewport based on the defined
// scrolling behavior.
func (m *Model) repositionView() {
	minimum := m.viewport.YOffset()
	maximum := minimum + m.viewport.Height() - 1
	if row := m.cursorLineNumber(); row < minimum {
		m.viewport.ScrollUp(minimum - row)
	} else if row > maximum {
		m.viewport.ScrollDown(row - maximum)
	}
}

// constrainCursorToView moves the cursor so it is within the visible viewport.
// It is used during free scrolling to prevent snap-back.
func (m *Model) constrainCursorToView() {
	minimum := m.viewport.YOffset()
	maximum := minimum + m.viewport.Height() - 1
	if row := m.cursorLineNumber(); row < minimum {
		m.setCursorLineRelative(minimum - row)
	} else if row > maximum {
		m.setCursorLineRelative(maximum - row)
	}
}

// SetLineCharacters sets whether the textarea should use stylized line-art
// characters for its scrollbar.
func (m *Model) SetLineCharacters(v bool) {
	m.LineCharacters = v
	m.InvalidateCache()
}

// Width returns the total visual width of the textarea (gutter + text + scrollbar).
func (m Model) Width() int {
	return m.totalWidth
}

// MoveToBegin moves the cursor to the beginning of the input.
func (m *Model) MoveToBegin() {
	m.row = 0
	m.SetCursorColumn(0)
	m.repositionView()
}

// MoveToEnd moves the cursor to the end of the input.
func (m *Model) MoveToEnd() {
	m.row = len(m.value) - 1
	m.SetCursorColumn(len(m.value[m.row]))
	m.repositionView()
}

// PageUp moves the cursor up by one page. First call snaps to the first visible
// line, subsequent calls move up by a full page.
func (m *Model) PageUp() {
	// If not on the first visible line, snap to it.
	if offset := m.viewport.YOffset() - m.cursorLineNumber(); offset < 0 {
		m.setCursorLineRelative(offset)
		return
	}

	// Already on first visible line, move up by a full page.
	m.setCursorLineRelative(-m.height)
}

// PageDown moves the cursor down by one page. First call snaps to the last
// visible line, subsequent calls move down by a full page.
func (m *Model) PageDown() {
	// If not on the last visible line, snap to it.
	if offset := m.cursorLineNumber() - m.viewport.YOffset(); offset < m.height-1 {
		m.setCursorLineRelative(m.height - 1 - offset)
		return
	}

	// Already on last visible line, move down by a full page.
	m.setCursorLineRelative(m.height)
}

// SetWidth sets the width of the textarea to fit exactly within the given width.
// This means that the textarea will account for the width of the prompt and
// whether or not line numbers are being shown.
//
// Ensure that SetWidth is called after setting the Prompt and ShowLineNumbers,
// It is important that the width of the textarea be exactly the given width
// and no more.
func (m *Model) SetWidth(w int) {
	// Update prompt width only if there is no prompt function as
	// [SetPromptFunc] updates the prompt width when it is called.
	if m.promptFunc == nil {
		// XXX: Do we even need this or can we calculate the prompt width
		// at render time?
		m.promptWidth = uniseg.StringWidth(m.Prompt)
	}

	// Add base style borders and padding to reserved outer width.
	reservedOuter := m.activeStyle().Base.GetHorizontalFrameSize()

	// Add prompt width to reserved inner width.
	reservedInner := m.promptWidth

	// Add line number width to reserved inner width.
	if m.ShowLineNumbers {
		// Single character left margin plus number width (min 3 digits) plus 1 cell gap.
		const margin = 1
		const gap = 1
		digits := max(3, numDigits(m.MaxHeight))

		reservedInner += margin + digits + gap
	}

	// Input width must be at least one more than the reserved inner and outer
	// width. This gives us a minimum input width of 1.
	minWidth := reservedInner + reservedOuter + 1
	inputWidth := max(w, minWidth)

	// Input width must be no more than maximum width.
	if m.MaxWidth > 0 {
		inputWidth = min(inputWidth, m.MaxWidth)
	}

	// Since the width of the viewport and input area is dependent on the width of
	// borders, prompt and line numbers, we need to calculate it by subtracting
	// the reserved width from them.

	// Always reserve 1 column for the scrollbar (or space if no scrollbar)
	reservedInner += 1

	m.totalWidth = inputWidth
	m.viewport.SetWidth(inputWidth - reservedOuter - 1)
	m.width = inputWidth - reservedOuter - reservedInner
	m.InvalidateCache()
}

// SetPromptFunc supersedes the Prompt field and sets a dynamic prompt instead.
//
// If the function returns a prompt that is shorter than the specified
// promptWidth, it will be padded to the left. If it returns a prompt that is
// longer, display artifacts may occur; the caller is responsible for computing
// an adequate promptWidth.
func (m *Model) SetPromptFunc(promptWidth int, fn func(PromptInfo) string) {
	m.promptFunc = fn
	m.promptWidth = promptWidth
}

// Height returns the current height of the textarea.
func (m Model) Height() int {
	return m.height
}

// SetHeight sets the height of the textarea.
func (m *Model) SetHeight(h int) {
	if m.MaxHeight > 0 {
		m.height = clamp(h, minHeight, m.MaxHeight)
		m.viewport.SetHeight(clamp(h, minHeight, m.MaxHeight))
	} else {
		m.height = max(h, minHeight)
		m.viewport.SetHeight(max(h, minHeight))
	}

	m.repositionView()
	m.InvalidateCache()
}

// isReadOnlyRow returns true if the current row shouldn't be edited at all
func (m *Model) isReadOnlyRow() bool {
	if m.row >= len(m.lineMeta) {
		return false
	}
	return m.lineMeta[m.row].ReadOnly
}

// isEditableAtCursor returns true if the cursor is at or after EditableStartCol
func (m *Model) isEditableAtCursor() bool {
	if m.row >= len(m.lineMeta) {
		return true
	}
	if m.lineMeta[m.row].ReadOnly {
		return false
	}
	if m.lineMeta[m.row].IsUserDefined {
		return true
	}
	return m.col >= m.lineMeta[m.row].EditableStartCol
}

// isBackspaceEditable returns true if a backward deletion is allowed
func (m *Model) isBackspaceEditable() bool {
	if m.row >= len(m.lineMeta) {
		return true
	}
	if m.lineMeta[m.row].ReadOnly {
		return false
	}
	if m.col == 0 {
		// Never join lines on Backspace — use Delete at end of the previous line instead.
		return false
	}
	if m.lineMeta[m.row].IsUserDefined {
		return true
	}
	return m.col > m.lineMeta[m.row].EditableStartCol
}

// HasValidationErrors returns true if any variable name in the editor is invalid
// or if any user-defined line has content but no '=' separator.
func (m *Model) HasValidationErrors() bool {
	if m.ValidateFunc == nil || m.ValidationType == "" {
		return false
	}

	for i, lineRunes := range m.value {
		if i >= len(m.lineMeta) {
			continue
		}
		meta := &m.lineMeta[i]
		if meta.ReadOnly || !meta.IsVariable {
			continue
		}

		// Find '=' index
		eqIdx := -1
		for j, r := range lineRunes {
			if r == '=' {
				eqIdx = j
				break
			}
		}

		if eqIdx > 0 {
			// Validate key
			key := string(lineRunes[:eqIdx])
			vType := m.ValidationType
			if vType == "APPNAME" {
				vType = m.ValidationAppName
			}
			if !m.ValidateFunc(key, vType) {
				return true
			}
		} else if meta.IsUserDefined && len(lineRunes) > 0 {
			// User defined line with content but no '=' is an incomplete variable
			return true
		}
	}
	return false
}

// InvalidateCache clears the rendered view cache.
func (m *Model) InvalidateCache() {
	m.cacheValid = false
	m.diffCache = make(map[int][]bool)
}

// CheckCache returns the cached rendered screen if it's still valid.
func (m *Model) CheckCache() (string, bool) {
	if m.cacheValid && m.lastView != "" {
		return m.lastView, true
	}
	return "", false
}

// SaveCache saves the newly generated screen string to the cache and marks it as valid.
func (m *Model) SaveCache(view string) string {
	m.lastView = view
	m.cacheValid = true
	return view
}

// Update is the Bubble Tea update loop.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focus {
		m.virtualCursor.Blur()
		return m, nil
	}

	// Keypresses, clicks, and release always invalidate cache as they represent interaction
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseClickMsg, tea.MouseReleaseMsg, tea.PasteMsg, pasteMsg:
		m.InvalidateCache()
	}

	// Clear per-message scrollbar-scroll flag. Handlers set this when the
	// scrollbar directly sets the viewport offset so repositionView() is skipped.
	prevSbScrolled := m.sbScrolled
	m.sbScrolled = false

	// Used to determine if the cursor should blink.
	oldRow, oldCol := m.cursorLineNumber(), m.col
	_ = prevSbScrolled // consumed below

	var cmds []tea.Cmd

	if m.value[m.row] == nil {
		m.value[m.row] = make([]rune, 0)
	}

	if m.MaxHeight > 0 && m.MaxHeight != m.cache.Capacity() {
		m.cache = memoization.NewMemoCache[line, [][]rune](m.MaxHeight)
	}

	switch msg := msg.(type) {
	case tea.PasteMsg:
		if !m.isEditableAtCursor() {
			break
		}
		m.pushUndoSnapshot()
		m.insertRunes([]rune(msg.Content), true)
		m.reclassifyCurrentLine()
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.KeyMap.Undo):
			m.Undo()
		case key.Matches(msg, m.KeyMap.Redo):
			m.Redo()
		case key.Matches(msg, m.KeyMap.Copy):
			if m.selActive {
				_ = clipboard.WriteAll(m.GetSelectedText())
			} else if m.row >= 0 && m.row < len(m.value) {
				lineStr := string(m.value[m.row])
				if eqIdx := strings.Index(lineStr, "="); eqIdx >= 0 {
					_ = clipboard.WriteAll(lineStr[eqIdx+1:])
				} else {
					_ = clipboard.WriteAll(lineStr)
				}
			}
		case key.Matches(msg, m.KeyMap.SelectRight):
			if !m.selActive || m.selRow != m.row {
				m.selRow = m.row
				m.selAnchorCol = m.col
			}
			if m.row == m.selRow && m.col < len(m.value[m.row]) {
				m.col++
			}
			if m.row == m.selRow {
				start, end := m.selAnchorCol, m.col
				if start > end {
					start, end = end, start
				}
				m.selStartCol = start
				m.selEndCol = end
				m.selActive = start < end
			}
		case key.Matches(msg, m.KeyMap.SelectLeft):
			if !m.selActive || m.selRow != m.row {
				m.selRow = m.row
				m.selAnchorCol = m.col
			}
			if m.row == m.selRow && m.col > 0 {
				m.col--
			}
			if m.row == m.selRow {
				start, end := m.selAnchorCol, m.col
				if start > end {
					start, end = end, start
				}
				m.selStartCol = start
				m.selEndCol = end
				m.selActive = start < end
			}
		case key.Matches(msg, m.KeyMap.SelectEnd):
			if !m.selActive || m.selRow != m.row {
				m.selRow = m.row
				m.selAnchorCol = m.col
			}
			if m.row == m.selRow {
				m.col = len(m.value[m.row])
				start, end := m.selAnchorCol, m.col
				if start > end {
					start, end = end, start
				}
				m.selStartCol = start
				m.selEndCol = end
				m.selActive = start < end
			}
		case key.Matches(msg, m.KeyMap.SelectHome):
			if !m.selActive || m.selRow != m.row {
				m.selRow = m.row
				m.selAnchorCol = m.col
			}
			if m.row == m.selRow {
				editStart := 0
				if m.row < len(m.lineMeta) {
					editStart = m.lineMeta[m.row].EditableStartCol
				}
				m.col = editStart
				start, end := m.selAnchorCol, m.col
				if start > end {
					start, end = end, start
				}
				m.selStartCol = start
				m.selEndCol = end
				m.selActive = start < end
			}
		case key.Matches(msg, m.KeyMap.DeleteAfterCursor):
			if !m.isEditableAtCursor() {
				break
			}
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			m.pushUndoSnapshot()
			if m.col >= len(m.value[m.row]) {
				m.mergeLineBelow(m.row)
				break
			}
			m.deleteAfterCursor()
		case key.Matches(msg, m.KeyMap.DeleteBeforeCursor):
			if !m.isBackspaceEditable() {
				break
			}
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			m.pushUndoSnapshot()
			if m.col <= 0 {
				m.mergeLineAbove(m.row)
				break
			}
			m.deleteBeforeCursor()
		case key.Matches(msg, m.KeyMap.DeleteCharacterBackward):
			if !m.isBackspaceEditable() {
				break
			}
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			m.pushUndoSnapshot()
			if m.col <= 0 {
				m.mergeLineAbove(m.row)
				break
			}
			if len(m.value[m.row]) > 0 {
				// Reverse of auto-prefix: if cursor is right after the app prefix and the
				// line starts with it, delete the entire prefix in one backspace.
				if m.AddPrefix != "" && m.row < len(m.lineMeta) && m.lineMeta[m.row].IsUserDefined {
					prefix := []rune(strings.ReplaceAll(m.AddPrefix, "APPNAME", m.ValidationAppName))
					if m.col == len(prefix) && len(m.value[m.row]) >= len(prefix) &&
						string(m.value[m.row][:len(prefix)]) == string(prefix) {
						m.invalidateDiffCache(m.row)
						m.value[m.row] = m.value[m.row][len(prefix):]
						m.SetCursorColumn(0)
						m.reclassifyCurrentLine()
						break
					}
				}
				m.invalidateDiffCache(m.row)
				m.value[m.row] = append(m.value[m.row][:max(0, m.col-1)], m.value[m.row][m.col:]...)
				if m.col > 0 {
					m.SetCursorColumn(m.col - 1)
				}
				m.reclassifyCurrentLine()
			}
		case key.Matches(msg, m.KeyMap.DeleteCharacterForward):
			if !m.isEditableAtCursor() {
				break
			}
			m.pushUndoSnapshot()
			if len(m.value[m.row]) > 0 && m.col < len(m.value[m.row]) {
				m.invalidateDiffCache(m.row)
				m.value[m.row] = slices.Delete(m.value[m.row], m.col, m.col+1)
				m.reclassifyCurrentLine()
			}
			if m.col >= len(m.value[m.row]) {
				m.mergeLineBelow(m.row)
				break
			}
		case key.Matches(msg, m.KeyMap.DeleteWordBackward):
			if !m.isBackspaceEditable() {
				break
			}
			m.pushUndoSnapshot()
			m.deleteWordLeft()
		case key.Matches(msg, m.KeyMap.DeleteWordForward):
			if !m.isEditableAtCursor() {
				break
			}
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			m.pushUndoSnapshot()
			if m.col >= len(m.value[m.row]) {
				m.mergeLineBelow(m.row)
				break
			}
			m.deleteWordRight()
		case key.Matches(msg, m.KeyMap.InsertNewline):
			m.pushUndoSnapshot()
			if m.row < len(m.value)-1 {
				m.CursorDown()
				m.CursorStart()
			} else {
				if m.MaxHeight > 0 && len(m.value) >= m.MaxHeight {
					return m, nil
				}
				m.AddVariable("", "")
			}
		case key.Matches(msg, m.KeyMap.SplitLine):
			if m.isReadOnlyRow() {
				break
			}
			// Block split on built-in variable lines — splitting the key would corrupt it.
			if m.row < len(m.lineMeta) && m.lineMeta[m.row].IsVariable && !m.lineMeta[m.row].IsUserDefined {
				break
			}
			if m.MaxHeight > 0 && len(m.value) >= m.MaxHeight {
				return m, nil
			}
			m.pushUndoSnapshot()
			m.splitLine(m.row, m.col)
		case key.Matches(msg, m.KeyMap.InsertLine):
			if m.isReadOnlyRow() {
				break
			}
			if m.MaxHeight > 0 && len(m.value) >= m.MaxHeight {
				return m, nil
			}
			m.pushUndoSnapshot()
			m.insertVariableAt(m.row+1, "", "")
		case key.Matches(msg, m.KeyMap.LineEnd):
			m.selActive = false
			m.CursorEnd()
		case key.Matches(msg, m.KeyMap.LineStart):
			m.selActive = false
			m.CursorStart()
		case key.Matches(msg, m.KeyMap.CharacterForward):
			m.selActive = false
			m.characterRight()
		case key.Matches(msg, m.KeyMap.LineNext):
			m.selActive = false
			m.CursorDown()
		case key.Matches(msg, m.KeyMap.WordForward):
			m.selActive = false
			m.wordRight()
		case key.Matches(msg, m.KeyMap.Paste):
			return m, Paste
		case key.Matches(msg, m.KeyMap.CharacterBackward):
			m.selActive = false
			m.characterLeft(false /* insideLine */)
		case key.Matches(msg, m.KeyMap.LinePrevious):
			m.selActive = false
			m.CursorUp()
		case key.Matches(msg, m.KeyMap.WordBackward):
			m.selActive = false
			m.wordLeft()
		case key.Matches(msg, m.KeyMap.InputBegin):
			m.selActive = false
			m.MoveToBegin()
		case key.Matches(msg, m.KeyMap.InputEnd):
			m.selActive = false
			m.MoveToEnd()
		case key.Matches(msg, m.KeyMap.PageUp):
			m.selActive = false
			m.PageUp()
		case key.Matches(msg, m.KeyMap.PageDown):
			m.selActive = false
			m.PageDown()
		case key.Matches(msg, m.KeyMap.LowercaseWordForward):
			m.invalidateDiffCache(m.row)
			m.pushUndoSnapshot()
			m.lowercaseRight()
		case key.Matches(msg, m.KeyMap.UppercaseWordForward):
			m.invalidateDiffCache(m.row)
			m.pushUndoSnapshot()
			m.uppercaseRight()
		case key.Matches(msg, m.KeyMap.CapitalizeWordForward):
			m.invalidateDiffCache(m.row)
			m.pushUndoSnapshot()
			m.capitalizeRight()
		case key.Matches(msg, m.KeyMap.TransposeCharacterBackward):
			m.invalidateDiffCache(m.row)
			m.pushUndoSnapshot()
			m.transposeLeft()

		case key.Matches(msg, m.KeyMap.ToggleInsert):
			m.Overwrite = !m.Overwrite

		default:
			if !m.isEditableAtCursor() {
				break
			}
			m.pushUndoSnapshot()
			// In overwrite mode, replace the character at cursor before inserting.
			if m.Overwrite && msg.Text != "" && m.col < len(m.value[m.row]) {
				m.invalidateDiffCache(m.row)
				m.value[m.row] = slices.Delete(m.value[m.row], m.col, m.col+1)
				m.reclassifyCurrentLine()
			}
			m.insertRunesFromUserInput([]rune(msg.Text))
			// Keep lineMeta in sync for user-defined lines as the user types
			// (updates IsVariable and EditableStartCol when '=' is added/removed).
			m.reclassifyCurrentLine()
			// When '=' is typed at the end of a line with no existing value,
			// auto-fill the default value if one is known.
			if msg.Text == "=" && m.DefaultValueFunc != nil {
				lineStr := string(m.value[m.row])
				eqIdx := strings.Index(lineStr, "=")
				if eqIdx >= 0 && strings.Count(lineStr, "=") == 1 && len(lineStr) == eqIdx+1 {
					varName := strings.TrimSpace(lineStr[:eqIdx])
					if varName != "" {
						def := m.DefaultValueFunc(varName)
						if def != "" && def != "''" {
							m.insertRunesFromUserInput([]rune(def))
						}
					}
				}
			}
		}

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			m.handleMouseClick(msg)
		}

	case tea.MouseMotionMsg:
		if m.IsDragging() {
			m.handleMouseMotion(msg)
			m.InvalidateCache()
		}

	case tea.MouseReleaseMsg:
		if m.IsDragging() { // Use public IsDragging to cover both types
			m.handleMouseRelease(msg)
			m.InvalidateCache()
		}

	case pasteMsg:
		if !m.isEditableAtCursor() {
			break
		}
		m.pushUndoSnapshot()
		m.insertRunesFromUserInput([]rune(msg))

	case pasteErrMsg:
		m.Err = msg
	}

	// Handle viewport update without resetting content here. 
	// repositionView() will handle scrolling the viewport based on cursor movement.
	oldY, oldX := m.viewport.YOffset(), m.viewport.XOffset()
	vp, cmd := m.viewport.Update(msg)
	m.viewport = &vp
	if m.viewport.YOffset() != oldY || m.viewport.XOffset() != oldX {
		m.InvalidateCache()
	}
	cmds = append(cmds, cmd)

	if m.useVirtualCursor {
		m.virtualCursor, cmd = m.virtualCursor.Update(msg)

		// If the cursor has moved, reset the blink state. This is a small UX
		// nuance that makes cursor movement obvious and feel snappy.
		newRow, newCol := m.cursorLineNumber(), m.col
		if (newRow != oldRow || newCol != oldCol) && m.virtualCursor.Mode() == cursor.CursorBlink {
			m.virtualCursor.IsBlinked = false
			cmd = m.virtualCursor.Blink()
		}
		cmds = append(cmds, cmd)
	}

	// Skip repositionView() when the scrollbar directly moved the viewport —
	// otherwise it would snap the view back to keep the cursor visible, preventing
	// the user from scrolling to non-editable lines (e.g. comments at the top).
	if !m.sbScrolled {
		m.repositionView()
	}

	return m, tea.Batch(cmds...)
}

// wordBoundsAt returns the [start, end) of the "word" at col in line.
// Treats '=' and whitespace as word separators so key and value are distinct.
func wordBoundsAt(line []rune, col int) (start, end int) {
	n := len(line)
	if n == 0 || col >= n {
		return col, col
	}
	isSep := func(r rune) bool { return r == '=' || unicode.IsSpace(r) }
	if isSep(line[col]) {
		return col, col
	}
	start = col
	for start > 0 && !isSep(line[start-1]) {
		start--
	}
	end = col
	for end < n && !isSep(line[end]) {
		end++
	}
	return start, end
}

func (m *Model) handleMouseClick(msg tea.MouseClickMsg) {
	styles := m.activeStyle()
	msg.X -= styles.Base.GetMarginLeft() + styles.Base.GetPaddingLeft() + styles.Base.GetBorderLeftSize()
	msg.Y -= styles.Base.GetMarginTop() + styles.Base.GetPaddingTop() + styles.Base.GetBorderTopSize()

	// Every left-click clears any prior text selection.
	m.selActive = false
	m.isSelecting = false

	// Gutter width (prompts + line numbers)
	gutterWidth := lipgloss.Width(m.promptView(0, -1)) + lipgloss.Width(m.lineNumberView(0, false, -1))

	total := m.totalDisplayLines()
	visible := m.height
	scrollbarX := m.width + gutterWidth
	
	// Check if click is on the scrollbar (last column of the viewport area)
	if total > visible && msg.X >= scrollbarX {
		if visible >= 3 {
			// Check up arrow
			if msg.Y == 0 {
				m.viewport.ScrollUp(1)
				m.constrainCursorToView()
				m.sbScrolled = true
				m.InvalidateCache()
				return
			}
			// Check down arrow
			if msg.Y == visible-1 {
				m.viewport.ScrollDown(1)
				m.constrainCursorToView()
				m.sbScrolled = true
				m.InvalidateCache()
				return
			}

			trackH := visible - 2
			maxOff := total - visible
			thumbH := max(1, trackH*visible/total)
			offset := m.viewport.YOffset()

			thumbStart := 0
			if maxOff > 0 {
				thumbStart = (trackH - thumbH) * offset / maxOff
			}
			thumbEnd := thumbStart + thumbH

			trackRelY := msg.Y - 1
			if trackRelY >= thumbStart && trackRelY < thumbEnd {
				m.isScrollbarDragging = true
				m.sbDragMouseOffsetY = trackRelY - thumbStart
			} else {
				if trackH > 1 {
					targetPct := float64(trackRelY) / float64(trackH-1)
					targetOffset := int(targetPct * float64(maxOff))
					m.viewport.SetYOffset(clamp(targetOffset, 0, maxOff))
				}
				m.constrainCursorToView()
				m.sbScrolled = true
				m.InvalidateCache()
			}
		} else {
			trackH := visible
			thumbH := max(1, trackH*visible/total)
			maxOff := total - visible

			offset := m.viewport.YOffset()
			thumbTrackStart := 0
			if maxOff > 0 {
				thumbTrackStart = (trackH - thumbH) * offset / maxOff
			}
			thumbEnd := thumbTrackStart + thumbH

			if msg.Y >= thumbTrackStart && msg.Y < thumbEnd {
				// Clicked on the thumb
				m.isScrollbarDragging = true
				m.sbDragMouseOffsetY = msg.Y - thumbTrackStart
			} else {
				if trackH > 1 {
					targetPct := float64(msg.Y) / float64(trackH-1)
					targetOffset := int(targetPct * float64(maxOff))
					m.viewport.SetYOffset(clamp(targetOffset, 0, maxOff))
				}
				m.constrainCursorToView()
				m.sbScrolled = true
				m.InvalidateCache()
			}
		}
		return
	}

	// Adjust for viewport scroll
	targetViewLine := msg.Y + m.viewport.YOffset()
	targetColX := msg.X - gutterWidth

	// Check if click is in the gutter area (line numbers)
	if msg.X < gutterWidth {
		// Find which logical row was clicked
		currViewLine := 0
		for l, lineRunes := range m.value {
			wrappedLines := m.memoizedWrap(lineRunes, m.width)
			numWrapped := len(wrappedLines)
			if targetViewLine >= currViewLine && targetViewLine < currViewLine+numWrapped {
				if m.lineMeta[l].IsUserDefined && !m.lineMeta[l].ReadOnly {
					m.isDragging = true
					m.draggedRow = l
					m.row = l
					m.CursorStart()
				}
				return
			}
			currViewLine += numWrapped
		}
	}

	// Find logical row and column by iterating through m.value and wrapped lines
	currViewLine := 0
	for l, lineRunes := range m.value {
		wrappedLines := m.memoizedWrap(lineRunes, m.width)
		numWrapped := len(wrappedLines)

		if targetViewLine >= currViewLine && targetViewLine < currViewLine+numWrapped {
			// Click is on this logical line
			m.row = l

			// Find which wrapped line it is
			wrappedLineIdx := targetViewLine - currViewLine

			// Find the character index in the logical line
			charIdx := 0
			for i := 0; i < wrappedLineIdx; i++ {
				charIdx += len(wrappedLines[i])
			}

			// Find the column within the wrapped line
			clickedWrappedCol := clamp(targetColX, 0, len(wrappedLines[wrappedLineIdx]))
			m.col = charIdx + clickedWrappedCol

			// Clamp to actual line length
			if m.col > len(lineRunes) {
				m.col = len(lineRunes)
			}

			// Multi-click detection: same row, same col, within 400 ms.
			const multiClickWindow = 400 * time.Millisecond
			now := time.Now()
			if m.row == m.lastClickRow && m.col == m.lastClickCol &&
				now.Sub(m.lastClickTime) <= multiClickWindow {
				m.clickCount++
			} else {
				m.clickCount = 1
			}
			m.lastClickTime = now
			m.lastClickRow = m.row
			m.lastClickCol = m.col

			switch m.clickCount {
			case 2:
				// Double-click: select the word at cursor ('=' is a boundary).
				s, e := wordBoundsAt(lineRunes, m.col)
				if s < e {
					m.selRow = m.row
					m.selStartCol = s
					m.selEndCol = e
					m.selAnchorCol = s
					m.selActive = true
					m.isSelecting = false // selection complete
				}
			case 3:
				// Triple-click on value side: select entire value (after '=').
				// On key side: no change — keep the word selection from double-click.
				eqIdx := -1
				for i, r := range lineRunes {
					if r == '=' {
						eqIdx = i
						break
					}
				}
				if eqIdx >= 0 && m.col > eqIdx {
					m.selRow = m.row
					m.selStartCol = eqIdx + 1
					m.selEndCol = len(lineRunes)
					m.selAnchorCol = eqIdx + 1
					m.selActive = m.selStartCol < m.selEndCol
					m.isSelecting = false
				}
				// On key side: selection stays as-is from double-click.
			default: // 1 or 4+
				if m.clickCount >= 4 {
					// Four clicks: select entire line.
					m.selRow = m.row
					m.selStartCol = 0
					m.selEndCol = len(lineRunes)
					m.selAnchorCol = 0
					m.selActive = m.selStartCol < m.selEndCol
					m.isSelecting = false
				} else {
					// Single click: just set anchor for potential drag.
					m.isSelecting = true
					m.selRow = m.row
					m.selAnchorCol = m.col
					m.selStartCol = m.col
					m.selEndCol = m.col
				}
			}

			return
		}
		currViewLine += numWrapped
	}
}

func (m *Model) handleMouseMotion(msg tea.MouseMotionMsg) {
	styles := m.activeStyle()
	msg.X -= styles.Base.GetMarginLeft() + styles.Base.GetPaddingLeft() + styles.Base.GetBorderLeftSize()
	msg.Y -= styles.Base.GetMarginTop() + styles.Base.GetPaddingTop() + styles.Base.GetBorderTopSize()

	if m.isScrollbarDragging {
		total := m.totalDisplayLines()
		visible := m.height
		if total > visible {
			if visible >= 3 {
				trackH := visible - 2
				maxOff := total - visible
				thumbH := max(1, trackH*visible/total)
				thumbTravel := trackH - thumbH
				if thumbTravel < 1 {
					thumbTravel = 1
				}

				trackRelY := msg.Y - 1
				thumbTrackStart := trackRelY - m.sbDragMouseOffsetY
				if thumbTrackStart < 0 {
					thumbTrackStart = 0
				}
				if thumbTrackStart > thumbTravel {
					thumbTrackStart = thumbTravel
				}

				newOff := thumbTrackStart * maxOff / thumbTravel
				m.viewport.SetYOffset(clamp(newOff, 0, maxOff))
			} else {
				trackH := visible
				maxOff := total - visible
				thumbH := max(1, trackH*visible/total)
				thumbTravel := trackH - thumbH
				if thumbTravel < 1 {
					thumbTravel = 1
				}

				thumbTrackStart := msg.Y - m.sbDragMouseOffsetY
				if thumbTrackStart < 0 {
					thumbTrackStart = 0
				}
				if thumbTrackStart > thumbTravel {
					thumbTrackStart = thumbTravel
				}

				newOff := thumbTrackStart * maxOff / thumbTravel
				m.viewport.SetYOffset(clamp(newOff, 0, maxOff))
			}
			m.constrainCursorToView()
			m.sbScrolled = true
		}
		return
	}

	if m.isSelecting {
		gutterWidth := lipgloss.Width(m.promptView(0, -1)) + lipgloss.Width(m.lineNumberView(0, false, -1))
		targetViewLine := msg.Y + m.viewport.YOffset()
		targetColX := msg.X - gutterWidth
		currViewLine := 0
		for l, lineRunes := range m.value {
			wrappedLines := m.memoizedWrap(lineRunes, m.width)
			numWrapped := len(wrappedLines)
			if targetViewLine >= currViewLine && targetViewLine < currViewLine+numWrapped {
				if l == m.selRow { // single-row selection only
					wrappedLineIdx := targetViewLine - currViewLine
					charIdx := 0
					for i := 0; i < wrappedLineIdx; i++ {
						charIdx += len(wrappedLines[i])
					}
					curCol := charIdx + clamp(targetColX, 0, len(wrappedLines[wrappedLineIdx]))
					if curCol > len(lineRunes) {
						curCol = len(lineRunes)
					}
					start, end := m.selAnchorCol, curCol
					if start > end {
						start, end = end, start
					}
					m.selStartCol = start
					m.selEndCol = end
					m.selActive = start < end
				}
				return
			}
			currViewLine += numWrapped
		}
		return
	}

	if !m.isDragging {
		return
	}


	targetViewLine := msg.Y + m.viewport.YOffset()

	// Find which logical row the mouse is over
	currViewLine := 0
	for l, lineRunes := range m.value {
		wrappedLines := m.memoizedWrap(lineRunes, m.width)
		numWrapped := len(wrappedLines)
		if targetViewLine >= currViewLine && targetViewLine < currViewLine+numWrapped {
			if l != m.row {
				// Attempt to swap
				if l < m.row {
					for m.row > l {
						oldRow := m.row
						m.MoveVariableUp()
						if m.row == oldRow {
							// Stuck (read-only row)
							break
						}
					}
				} else {
					for m.row < l {
						oldRow := m.row
						m.MoveVariableDown()
						if m.row == oldRow {
							// Stuck
							break
						}
					}
				}
			}
			return
		}
		currViewLine += numWrapped
	}
}

func (m *Model) handleMouseRelease(msg tea.MouseReleaseMsg) {
	m.isDragging = false
	m.isScrollbarDragging = false
	m.isSelecting = false
	// selActive persists — selection remains visible until the next left-click.
}

// IsDragging returns true if the user is currently dragging a line, scrollbar, or text selection.
func (m Model) IsDragging() bool {
	return m.isDragging || m.isScrollbarDragging || m.isSelecting
}

// IsScrollbarDragging reports whether the scrollbar thumb is currently being dragged.
func (m Model) IsScrollbarDragging() bool {
	return m.isScrollbarDragging
}

// renderRunes formats runes with partial highlighting.
func (m *Model) invalidateDiffCache(row int) {
	if m.diffCache != nil {
		delete(m.diffCache, row)
	}
	m.cacheValid = false
}

func (m *Model) getDiffMask(row int) []bool {
	if m.diffCache == nil {
		m.diffCache = make(map[int][]bool)
	}
	if mask, ok := m.diffCache[row]; ok {
		return mask
	}

	if row >= len(m.lineMeta) || row >= len(m.value) {
		return nil
	}
	meta := m.lineMeta[row]
	if meta.DefaultValue == "" {
		return nil
	}

	lineRunes := m.value[row]
	if meta.EditableStartCol >= len(lineRunes) {
		return nil
	}

	valuePartRunes := lineRunes[meta.EditableStartCol:]
	// Filter out trailing newlines for diff purposes
	for len(valuePartRunes) > 0 && (valuePartRunes[len(valuePartRunes)-1] == '\n' || valuePartRunes[len(valuePartRunes)-1] == '\r') {
		valuePartRunes = valuePartRunes[:len(valuePartRunes)-1]
	}

	valuePart := string(valuePartRunes)
	defValue := meta.DefaultValue

	diffs := m.dmp.DiffMain(defValue, valuePart, false)

	mask := make([]bool, len(valuePartRunes))
	cursor := 0
	for _, d := range diffs {
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			cursor += len([]rune(d.Text))
		case diffmatchpatch.DiffInsert:
			runes := []rune(d.Text)
			for i := 0; i < len(runes); i++ {
				if cursor+i < len(mask) {
					mask[cursor+i] = true
				}
			}
			cursor += len(runes)
		case diffmatchpatch.DiffDelete:
			// Deletions don't occupy space in the buffer
		}
	}
	m.diffCache[row] = mask
	return mask
}

func (m *Model) renderRunes(runes []rune, l int, startIdx int, baseStyle lipgloss.Style) string {
	if l >= len(m.lineMeta) {
		return baseStyle.Render(string(runes))
	}
	meta := &m.lineMeta[l]
	if meta.PendingDelete {
		return m.activeStyle().PendingDeleteText.Inherit(baseStyle).Render(string(runes))
	}
	if meta.ReadOnly {
		return m.activeStyle().ReadOnlyText.Inherit(baseStyle).Render(string(runes))
	}
	if !meta.IsVariable && !meta.IsUserDefined {
		return baseStyle.Render(string(runes))
	}

	// Determine if the current variable name is valid
	keyIsValid := true
	keyIsDuplicate := false
	if (meta.IsVariable || meta.IsUserDefined) && m.ValidateFunc != nil && m.ValidationType != "" {
		lineRunes := m.value[l]
		eqIdx := -1
		for i, r := range lineRunes {
			if r == '=' {
				eqIdx = i
				break
			}
		}
		if eqIdx > 0 {
			key := strings.TrimSpace(string(lineRunes[:eqIdx]))
			vType := m.ValidationType
			if vType == "APPNAME" {
				vType = m.ValidationAppName
			}
			keyIsValid = m.ValidateFunc(key, vType)

			// Duplicate check using the pre-calculated map
			if m.duplicateKeys != nil && m.duplicateKeys[key] > 1 {
				keyIsDuplicate = true
			}
		} else if meta.IsUserDefined && len(lineRunes) > 0 {
			// User defined line with content but no '=' is an incomplete variable
			keyIsValid = false
		}
	}

	var b strings.Builder
	styles := m.activeStyle()
	modStyle := styles.ModifiedText.Inherit(baseStyle)
	invalidStyle := styles.InvalidText.Inherit(baseStyle)
	duplicateStyle := styles.DuplicateText.Inherit(baseStyle)
	builtinKeyStyle := styles.BuiltinText.Inherit(baseStyle).Inline(true)
	userKeyStyle := styles.UserDefinedText.Inherit(baseStyle).Inline(true)

	for i, r := range runes {
		fullIdx := startIdx + i

		// Selection highlight takes priority over other styles.
		if m.selActive && l == m.selRow && fullIdx >= m.selStartCol && fullIdx < m.selEndCol {
			b.WriteString(m.activeStyle().SelectionText.Inherit(baseStyle).Render(string(r)))
			continue
		}

		// Key part highlighting
		if fullIdx < meta.EditableStartCol-1 {
			if !keyIsValid {
				b.WriteString(invalidStyle.Render(string(r)))
				continue
			}
			if keyIsDuplicate {
				b.WriteString(duplicateStyle.Render(string(r)))
				continue
			}
			if meta.IsUserDefined {
				b.WriteString(userKeyStyle.Render(string(r)))
			} else {
				b.WriteString(builtinKeyStyle.Render(string(r)))
			}
			continue
		}

		// '=' or following content
		if fullIdx < meta.EditableStartCol {
			b.WriteString(baseStyle.Render(string(r)))
		} else {
			valIdx := fullIdx - meta.EditableStartCol
			mask := m.getDiffMask(l)

			isModifiedChar := false
			if mask != nil && valIdx >= 0 && valIdx < len(mask) {
				isModifiedChar = mask[valIdx]
			}

			// Note: We don't use meta.DefaultValue == "" || meta.IsUserDefined || meta.IsNewLine logic here
			// because localized diffing only applies when we have a default value to compare against.
			// Variable headers/key/equals are handled above or in caller.
			if r == '\n' || r == '\r' || !isModifiedChar {
				b.WriteString(baseStyle.Render(string(r)))
			} else {
				b.WriteString(modStyle.Render(string(r)))
			}
		}
	}
	return b.String()
}

func (m *Model) view() string {
	// Pre-calculate duplicates for rendering
	m.duplicateKeys = make(map[string]int)
	for _, lineRunes := range m.value {
		eqIdx := -1
		for i, r := range lineRunes {
			if r == '=' {
				eqIdx = i
				break
			}
		}
		if eqIdx > 0 {
			key := strings.TrimSpace(string(lineRunes[:eqIdx]))
			m.duplicateKeys[key]++
		}
	}

	if len(m.Value()) == 0 && m.row == 0 && m.col == 0 && m.Placeholder != "" {
		return m.placeholderView()
	}
	m.virtualCursor.TextStyle = m.activeStyle().computedCursorLine()

	var (
		s                strings.Builder
		style            lipgloss.Style
		widestLineNumber int
		lineInfo         = m.LineInfo()
		styles           = m.activeStyle()
	)

	displayLine := 0
	for l, line := range m.value {
		wrappedLines := m.memoizedWrap(line, m.width)

		if m.row == l {
			style = styles.computedCursorLine()
		} else {
			style = styles.computedText()
		}

		charIndex := 0
		for wl, wrappedLine := range wrappedLines {
			dataLineIdx := l
			if wl > 0 {
				dataLineIdx = -1
			}
			prompt := m.promptView(displayLine, dataLineIdx)
			prompt = styles.computedPrompt().Render(prompt)
			s.WriteString(style.Render(prompt))
			displayLine++

			var ln string
			if m.ShowLineNumbers {
				if wl == 0 { // normal line
					isCursorLine := m.row == l
					s.WriteString(m.lineNumberView(l+1, isCursorLine, l))
				} else { // soft wrapped line
					isCursorLine := m.row == l
					s.WriteString(m.lineNumberView(-1, isCursorLine, -1))
				}
			}

			// Note the widest line number for padding purposes later.
			lnw := uniseg.StringWidth(ln)
			if lnw > widestLineNumber {
				widestLineNumber = lnw
			}

			strwidth := uniseg.StringWidth(string(wrappedLine))
			padding := m.width - strwidth
			// If the trailing space causes the line to be wider than the
			// width, we should not draw it to the screen since it will result
			// in an extra space at the end of the line which can look off when
			// the cursor line is showing.
			if strwidth > m.width {
				// The character causing the line to be wider than the width is
				// guaranteed to be a space since any other character would
				// have been wrapped.
				wrappedLine = []rune(strings.TrimSuffix(string(wrappedLine), " "))
				padding -= m.width - strwidth
			}
			if m.row == l && lineInfo.RowOffset == wl {
				s.WriteString(m.renderRunes(wrappedLine[:lineInfo.ColumnOffset], l, charIndex, style))
				if m.col >= len(line) && lineInfo.CharOffset >= m.width {
					m.virtualCursor.SetChar(" ")
					s.WriteString(m.virtualCursor.View())
				} else {
					m.virtualCursor.SetChar(string(wrappedLine[lineInfo.ColumnOffset]))
					s.WriteString(style.Render(m.virtualCursor.View()))
					s.WriteString(m.renderRunes(wrappedLine[lineInfo.ColumnOffset+1:], l, charIndex+lineInfo.ColumnOffset+1, style))
				}
			} else {
				s.WriteString(m.renderRunes(wrappedLine, l, charIndex, style))
			}
			s.WriteString(style.Render(strings.Repeat(" ", max(0, padding))))
			s.WriteRune('\n')
			charIndex += len(wrappedLine)
		}
	}

	// Always show at least `m.Height` lines at all times.
	// To do this we can simply pad out a few extra new lines in the view.
	for i := displayLine; i < m.height; i++ {
		s.WriteString(m.promptView(i, -1))

		// Write end of buffer content
		leftGutter := string(m.EndOfBufferCharacter)
		rightGapWidth := m.Width() - uniseg.StringWidth(leftGutter) + widestLineNumber
		rightGap := strings.Repeat(" ", max(0, rightGapWidth))
		s.WriteString(styles.computedEndOfBuffer().Render(leftGutter + rightGap))
		s.WriteRune('\n')
	}

	m.SaveCache(s.String())
	return s.String()
}

// View renders the text area in its current state.
func (m Model) View() string {
	if cached, ok := m.CheckCache(); ok {
		return cached
	}

	// XXX: This is a workaround for the case where the viewport hasn't
	// been initialized yet like during the initial render. In that case,
	// we need to render the view again because Update hasn't been called
	// yet to set the content of the viewport.
	// We save and restore the YOffset because SetContent resets it to 0.
	currOffset := m.viewport.YOffset()
	m.viewport.SetContent(m.view())
	m.viewport.SetYOffset(currOffset)

	view := m.viewport.View()

	// Scrollbar column — delegate to injected renderer when available.
	total := m.totalDisplayLines()
	visible := m.height
	offset := m.viewport.YOffset()
	if m.ScrollbarFunc != nil {
		view = m.ScrollbarFunc(view, total, visible, offset, true, m.LineCharacters)
	} else {
		// Built-in fallback scrollbar (used when no ScrollbarFunc is injected).
		lines := strings.Split(view, "\n")
		if total > visible && visible >= 3 {
			trackH := visible - 2 // rows 1..visible-2 are the track
			maxOff := total - visible
			thumbH := max(1, trackH*visible/total)
			thumbStart := 0
			if maxOff > 0 {
				thumbStart = (trackH - thumbH) * offset / maxOff
			}
			thumbEnd := thumbStart + thumbH

			var trackChar, thumbChar, upArrow, downArrow string
			if m.LineCharacters {
				trackChar, thumbChar, upArrow, downArrow = "░", "█", "▴", "▾"
			} else {
				trackChar, thumbChar, upArrow, downArrow = ";", "#", "^", "v"
			}

			for i := 0; i < len(lines) && i < visible; i++ {
				var char string
				switch {
				case i == 0:
					char = upArrow
				case i == visible-1:
					char = downArrow
				case i-1 >= thumbStart && i-1 < thumbEnd:
					char = thumbChar
				default:
					char = trackChar
				}
				if char == thumbChar || char == upArrow || char == downArrow {
					lines[i] += m.activeStyle().ScrollbarThumb.Render(char)
				} else {
					lines[i] += m.activeStyle().ScrollbarTrack.Render(char)
				}
			}
		} else {
			for i := 0; i < len(lines) && i < visible; i++ {
				lines[i] += " "
			}
		}
		view = strings.Join(lines, "\n")
	}

	styles := m.activeStyle()
	return styles.Base.Render(view)
}

// totalDisplayLines returns the total number of lines including soft wraps.
func (m Model) totalDisplayLines() int {
	lines := 0
	for i := range m.value {
		lines += len(m.memoizedWrap(m.value[i], m.width))
	}
	return lines
}

// promptView renders a single line of the prompt.
// promptView renders the gutter character for a display line.
// dataLine is the index into m.value/m.lineMeta for the logical line being rendered;
// pass -1 for soft-wrap continuation rows and end-of-buffer rows (no marker shown).
func (m Model) promptView(displayLine, dataLine int) (prompt string) {
	styles := m.activeStyle()

	// Show diff markers in the gutter for the first row of each logical line.
	if dataLine >= 0 && dataLine < len(m.lineMeta) {
		meta := m.lineMeta[dataLine]
		if meta.PendingDelete {
			return styles.GutterDeleted.Render("-")
		}
		if meta.IsInvalid {
			return styles.GutterInvalid.Render("!")
		}
		if meta.IsVariable {
			lineContent := string(m.value[dataLine])
			// + for lines explicitly flagged as new, or for lines that were blank at
			// load time and now have content (user typed into a blank line).
			if meta.IsNewLine || meta.InitialLine == "" {
				return styles.GutterAdded.Render("+")
			}
			if !meta.ReadOnly && meta.InitialLine != "" && lineContent != meta.InitialLine {
				return styles.GutterModified.Render("~")
			}
		}
	}

	prompt = m.Prompt
	if m.promptFunc == nil {
		return prompt
	}
	prompt = m.promptFunc(PromptInfo{
		LineNumber: displayLine,
		Focused:    m.focus,
	})
	width := lipgloss.Width(prompt)
	if width < m.promptWidth {
		prompt = fmt.Sprintf("%*s%s", m.promptWidth-width, "", prompt)
	}

	return styles.computedPrompt().Render(prompt)
}

// lineNumberView renders the line number.
//
// If n is less than 0, a space styled as a line number is returned
// instead. Such cases are used for soft-wrapped lines.
//
// isCursorLine indicates whether this line number is for a 'cursorline' line.
// dataLine is the index into m.value/m.lineMeta (-1 if not applicable).
func (m Model) lineNumberView(n int, isCursorLine bool, dataLine int) (str string) {
	if !m.ShowLineNumbers {
		return ""
	}

	if n <= 0 {
		str = " "
	} else {
		str = strconv.Itoa(n)
	}

	lineNumberStyle := m.activeStyle().computedLineNumber()
	if isCursorLine {
		lineNumberStyle = m.activeStyle().computedLineNumberSelected()
	}

	// Tint line numbers whose value differs from the template default.
	if n > 0 && dataLine >= 0 {
		mask := m.getDiffMask(dataLine)
		for _, changed := range mask {
			if changed {
				if isCursorLine {
					lineNumberStyle = m.activeStyle().computedLineNumberModifiedSelected()
				} else {
					lineNumberStyle = m.activeStyle().computedLineNumberModified()
				}
				break
			}
		}
	}

	// Format line number dynamically based on the maximum number of lines.
	// Minimum of 3 digits for consistent alignment as per user request.
	digits := max(3, numDigits(m.MaxHeight))
	
	// Apply line number style ONLY to the digits themselves.
	// The outer right spacing is rendered natively so it inherits 
	// the dialogue base background color rather than the line number background.
	formattedNum := fmt.Sprintf("%*v", digits, str)
	
	return lineNumberStyle.Render(formattedNum) + " "
}

// placeholderView returns the prompt and placeholder, if any.
func (m Model) placeholderView() string {
	var (
		s      strings.Builder
		p      = m.Placeholder
		styles = m.activeStyle()
	)
	// word wrap lines
	pwordwrap := ansi.Wordwrap(p, m.width, "")
	// hard wrap lines (handles lines that could not be word wrapped)
	pwrap := ansi.Hardwrap(pwordwrap, m.width, true)
	// split string by new lines
	plines := strings.Split(strings.TrimSpace(pwrap), "\n")

	for i := range m.height {
		isLineNumber := len(plines) > i

		lineStyle := styles.computedPlaceholder()
		if len(plines) > i {
			lineStyle = styles.computedCursorLine()
		}

		// render prompt
		prompt := m.promptView(i, -1)
		prompt = styles.computedPrompt().Render(prompt)
		s.WriteString(lineStyle.Render(prompt))

		// when show line numbers enabled:
		// - render line number for only the cursor line
		// - indent other placeholder lines
		// this is consistent with vim with line numbers enabled
		if m.ShowLineNumbers {
			var ln int

			switch {
			case i == 0:
				ln = i + 1
				fallthrough
			case len(plines) > i:
				s.WriteString(m.lineNumberView(ln, isLineNumber, -1))
			default:
			}
		}

		switch {
		// first line
		case i == 0:
			// first character of first line as cursor with character
			m.virtualCursor.TextStyle = styles.computedPlaceholder()

			ch, rest, _, _ := uniseg.FirstGraphemeClusterInString(plines[0], 0)
			m.virtualCursor.SetChar(ch)
			s.WriteString(lineStyle.Render(m.virtualCursor.View()))

			// the rest of the first line
			s.WriteString(lineStyle.Render(styles.computedPlaceholder().Render(rest)))

			// extend the first line with spaces to fill the width, so that
			// the entire line is filled when cursorline is enabled.
			gap := strings.Repeat(" ", max(0, m.width-lipgloss.Width(plines[0])))
			s.WriteString(lineStyle.Render(gap))
		// remaining lines
		case len(plines) > i:
			// current line placeholder text
			if len(plines) > i {
				placeholderLine := plines[i]
				gap := strings.Repeat(" ", max(0, m.width-uniseg.StringWidth(plines[i])))
				s.WriteString(lineStyle.Render(placeholderLine + gap))
			}
		default:
			// end of line buffer character
			eob := styles.computedEndOfBuffer().Render(string(m.EndOfBufferCharacter))
			s.WriteString(eob)
		}

		// terminate with new line
		s.WriteRune('\n')
	}

	m.viewport.SetContent(s.String())
	return styles.Base.Render(m.viewport.View())
}

// Blink returns the blink command for the virtual cursor.
func Blink() tea.Msg {
	return cursor.Blink()
}

// Cursor returns a [tea.Cursor] for rendering a real cursor in a Bubble Tea
// program. This requires that [Model.VirtualCursor] is set to false.
//
// Note that you will almost certainly also need to adjust the offset cursor
// position per the textarea's per the textarea's position in the terminal.
//
// Example:
//
//	// In your top-level View function:
//	f := tea.NewFrame(m.textarea.View())
//	f.Cursor = m.textarea.Cursor()
//	f.Cursor.Position.X += offsetX
//	f.Cursor.Position.Y += offsetY
func (m Model) Cursor() *tea.Cursor {
	if m.useVirtualCursor || !m.Focused() {
		return nil
	}

	lineInfo := m.LineInfo()
	w := lipgloss.Width
	baseStyle := m.activeStyle().Base

	xOffset := lineInfo.CharOffset +
		w(m.promptView(0, -1)) +
		w(m.lineNumberView(0, false, -1)) +
		baseStyle.GetMarginLeft() +
		baseStyle.GetPaddingLeft() +
		baseStyle.GetBorderLeftSize()

	yOffset := m.cursorLineNumber() -
		m.viewport.YOffset() +
		baseStyle.GetMarginTop() +
		baseStyle.GetPaddingTop() +
		baseStyle.GetBorderTopSize()

	c := tea.NewCursor(xOffset, yOffset)
	c.Blink = m.styles.Cursor.Blink
	c.Color = m.styles.Cursor.Color
	c.Shape = m.styles.Cursor.Shape
	return c
}

func (m Model) memoizedWrap(runes []rune, width int) [][]rune {
	input := line{runes: runes, width: width}
	if v, ok := m.cache.Get(input); ok {
		return v
	}
	v := wrap(runes, width)
	m.cache.Set(input, v)
	return v
}

// cursorLineNumber returns the line number that the cursor is on.
// This accounts for soft wrapped lines.
func (m Model) cursorLineNumber() int {
	line := 0
	for i := range m.row {
		// Calculate the number of lines that the current line will be split
		// into.
		line += len(m.memoizedWrap(m.value[i], m.width))
	}
	line += m.LineInfo().RowOffset
	return line
}

// mergeLineBelow merges the current line the cursor is on with the line below.
func (m *Model) mergeLineBelow(row int) {
	m.diffCache = make(map[int][]bool)
	if row >= len(m.value)-1 {
		return
	}

	// To perform a merge, we will need to combine the two lines and then
	m.value[row] = append(m.value[row], m.value[row+1]...)

	// Shift all lines up by one
	for i := row + 1; i < len(m.value)-1; i++ {
		m.value[i] = m.value[i+1]
	}
	if row+1 < len(m.lineMeta) {
		m.lineMeta = append(m.lineMeta[:row+1], m.lineMeta[row+2:]...)
	}

	// And, remove the last line
	if len(m.value) > 0 {
		m.value = m.value[:len(m.value)-1]
	}
}

// mergeLineAbove merges the current line the cursor is on with the line above.
func (m *Model) mergeLineAbove(row int) {
	m.diffCache = make(map[int][]bool)
	if row <= 0 {
		return
	}

	m.col = len(m.value[row-1])
	m.row = m.row - 1

	// To perform a merge, we will need to combine the two lines and then
	m.value[row-1] = append(m.value[row-1], m.value[row]...)

	// Shift all lines up by one
	for i := row; i < len(m.value)-1; i++ {
		m.value[i] = m.value[i+1]
	}
	if row < len(m.lineMeta) {
		m.lineMeta = append(m.lineMeta[:row], m.lineMeta[row+1:]...)
	}

	// And, remove the last line
	if len(m.value) > 0 {
		m.value = m.value[:len(m.value)-1]
	}
}

// reclassifyCurrentLine updates IsVariable, EditableStartCol, and IsUserDefined
// for the current row as the user types. Keeps rendering and key-lock correct
// without a full ReclassifyEnv pass.
func (m *Model) reclassifyCurrentLine() {
	if m.row >= len(m.lineMeta) || m.row >= len(m.value) {
		return
	}
	meta := &m.lineMeta[m.row]
	// Skip pre-existing built-in variables — their key is locked.
	if meta.IsVariable && !meta.IsUserDefined && !meta.IsNewLine {
		return
	}
	line := m.value[m.row]
	eqIdx := -1
	for i, r := range line {
		if r == '=' {
			eqIdx = i
			break
		}
	}
	if eqIdx >= 0 {
		meta.IsVariable = true
		meta.EditableStartCol = eqIdx + 1
		meta.IsUserDefined = true
	} else {
		meta.IsVariable = false
		meta.EditableStartCol = 0
	}
}

func (m *Model) splitLine(row, col int) {
	m.diffCache = make(map[int][]bool)
	// To perform a split, take the current line and keep the content before
	// the cursor, take the content after the cursor and make it the content of
	// the line underneath, and shift the remaining lines down by one
	head, tailSrc := m.value[row][:col], m.value[row][col:]
	tail := make([]rune, len(tailSrc))
	copy(tail, tailSrc)

	m.value = append(m.value[:row+1], m.value[row:]...)

	m.value[row] = head
	m.value[row+1] = tail

	// Duplicate meta if it exists
	if row < len(m.lineMeta) {
		oldMeta := m.lineMeta[row]
		newMeta := oldMeta
		if oldMeta.IsUserDefined {
			// New user-defined line starts fresh for a new key
			newMeta.EditableStartCol = 0
			newMeta.IsVariable = true
			newMeta.DefaultValue = ""
		}
		m.lineMeta = append(m.lineMeta[:row+1], append([]Line{newMeta}, m.lineMeta[row+1:]...)...)
	}

	m.col = 0
	m.row++
}

// Paste is a command for pasting from the clipboard into the text input.
func Paste() tea.Msg {
	str, err := clipboard.ReadAll()
	if err != nil {
		return pasteErrMsg{err}
	}
	return pasteMsg(str)
}

func wrap(runes []rune, width int) [][]rune {
	var (
		lines  = [][]rune{{}}
		word   = []rune{}
		row    int
		spaces int
	)

	// Word wrap the runes

	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			word = append(word, r)
		}

		if spaces > 0 { //nolint:nestif
			if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces > width {
				row++
				lines = append(lines, []rune{})
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], repeatSpaces(spaces)...)
				spaces = 0
				word = nil
			} else {
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], repeatSpaces(spaces)...)
				spaces = 0
				word = nil
			}
		} else {
			// If the last character is a double-width rune, then we may not be able to add it to this line
			// as it might cause us to go past the width.
			lastCharLen := rw.RuneWidth(word[len(word)-1])
			if uniseg.StringWidth(string(word))+lastCharLen > width {
				// If the current line has any content, let's move to the next
				// line because the current word fills up the entire line.
				if len(lines[row]) > 0 {
					row++
					lines = append(lines, []rune{})
				}
				lines[row] = append(lines[row], word...)
				word = nil
			}
		}
	}

	if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces >= width {
		lines = append(lines, []rune{})
		lines[row+1] = append(lines[row+1], word...)
		// We add an extra space at the end of the line to account for the
		// trailing space at the end of the previous soft-wrapped lines so that
		// behaviour when navigating is consistent and so that we don't need to
		// continually add edges to handle the last line of the wrapped input.
		spaces++
		lines[row+1] = append(lines[row+1], repeatSpaces(spaces)...)
	} else {
		lines[row] = append(lines[row], word...)
		spaces++
		lines[row] = append(lines[row], repeatSpaces(spaces)...)
	}

	return lines
}

func repeatSpaces(n int) []rune {
	return []rune(strings.Repeat(string(' '), n))
}

// numDigits returns the number of digits in an integer.
func numDigits(n int) int {
	if n == 0 {
		return 1
	}
	count := 0
	num := abs(n)
	for num > 0 {
		count++
		num /= 10
	}
	return count
}

func clamp(v, low, high int) int {
	if high < low {
		low, high = high, low
	}
	return min(high, max(low, v))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
