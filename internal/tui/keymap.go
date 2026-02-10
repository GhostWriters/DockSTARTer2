package tui

import (
	"github.com/charmbracelet/bubbles/key"
)

// KeyMap defines all key bindings for the TUI.
// Groups:
//   - Navigation:   Up, Down (list), Left, Right (buttons)
//   - Focus:        Tab, ShiftTab (screen-level element cycling)
//   - Dialog chrome: ChromeFocus (cycles focus to/from title-bar icons like ?, X)
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

	// Dialog chrome focus — cycles between title-bar icons (?, X, etc.) and main content.
	// Title-bar icons are discoverable alternatives to keyboard shortcuts:
	//   ? icon → same as pressing ?/F1 (help)
	//   X icon → same as pressing Esc (close)
	ChromeFocus key.Binding

	// Actions
	Enter key.Binding
	Esc   key.Binding

	// Viewport scrolling (programbox)
	PageUp       key.Binding
	PageDown     key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding

	// Utility
	Help      key.Binding
	ForceQuit key.Binding
}

// ShortHelp returns bindings shown in the compact helpline.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Esc, k.Help}
}

// FullHelp returns all bindings grouped into two columns for better vertical scaling.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.Tab, k.ShiftTab, k.ChromeFocus},
		{k.Enter, k.Esc, k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown, k.Help, k.ForceQuit},
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
		key.WithKeys("tab"),
		key.WithHelp("tab", "next screen element"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev screen element"),
	),
	ChromeFocus: key.NewBinding(
		key.WithKeys("ctrl+ "),
		key.WithHelp("ctrl+space", "focus title-bar icons"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select/confirm"),
	),
	Esc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back/exit"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "scroll up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdown", "scroll down"),
	),
	HalfPageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "half page up"),
	),
	HalfPageDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "half page down"),
	),
	Help: key.NewBinding(
		key.WithKeys("?", "ctrl+h", "f1"),
		key.WithHelp("?/F1", "help"),
	),
	ForceQuit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "force quit"),
	),
}
