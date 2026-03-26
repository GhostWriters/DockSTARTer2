package tui

import (
	"charm.land/bubbles/v2/key"
)

// KeyMap defines all key bindings for the TUI.
// Groups:
//   - Navigation:   Up, Down (list), Left, Right (buttons)
//   - Focus:        Tab, ShiftTab (screen-level element cycling)
//   - Action:       Enter (select/confirm), Esc (back/exit)
//   - Confirm:      Yes, No
//   - Scroll:       PageUp, PageDown, HalfPageUp, HalfPageDown (viewport)
//   - Utility:      Help, ForceQuit
type KeyMap struct {
	// List navigation
	Up   key.Binding
	Down key.Binding

	// Button navigation
	Left  key.Binding
	Right key.Binding

	// Screen-level element cycling (different windows/panels, header widgets, etc.)
	Tab      key.Binding
	ShiftTab key.Binding

	// Internal screen cycling (tab/shift-tab)
	CycleTab      key.Binding
	CycleShiftTab key.Binding

	// Actions
	Enter key.Binding
	Space key.Binding
	Esc   key.Binding

	// Viewport scrolling (programbox)
	PageUp       key.Binding
	PageDown     key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	Home         key.Binding
	End          key.Binding

	// Utility
	Help      key.Binding
	ForceQuit key.Binding

	// Log panel
	ToggleLog key.Binding

	// Mouse (Mock bindings for help display)
	MouseLeft  key.Binding
	MouseRight key.Binding
	MouseWheel key.Binding

	// Environment Editor specific keys (shortcuts defined in textarea and tabbed_vars_editor)
	EnvRefresh    key.Binding
	EnvAddVar     key.Binding
	EnvDelete     key.Binding
	EnvReorderU   key.Binding
	EnvReorderD   key.Binding
	EnvInsert     key.Binding
	EnvSplitLine  key.Binding
	EnvEditValue  key.Binding
	EnvNextTab    key.Binding
	EnvPrevTab    key.Binding
}

// ShortHelp returns bindings shown in the compact helpline.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Esc, k.Help}
}

// FullHelp returns all bindings grouped into columns for better vertical scaling.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys("up"), key.WithHelp("↑/↓/scroll", "up/down")),
			key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup/pgdn", "page up/down")),
			key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u/ctrl+d", "half page up/down")),
			key.NewBinding(key.WithKeys("home"), key.WithHelp("home/end", "top/bottom")),
			key.NewBinding(key.WithKeys("left"), key.WithHelp("←/→", "previous/next button")),
			key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n/p", "next/previous element")),
			k.CycleTab,
			k.CycleShiftTab,
		},
		{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter/left click", "activate button")),
			key.NewBinding(key.WithKeys("space"), key.WithHelp("space/middle click", "toggle item")),
			k.MouseRight,
			k.Esc,
			k.ToggleLog,
			k.Help,
			k.ForceQuit,
		},
	}
}

// Keys is the default key map used throughout the TUI.
var Keys = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("↑", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "move down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left"),
		key.WithHelp("←", "prev button"),
	),
	Right: key.NewBinding(
		key.WithKeys("right"),
		key.WithHelp("→", "next button"),
	),
	Tab: key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "next screen element"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "prev screen element"),
	),
	CycleTab: key.NewBinding(
		key.WithKeys("tab", "."),
		key.WithHelp("tab/.", "next focus"),
	),
	CycleShiftTab: key.NewBinding(
		key.WithKeys("shift+tab", ","),
		key.WithHelp("shift+tab/,", "prev focus"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "activate button"),
	),
	Space: key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("space", "toggle item"),
	),
	Esc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back/exit"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "ctrl+b", "ctrl+up"),
		key.WithHelp("pgup/ctrl+up", "scroll up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "ctrl+f", "ctrl+down"),
		key.WithHelp("pgdn/ctrl+down", "scroll down"),
	),
	HalfPageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "half page up"),
	),
	HalfPageDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "half page down"),
	),
	Home: key.NewBinding(
		key.WithKeys("home", "ctrl+home"),
		key.WithHelp("home", "top"),
	),
	End: key.NewBinding(
		key.WithKeys("end", "ctrl+end"),
		key.WithHelp("end", "bottom"),
	),
	Help: key.NewBinding(
		key.WithKeys("?", "f1"),
		key.WithHelp("?/F1", "help"),
	),
	ForceQuit: key.NewBinding(
		key.WithKeys("ctrl+\\"),
		key.WithHelp("ctrl+\\", "force quit"),
	),
	ToggleLog: key.NewBinding(
		key.WithKeys("f10", "ctrl+l"),
		key.WithHelp("f10/ctrl+l", "toggle log panel"),
	),
	MouseLeft: key.NewBinding(
		key.WithHelp("left click", "select/confirm"),
	),
	MouseRight: key.NewBinding(
		key.WithHelp("right click", "context menu/help"),
	),
	MouseWheel: key.NewBinding(
		key.WithHelp("scroll wheel", "scroll list/logs"),
	),
	EnvRefresh: key.NewBinding(
		key.WithKeys("f5", "ctrl+r"),
		key.WithHelp("F5/Ctrl+R", "refresh"),
	),
	EnvAddVar: key.NewBinding(
		key.WithKeys("ctrl+a"),
		key.WithHelp("ctrl+a", "add var"),
	),
	EnvDelete: key.NewBinding(
		key.WithKeys("ctrl+d", "alt+backspace"),
		key.WithHelp("ctrl+d", "delete var"),
	),
	EnvReorderU: key.NewBinding(
		key.WithKeys("ctrl+up", "alt+up"),
		key.WithHelp("ctrl+up/alt+up", "move up"),
	),
	EnvReorderD: key.NewBinding(
		key.WithKeys("ctrl+down", "alt+down"),
		key.WithHelp("ctrl+down/alt+down", "move down"),
	),
	EnvInsert: key.NewBinding(
		key.WithKeys("ctrl+o"),
		key.WithHelp("ctrl+o", "insert row"),
	),
	EnvSplitLine: key.NewBinding(
		key.WithKeys("ctrl+j"),
		key.WithHelp("Ctrl+J", "split line at cursor"),
	),
	EnvEditValue: key.NewBinding(
		key.WithKeys("f2"),
		key.WithHelp("F2", "edit value"),
	),
	EnvNextTab: key.NewBinding(
		key.WithKeys("ctrl+right", "alt+right"),
		key.WithHelp("ctrl+→/alt+→", "next tab"),
	),
	EnvPrevTab: key.NewBinding(
		key.WithKeys("ctrl+left", "alt+left"),
		key.WithHelp("ctrl+←/alt+←", "prev tab"),
	),
}
