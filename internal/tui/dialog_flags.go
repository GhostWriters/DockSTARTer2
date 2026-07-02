package tui

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"sort"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// FlagsToggleDialog is the dialog for toggling global application flags.
// Built as an outer container displayengine.MenuModel (title, buttons) with the checkbox
// list as a single submenu-mode content section, matching the pattern used
// by Main Menu/Config Menu/Options Menu/Config Apps Menu.
type FlagsToggleDialog struct {
	outer *displayengine.MenuModel
	list  *displayengine.MenuModel
}

type TriggerApplyFlagsMsg struct{}

// NewFlagsToggleDialog creates a new FlagsToggleDialog
func NewFlagsToggleDialog() *FlagsToggleDialog {
	items := []displayengine.MenuItem{
		{
			Tag:        "DEBUG",
			Desc:       "Debug logging",
			Help:       "Shows internal debugging information",
			IsCheckbox: true,
			Selectable: true,
			Checked:    console.Debug(),
			Selected:   console.Debug(),
		},
		{
			Tag:        "FORCE",
			Desc:       "Force operations",
			Help:       "Forces operations that might otherwise be skipped",
			IsCheckbox: true,
			Selectable: true,
			Checked:    console.Force(),
			Selected:   console.Force(),
		},
		{
			Tag:        "VERBOSE",
			Desc:       "Verbose logging",
			Help:       "Shows more detailed progress information",
			IsCheckbox: true,
			Selectable: true,
			Checked:    console.Verbose(),
			Selected:   console.Verbose(),
		},
		{
			Tag:        "YES",
			Desc:       "Answer yes",
			Help:       "Automatically answers 'yes' to confirmation prompts",
			IsCheckbox: true,
			Selectable: true,
			Checked:    console.AssumeYes(),
			Selected:   console.AssumeYes(),
		},
	}

	// Sort items by Tag to ensure alphabetical order
	sort.Slice(items, func(i, j int) bool {
		return items[i].Tag < items[j].Tag
	})

	// displayengine.IDListPanel (not e.g. "global_flags") deliberately avoids being a
	// substring of the outer's own id ("global_flags_outer") -- MatchesID
	// uses strings.Contains, so a section id that's a prefix of the outer's
	// id would incorrectly claim hits meant for the outer (e.g. its Done/
	// Cancel button clicks), which never reach the outer's own button-click
	// switch as a result. Matches the id convention every other migrated
	// section-owning screen already uses for its inner list.
	list := displayengine.NewMenuModel(displayengine.IDListPanel, "", "", items)
	list.SetCheckboxMode(true) // Use the standard checkbox mode like app_selection.go
	list.SetSubMenuMode(true)
	list.SetVariableHeight(false)
	list.SetIsDialog(false)
	list.SetButtons([]displayengine.ButtonDef{})
	list.SetMaximized(true)
	// viewWithSections already wraps every content section in its own
	// ContentSideMargin padding; suppress the section's own internal left
	// margin to avoid doubling up (matches the convention used by every
	// other migrated section-owning screen).
	list.SetNoLeftMargin(true)

	outer := displayengine.NewMenuModel("global_flags_outer", "Application Flags", "", nil)
	outer.SetMaximized(false) // Ensure it is NOT maximized -- grow to fit, matching original behavior
	outer.SetIsDialog(true)   // Mark this menu as a modal dialog so it elevates ZOrder
	outer.SetDialogType(displayengine.DialogTypeConfirm)
	outer.SetShowButtons(true)
	outer.SetButtons([]displayengine.ButtonDef{
		{Label: "Done", ZoneID: "btn-select", Action: func() tea.Msg { return TriggerApplyFlagsMsg{} }, Help: "Apply flag changes and close."},
		{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Close without applying."},
	})
	outer.SetEscAction(CloseDialog())
	outer.AddContentSection(displayengine.NewPlainTextSection("global_flags_subtitle", "Toggle runtime flags"))
	outer.AddContentSection(list)

	return &FlagsToggleDialog{outer: outer, list: list}
}

// Init implements tea.Model
func (d *FlagsToggleDialog) Init() tea.Cmd {
	return d.outer.Init()
}

// Update implements tea.Model
func (d *FlagsToggleDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case TriggerApplyFlagsMsg:
		for _, it := range d.list.GetItems() {
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
			func() tea.Msg { return displayengine.RefreshHeaderMsg{} },
			CloseDialog(),
		)
	}

	var cmd tea.Cmd
	var newOuter tea.Model
	newOuter, cmd = d.outer.Update(msg)
	if outer, ok := newOuter.(*displayengine.MenuModel); ok {
		d.outer = outer
	}
	// Sync d.list from outer's content sections so GetItems() in
	// TriggerApplyFlagsMsg above reflects the latest checkbox state.
	secs := d.outer.GetContentSections()
	if len(secs) >= 2 {
		if mm, ok := secs[1].(*displayengine.MenuModel); ok {
			d.list = mm
		}
	}

	return d, cmd
}

// View implements tea.Model
func (d *FlagsToggleDialog) View() tea.View {
	return d.outer.View()
}

// ViewString implements ViewStringer for overlay compositing
func (d *FlagsToggleDialog) ViewString() string {
	return d.outer.ViewString()
}

// SetSize implements sizing
func (d *FlagsToggleDialog) SetSize(width, height int) {
	if width > 60 {
		width = 60
	}
	d.outer.SetSize(width, height)
}

// IsMaximized lets the AppModel know its size state
func (d *FlagsToggleDialog) IsMaximized() bool {
	return d.outer.IsMaximized()
}

// SetFocused propagates focus state
func (d *FlagsToggleDialog) SetFocused(f bool) {
	d.outer.SetFocused(f)
}

// Layers implements LayeredView for compositing
func (d *FlagsToggleDialog) Layers() []*lipgloss.Layer {
	return d.outer.Layers()
}

// GetHitRegions implements displayengine.HitRegionProvider for mouse hit testing
func (d *FlagsToggleDialog) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	return d.outer.GetHitRegions(offsetX, offsetY)
}

// IsScrollbarDragging contributes to the sbDragger interface for mouse motion forwarding
func (d *FlagsToggleDialog) IsScrollbarDragging() bool {
	return d.outer.IsScrollbarDragging()
}

// HelpText returns help info
func (d *FlagsToggleDialog) HelpText() string {
	return d.outer.HelpText()
}
