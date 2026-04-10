package tui

import (
	"DockSTARTer2/internal/tui/components/sinput"
	"strings"

	"github.com/atotto/clipboard"

	tea "charm.land/bubbletea/v2"
)

// ShowInputContextMenu builds a right-click context menu for a sinput field and
// returns a Cmd that opens it. Call from a dialog's LayerHitMsg handler when
// msg.Button == tea.MouseRight on the input hit region.
func ShowInputContextMenu(input sinput.Model, x, y, screenW, screenH int) tea.Cmd {
	items := buildInputContextMenuItems(input)
	return func() tea.Msg {
		return ShowDialogMsg{Dialog: NewContextMenuModel(x, y, screenW, screenH, items)}
	}
}

func buildInputContextMenuItems(input sinput.Model) []ContextMenuItem {
	sel := input.SelectedText()
	hasValue := input.Value() != ""
	hasClip := false
	if clip, err := clipboard.ReadAll(); err == nil && clip != "" {
		hasClip = true
	}

	// Copy label reflects whether there is a selection.
	copyLabel := "Copy"
	copyHelp := "Copy all text to clipboard."
	if sel != "" {
		copyLabel = "Copy Selection"
		copyHelp = "Copy selected text to clipboard."
	}

	header := strings.TrimSpace(input.Prompt)
	if header == "" || header == ">" || header == ":" {
		header = strings.TrimSpace(input.Placeholder)
	}
	if header == "" {
		header = "Input"
	}

	items := []ContextMenuItem{
		{IsHeader: true, Label: header},
		{IsSeparator: true},
	}

	clipItems := []ContextMenuItem{
		{
			Label:    copyLabel,
			Help:     copyHelp,
			Disabled: !hasValue,
			Action: func() tea.Msg {
				text := sel
				if text == "" {
					text = input.Value()
				}
				_ = clipboard.WriteAll(text)
				return CloseDialogMsg{}
			},
		},
		{
			Label:    "Cut",
			Help:     "Copy text to clipboard and delete it.",
			Disabled: !hasValue,
			Action: func() tea.Msg {
				return CloseDialogMsg{Result: sinput.CutMsg{}, ForwardToParent: true}
			},
		},
		{IsSeparator: true},
		{
			Label:    "Paste",
			Help:     "Insert clipboard text at the cursor.",
			Disabled: !hasClip,
			Action: func() tea.Msg {
				text, _ := clipboard.ReadAll()
				return CloseDialogMsg{Result: sinput.PasteMsg{Text: text}, ForwardToParent: true}
			},
		},
		{IsSeparator: true},
		{
			Label:    "Select All",
			Help:     "Select all text in the field.",
			Disabled: !hasValue,
			Action: func() tea.Msg {
				return CloseDialogMsg{Result: sinput.SelectAllMsg{}, ForwardToParent: true}
			},
		},
	}

	return AppendContextMenuTail(items, clipItems, nil)
}
