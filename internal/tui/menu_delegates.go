package tui

import (
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// menuItemDelegate implements list.ItemDelegate for standard navigation menus.
// Rendering is handled by renderVariableHeightList / renderFlowContent; these
// stubs satisfy the interface so the bubbles list can track cursor and scroll state.
type menuItemDelegate struct {
	menuID              string
	maxTagLen           int
	focused             bool
	flowMode            bool
	showLockGutter      bool
	activityGutterWidth int
	paddingWidth        int
}

func (d menuItemDelegate) Height() int                             { return 1 }
func (d menuItemDelegate) Spacing() int                            { return 0 }
func (d menuItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d menuItemDelegate) Render(_ io.Writer, _ list.Model, _ int, _ list.Item) {}

// checkboxItemDelegate implements list.ItemDelegate for single-column checkbox menus.
// Rendering is handled by renderVariableHeightList / renderFlowContent.
type checkboxItemDelegate struct {
	menuID              string
	maxTagLen           int
	focused             bool
	flowMode            bool
	showLockGutter      bool
	activityGutterWidth int
	paddingWidth        int
}

func (d checkboxItemDelegate) Height() int                             { return 1 }
func (d checkboxItemDelegate) Spacing() int                            { return 0 }
func (d checkboxItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d checkboxItemDelegate) Render(_ io.Writer, _ list.Model, _ int, _ list.Item) {}

// groupedItemDelegate implements list.ItemDelegate for the hierarchical app-selection list.
// Rendering is handled by renderVariableHeightList.
type groupedItemDelegate struct {
	maxTagLen           int
	focused             bool
	activeCol           CheckboxColumn
	showLockGutter      bool
	activityGutterWidth int
}

func (d groupedItemDelegate) Height() int                             { return 1 }
func (d groupedItemDelegate) Spacing() int                            { return 0 }
func (d groupedItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d groupedItemDelegate) Render(_ io.Writer, _ list.Model, _ int, _ list.Item) {}
