package classic

import tea "charm.land/bubbletea/v2"

// ContextMenuItem is a single entry in a ContextMenuModel.
type ContextMenuItem struct {
	Label       string            // Displayed text (ignored when IsSeparator is true)
	SubLabel    string            // Optional second line shown below Label (e.g. the actual value)
	Help        string            // Optional help text (shown in helpline when item is focused)
	IsSeparator bool              // When true, renders as a horizontal divider and is not selectable
	IsHeader    bool              // When true, renders Label as a non-selectable title row
	Disabled    bool              // When true, renders dimmed and is not selectable or executable
	Action      tea.Cmd           // Executed when the item is selected; should close the dialog itself
	SubItems    []ContextMenuItem // When non-empty, selecting opens a submenu instead of Action
}
