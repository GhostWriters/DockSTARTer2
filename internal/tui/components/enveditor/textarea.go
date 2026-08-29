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

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/tui/components/enveditor/memoization"
	"DockSTARTer2/internal/tui/components/enveditor/runeutil"
	"DockSTARTer2/internal/tui/glyphs"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/input"
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
	InputBegin              key.Binding
	InputEnd                key.Binding

	InsertLine key.Binding
	Undo       key.Binding
	Redo       key.Binding

	// Copy selection or value to clipboard
	Copy key.Binding

	// Cut copies the selection (see Copy) then deletes it (see
	// deleteSelectionDS2 -- read-only lines within the selection are kept).
	Cut key.Binding

	// Keyboard text selection (shift+arrow)
	SelectLeft         key.Binding
	SelectRight        key.Binding
	SelectHome         key.Binding
	SelectEnd          key.Binding
	SelectWordForward  key.Binding
	SelectWordBackward key.Binding
	SelectLineUp       key.Binding
	SelectLineDown     key.Binding
	SelectAll          key.Binding

	// ToggleInsert switches between insert and overwrite mode.
	ToggleInsert key.Binding
}

// DefaultKeyMap returns the default set of key bindings for navigating and acting
// upon the textarea.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		CharacterForward:   key.NewBinding(key.WithKeys("right"), key.WithHelp("right", "character forward")),
		CharacterBackward:  key.NewBinding(key.WithKeys("left"), key.WithHelp("left", "character backward")),
		LineNext:           key.NewBinding(key.WithKeys("down"), key.WithHelp("down", "next line")),
		LinePrevious:       key.NewBinding(key.WithKeys("up"), key.WithHelp("up", "previous line")),
		DeleteWordBackward: key.NewBinding(key.WithKeys("ctrl+backspace", "alt+backspace", "ctrl+alt+backspace"), key.WithHelp("alt+bksp", "delete word backward")),
		DeleteWordForward:  key.NewBinding(key.WithKeys("ctrl+delete", "alt+delete", "ctrl+alt+delete"), key.WithHelp("alt+del", "delete word forward")),
		DeleteAfterCursor:  key.NewBinding(key.WithKeys("ctrl+k", "alt+k", "ctrl+alt+k"), key.WithHelp("alt+k", "delete after cursor")),
		DeleteBeforeCursor: key.NewBinding(key.WithKeys("ctrl+u", "alt+u", "ctrl+alt+u"), key.WithHelp("alt+u", "delete before cursor")),
		InsertNewline:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "insert newline")),
		SplitLine:          key.NewBinding(key.WithKeys("ctrl+j", "alt+j", "ctrl+alt+j"), key.WithHelp("alt+j", "split line at cursor")),
		// ctrl+h: some clients send raw BS (0x08) for Backspace instead of DEL.
		DeleteCharacterBackward: key.NewBinding(key.WithKeys("backspace", "ctrl+h"), key.WithHelp("bksp", "delete character backward")),
		DeleteCharacterForward:  key.NewBinding(key.WithKeys("delete"), key.WithHelp("del", "delete character forward")),
		LineStart:               key.NewBinding(key.WithKeys("home"), key.WithHelp("home", "line start")),
		LineEnd:                 key.NewBinding(key.WithKeys("end"), key.WithHelp("end", "line end")),
		PageUp:                  key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown:                key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "page down")),
		Paste:                   key.NewBinding(key.WithKeys("ctrl+v", "alt+v", "ctrl+alt+v"), key.WithHelp("alt+v", "paste")),
		InputBegin:              key.NewBinding(key.WithKeys("ctrl+home", "alt+<", "ctrl+alt+home", "ctrl+alt+<"), key.WithHelp("alt+<", "input begin")),
		InputEnd:                key.NewBinding(key.WithKeys("ctrl+end", "alt+>", "ctrl+alt+end", "ctrl+alt+>"), key.WithHelp("alt+>", "input end")),

		InsertLine:         key.NewBinding(key.WithKeys("ctrl+o", "alt+o", "ctrl+alt+o"), key.WithHelp("alt+o", "insert line")),
		Undo:               key.NewBinding(key.WithKeys("ctrl+z", "alt+z", "ctrl+alt+z"), key.WithHelp("alt+z", "undo")),
		Redo:               key.NewBinding(key.WithKeys("ctrl+y", "alt+y", "ctrl+alt+y"), key.WithHelp("alt+y", "redo")),
		Copy:               key.NewBinding(key.WithKeys("ctrl+c", "alt+c", "ctrl+alt+c"), key.WithHelp("alt+c", "copy")),
		Cut:                key.NewBinding(key.WithKeys("ctrl+x", "alt+x", "ctrl+alt+x"), key.WithHelp("alt+x", "cut")),
		SelectLeft:         key.NewBinding(key.WithKeys("shift+left"), key.WithHelp("shift+left", "select left")),
		SelectRight:        key.NewBinding(key.WithKeys("shift+right"), key.WithHelp("shift+right", "select right")),
		SelectHome:         key.NewBinding(key.WithKeys("shift+home"), key.WithHelp("shift+home", "select to start")),
		SelectEnd:          key.NewBinding(key.WithKeys("shift+end"), key.WithHelp("shift+end", "select to end")),
		SelectWordForward:  key.NewBinding(key.WithKeys("ctrl+shift+right", "alt+shift+right", "alt+shift+f"), key.WithHelp("alt+shift+right", "select word forward")),
		SelectWordBackward: key.NewBinding(key.WithKeys("ctrl+shift+left", "alt+shift+left", "alt+shift+b"), key.WithHelp("alt+shift+left", "select word backward")),
		SelectLineUp:       key.NewBinding(key.WithKeys("shift+up"), key.WithHelp("shift+up", "select line up")),
		SelectLineDown:     key.NewBinding(key.WithKeys("shift+down"), key.WithHelp("shift+down", "select line down")),
		SelectAll:          key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("ctrl+g", "select all")),
		ToggleInsert:       key.NewBinding(key.WithKeys("insert"), key.WithHelp("insert", "toggle insert/overwrite")),
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

// CursorStyle is the style for real and virtual cursors.
type CursorStyle struct {
	// Style styles the cursor block.
	//
	// For real cursors, the foreground color set here will be used as the
	// cursor color.
	Color color.Color

	// Style is the full style (fg, bg, and attributes) used to render the
	// virtual cursor's solid / blink-visible phase. Unlike Color, this is
	// not reduced to a single foreground -- a theme's TextCursor entry may
	// also set a background or attributes (e.g. Bold), and those should
	// carry through rather than being discarded.
	Style lipgloss.Style

	// FlashStyle is the full style used for the virtual cursor's
	// blink-hidden phase. Deliberately a second theme-defined style rather
	// than derived from the character underneath: the cursor should look
	// the same regardless of what it's sitting on top of (selected text,
	// a comment, etc.), same as a native terminal cursor does.
	FlashStyle lipgloss.Style

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
	Base                      lipgloss.Style
	Text                      lipgloss.Style
	LineNumber                lipgloss.Style
	LineNumberFocused         lipgloss.Style // cursor line
	LineNumberModified        lipgloss.Style // line differs from default
	LineNumberModifiedFocused lipgloss.Style // cursor line + differs from default
	LineNumberBrackets        lipgloss.Style // focused-line bracket indicator
	CursorLine                lipgloss.Style
	EndOfBuffer               lipgloss.Style
	Placeholder               lipgloss.Style
	Prompt                    lipgloss.Style
	ModifiedText              lipgloss.Style
	ReadOnlyText              lipgloss.Style
	CommentText               lipgloss.Style
	InvalidText               lipgloss.Style
	DuplicateText             lipgloss.Style
	BuiltinText               lipgloss.Style
	PendingDeleteText         lipgloss.Style
	GutterAdded               lipgloss.Style // + marker for new lines
	GutterDeleted             lipgloss.Style // - marker for pending-delete lines
	GutterModified            lipgloss.Style // ~ marker for changed lines
	GutterInvalid             lipgloss.Style // ! marker for protected vars entered in user-defined section
	ScrollbarTrack            lipgloss.Style
	ScrollbarThumb            lipgloss.Style
	SelectionText             lipgloss.Style
}

func (s StyleState) computedCursorLine() lipgloss.Style {
	return s.CursorLine.Inherit(s.Base).Inline(true)
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

	// LineNumberBrackets, if enabled, wraps the focused line's number in a
	// pair of themed brackets (LineNumberBrackets style, same glyphs as the
	// app's other focused-row bracket indicators). LineNumberBracketOpen/
	// Close hold the actual glyphs to use, set by the caller.
	LineNumberBrackets     bool
	LineNumberBracketOpen  string
	LineNumberBracketClose string

	// EndOfBufferCharacter is displayed at the end of the input.
	EndOfBufferCharacter rune

	// KeyMap encodes the keybindings recognized by the widget.
	KeyMap KeyMap

	// virtualCursor manages the virtual cursor.
	virtualCursor cursor.Model

	// blinkAnchor is the wall-clock reference point the virtual cursor's
	// blink phase is computed from at render time (see view()): elapsed
	// time since blinkAnchor, modulo twice the blink speed, determines
	// whether the cursor is currently shown or hidden. This is level-
	// triggered -- "what is the state right now" -- rather than relying on
	// cursor.Model's own async BlinkMsg chain (id/tag-matched messages that
	// have to round-trip through this component, the screen, and back), a
	// chain that's easy to silently break by triggering a Focus()/Blink()
	// call from an unrelated code path in between. Reset to time.Now()
	// whenever the cursor should appear solid/visible right away: real
	// interaction (movement, click), gaining focus, or entering a blink-
	// eligible state.
	blinkAnchor time.Time

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
	ScrollbarFunc func(content string, total, visible, offset int, lineChars bool) string

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
	isScrollbarDragging bool
	sbDragMouseOffsetY  int // relative offset of mouse within thumb when drag started
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

	// Text selection state (multi-row; see selection.go). selecting tracks
	// an in-progress mouse drag; selAnchor/selHead/hasSelection are the
	// selection itself.
	selecting    bool
	selAnchor    Position
	selHead      Position
	hasSelection bool

	// Multi-click tracking (double/triple/quad click selection)
	lastClickTime time.Time
	lastClickRow  int
	lastClickCol  int
	clickCount    int

	// Total visual width set by SetWidth
	totalWidth int

	// Memoization for expensive rendering
	lastView    string
	cacheValid  bool // Indicates if lastView is up-to-date with current state
	dmp         *diffmatchpatch.DiffMatchPatch
	diffCache   map[int][]bool      // row index -> modified mask (true = modified)
	defaultFunc func(string) string // stored at ParseEnv/ReclassifyEnv time; resolves defaults for new vars

	// Intelligent variable addition settings.
	AddPrefix          string
	ValidationType     string // _GLOBAL_, _BARE_, or APPNAME (actual app name)
	ValidationAppName  string // Actual app name if ValidationType is APPNAME
	ValidationIsGlobal bool   // If true, the editor is showing the full .env names; if false, it shows bare names that need prefixing for validation
	ValidateFunc       func(string, string) bool

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

		viewport:  &vp,
		dmp:       diffmatchpatch.New(),
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
		Base:                      lipgloss.NewStyle(),
		CursorLine:                lipgloss.NewStyle(),
		LineNumber:                lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("249"), lipgloss.Color("7"))),
		LineNumberFocused:         lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("240"), lipgloss.Color("240"))),
		LineNumberModified:        lipgloss.NewStyle().Foreground(lipgloss.Color("3")), // Yellow
		LineNumberModifiedFocused: lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
		EndOfBuffer:               lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("254"), lipgloss.Color("0"))),
		Placeholder:               lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Prompt:                    lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		Text:                      lipgloss.NewStyle(),
		ModifiedText:              lipgloss.NewStyle().Foreground(lipgloss.Color("3")),   // Yellow
		ReadOnlyText:              lipgloss.NewStyle().Foreground(lipgloss.Color("240")), // Dark Grey
		CommentText:               lipgloss.NewStyle().Foreground(lipgloss.Color("240")), // Default to same as ReadOnly
		InvalidText:               lipgloss.NewStyle().Foreground(lipgloss.Color("9")),   // Red
		DuplicateText:             lipgloss.NewStyle().Foreground(lipgloss.Color("13")),  // Magenta
		BuiltinText:               lipgloss.NewStyle(),                                   // Inherit from text by default
		PendingDeleteText:         lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("240")),
		GutterAdded:               lipgloss.NewStyle().Foreground(lipgloss.Color("2")), // Green
		GutterDeleted:             lipgloss.NewStyle().Foreground(lipgloss.Color("1")), // Red
		GutterModified:            lipgloss.NewStyle().Foreground(lipgloss.Color("3")), // Yellow
		GutterInvalid:             lipgloss.NewStyle().Foreground(lipgloss.Color("9")), // Bright red
		ScrollbarTrack:            lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		ScrollbarThumb:            lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		SelectionText:             lipgloss.NewStyle().Reverse(true),
	}
	s.Blurred = StyleState{
		Base:                      lipgloss.NewStyle(),
		CursorLine:                lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("245"), lipgloss.Color("7"))),
		LineNumber:                lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("249"), lipgloss.Color("7"))),
		LineNumberFocused:         lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("249"), lipgloss.Color("7"))),
		LineNumberModified:        lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		LineNumberModifiedFocused: lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),
		EndOfBuffer:               lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("254"), lipgloss.Color("0"))),
		Placeholder:               lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Prompt:                    lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		Text:                      lipgloss.NewStyle().Foreground(lightDark(lipgloss.Color("245"), lipgloss.Color("7"))),
		ModifiedText:              lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		ReadOnlyText:              lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		CommentText:               lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		InvalidText:               lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		DuplicateText:             lipgloss.NewStyle().Foreground(lipgloss.Color("13")),
		BuiltinText:               lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		PendingDeleteText:         lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("240")),
		GutterAdded:               lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		GutterDeleted:             lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		GutterModified:            lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		GutterInvalid:             lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		ScrollbarTrack:            lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		ScrollbarThumb:            lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		SelectionText:             lipgloss.NewStyle().Reverse(true),
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

// VirtualCursor returns whether or not the virtual cursor is enabled.
func (m Model) VirtualCursor() bool {
	return m.useVirtualCursor
}

// SetVirtualCursor sets whether or not to use the virtual cursor.
func (m *Model) SetVirtualCursor(v bool) {
	m.useVirtualCursor = v
	m.updateVirtualCursorStyle()
}

// updateVirtualCursorStyle sets styling and blink mode on the virtual
// cursor based on the textarea's style settings and current Overwrite
// state. Resets blinkAnchor so entering a new state (e.g. leaving
// overwrite mode) shows the cursor solid/visible immediately rather than
// wherever the blink phase happened to be. No Cmd is needed here: view()
// computes the blink phase fresh from blinkAnchor on every render, so the
// very next render (which bubbletea triggers after any Update call
// regardless) already reflects the new state.
func (m *Model) updateVirtualCursorStyle() {
	if !m.useVirtualCursor {
		m.virtualCursor.SetMode(cursor.CursorHide)
		return
	}

	m.virtualCursor.Style = lipgloss.NewStyle().Foreground(m.styles.Cursor.Color)
	m.blinkAnchor = time.Now()

	// Overwrite mode gets a solid, non-blinking cursor. The virtual cursor
	// has no way to represent a bar-vs-block shape difference the way a
	// native terminal cursor can, so blink-vs-static stands in for it
	// instead: insert mode blinks (matching a bar cursor's usual behavior),
	// overwrite mode stays solid.
	if m.Overwrite {
		m.virtualCursor.SetMode(cursor.CursorStatic)
		return
	}

	// By default, the blink speed of the cursor is set to a default
	// internally.
	if m.styles.Cursor.Blink {
		if m.styles.Cursor.BlinkSpeed > 0 {
			// Aligned to RefreshRate the same way console.SpinnerSpeed is
			// (see AlignToRefreshRate) -- otherwise, even with the phase
			// itself always computed fresh at render time (see blinkAnchor's
			// doc comment), an on/off duration that isn't a whole multiple
			// of the repaint cadence rounds to whichever render boundary it
			// happens to land nearest, wobbling slightly cycle to cycle
			// instead of keeping a perfectly even rhythm.
			alignedMS := console.AlignToRefreshRate(int(m.styles.Cursor.BlinkSpeed/time.Millisecond), console.RefreshRate)
			m.virtualCursor.BlinkSpeed = time.Duration(alignedMS) * time.Millisecond
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
						// Internally prepend app prefix for validation if we're in an app-specific tab (bare names)
						vKey := key
						if !m.ValidationIsGlobal && vType != "" && vType != "_GLOBAL_" && vType != "_BARE_" {
							if strings.HasSuffix(vType, ":") {
								vKey = vType + key
							} else {
								vKey = vType + ":" + key
							}
						}
						if m.ValidateFunc != nil && !m.ValidateFunc(vKey, vType) {
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
	pasteStartRow := m.row
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

		// This splitting logic is upstream's own (no lineMeta concept
		// there) -- DS2's lineMeta array is index-aligned with m.value by
		// row and must grow in lockstep, or every row after the paste
		// point reads another line's metadata (breaks duplicate/variable/
		// read-only detection for the rest of the buffer). Insert a fresh
		// entry per new row, then classify each one from its actual pasted
		// content the same way typing a line does.
		m.syncLineMetaAfterMultilinePaste(pasteStartRow, numExtraLines)
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
	// Only reset blinkAnchor on an actual transition into focus, not a
	// redundant re-affirmation -- renderPane calls Focus() on every render
	// of the focused pane to keep its focus state in sync (see its own doc
	// comment), and resetting the anchor every render would keep the
	// cursor permanently at phase 0 (visible), never blinking.
	if !m.focus {
		m.blinkAnchor = time.Now()
	}
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
func (m *Model) transposeLeft() { //nolint:unused
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
		oldRow, oldCol := m.row, m.col
		m.characterLeft(true /* insideLine */)
		if m.row == oldRow && m.col == oldCol {
			// characterLeft is a no-op at the buffer start (or inside a
			// leading-whitespace-only prefix, where the cursor still
			// reaches (0, 0) mid-loop) -- without this check the loop
			// below never finds a non-space rune to stop at and spins
			// forever, freezing the whole Bubble Tea event loop, not just
			// this editor. See charmbracelet/bubbles#1036.
			return
		}
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
func (m *Model) uppercaseRight() { //nolint:unused
	m.invalidateDiffCache(m.row)
	m.doWordRight(func(_ int, i int) {
		m.value[m.row][i] = unicode.ToUpper(m.value[m.row][i])
	})
}

// lowercaseRight changes the word to the right to lowercase.
func (m *Model) lowercaseRight() { //nolint:unused
	m.invalidateDiffCache(m.row)
	m.doWordRight(func(_ int, i int) {
		m.value[m.row][i] = unicode.ToLower(m.value[m.row][i])
	})
}

// capitalizeRight changes the word to the right to title case.
func (m *Model) capitalizeRight() { //nolint:unused
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
		m.replaceSelectionForInsert()
		m.insertRunes([]rune(msg.Content), true)
		m.reclassifyCurrentLine()
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.KeyMap.Undo):
			m.Undo()
		case key.Matches(msg, m.KeyMap.Redo):
			m.Redo()
		case key.Matches(msg, m.KeyMap.Copy):
			if m.HasSelection() {
				cmds = append(cmds, copyToClipboard(m.SelectedText()))
			} else if m.row >= 0 && m.row < len(m.value) {
				lineStr := string(m.value[m.row])
				if eqIdx := strings.Index(lineStr, "="); eqIdx >= 0 {
					cmds = append(cmds, copyToClipboard(lineStr[eqIdx+1:]))
				} else {
					cmds = append(cmds, copyToClipboard(lineStr))
				}
			}
		case key.Matches(msg, m.KeyMap.Cut):
			if m.HasSelection() {
				cmds = append(cmds, copyToClipboard(m.SelectedText()))
				m.DeleteSelection()
			}
		case key.Matches(msg, m.KeyMap.SelectRight):
			m.startKeyboardSelection()
			if m.col < len(m.value[m.row]) {
				m.col++
			}
			m.updateKeyboardSelection()
		case key.Matches(msg, m.KeyMap.SelectLeft):
			m.startKeyboardSelection()
			if m.col > 0 {
				m.col--
			}
			m.updateKeyboardSelection()
		case key.Matches(msg, m.KeyMap.SelectWordForward):
			m.startKeyboardSelection()
			m.wordRight()
			m.updateKeyboardSelection()
		case key.Matches(msg, m.KeyMap.SelectWordBackward):
			m.startKeyboardSelection()
			m.wordLeft()
			m.updateKeyboardSelection()
		case key.Matches(msg, m.KeyMap.SelectLineUp):
			m.startKeyboardSelection()
			m.CursorUp()
			m.updateKeyboardSelection()
		case key.Matches(msg, m.KeyMap.SelectLineDown):
			m.startKeyboardSelection()
			m.CursorDown()
			m.updateKeyboardSelection()
		case key.Matches(msg, m.KeyMap.SelectEnd):
			m.startKeyboardSelection()
			m.col = len(m.value[m.row])
			m.updateKeyboardSelection()
		case key.Matches(msg, m.KeyMap.SelectHome):
			m.startKeyboardSelection()
			editStart := 0
			if m.row < len(m.lineMeta) {
				editStart = m.lineMeta[m.row].EditableStartCol
			}
			m.col = editStart
			m.updateKeyboardSelection()
		case key.Matches(msg, m.KeyMap.SelectAll):
			m.SelectAll()
		case key.Matches(msg, m.KeyMap.DeleteAfterCursor):
			if !m.isEditableAtCursor() {
				break
			}
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			m.pushUndoSnapshot()
			if m.HasSelection() {
				m.deleteSelectionDS2()
				break
			}
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
			if m.HasSelection() {
				m.deleteSelectionDS2()
				break
			}
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
			if m.HasSelection() {
				m.deleteSelectionDS2()
				break
			}
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
			if m.HasSelection() {
				m.pushUndoSnapshot()
				m.deleteSelectionDS2()
			} else if len(m.value[m.row]) > 0 && m.col < len(m.value[m.row]) {
				m.pushUndoSnapshot()
				m.invalidateDiffCache(m.row)
				m.value[m.row] = slices.Delete(m.value[m.row], m.col, m.col+1)
				m.reclassifyCurrentLine()
			}
			// At end of line: do nothing — joining lines via Del is too easy to do accidentally.
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
			if m.col >= len(m.value[m.row]) {
				// At end of line: do nothing — joining lines is not supported yet.
				break
			}
			m.pushUndoSnapshot()
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
			m.ClearSelection()
			m.CursorEnd()
		case key.Matches(msg, m.KeyMap.LineStart):
			m.ClearSelection()
			m.CursorStart()
		case key.Matches(msg, m.KeyMap.CharacterForward):
			m.ClearSelection()
			m.characterRight()
		case key.Matches(msg, m.KeyMap.LineNext):
			m.ClearSelection()
			m.CursorDown()
		case key.Matches(msg, m.KeyMap.Paste):
			return m, Paste
		case key.Matches(msg, m.KeyMap.CharacterBackward):
			m.ClearSelection()
			m.characterLeft(false /* insideLine */)
		case key.Matches(msg, m.KeyMap.LinePrevious):
			m.ClearSelection()
			m.CursorUp()
		case key.Matches(msg, m.KeyMap.InputBegin):
			m.ClearSelection()
			m.MoveToBegin()
		case key.Matches(msg, m.KeyMap.InputEnd):
			m.ClearSelection()
			m.MoveToEnd()
		case key.Matches(msg, m.KeyMap.PageUp):
			m.ClearSelection()
			m.PageUp()
		case key.Matches(msg, m.KeyMap.PageDown):
			m.ClearSelection()
			m.PageDown()
		case key.Matches(msg, m.KeyMap.ToggleInsert):
			m.Overwrite = !m.Overwrite
			m.updateVirtualCursorStyle()

		default:
			if !m.isEditableAtCursor() {
				break
			}
			m.pushUndoSnapshot()
			if m.HasSelection() {
				// Typing over a selection replaces it -- takes priority over
				// Overwrite mode, which only applies to a bare cursor.
				m.replaceSelectionForInsert()
			} else if m.Overwrite && msg.Text != "" && m.col < len(m.value[m.row]) {
				// In overwrite mode, replace the character at cursor before inserting.
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
						} else if def == "''" && m.row < len(m.lineMeta) && m.lineMeta[m.row].IsUserDefined {
							// User-defined var with no known default — insert '' and place
							// cursor between the quotes so the user can type immediately.
							m.insertRunesFromUserInput([]rune("''"))
							m.col--
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
		m.replaceSelectionForInsert()
		m.insertRunes([]rune(msg), true)

	case pasteErrMsg:
		// The exec-based read (atotto/clipboard, needs local X11/Wayland)
		// failed -- most commonly because this is an SSH session to a
		// headless host with no display at all. Fall back to asking the
		// local terminal itself via OSC52; see copyToClipboard's doc
		// comment for why this covers that case. The reply comes back
		// asynchronously as input.ClipboardEvent, handled below.
		m.Err = msg
		cmds = append(cmds, tea.Raw(ansi.RequestSystemClipboard))

	case input.ClipboardEvent:
		if !m.isEditableAtCursor() || msg.Content == "" {
			break
		}
		m.pushUndoSnapshot()
		m.replaceSelectionForInsert()
		m.insertRunes([]rune(msg.Content), true)
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
		// The blink phase itself is computed fresh from blinkAnchor at
		// render time (see view()), not driven by cursor.Model's own
		// BlinkMsg chain or a private ticker of our own -- the app's
		// existing global tick (see model.go's globalTickMsg) already
		// forces a repaint at RefreshRate regardless, which is what
		// actually makes the phase change visible while idle. So all
		// that's needed here is resetting the anchor on real interaction
		// (the cursor moved, or the message is a click that might not move
		// it, e.g. clicking the same character), so movement/clicking
		// always shows the cursor solid immediately instead of wherever
		// the blink phase happened to be.
		newRow, newCol := m.cursorLineNumber(), m.col
		_, isClick := msg.(tea.MouseClickMsg)
		if newRow != oldRow || newCol != oldCol || isClick {
			m.blinkAnchor = time.Now()
		}
	}

	// If the scrollbar moved the viewport, constrain the cursor so it remains on screen.
	// Otherwise, reposition the viewport to follow the cursor (standard typing behavior).
	if m.sbScrolled {
		m.constrainCursorToView()
	} else {
		m.repositionView()
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) view() string {
	// Compute the virtual cursor's blink phase fresh from blinkAnchor --
	// what is the state right now -- rather than trusting cursor.Model's
	// own IsBlinked field, which only updates via its BlinkMsg chain (see
	// blinkAnchor's doc comment for why that chain is fragile here). Only
	// when actually focused: an unfocused editor (e.g. the inactive pane
	// in split mode, or this one after focus moved to the button row)
	// still renders its own cursor position marker below regardless of
	// focus, so IsBlinked must be forced to the permanently-hidden state
	// here -- matching cursor.Model.Blur()'s own intent -- or every open
	// pane would show a cursor at once.
	if m.useVirtualCursor {
		if m.focus && m.virtualCursor.Mode() == cursor.CursorBlink {
			phase := time.Since(m.blinkAnchor) / m.virtualCursor.BlinkSpeed
			m.virtualCursor.IsBlinked = phase%2 == 1
		} else if !m.focus {
			m.virtualCursor.IsBlinked = true
		}
	}

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
			var prompt string
			if wl == 0 {
				prompt = m.promptView(displayLine, l)
				prompt = styles.computedPrompt().Render(prompt)
			} else {
				prompt = m.promptContinuationView(l)
			}
			s.WriteString(style.Render(prompt))
			displayLine++

			var ln string
			if m.ShowLineNumbers {
				if wl == 0 { // normal line
					isCursorLine := m.row == l
					s.WriteString(m.lineNumberView(l+1, isCursorLine, l))
				} else { // soft wrapped line
					isCursorLine := m.row == l
					s.WriteString(m.lineNumberView(-1, isCursorLine, l))
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
				hasChar := lineInfo.ColumnOffset < len(wrappedLine)
				cursorChar := " "
				if hasChar {
					cursorChar = string(wrappedLine[lineInfo.ColumnOffset])
				}
				switch {
				case !m.useVirtualCursor:
					// Hardware cursor is active -- render exactly like any
					// other character (selection, validation, etc. all
					// still apply via renderRunes) since the terminal's own
					// cursor is what marks this position, not this text.
					s.WriteString(m.renderRunes([]rune(cursorChar), l, charIndex+lineInfo.ColumnOffset, style))
				case !m.isEditableAtCursor():
					// The virtual cursor can only really fake a bar (blink)
					// or block (reverse) shape -- neither reads as "you
					// can't type here" the way a native terminal's
					// underline cursor shape does (see GetInputCursor's
					// CursorUnderline case). Underlining the cursor's own
					// solid style borrows the same theme accent the other
					// virtual-cursor states use, without implying overwrite
					// mode.
					s.WriteString(m.styles.Cursor.Style.Underline(true).Render(cursorChar))
				case m.virtualCursor.Mode() == cursor.CursorStatic:
					// Solid (non-blinking, e.g. overwrite mode): the
					// TextCursorFlash style reads as a more solid block
					// than plain TextCursor, which is what "solid" should
					// look like.
					s.WriteString(m.styles.Cursor.FlashStyle.Render(cursorChar))
				case m.virtualCursor.Mode() == cursor.CursorBlink && m.virtualCursor.IsBlinked:
					// Blink-hidden phase: the theme's TextCursorFlash style
					// (defaults to TextCursor with Reverse toggled on -- see
					// .FALLBACKS.ds2theme) rather than plain text, so the
					// cursor position stays visible through both blink
					// phases instead of disappearing for half of each
					// cycle.
					s.WriteString(m.styles.Cursor.FlashStyle.Render(cursorChar))
				default:
					// Blink-visible phase: the theme's own TextCursor
					// style, full stop -- not derived from whatever
					// character/style happens to be underneath (selection,
					// comment, etc.), same as a native terminal's cursor
					// always looks the same regardless of what it's
					// sitting on top of.
					s.WriteString(m.styles.Cursor.Style.Render(cursorChar))
				}
				if hasChar {
					s.WriteString(m.renderRunes(wrappedLine[lineInfo.ColumnOffset+1:], l, charIndex+lineInfo.ColumnOffset+1, style))
				}
			} else {
				s.WriteString(m.renderRunes(wrappedLine, l, charIndex, style))
			}
			s.WriteString(style.Render(strutil.Repeat(" ", max(0, padding))))
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
		rightGap := strutil.Repeat(" ", max(0, rightGapWidth))
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
		view = m.ScrollbarFunc(view, total, visible, offset, m.LineCharacters)
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

func (m Model) promptView(displayLine, dataLine int) (prompt string) {
	styles := m.activeStyle()

	// Show diff markers in the gutter for the first row of each logical line.
	if dataLine >= 0 && dataLine < len(m.lineMeta) {
		gutterStyle, hasMarker := m.gutterStyleFor(dataLine)
		if hasMarker {
			meta := m.lineMeta[dataLine]
			var char string
			switch {
			case meta.PendingDelete:
				char = "-"
			case meta.IsInvalid:
				char = glyphs.InvalidMarker
			case meta.IsNewLine || meta.InitialLine == "":
				char = "+"
			case string(m.value[dataLine]) != meta.InitialLine:
				char = "~"
			default:
				// Content matches InitialLine exactly -- the only other
				// reason gutterStyleFor flagged this line is isDirectlyMoved.
				char = "M"
			}
			return gutterStyle.Render(char)
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
		lineNumberStyle = m.activeStyle().computedLineNumberFocused()
	}

	// Tint line numbers whose value differs from the template default.
	// User-defined lines with no known default are entirely new — always tint.
	// Applied to both the numbered first segment and blank continuation segments
	// of soft-wrapped lines (dataLine >= 0 for both cases).
	if dataLine >= 0 && dataLine < len(m.lineMeta) {
		if dl := m.lineMeta[dataLine]; dl.IsUserDefined && dl.IsVariable && dl.DefaultValue == "" {
			if isCursorLine {
				lineNumberStyle = m.activeStyle().computedLineNumberModifiedFocused()
			} else {
				lineNumberStyle = m.activeStyle().computedLineNumberModified()
			}
		}
	}
	if dataLine >= 0 && dataLine < len(m.lineMeta) {
		meta := &m.lineMeta[dataLine]
		isModified := false

		// Check if the entire line differs from the one initially loaded from file,
		// or if it was directly reordered via MoveVariableUp/Down and hasn't
		// returned to its original neighbor. This captures key changes, which
		// getDiffMask (focused on values) skips.
		if meta.InitialLine != "" && (string(m.value[dataLine]) != meta.InitialLine || m.isDirectlyMoved(dataLine)) {
			isModified = true
		}

		// Supplement with value-part diff against template default.
		if !isModified {
			mask := m.getDiffMask(dataLine)
			for _, changed := range mask {
				if changed {
					isModified = true
					break
				}
			}
		}

		if isModified {
			if isCursorLine {
				lineNumberStyle = m.activeStyle().computedLineNumberModifiedFocused()
			} else {
				lineNumberStyle = m.activeStyle().computedLineNumberModified()
			}
		}
	}

	// Format line number dynamically based on the maximum number of lines.
	// Minimum of 3 digits for consistent alignment as per user request.
	digits := max(3, numDigits(m.MaxHeight))

	// Apply line number style ONLY to the digits themselves.
	// The outer spacing is rendered natively so it inherits the dialogue
	// base background color rather than the line number background.
	formattedNum := fmt.Sprintf("%*v", digits, str)

	// A single-cell slot is always reserved on each side of the digits for
	// the bracket indicator, blank when not the cursor line (or when the
	// option is off) so the digits never shift -- same convention as the
	// app's other focused-row bracket indicators. n > 0 excludes a
	// soft-wrapped continuation row (blank line number): isCursorLine alone
	// stays true for every wrapped segment of the cursor's line, which would
	// otherwise bracket the blank slot on those rows too.
	openChar := " "
	closeChar := " "
	if isCursorLine && m.LineNumberBrackets && n > 0 {
		bracketStyle := m.activeStyle().computedLineNumberBrackets()
		openChar = bracketStyle.Render(m.LineNumberBracketOpen)
		closeChar = bracketStyle.Render(m.LineNumberBracketClose)
	}

	return openChar + lineNumberStyle.Render(formattedNum) + closeChar
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
			gap := strutil.Repeat(" ", max(0, m.width-lipgloss.Width(plines[0])))
			s.WriteString(lineStyle.Render(gap))
		// remaining lines
		case len(plines) > i:
			// current line placeholder text
			if len(plines) > i {
				placeholderLine := plines[i]
				gap := strutil.Repeat(" ", max(0, m.width-uniseg.StringWidth(plines[i])))
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
// position per the textarea's position in the terminal.
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

// copyToClipboard writes text to the clipboard via both the local exec-based
// mechanism (atotto/clipboard -- works when the session has real X11/Wayland
// access) and OSC52 (works over SSH regardless of whether the remote host
// has a display at all, since it asks the local terminal itself to set its
// own clipboard -- same tea.Raw-through-the-input-loop mechanism as the DA1
// graphics-capability query in internal/graphics/query.go). A terminal that
// doesn't understand OSC52 just ignores the sequence, so sending both is
// harmless -- there's no reliable way to know in advance which one (if
// either) will actually work.
func copyToClipboard(text string) tea.Cmd {
	_ = clipboard.WriteAll(text)
	return tea.Raw(ansi.SetSystemClipboard(text))
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
			// as it might cause us to go past the width. Must include lines[row]'s
			// own existing width here too (as the spaces-pending branch above
			// does) -- otherwise a long unspaced token (e.g. a .env token/key
			// with no internal spaces) can keep appending onto a row that
			// already has content, silently overflowing width without ever
			// triggering a wrap, which desyncs wrap()'s row boundaries from
			// what's actually rendered and breaks click/cursor column mapping.
			lastCharLen := rw.RuneWidth(word[len(word)-1])
			if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+lastCharLen > width {
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
	return []rune(strutil.Repeat(string(' '), n))
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
