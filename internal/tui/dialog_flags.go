package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
)

// FlagsToggleDialog is the dialog for toggling global application flags.
// Built as an outer container displayengine.MenuModel (title, buttons) with
// submenu-mode content sections, matching the pattern used by Main Menu/Config
// Menu/Options Menu/Config Apps Menu. The on/off flags are checkboxes in one
// section; VERBOSE, whose level is picked from a dropdown and is changed most
// often, has its own section above them, its tag and value lined up with theirs.
type FlagsToggleDialog struct {
	outer     *displayengine.MenuModel
	list      *displayengine.MenuModel // the checkbox flags
	levels    *displayengine.MenuModel // the VERBOSE row
	verbosity int                      // the VERBOSE level picked, applied on Done
}

// flagsLevelsID is the VERBOSE section's id. Like displayengine.IDListPanel, it
// must not be a substring of the outer's id, since MatchesID uses strings.Contains.
const flagsLevelsID = "flags_level_panel"

type TriggerApplyFlagsMsg struct{}

// setVerbosityMsg carries the level picked in the VERBOSE dropdown back to the dialog.
type setVerbosityMsg struct{ level int }

// verbosityLevels names each verbosity level, for the VERBOSE dropdown.
var verbosityLevels = []struct{ name, help string }{
	{"Off", "Normal output"},
	{"1 Details", "-v: also Docker image details, and every dependency and health check line"},
	{"2 Layers", "-vv: also Docker image layers"},
}

// verbosityName returns level's name in the VERBOSE dropdown.
func verbosityName(level int) string {
	return verbosityLevels[min(max(level, 0), len(verbosityLevels)-1)].name
}

// NewFlagsToggleDialog creates a new FlagsToggleDialog
func NewFlagsToggleDialog() *FlagsToggleDialog {
	d := &FlagsToggleDialog{verbosity: console.VerboseLevel()}
	// Most used first: YES, then FORCE (riskier), then DEBUG (troubleshooting).
	items := []displayengine.MenuItem{
		{
			Tag:        "YES",
			Desc:       "Answer yes",
			Help:       "Automatically answers 'yes' to confirmation prompts",
			IsCheckbox: true,
			Selectable: true,
			Checked:    console.AssumeYes(),
			Selected:   console.AssumeYes(),
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
			Tag:        "DEBUG",
			Desc:       "Debug logging",
			Help:       "Shows internal debugging information",
			IsCheckbox: true,
			Selectable: true,
			Checked:    console.Debug(),
			Selected:   console.Debug(),
		},
	}

	// displayengine.IDListPanel (not e.g. "global_flags") deliberately avoids being a
	// substring of the outer's own id ("global_flags_outer") -- MatchesID
	// uses strings.Contains, so a section id that's a prefix of the outer's
	// id would incorrectly claim hits meant for the outer (e.g. its Done/
	// Cancel button clicks), which never reach the outer's own button-click
	// switch as a result. Matches the id convention every other migrated
	// section-owning screen already uses for its inner list.
	list := displayengine.NewMenuModel(displayengine.IDListPanel, "", "", items)
	list.SetSubMenuMode(true)
	list.SetVariableHeight(true)
	list.SetIsDialog(false)
	list.SetButtons([]displayengine.ButtonDef{})
	list.SetMaximized(true)
	// viewWithSections already wraps every content section in its own
	// ContentSideMargin padding; suppress the section's own internal left
	// margin to avoid doubling up (matches the convention used by every
	// other migrated section-owning screen).
	list.SetNoLeftMargin(true)

	// The VERBOSE section keeps a blank checkbox column, and both sections share the
	// widest tag, so its tag and value line up with the flags below. It's fixed
	// height: two variable-height sections would split the rows evenly.
	levels := displayengine.NewMenuModel(flagsLevelsID, "", "", []displayengine.MenuItem{d.verbosityItem()})
	levels.SetSubMenuMode(true)
	levels.SetIsDialog(false)
	levels.SetButtons([]displayengine.ButtonDef{})
	levels.SetMaximized(true)
	levels.SetNoLeftMargin(true)
	levels.SetCheckboxColumn(true)
	tagWidth := 0
	for _, it := range append(list.GetItems(), levels.GetItems()...) {
		tagWidth = max(tagWidth, lipgloss.Width(it.Tag))
	}
	list.SetMinTagWidth(tagWidth)
	levels.SetMinTagWidth(tagWidth)

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
	outer.AddContentSection(levels)
	outer.AddContentSection(list)

	d.outer, d.list, d.levels = outer, list, levels
	return d
}

// verbosityItem returns the VERBOSE row: the level picked as a dropdown value, opening
// the level list when selected.
func (d *FlagsToggleDialog) verbosityItem() displayengine.MenuItem {
	return displayengine.MenuItem{
		Tag:        "VERBOSE",
		Desc:       displayengine.DropdownValueMarkup(verbosityName(d.verbosity)),
		Help:       "Shows more detailed output (Select/Enter for list)",
		Selectable: true,
		Action:     d.showVerbosityDropdown(),
	}
}

// showVerbosityDropdown opens the list of verbosity levels over the dialog.
func (d *FlagsToggleDialog) showVerbosityDropdown() tea.Cmd {
	return func() tea.Msg {
		items := make([]displayengine.MenuItem, len(verbosityLevels))
		for i, l := range verbosityLevels {
			items[i] = displayengine.MenuItem{
				Tag:           l.name,
				Help:          l.help,
				IsRadioButton: true,
				Selectable:    true,
				Checked:       i == min(d.verbosity, len(verbosityLevels)-1),
			}
		}
		menu := displayengine.NewMenuModel("verbosity_dropdown", "Verbose", "Select the verbosity level", items)
		menu.SetUpdateInterceptor(RadioGroupInterceptor("verbosity_dropdown"))
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Done", ZoneID: "btn-select", Help: "Confirm the marked level.", Action: func() tea.Msg {
				for i, it := range menu.GetItems() {
					if it.Checked {
						return CloseDialogThen(func() tea.Msg { return setVerbosityMsg{i} })()
					}
				}
				return displayengine.CloseDialogMsg{}
			}},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		menu.Select(min(d.verbosity, len(verbosityLevels)-1))
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

// Init implements tea.Model
func (d *FlagsToggleDialog) Init() tea.Cmd {
	return d.outer.Init()
}

// Update implements tea.Model
func (d *FlagsToggleDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case setVerbosityMsg:
		d.verbosity = msg.level
		d.levels.SetItems([]displayengine.MenuItem{d.verbosityItem()})
		return d, nil
	case TriggerApplyFlagsMsg:
		for _, it := range d.list.GetItems() {
			switch it.Tag {
			case "DEBUG":
				console.SetDebug(it.Selected)
			case "FORCE":
				console.SetForce(it.Selected)
			case "YES":
				console.SetAssumeYes(it.Selected)
			}
		}
		console.SetVerboseLevel(d.verbosity)
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
	// Sync d.list and d.levels from outer's content sections so GetItems() in
	// TriggerApplyFlagsMsg above reflects their latest state.
	for _, sec := range d.outer.GetContentSections() {
		if mm, ok := sec.(*displayengine.MenuModel); ok {
			switch mm.ID() {
			case displayengine.IDListPanel:
				d.list = mm
			case flagsLevelsID:
				d.levels = mm
			}
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

// ClearProcessingState stops the spinner the VERBOSE row started when it opened its
// dropdown; AppModel calls it as it hides this dialog behind the dropdown. The outer
// menu clears its sections too.
func (d *FlagsToggleDialog) ClearProcessingState() {
	d.outer.ClearProcessingState()
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
