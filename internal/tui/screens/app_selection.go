package screens

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// AppSelectionScreen wraps MenuModel to provide a custom Legend help panel.
type AppSelectionScreen struct {
	menu *tui.MenuModel
}

func (s *AppSelectionScreen) Init() tea.Cmd                             { return s.menu.Init() }
func (s *AppSelectionScreen) View() tea.View                           { return s.menu.View() }
func (s *AppSelectionScreen) ViewString() string                       { return s.menu.ViewString() }
func (s *AppSelectionScreen) Title() string                            { return s.menu.Title() }
func (s *AppSelectionScreen) HelpText() string                         { return s.menu.HelpText() }
func (s *AppSelectionScreen) SetSize(w, h int)                         { s.menu.SetSize(w, h) }
func (s *AppSelectionScreen) SetFocused(f bool)                        { s.menu.SetFocused(f) }
func (s *AppSelectionScreen) IsMaximized() bool                        { return s.menu.IsMaximized() }
func (s *AppSelectionScreen) HasDialog() bool                          { return s.menu.HasDialog() }
func (s *AppSelectionScreen) MenuName() string                         { return s.menu.MenuName() }
func (s *AppSelectionScreen) Layers() []*lipgloss.Layer                { return s.menu.Layers() }
func (s *AppSelectionScreen) GetHitRegions(x, y int) []tui.HitRegion  { return s.menu.GetHitRegions(x, y) }

func (s *AppSelectionScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := s.menu.Update(msg)
	if mm, ok := m.(*tui.MenuModel); ok {
		s.menu = mm
	}
	return s, cmd
}

func (s *AppSelectionScreen) HelpContext(maxWidth int) tui.HelpContext {
	legend := "| " +
		"{{|MarkerAdded|}}+{{[-]}} Added | " +
		"{{|MarkerDeleted|}}-{{[-]}} Deleted | " +
		"{{|MarkerModified|}}r{{[-]}} Referenced | " +
		"{{|MarkerAdded|}}R{{[-]}} Referenced & Added |"
	inner := s.menu.HelpContext(maxWidth)
	itemText := inner.ItemText

	items := s.menu.GetItems()
	idx := s.menu.Index()
	if idx >= 0 && idx < len(items) {
		item := items[idx]
		if item.BaseApp != "" && !item.IsSeparator {
			ctx := context.Background()
			appMeta, _ := appenv.LoadAppMeta(ctx, item.BaseApp)
			var parts []string
			if appMeta != nil && appMeta.App.HelpText != "" {
				parts = append(parts, appMeta.App.HelpText)
			} else {
				// Fall back to labels.yml description (same as shown in the list)
				if desc := appenv.GetDescriptionFromTemplate(ctx, item.BaseApp, ""); desc != "" {
					parts = append(parts, desc)
				}
			}
			if appMeta != nil && appMeta.App.Website != "" {
				parts = append(parts, "Website: "+appMeta.App.Website)
			}
			if appenv.IsAppDeprecated(ctx, item.BaseApp) {
				parts = append(parts, "{{|TitleError|}}⚠ This app is deprecated.{{[-]}}")
			}
			if len(parts) > 0 {
				itemText = strings.Join(parts, "\n\n")
			}
		}
	}

	return tui.HelpContext{
		ScreenName: inner.ScreenName,
		PageTitle:  "Legend",
		PageText:   legend,
		ItemTitle:  inner.ItemTitle,
		ItemText:   itemText,
	}
}

// NewAppSelectionScreen creates the app selection screen.
// isRoot suppresses the Back button when this is the entry point.
func NewAppSelectionScreen(conf config.AppConfig, isRoot bool) *AppSelectionScreen {
	// computeChanges and buildChangeSummary are declared as vars here so that
	// backAction (needed by NewMenuModel) can close over them before their
	// implementations are defined below.
	type changeSet struct {
		toEnable  []string
		toDisable []string
		niceNames map[string]string
		envFile   string
	}
	var computeChanges func() changeSet
	var buildChangeSummary func(changeSet) string

	var backAction tea.Cmd
	if !isRoot {
		backAction = func() tea.Msg {
			cs := computeChanges()
			msg := buildChangeSummary(cs)
			if msg != "No changes pending." {
				msg += "\n\nGo back and discard these changes?"
			} else {
				msg = "Go back?"
			}
			if !tui.Confirm("Go Back", msg, false) {
				return nil
			}
			return tui.NavigateBackMsg{}
		}
	}

	menu := tui.NewMenuModel(
		"app-select",
		"Select Applications",
		"Choose which apps you would like to install:\nUse {{|KeyCap|}}[up]{{[-]}}/{{|KeyCap|}}[down]{{[-]}} and {{|KeyCap|}}[space]{{[-]}} to select; {{|KeyCap|}}[ctrl+→]{{[-]}}/{{|KeyCap|}}[alt+→]{{[-]}} to manage instances.",
		nil,
		backAction,
	)
	menu.SetMenuName("app-select")
	menu.SetHelpItemPrefix("App")
	menu.SetButtonLabels("Done", "Back", "Exit")
	menu.SetShowExit(true)
	menu.SetGroupedMode(true)
	menu.SetVariableHeight(true)
	menu.SetMaximized(true)
	menu.SetSubMenuMode(false)
	menu.SetFocusedItem(tui.FocusSelectBtn) // Select button always highlighted; list cursor visible via IsActive()

	// Inline editing state — shared by all closures below via reference.
	var isEditing bool
	var editingBaseApp string
	var editingIdx int
	var editContent string
	var editError string
	var isRenaming bool               // true when renaming an existing sub-item (vs adding new)
	var renamingOriginal tui.MenuItem // original sub-item being renamed (for cancel restore)
	// When startEditing converts a simple checkbox row into a group header (because the
	// app has no non-base instances yet), we record the original so cancelEdit can restore it.
	var convertedFromSimple bool
	var convertedSimpleOriginal tui.MenuItem

	// editingTag builds the display string for the inline editing row.
	// niceName is the base app's display name (e.g. "Adminer").
	// content is the uppercase internal suffix typed so far (e.g. "4K").
	editingTag := func(niceName, content, errMsg string) string {
		cursor := "▌"
		var display string
		if content == "" {
			display = niceName // just the base; typing appends __Suffix
		} else {
			display = niceName + "__" + appenv.CapitalizeFirstLetter(content)
		}
		display += cursor
		if errMsg != "" {
			display += "  {{|StatusError|}}" + errMsg + "{{[-]}}"
		}
		return display
	}

	// refreshEditRow updates the IsEditing item's display in-place.
	refreshEditRow := func() {
		items := menu.GetItems()
		if editingIdx < 0 || editingIdx >= len(items) {
			return
		}
		ctx := context.Background()
		niceName := appenv.GetNiceName(ctx, editingBaseApp)
		updated := items[editingIdx]
		updated.Tag = editingTag(niceName, editContent, editError)
		menu.SetItem(editingIdx, updated)
	}

	// collapseGroupIfNeeded checks whether a group still has non-base sub-rows
	// after a removal and, if not, replaces the group header + add-row with a
	// simple checkbox.  Returns the updated item slice and a bool indicating
	// whether a collapse occurred.
	collapseGroupIfNeeded := func(items []tui.MenuItem, base string) ([]tui.MenuItem, bool) {
		// Count remaining non-base sub-rows for this base app.
		var nonBaseCount int
		for _, item := range items {
			if item.IsSubItem && item.BaseApp == base && appenv.AppNameToInstanceName(item.Metadata["appName"]) != "" {
				nonBaseCount++
			}
		}
		if nonBaseCount > 0 {
			return items, false
		}
		// No non-base instances left — collapse to a simple checkbox.
		// Read WasAdded/IsReferenced from the group header and current Checked from the base sub-row.
		var wasAdded, checked, isReferenced bool
		for _, item := range items {
			if item.IsGroupHeader && item.BaseApp == base {
				wasAdded = item.WasAdded
				isReferenced = item.IsReferenced
				checked = item.WasAdded // default to original state
			}
			if item.IsSubItem && item.BaseApp == base && appenv.AppNameToInstanceName(item.Metadata["appName"]) == "" {
				checked = item.Checked // base sub-row carries the toggled state
			}
		}
		ctx := context.Background()
		envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
		niceName := appenv.GetNiceName(ctx, base)
		desc := appenv.GetDescriptionFromTemplate(ctx, base, envFile)
		simpleRow := tui.MenuItem{
			Tag:          niceName,
			Desc:         "{{|ListApp|}}" + desc,
			Help:         fmt.Sprintf("Toggle %s. Press Ctrl+Right to add instances.", niceName),
			IsCheckbox:   true,
			Checked:      checked,
			WasAdded:     wasAdded,
			IsReferenced: isReferenced,
			BaseApp:      base,
			Metadata:     map[string]string{"appName": base},
		}
		// Remove header, sub-items, and add-row; insert simple checkbox in their place.
		newItems := make([]tui.MenuItem, 0, len(items))
		inserted := false
		for _, item := range items {
			if item.BaseApp == base && (item.IsGroupHeader || item.IsSubItem || item.IsAddInstance) {
				if !inserted {
					newItems = append(newItems, simpleRow)
					inserted = true
				}
				continue
			}
			newItems = append(newItems, item)
		}
		return newItems, true
	}

	// refreshGroupHeaders syncs IsGroupHeader Checked state from sub-rows.
	refreshGroupHeaders := func(items []tui.MenuItem) []tui.MenuItem {
		subChecked := make(map[string]bool)
		for _, item := range items {
			if item.IsSubItem && item.Checked {
				subChecked[item.BaseApp] = true
			}
		}
		for i, item := range items {
			if item.IsGroupHeader {
				items[i].Checked = subChecked[item.BaseApp]
			}
		}
		return items
	}

	// cancelEdit removes (or restores) the editing row and resets editing state.
	cancelEdit := func() {
		items := menu.GetItems()
		newItems := make([]tui.MenuItem, 0, len(items))
		if isRenaming {
			// Restore the original sub-item in-place.
			for i, item := range items {
				if i == editingIdx {
					newItems = append(newItems, renamingOriginal)
				} else {
					newItems = append(newItems, item)
				}
			}
		} else if convertedFromSimple {
			// Restore the simple checkbox row and drop the editing row.
			for i, item := range items {
				if item.IsGroupHeader && item.BaseApp == editingBaseApp {
					newItems = append(newItems, convertedSimpleOriginal)
				} else if i == editingIdx {
					// skip the editing row
				} else {
					newItems = append(newItems, item)
				}
			}
		} else {
			// Remove the inserted editing row.
			for i, item := range items {
				if i != editingIdx {
					newItems = append(newItems, item)
				}
			}
		}
		isEditing = false
		isRenaming = false
		convertedFromSimple = false
		convertedSimpleOriginal = tui.MenuItem{}
		renamingOriginal = tui.MenuItem{}
		editingBaseApp = ""
		editContent = ""
		editError = ""
		editingIdx = -1
		menu.SetItems(newItems)
	}

	// confirmEdit validates and commits the new instance name.
	var confirmEdit func()
	confirmEdit = func() {
		base := editingBaseApp
		suffix := editContent

		var newAppName string
		if suffix == "" {
			newAppName = base
		} else {
			newAppName = base + "__" + suffix
		}

		// Renaming with unchanged name → just cancel.
		if isRenaming && newAppName == renamingOriginal.Metadata["appName"] {
			cancelEdit()
			return
		}

		if suffix != "" && !appenv.InstanceNameIsValid(suffix) {
			editError = "reserved name"
			refreshEditRow()
			return
		}
		if !appenv.IsAppNameValid(newAppName) {
			editError = "invalid name"
			refreshEditRow()
			return
		}

		// Duplicate check (ignore the item being renamed).
		items := menu.GetItems()
		for _, item := range items {
			if item.IsSubItem && item.BaseApp == base && item.Metadata["appName"] == newAppName {
				editError = "already exists"
				refreshEditRow()
				return
			}
		}

		ctx := context.Background()
		baseNiceName := appenv.GetNiceName(ctx, base)
		displayName := appenv.InstanceDisplayName(baseNiceName, newAppName)

		checkedState := true // new instances default to enabled
		if isRenaming {
			checkedState = renamingOriginal.Checked
		}

		newSubRow := tui.MenuItem{
			Tag:        displayName,
			Help:       fmt.Sprintf("Toggle %s", displayName),
			IsSubItem:  true,
			IsCheckbox: true,
			IsNew:      true, // always renameable until saved (rename = disable old + enable new)
			Checked:    checkedState,
			BaseApp:    base,
			Metadata:   map[string]string{"appName": newAppName},
		}

		// Replace editing row with the new sub-item, then refresh headers.
		newItems := make([]tui.MenuItem, 0, len(items))
		for i, item := range items {
			if i == editingIdx {
				newItems = append(newItems, newSubRow)
			} else {
				newItems = append(newItems, item)
			}
		}
		newItems = refreshGroupHeaders(newItems)

		// When a named instance (suffix != "") was just added (not renamed), remove
		// the unchecked non-referenced base sub-row — it was never enabled and only
		// confuses the user by lingering alongside the new named instance.
		if suffix != "" && !isRenaming {
			cleaned := newItems[:0:len(newItems)]
			for _, it := range newItems {
				if it.IsSubItem && it.BaseApp == base &&
					it.Metadata["appName"] == base &&
					!it.Checked && !it.IsReferenced {
					continue // drop phantom unchecked base sub-row
				}
				cleaned = append(cleaned, it)
			}
			// Only apply if named instances still exist (don't orphan the group).
			hasNamed := false
			for _, it := range cleaned {
				if it.IsSubItem && it.BaseApp == base && it.Metadata["appName"] != base {
					hasNamed = true
					break
				}
			}
			if hasNamed {
				newItems = cleaned
			}
		}

		isEditing = false
		isRenaming = false
		convertedFromSimple = false
		convertedSimpleOriginal = tui.MenuItem{}
		renamingOriginal = tui.MenuItem{}
		editingBaseApp = ""
		editContent = ""
		editError = ""
		editingIdx = -1

		menu.SetItems(newItems)
	}

	// startEditing inserts an IsEditing row after the last sub-item of baseApp's group.
	// For apps that currently show as a simple checkbox (no non-base instances), it first
	// promotes the row to a group header so the editing row can appear beneath it.
	startEditing := func(baseApp string) {
		if isEditing {
			return
		}
		items := menu.GetItems()

		insertAt := -1
		simpleIdx := -1 // index of a simple checkbox row for baseApp (no group header yet)
		inGroup := false

		for i, item := range items {
			if item.IsGroupHeader && item.BaseApp == baseApp {
				inGroup = true
				insertAt = i + 1
				continue
			}
			if inGroup {
				if item.IsSubItem && item.BaseApp == baseApp {
					insertAt = i + 1
				} else if item.IsAddInstance && item.BaseApp == baseApp {
					insertAt = i // insert editing row before the [+] row
					break
				} else {
					break
				}
			}
			// Simple (non-grouped) checkbox row for this base app.
			if !item.IsGroupHeader && !item.IsSubItem && item.IsCheckbox && item.BaseApp == baseApp {
				simpleIdx = i
				insertAt = i + 1
				break
			}
		}
		if insertAt < 0 {
			return
		}

		editRow := tui.MenuItem{
			Tag:        editingTag(appenv.GetNiceName(context.Background(), baseApp), "", ""),
			Help:       "Type instance suffix (blank = base). Enter to confirm, Esc to cancel.",
			IsEditing:  true,
			IsCheckbox: true,
			Checked:    true, // new instances default to enabled
			BaseApp:    baseApp,
		}

		// Promote simple checkbox → group header if needed.
		workItems := items
		if simpleIdx >= 0 {
			ctx := context.Background()
			niceName := appenv.GetNiceName(ctx, baseApp)
			orig := items[simpleIdx]
			convertedSimpleOriginal = orig
			promoted := tui.MenuItem{
				Tag:           orig.Tag,
				Desc:          orig.Desc,
				Help:          fmt.Sprintf("Press Ctrl+Right to manage %s instances", niceName),
				IsGroupHeader: true,
				Checked:       orig.Checked,
				IsReferenced:  orig.IsReferenced,
				BaseApp:       baseApp,
				Metadata:      orig.Metadata,
			}
			workItems = make([]tui.MenuItem, len(items))
			copy(workItems, items)
			workItems[simpleIdx] = promoted
			convertedFromSimple = true
		}

		newItems := make([]tui.MenuItem, 0, len(workItems)+1)
		newItems = append(newItems, workItems[:insertAt]...)
		newItems = append(newItems, editRow)
		newItems = append(newItems, workItems[insertAt:]...)

		isEditing = true
		editingBaseApp = baseApp
		editingIdx = insertAt
		editContent = ""
		editError = ""

		menu.SetItems(newItems)
		menu.Select(insertAt)
	}

	// startRenaming replaces a sub-item with an editing row pre-filled with its current suffix.
	// Any sub-item (new or pre-existing) can be renamed; the save logic handles the
	// old name as a disable and the new name as an enable.
	startRenaming := func(subIdx int) {
		if isEditing {
			return
		}
		items := menu.GetItems()
		if subIdx < 0 || subIdx >= len(items) || !items[subIdx].IsSubItem {
			return
		}
		if items[subIdx].IsReferenced {
			return // referenced items are locked; they have existing config and cannot be renamed
		}
		subItem := items[subIdx]
		suffix := appenv.AppNameToInstanceName(subItem.Metadata["appName"])

		editRow := tui.MenuItem{
			Tag:        editingTag(appenv.GetNiceName(context.Background(), subItem.BaseApp), suffix, ""),
			Help:       "Edit instance name. Enter to confirm, Esc to cancel.",
			IsEditing:  true,
			IsCheckbox: subItem.IsCheckbox,
			Checked:    subItem.Checked,
			BaseApp:    subItem.BaseApp,
		}

		newItems := make([]tui.MenuItem, len(items))
		copy(newItems, items)
		newItems[subIdx] = editRow

		isEditing = true
		isRenaming = true
		renamingOriginal = subItem
		editingBaseApp = subItem.BaseApp
		editingIdx = subIdx
		editContent = suffix
		editError = ""

		menu.SetItems(newItems)
		menu.Select(subIdx)
	}

	// expandGroup promotes a simple checkbox row into a group view (header +
	// base sub-row + [+] row) without opening inline editing.  The user can
	// then navigate the expanded list and press Space to toggle, Right/F2 to
	// rename a new instance, or Left (from the header) to collapse it back.
	expandGroup := func(baseApp string) {
		if isEditing {
			return
		}
		items := menu.GetItems()
		simpleIdx := -1
		for i, item := range items {
			if item.IsCheckbox && !item.IsGroupHeader && !item.IsSubItem && item.BaseApp == baseApp {
				simpleIdx = i
				break
			}
		}
		if simpleIdx < 0 {
			return
		}
		orig := items[simpleIdx]
		ctx := context.Background()
		envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
		niceName := appenv.GetNiceName(ctx, baseApp)
		desc := appenv.GetDescriptionFromTemplate(ctx, baseApp, envFile)
		groupHeader := tui.MenuItem{
			Tag:           niceName,
			Desc:          "{{|ListApp|}}" + desc,
			Help:          fmt.Sprintf("Press Ctrl+Right to manage %s instances. Ctrl+Left to collapse.", niceName),
			IsGroupHeader: true,
			Checked:       orig.Checked,
			WasAdded:      orig.WasAdded,
			IsReferenced:  orig.IsReferenced,
			BaseApp:       baseApp,
			Metadata:      orig.Metadata,
		}
		baseSubRow := tui.MenuItem{
			Tag:          niceName,
			Help:         fmt.Sprintf("Toggle %s", niceName),
			IsSubItem:    true,
			IsCheckbox:   true,
			Checked:      orig.Checked,
			WasAdded:     orig.WasAdded,
			IsReferenced: orig.IsReferenced,
			BaseApp:      baseApp,
			Metadata:     orig.Metadata,
		}
		addRow := tui.MenuItem{
			Tag:           "+ Add instance\u2026",
			Help:          fmt.Sprintf("Press Space/Enter or Ctrl+Right to add a %s instance", niceName),
			IsAddInstance: true,
			BaseApp:       baseApp,
		}
		newItems := make([]tui.MenuItem, 0, len(items)+2)
		for i, item := range items {
			if i == simpleIdx {
				newItems = append(newItems, groupHeader, baseSubRow, addRow)
			} else {
				newItems = append(newItems, item)
			}
		}
		menu.SetItems(newItems)
		menu.Select(simpleIdx + 1) // move cursor to base sub-row
	}

	// refreshItems rebuilds the list from current env/template state.
	//
	// Apps with NO non-base instances are shown as a simple checkbox row — Space
	// toggles them directly and Right opens inline editing to add the first instance.
	//
	// Apps that have at least one non-base instance (e.g. RADARR__4K) are shown as a
	// group header row followed by indented sub-rows for every added instance.  The
	// sub-list is always visible; there is no expand/collapse toggle.
	refreshItems := func() {
		ctx := context.Background()
		envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)

		nonDeprecated, _ := appenv.ListNonDeprecatedApps(ctx)
		added, _ := appenv.ListAddedApps(ctx, envFile)

		baseAppsSet := make(map[string]bool)
		for _, a := range nonDeprecated {
			baseAppsSet[appenv.AppNameToBaseAppName(a)] = true
		}
		for _, a := range added {
			baseAppsSet[appenv.AppNameToBaseAppName(a)] = true
		}

		addedByBase := make(map[string][]string)
		for _, a := range added {
			base := appenv.AppNameToBaseAppName(a)
			addedByBase[base] = append(addedByBase[base], a)
		}

		enabledApps, _ := appenv.ListEnabledApps(conf)
		enabledMap := make(map[string]bool)
		for _, a := range enabledApps {
			enabledMap[a] = true
		}

		// addedMap tracks what was present in .env at screen load (for gutter diff).
		addedMap := make(map[string]bool)
		for _, a := range added {
			addedMap[a] = true
		}

		// referencedByBase: non-base instances that are referenced in env/compose but not added.
		// These get shown as locked (IsReferenced) sub-rows with an R gutter marker.
		// referencedBaseSet: base apps that are referenced but not added (for simple row R marker).
		referenced, _ := appenv.ListReferencedApps(ctx, conf)
		referencedByBase := make(map[string][]string)
		referencedBaseSet := make(map[string]bool)
		for _, r := range referenced {
			base := appenv.AppNameToBaseAppName(r)
			if base == r {
				// Base app referenced but not added — mark for R gutter on simple row.
				if !addedMap[r] {
					referencedBaseSet[r] = true
				}
				continue
			}
			if addedMap[r] {
				continue // already formally added
			}
			if !baseAppsSet[base] {
				continue // unknown base app
			}
			referencedByBase[base] = append(referencedByBase[base], r)
		}

		var baseApps []string
		for base := range baseAppsSet {
			baseApps = append(baseApps, base)
		}
		slices.Sort(baseApps)

		var items []tui.MenuItem
		var lastLetter string

		for _, base := range baseApps {
			letter := strings.ToUpper(base[:1])
			if letter != lastLetter {
				if lastLetter != "" {
					items = append(items, tui.MenuItem{IsSeparator: true})
				}
				lastLetter = letter
			}

			niceName := appenv.GetNiceName(ctx, base)
			desc := appenv.GetDescriptionFromTemplate(ctx, base, envFile)

			instances := addedByBase[base]
			slices.Sort(instances)
			refInstances := referencedByBase[base]
			slices.Sort(refInstances)

			// Count non-base instances across both added and referenced sets.
			var nonBaseCount int
			for _, inst := range instances {
				if appenv.AppNameToInstanceName(inst) != "" {
					nonBaseCount++
				}
			}
			nonBaseCount += len(refInstances) // all refInstances are non-base by construction

			if nonBaseCount == 0 {
				// Simple checkbox row — no instances or only the bare base app added.
				items = append(items, tui.MenuItem{
					Tag:          niceName,
					Desc:         "{{|ListApp|}}" + desc,
					Help:         fmt.Sprintf("Toggle %s. Press Ctrl+Right to add instances.", niceName),
					IsCheckbox:   true,
					Checked:      addedMap[base],
					WasAdded:     addedMap[base],
					IsReferenced: referencedBaseSet[base],
					BaseApp:      base,
					Metadata:     map[string]string{"appName": base},
				})
			} else {
				// Group header + sub-rows for every explicitly added instance.
				anyEnabled := false
				for _, inst := range instances {
					if enabledMap[inst] {
						anyEnabled = true
						break
					}
				}

				items = append(items, tui.MenuItem{
					Tag:           niceName,
					Desc:          "{{|ListApp|}}" + desc,
					Help:          fmt.Sprintf("Press Ctrl+Right to manage %s instances", niceName),
					IsGroupHeader: true,
					Checked:       anyEnabled,
					IsReferenced:  referencedBaseSet[base],
					BaseApp:       base,
					Metadata:      map[string]string{"appName": base},
				})

				// Merge added and referenced instances, sorted by app name.
				type instEntry struct {
					appName      string
					isReferenced bool
				}
				var allInsts []instEntry
				for _, inst := range instances {
					allInsts = append(allInsts, instEntry{inst, false})
				}
				for _, inst := range refInstances {
					allInsts = append(allInsts, instEntry{inst, true})
				}
				slices.SortFunc(allInsts, func(a, b instEntry) int {
					return strings.Compare(a.appName, b.appName)
				})

				for _, ie := range allInsts {
					displayName := appenv.InstanceDisplayName(niceName, ie.appName)
					if ie.isReferenced {
						items = append(items, tui.MenuItem{
							Tag:          displayName,
							Help:         fmt.Sprintf("%s — referenced in config but not added", displayName),
							IsSubItem:    true,
							IsCheckbox:   true,
							IsReferenced: true,
							Checked:      false,
							WasAdded:     false,
							BaseApp:      base,
							Metadata:     map[string]string{"appName": ie.appName},
						})
					} else {
						items = append(items, tui.MenuItem{
							Tag:        displayName,
							Help:       fmt.Sprintf("Toggle %s", displayName),
							IsSubItem:  true,
							IsCheckbox: true,
							Checked:    addedMap[ie.appName],
							WasAdded:   addedMap[ie.appName],
							BaseApp:    base,
							Metadata:   map[string]string{"appName": ie.appName},
						})
					}
				}
				// Dedicated [+] row so users can add another instance.
				items = append(items, tui.MenuItem{
					Tag:           "+ Add instance\u2026",
					Help:          fmt.Sprintf("Press Space/Enter or Ctrl+Right to add another %s instance", niceName),
					IsAddInstance: true,
					BaseApp:       base,
				})
			}
		}

		menu.SetItems(items)
	}

	computeChanges = func() changeSet {
		envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
		niceNames := make(map[string]string)
		originalAdded, _ := appenv.ListAddedApps(context.Background(), envFile)
		originalMap := make(map[string]bool)
		for _, a := range originalAdded {
			originalMap[a] = true
		}
		var toEnable, toDisable []string
		for _, item := range menu.GetItems() {
			if item.IsGroupHeader || item.IsSeparator || item.IsEditing {
				continue
			}
			appName := item.Metadata["appName"]
			if appName == "" {
				continue
			}
			niceNames[appName] = item.Tag
			if item.Checked && !originalMap[appName] {
				toEnable = append(toEnable, appName)
			} else if !item.Checked && originalMap[appName] {
				toDisable = append(toDisable, appName)
			}
		}
		return changeSet{toEnable: toEnable, toDisable: toDisable, niceNames: niceNames, envFile: envFile}
	}

	// buildChangeSummary returns a human-readable summary of pending changes.
	buildChangeSummary = func(cs changeSet) string {
		if len(cs.toEnable) == 0 && len(cs.toDisable) == 0 {
			return "No changes pending."
		}
		// Align app names under the longest label ("Remove: " = 8 chars).
		const indent = "        " // 8 spaces — matches "Remove: "
		var lines []string
		if len(cs.toEnable) > 0 {
			for i, app := range cs.toEnable {
				name := "{{|ProgressWaiting|}}" + cs.niceNames[app] + "{{[-]}}"
				if i == 0 {
					lines = append(lines, "{{|ProgressWaiting|}}Add:{{[-]}}    "+name)
				} else {
					lines = append(lines, indent+name)
				}
			}
		}
		if len(cs.toDisable) > 0 {
			for i, app := range cs.toDisable {
				name := "{{|ProgressWaiting|}}" + cs.niceNames[app] + "{{[-]}}"
				if i == 0 {
					lines = append(lines, "{{|ProgressWaiting|}}Remove:{{[-]}} "+name)
				} else {
					lines = append(lines, indent+name)
				}
			}
		}
		return strings.Join(lines, "\n")
	}

	handleSave := func() tea.Msg {
		cs := computeChanges()
		toEnable := cs.toEnable
		toDisable := cs.toDisable
		niceNames := cs.niceNames
		envFile := cs.envFile

		if len(toEnable) == 0 && len(toDisable) == 0 {
			return tui.NavigateBackMsg{}
		}

		var toEnableNice []string
		for _, app := range toEnable {
			toEnableNice = append(toEnableNice, niceNames[app])
		}
		var toDisableNice []string
		for _, app := range toDisable {
			toDisableNice = append(toDisableNice, niceNames[app])
		}

		dialog := tui.NewProgramBoxModel("{{|TitleSuccess|}}Enabling Selected Applications", "", "")
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)
		dialog.SetAutoClose(false, 0)

		if len(toDisable) > 0 {
			dialog.AddTask("Removing applications", "ds --remove", toDisableNice)
		}
		if len(toEnable) > 0 {
			dialog.AddTask("Adding applications", "ds --add", toEnableNice)
		}
		dialog.AddTask("Updating variable files", "", nil)

		task := func(ctx context.Context, w io.Writer) error {
			ctx = console.WithTUIWriter(ctx, w)
			totalSteps := len(toEnable) + len(toDisable) + 1
			completedSteps := 0

			updateProgress := func() {
				tui.Send(tui.UpdatePercentMsg{Percent: float64(completedSteps) / float64(totalSteps)})
			}

			if len(toDisable) > 0 {
				tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusInProgress, ActiveApp: ""})
				for _, app := range toDisable {
					tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusInProgress, ActiveApp: niceNames[app]})
					_ = appenv.Remove(ctx, []string{app}, conf, true)
					completedSteps++
					updateProgress()
				}
				tui.Send(tui.UpdateTaskMsg{Label: "Removing applications", Status: tui.StatusCompleted, ActiveApp: ""})
			}

			if len(toEnable) > 0 {
				tui.Send(tui.UpdateTaskMsg{Label: "Adding applications", Status: tui.StatusInProgress, ActiveApp: ""})
				for _, app := range toEnable {
					tui.Send(tui.UpdateTaskMsg{Label: "Adding applications", Status: tui.StatusInProgress, ActiveApp: niceNames[app]})
					_ = appenv.Enable(ctx, []string{app}, conf)
					completedSteps++
					updateProgress()
				}
				tui.Send(tui.UpdateTaskMsg{Label: "Adding applications", Status: tui.StatusCompleted, ActiveApp: ""})
			}

			tui.Send(tui.UpdateTaskMsg{Label: "Updating variable files", Status: tui.StatusInProgress, ActiveApp: ""})
			_ = appenv.Update(ctx, console.Force(), envFile)
			completedSteps++
			tui.Send(tui.UpdateTaskMsg{Label: "Updating variable files", Status: tui.StatusCompleted, ActiveApp: ""})
			updateProgress()

			return nil
		}
		dialog.SetTask(task)

		return tui.FinalizeSelectionMsg{Dialog: dialog}
	}

	menu.SetEnterAction(func() tea.Msg {
		cs := computeChanges()
		msg := buildChangeSummary(cs)
		if msg == "No changes pending." {
			msg = "No changes to apply. Go back?"
		} else {
			msg += "\n\nApply these changes?"
		}
		if !tui.Confirm("Apply Changes", msg, true) {
			return nil
		}
		return handleSave()
	})

	// toggleItem runs the same toggle logic as pressing Space on an item.
	// Used by both key and mouse click handlers.
	toggleItem := func(m *tui.MenuModel, idx int) bool {
		items := m.GetItems()
		if idx < 0 || idx >= len(items) {
			return false
		}
		item := items[idx]
		if item.IsGroupHeader || item.IsAddInstance {
			startEditing(item.BaseApp)
			return true
		}
		if item.IsCheckbox || item.IsSubItem {
			if item.IsSubItem && item.IsNew && item.Checked {
				newItems := make([]tui.MenuItem, 0, len(items)-1)
				for i2, it := range items {
					if i2 != idx {
						newItems = append(newItems, it)
					}
				}
				newItems, _ = collapseGroupIfNeeded(newItems, item.BaseApp)
				m.SetItems(refreshGroupHeaders(newItems))
				return true
			}
			updated := item
			updated.Checked = !updated.Checked
			m.SetItem(idx, updated)
			m.SetItems(refreshGroupHeaders(m.GetItems()))
			return true
		}
		return true // consume separators etc. to prevent button trigger
	}

	menu.SetUpdateInterceptor(func(msg tea.Msg, m *tui.MenuModel) (tea.Cmd, bool) {
		isSubRow := func(it tui.MenuItem) bool {
			return it.IsSubItem || it.IsAddInstance || it.IsEditing
		}

		navUp := func() {
			items := m.GetItems()
			idx := m.Index()
			if idx < 0 || idx >= len(items) {
				return
			}
			item := items[idx]
			if isSubRow(item) {
				for i := idx - 1; i >= 0; i-- {
					if items[i].IsGroupHeader {
						break
					}
					if isSubRow(items[i]) && items[i].BaseApp == item.BaseApp {
						m.Select(i)
						return
					}
				}
				return
			}
			for i := idx - 1; i >= 0; i-- {
				if !items[i].IsSeparator && !isSubRow(items[i]) {
					m.Select(i)
					return
				}
			}
		}

		navDown := func() {
			items := m.GetItems()
			idx := m.Index()
			if idx < 0 || idx >= len(items) {
				return
			}
			item := items[idx]
			if isSubRow(item) {
				for i := idx + 1; i < len(items); i++ {
					if !isSubRow(items[i]) {
						break
					}
					if items[i].BaseApp == item.BaseApp {
						m.Select(i)
						return
					}
				}
				return
			}
			for i := idx + 1; i < len(items); i++ {
				if !items[i].IsSeparator && !isSubRow(items[i]) {
					m.Select(i)
					return
				}
			}
		}

		// collapseOtherGroups collapses every expanded group except keepBase,
		// then re-selects the keepBase item (its index may shift after collapses).
		collapseOtherGroups := func(keepBase string) {
			cur := m.GetItems()
			changed := false
			seen := map[string]bool{}
			for _, it := range cur {
				if it.IsGroupHeader && it.BaseApp != keepBase && !seen[it.BaseApp] {
					seen[it.BaseApp] = true
				}
			}
			for base := range seen {
				if newItems, ok := collapseGroupIfNeeded(cur, base); ok {
					cur = newItems
					changed = true
				}
			}
			if !changed {
				return
			}
			m.SetItems(cur)
			for i, it := range cur {
				if it.BaseApp == keepBase {
					m.Select(i)
					return
				}
			}
		}

		// Rebuild list on template update.
		if _, ok := msg.(tui.TemplateUpdateSuccessMsg); ok {
			refreshItems()
			return nil, true
		}

		// Mouse wheel uses the same navigation as up/down arrows.
		if wheelMsg, ok := msg.(tui.LayerWheelMsg); ok {
			if isEditing {
				return nil, true
			}
			if wheelMsg.Button == tea.MouseWheelUp {
				navUp()
			} else if wheelMsg.Button == tea.MouseWheelDown {
				navDown()
			}
			return nil, true
		}
		if wheelMsg, ok := msg.(tea.MouseWheelMsg); ok {
			if isEditing {
				return nil, true
			}
			if wheelMsg.Button == tea.MouseWheelUp {
				navUp()
			} else if wheelMsg.Button == tea.MouseWheelDown {
				navDown()
			}
			return nil, true
		}

		// Handle mouse left-clicks on list items.
		// Split-click behaviour:
		//   Group headers: any click — collapse if submenu is open, else enter/expand.
		//   Other rows: LEFT of the app name = checkbox toggle; ON/RIGHT = expand/rename.
		// Without this, clicks fall through to menu_update.go's handleSpace/handleEnter
		// which doesn't know about our custom toggle rules (IsNew removal, refreshGroupHeaders).
		if hitMsg, ok := msg.(tui.LayerHitMsg); ok && hitMsg.Button == tea.MouseLeft {
			// Block all mouse clicks while the user is in inline editing mode.
			// They must press Enter (confirm) or Esc (cancel) first.
			if isEditing {
				return nil, true
			}
			if idx, ok := tui.ParseMenuItemIndex(hitMsg.ID, m.ID()); ok {
				items := m.GetItems()
				if idx >= 0 && idx < len(items) {
					item := items[idx]
					m.Select(idx)

					// Separators and live-editing rows: consume without action.
					if item.IsSeparator || item.IsEditing {
						return nil, true
					}

					// [+] Add instance: always open editing.
					if item.IsAddInstance {
						startEditing(item.BaseApp)
						return nil, true
					}

					// Group headers: any click toggles submenu open/closed.
					// If sub-items are currently visible → collapse (Ctrl+Left behaviour).
					// If no sub-items are visible → enter submenu / start editing.
					if item.IsGroupHeader {
						base := item.BaseApp
						submenuOpen := false
						for i := idx + 1; i < len(items); i++ {
							if items[i].IsGroupHeader {
								break
							}
							if (items[i].IsSubItem || items[i].IsAddInstance) && items[i].BaseApp == base {
								submenuOpen = true
								break
							}
						}
						if submenuOpen {
							newItems, collapsed := collapseGroupIfNeeded(items, base)
							if collapsed {
								m.SetItems(newItems)
							}
							// If can't fully collapse (non-base instances exist), header
							// is still selected/highlighted — nothing more to do.
						} else {
							// Not yet expanded: jump to first sub-item, or start editing.
							entered := false
							for i := idx + 1; i < len(items); i++ {
								if items[i].IsGroupHeader {
									break
								}
								if items[i].IsSubItem && items[i].BaseApp == base {
									m.Select(i)
									entered = true
									break
								}
							}
							if !entered {
								startEditing(base)
							}
						}
						collapseOtherGroups(base)
						return nil, true
					}

					// Compute the column where the tag text (app name) begins.
					// Layout: gutter(1) [+ indent(4) for sub-items] + glyph + space
					// Unicode glyphs are 1 column wide; ASCII glyphs are 4 chars ("[x] ").
					ctx := tui.GetActiveContext()
					var nameStartCol int
					if item.IsSubItem {
						if ctx.LineCharacters {
							nameStartCol = 7 // gutter(1) + indent(4) + glyph(1) + space(1)
						} else {
							nameStartCol = 10 // gutter(1) + indent(4) + "[x] "(4) + space(1)
						}
					} else {
						if ctx.LineCharacters {
							nameStartCol = 3 // gutter(1) + glyph(1) + space(1)
						} else {
							nameStartCol = 6 // gutter(1) + "[x] "(4) + space(1)
						}
					}

					// relX is the click column relative to the left edge of the row.
					// hitMsg.X is absolute screen X; hitMsg.Hit.X is the region's absolute X.
					relX := hitMsg.X - hitMsg.Hit.X
					if relX < nameStartCol {
						// ── Left zone (glyph/checkbox area): toggle checkbox.
						toggleItem(m, idx)
						if item.IsCheckbox {
							collapseOtherGroups(item.BaseApp)
						}
						return nil, true
					} else {
						// ── Right zone (name area): expand / rename action ──
						if item.IsSubItem {
							if !item.IsReferenced {
								startRenaming(idx)
							}
						} else if item.IsCheckbox {
							expandGroup(item.BaseApp)
							collapseOtherGroups(item.BaseApp)
						}
						return nil, true
					}
				}
			}
			return nil, false
		}

		keyMsg, ok := msg.(tea.KeyPressMsg)
		if !ok {
			return nil, false
		}

		// While editing, consume all keys internally — never let them reach the menu.
		if isEditing {
			switch keyMsg.String() {
			case "esc":
				cancelEdit()
			case "enter":
				confirmEdit()
			case "backspace", "ctrl+h":
				if len(editContent) > 0 {
					runes := []rune(editContent)
					editContent = string(runes[:len(runes)-1])
					editError = ""
					refreshEditRow()
				}
			case "up", "down", "left", "right", "ctrl+left", "ctrl+right", "alt+left", "alt+right", "tab", "shift+tab":
				// Block all navigation while editing.
			default:
				if keyMsg.Text != "" {
					editContent += strings.ToUpper(keyMsg.Text)
					editError = ""
					refreshEditRow()
				} else {
					return nil, false // pass through ctrl+c etc.
				}
			}
			return nil, true
		}

		// Get the currently focused item.
		items := m.GetItems()
		idx := m.Index()
		if idx < 0 || idx >= len(items) {
			return nil, false
		}
		item := items[idx]

		// findHeader returns the index of the group header for baseApp, searching backward from start.
		findHeader := func(start int, baseApp string) int {
			for i := start; i >= 0; i-- {
				if items[i].IsGroupHeader && items[i].BaseApp == baseApp {
					return i
				}
			}
			return -1
		}

		switch keyMsg.String() {
		case "up":
			navUp()
			return nil, true

		case "down":
			navDown()
			return nil, true

		case "pgup", "ctrl+b", "ctrl+up",
			"ctrl+u": // half-page up
			if isSubRow(item) {
				// Clamp to first sub-row of this group.
				first := idx
				for i := idx - 1; i >= 0; i-- {
					if items[i].IsGroupHeader {
						break
					}
					if isSubRow(items[i]) && items[i].BaseApp == item.BaseApp {
						first = i
					}
				}
				m.Select(first)
			} else {
				// Main list: step backward through main-list items, skipping sub-rows.
				const pageSize = 5
				moved, cur := 0, idx
				for i := idx - 1; i >= 0 && moved < pageSize; i-- {
					if !items[i].IsSeparator && !isSubRow(items[i]) {
						cur = i
						moved++
					}
				}
				m.Select(cur)
			}
			return nil, true

		case "pgdown", "ctrl+f", "ctrl+down",
			"ctrl+d": // half-page down
			if isSubRow(item) {
				// Clamp to last sub-row of this group.
				last := idx
				for i := idx + 1; i < len(items); i++ {
					if !isSubRow(items[i]) || items[i].BaseApp != item.BaseApp {
						break
					}
					last = i
				}
				m.Select(last)
			} else {
				// Main list: step forward through main-list items, skipping sub-rows.
				const pageSize = 5
				moved, cur := 0, idx
				for i := idx + 1; i < len(items) && moved < pageSize; i++ {
					if !items[i].IsSeparator && !isSubRow(items[i]) {
						cur = i
						moved++
					}
				}
				m.Select(cur)
			}
			return nil, true

		case "home", "ctrl+home":
			if isSubRow(item) {
				first := idx
				for i := idx - 1; i >= 0; i-- {
					if items[i].IsGroupHeader {
						break
					}
					if isSubRow(items[i]) && items[i].BaseApp == item.BaseApp {
						first = i
					}
				}
				m.Select(first)
			} else {
				for i := 0; i < len(items); i++ {
					if !items[i].IsSeparator && !isSubRow(items[i]) {
						m.Select(i)
						break
					}
				}
			}
			return nil, true

		case "end", "ctrl+end":
			if isSubRow(item) {
				last := idx
				for i := idx + 1; i < len(items); i++ {
					if !isSubRow(items[i]) || items[i].BaseApp != item.BaseApp {
						break
					}
					last = i
				}
				m.Select(last)
			} else {
				for i := len(items) - 1; i >= 0; i-- {
					if !items[i].IsSeparator && !isSubRow(items[i]) {
						m.Select(i)
						break
					}
				}
			}
			return nil, true

		case "space":
			// Space always toggles the list item — never fires a button.
			// "enter" is not intercepted: falls through to menu_update.go → fires focused button.
			toggleItem(m, idx)
			return nil, true

		case "f2":
			// F2 on a sub-item renames it.
			if item.IsSubItem {
				startRenaming(idx)
				return nil, true
			}
			// F2 on [+] row opens new-instance editing.
			if item.IsAddInstance {
				startEditing(item.BaseApp)
				return nil, true
			}
			return nil, false

		case "ctrl+right", "alt+right":
			if item.IsGroupHeader {
				// Enter submenu: jump to first sub-item, or open editing if none yet.
				base := item.BaseApp
				for i := idx + 1; i < len(items); i++ {
					if items[i].IsGroupHeader {
						break
					}
					if items[i].IsSubItem && items[i].BaseApp == base {
						m.Select(i)
						return nil, true
					}
				}
				startEditing(base)
				return nil, true
			}
			if item.IsSubItem {
				// Ctrl+Right on a sub-item renames it (F2 equivalent).
				startRenaming(idx)
				return nil, true
			}
			// Simple checkbox: expand group and enter submenu (cursor → base sub-row).
			if item.IsCheckbox {
				expandGroup(item.BaseApp)
				return nil, true
			}
			return nil, false

		default:
			// Printable key on [+] Add instance row: open editing and pre-fill the character,
			// preventing the key from falling through to menu_update.go's button hotkeys.
			if item.IsAddInstance && keyMsg.Text != "" {
				startEditing(item.BaseApp)
				editContent = strings.ToUpper(keyMsg.Text)
				editError = ""
				refreshEditRow()
				return nil, true
			}

		case "ctrl+left", "alt+left":
			// Ctrl+Left from anywhere in the submenu: jump to header and collapse if no non-base instances.
			if isSubRow(item) {
				headerIdx := findHeader(idx-1, item.BaseApp)
				if headerIdx >= 0 {
					newItems, collapsed := collapseGroupIfNeeded(items, item.BaseApp)
					if collapsed {
						m.SetItems(newItems)
						m.Select(headerIdx)
					} else {
						m.Select(headerIdx)
					}
				}
				return nil, true
			}
			// Ctrl+Left on group header: collapse if no non-base instances remain.
			if item.IsGroupHeader {
				newItems, collapsed := collapseGroupIfNeeded(items, item.BaseApp)
				if collapsed {
					m.SetItems(newItems)
				}
				return nil, true
			}
			return nil, false
		}

		return nil, false
	})

	refreshItems()
	return &AppSelectionScreen{menu: menu}
}
