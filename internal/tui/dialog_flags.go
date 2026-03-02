package tui

import (
	"DockSTARTer2/internal/console"
	"sort"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// FlagsToggleDialog is the dialog for toggling global application flags
type FlagsToggleDialog struct {
	menu *MenuModel
}

type TriggerApplyFlagsMsg struct{}

// NewFlagsToggleDialog creates a new FlagsToggleDialog
func NewFlagsToggleDialog() *FlagsToggleDialog {
	items := []MenuItem{
		{
			Tag:        "DEBUG",
			Desc:       "Debug logging",
			Help:       "Shows internal debugging information",
			Selectable: true,
			Selected:   console.Debug(),
		},
		{
			Tag:        "FORCE",
			Desc:       "Force operations",
			Help:       "Forces operations that might otherwise be skipped",
			Selectable: true,
			Selected:   console.Force(),
		},
		{
			Tag:        "VERBOSE",
			Desc:       "Verbose logging",
			Help:       "Shows more detailed progress information",
			Selectable: true,
			Selected:   console.Verbose(),
		},
		{
			Tag:        "YES",
			Desc:       "Answer yes",
			Help:       "Automatically answers 'yes' to confirmation prompts",
			Selectable: true,
			Selected:   console.AssumeYes(),
		},
	}

	// Sort items by Tag to ensure alphabetical order
	sort.Slice(items, func(i, j int) bool {
		return items[i].Tag < items[j].Tag
	})

	menu := NewMenuModel("global_flags", "Application Flags", "Toggle runtime flags", items, CloseDialog())
	menu.SetCheckboxMode(true) // Use the standard checkbox mode like app_selection.go
	menu.SetMaximized(false)   // Ensure it is NOT maximized
	menu.SetIsDialog(true)     // Mark this menu as a modal dialog so it elevates ZOrder
	menu.SetButtonLabels("Done", "", "")
	menu.SetShowExit(false)

	menu.SetEnterAction(func() tea.Msg { return TriggerApplyFlagsMsg{} })
	menu.SetEscAction(CloseDialog())

	return &FlagsToggleDialog{menu: &menu}
}

// Init implements tea.Model
func (d *FlagsToggleDialog) Init() tea.Cmd {
	return d.menu.Init()
}

// Update implements tea.Model
func (d *FlagsToggleDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case TriggerApplyFlagsMsg:
		for _, it := range d.menu.GetItems() {
			switch it.Tag {
			case "VERBOSE":
				console.SetVerbose(it.Selected)
			case "DEBUG":
				console.SetDebug(it.Selected)
			case "FORCE":
				console.SetForce(it.Selected)
			case "YES":
				console.SetAssumeYes(it.Selected)
			}
		}
		// Refresh header and close dialog
		return d, tea.Batch(
			func() tea.Msg { return RefreshHeaderMsg{} },
			CloseDialog(),
		)
	}

	var cmd tea.Cmd
	var newMenu tea.Model
	newMenu, cmd = d.menu.Update(msg)
	if menu, ok := newMenu.(*MenuModel); ok {
		d.menu = menu
	}

	return d, cmd
}

// View implements tea.Model
func (d *FlagsToggleDialog) View() tea.View {
	return d.menu.View()
}

// ViewString implements ViewStringer for overlay compositing
func (d *FlagsToggleDialog) ViewString() string {
	return d.menu.ViewString()
}

// SetSize implements sizing
func (d *FlagsToggleDialog) SetSize(width, height int) {
	d.menu.SetSize(width, height)
}

// IsMaximized lets the AppModel know its size state
func (d *FlagsToggleDialog) IsMaximized() bool {
	return d.menu.IsMaximized()
}

// SetFocused propagates focus state
func (d *FlagsToggleDialog) SetFocused(f bool) {
	d.menu.SetFocused(f)
}

// Layers implements LayeredView for compositing
func (d *FlagsToggleDialog) Layers() []*lipgloss.Layer {
	return d.menu.Layers()
}

// GetHitRegions implements HitRegionProvider for mouse hit testing
func (d *FlagsToggleDialog) GetHitRegions(offsetX, offsetY int) []HitRegion {
	return d.menu.GetHitRegions(offsetX, offsetY)
}

// HelpText returns help info
func (d *FlagsToggleDialog) HelpText() string {
	return d.menu.HelpText()
}
