package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/displayengine"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// appSelectShowSpinnerMsg is sent after a short delay to show the loading spinner,
// but only if the app list hasn't finished loading yet.
type appSelectShowSpinnerMsg struct{}
type appSelectAppliedMsg struct{}

func showSpinnerAfterDelayCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return appSelectShowSpinnerMsg{}
	})
}

// appSelectLoadedMsg carries the result of the async app list load.
type appSelectLoadedMsg struct {
	baseApps          []string
	addedByBase       map[string][]string
	addedMap          map[string]bool
	enabledMap        map[string]bool
	wasEnabledMap     map[string]bool
	referencedByBase  map[string][]string
	referencedBaseSet map[string]bool
	envFile           string
}

// loadAppSelectItemsCmd returns a tea.Cmd that performs all filesystem I/O for
// the app selection list in a background goroutine.
func (s *AppSelectionScreen) loadAppSelectItemsCmd() tea.Cmd {
	conf := s.conf
	return func() tea.Msg {
		ctx := context.Background()
		envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)

		nonDeprecated, err := appenv.ListNonDeprecatedApps(ctx)
		if err != nil {
			nonDeprecated = []string{}
		}
		added, err := appenv.ListAddedApps(ctx, envFile)
		if err != nil {
			added = []string{}
		}

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

		enabledApps, err := appenv.ListEnabledApps(conf)
		if err != nil {
			enabledApps = []string{}
		}
		enabledMap := make(map[string]bool)
		for _, a := range enabledApps {
			enabledMap[a] = true
		}
		wasEnabledMap := enabledMap

		addedMap := make(map[string]bool)
		for _, a := range added {
			addedMap[a] = true
		}

		referenced, err := appenv.ListReferencedApps(ctx, conf)
		if err != nil {
			referenced = []string{}
		}
		referencedByBase := make(map[string][]string)
		referencedBaseSet := make(map[string]bool)
		for _, r := range referenced {
			base := appenv.AppNameToBaseAppName(r)
			if base == r {
				if !addedMap[r] {
					referencedBaseSet[r] = true
				}
				continue
			}
			if addedMap[r] {
				continue
			}
			referencedByBase[base] = append(referencedByBase[base], r)
		}

		var baseApps []string
		for base := range baseAppsSet {
			baseApps = append(baseApps, base)
		}
		slices.Sort(baseApps)

		return appSelectLoadedMsg{
			baseApps:          baseApps,
			addedByBase:       addedByBase,
			addedMap:          addedMap,
			enabledMap:        enabledMap,
			wasEnabledMap:     wasEnabledMap,
			referencedByBase:  referencedByBase,
			referencedBaseSet: referencedBaseSet,
			envFile:           envFile,
		}
	}
}

// applyLoadedItems populates the menu from data returned by loadAppSelectItemsCmd.
func (s *AppSelectionScreen) applyLoadedItems(data appSelectLoadedMsg) {
	ctx := context.Background()
	baseApps := data.baseApps
	addedByBase := data.addedByBase
	addedMap := data.addedMap
	enabledMap := data.enabledMap
	wasEnabledMap := data.wasEnabledMap
	referencedByBase := data.referencedByBase
	referencedBaseSet := data.referencedBaseSet
	envFile := data.envFile

	var items []displayengine.MenuItem
	var lastLetter string

	for _, base := range baseApps {
		letter := ""
		if len(base) > 0 {
			letter = strings.ToUpper(base[:1])
		}
		if letter != lastLetter && letter != "" {
			if lastLetter != "" {
				items = append(items, displayengine.MenuItem{IsSeparator: true})
			}
			lastLetter = letter
		}

		niceName := appenv.GetNiceName(ctx, base)
		desc := displayengine.GetPlainText(appenv.GetDescriptionFromTemplate(ctx, base, envFile))

		instances := addedByBase[base]
		slices.Sort(instances)
		refInstances := referencedByBase[base]
		slices.Sort(refInstances)

		var nonBaseCount int
		for _, inst := range instances {
			if appenv.AppNameToInstanceName(inst) != "" {
				nonBaseCount++
			}
		}
		nonBaseCount += len(refInstances)

		docsURL := appenv.AppURL(base)

		if nonBaseCount == 0 {
			items = append(items, displayengine.MenuItem{
				Tag:               niceName,
				Desc:              "{{|ListItem|}}" + desc,
				Help:              appenv.StyledAppName(ctx, base),
				Selectable:        true,
				IsCheckbox:        true,
				Checked:           addedMap[base],
				WasAdded:          addedMap[base],
				Enabled:           enabledMap[base],
				WasEnabled:        wasEnabledMap[base],
				ShowEnabledGutter: addedMap[base],
				IsReferenced:      referencedBaseSet[base],
				BaseApp:           base,
				IsDestructive:     true,
				Metadata:          map[string]string{"appName": base, "docsURL": docsURL},
			})
		} else {
			anyEnabled := false
			for _, inst := range instances {
				if enabledMap[inst] {
					anyEnabled = true
					break
				}
			}

			items = append(items, displayengine.MenuItem{
				Tag:               niceName,
				Desc:              "{{|ListItem|}}" + desc,
				Help:              fmt.Sprintf("Press Ctrl/Alt+Right to manage %s instances", niceName),
				Selectable:        true,
				IsGroupHeader:     true,
				Checked:           anyEnabled,
				IsCheckbox:        false,
				Enabled:           enabledMap[base],
				WasEnabled:        wasEnabledMap[base],
				ShowEnabledGutter: false,
				IsReferenced:      referencedBaseSet[base],
				BaseApp:           base,
				IsDestructive:     true,
				Metadata:          map[string]string{"appName": base, "docsURL": docsURL},
			})

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
					items = append(items, displayengine.MenuItem{
						Tag:               displayName,
						Help:              fmt.Sprintf("%s — referenced in config but not added", displayName),
						Selectable:        true,
						IsSubItem:         true,
						IsCheckbox:        true,
						IsReferenced:      true,
						Checked:           false,
						WasAdded:          false,
						Enabled:           false,
						WasEnabled:        false,
						ShowEnabledGutter: false,
						BaseApp:           base,
						IsDestructive:     true,
						Metadata:          map[string]string{"appName": ie.appName, "docsURL": docsURL},
					})
				} else {
					items = append(items, displayengine.MenuItem{
						Tag:               displayName,
						Help:              appenv.StyledAppName(ctx, ie.appName),
						Selectable:        true,
						IsSubItem:         true,
						IsCheckbox:        true,
						Checked:           addedMap[ie.appName],
						WasAdded:          addedMap[ie.appName],
						Enabled:           enabledMap[ie.appName],
						WasEnabled:        wasEnabledMap[ie.appName],
						ShowEnabledGutter: addedMap[ie.appName],
						BaseApp:           base,
						IsDestructive:     true,
						Metadata:          map[string]string{"appName": ie.appName, "docsURL": docsURL},
					})
				}
			}
			items = append(items, displayengine.MenuItem{
				Tag:           "+ Add instance\u2026",
				Help:          fmt.Sprintf("Press Space/Enter or Ctrl/Alt+Right to add another %s instance", niceName),
				IsAddInstance: true,
				IsDestructive: true,
				BaseApp:       base,
			})
		}
	}

	s.menu.SetItems(items)
}

func (s *AppSelectionScreen) expandGroup(baseApp string) {
	if s.isEditing {
		return
	}
	items := s.menu.GetItems()
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
	envFile := filepath.Join(s.conf.ComposeDir, constants.EnvFileName)
	niceName := appenv.GetNiceName(ctx, baseApp)
	desc := displayengine.GetPlainText(appenv.GetDescriptionFromTemplate(ctx, baseApp, envFile))
	groupHeader := displayengine.MenuItem{
		Tag:           niceName,
		Desc:          "{{|ListItem|}}" + desc,
		Help:          fmt.Sprintf("Press Ctrl/Alt+Right to manage %s instances. Ctrl/Alt+Left to collapse.", niceName),
		IsGroupHeader: true,
		Checked:       orig.Checked,
		WasAdded:      orig.WasAdded,
		Enabled:       orig.Enabled,
		WasEnabled:    orig.WasEnabled,
		Selectable:    true,
		IsReferenced:  orig.IsReferenced,
		BaseApp:       baseApp,
		IsDestructive: true,
		Metadata:      orig.Metadata,
	}
	baseSubRow := displayengine.MenuItem{
		Tag:               niceName,
		Help:              appenv.StyledAppName(ctx, baseApp),
		IsSubItem:         true,
		IsCheckbox:        true,
		Selectable:        true,
		Checked:           orig.Checked,
		WasAdded:          orig.WasAdded,
		Enabled:           orig.Enabled,
		WasEnabled:        orig.WasEnabled,
		ShowEnabledGutter: orig.Checked,
		IsReferenced:      orig.IsReferenced,
		BaseApp:           baseApp,
		IsDestructive:     true,
		Metadata:          orig.Metadata,
	}
	addRow := displayengine.MenuItem{
		Tag:           "+ Add instance\u2026",
		Help:          fmt.Sprintf("Press Space/Enter or Ctrl/Alt+Right to add a %s instance", niceName),
		IsAddInstance: true,
		Selectable:    true,
		IsDestructive: true,
		BaseApp:       baseApp,
	}
	newItems := make([]displayengine.MenuItem, 0, len(items)+2)
	for i, item := range items {
		if i == simpleIdx {
			newItems = append(newItems, groupHeader, baseSubRow, addRow)
		} else {
			newItems = append(newItems, item)
		}
	}
	s.menu.SetItems(newItems)
	s.menu.Select(simpleIdx + 1)
}

func (s *AppSelectionScreen) isPhantom(it displayengine.MenuItem) bool {
	if !it.IsSubItem || it.IsEditing {
		return false
	}
	// Only named instances can be phantoms (the base instance row is preserved)
	if appenv.AppNameToInstanceName(it.Metadata["appName"]) == "" {
		return false
	}
	return !it.Checked && !it.IsReferenced && !it.WasAdded
}

func (s *AppSelectionScreen) collapseGroupIfNeeded(items []displayengine.MenuItem, base string) ([]displayengine.MenuItem, bool) {
	var activeNamedCount int
	for _, item := range items {
		if item.IsSubItem && item.BaseApp == base && !s.isPhantom(item) && appenv.AppNameToInstanceName(item.Metadata["appName"]) != "" {
			activeNamedCount++
		}
	}
	if activeNamedCount > 0 {
		return items, false
	}
	var wasAdded, checked, isReferenced, enabled, wasEnabled bool
	for _, item := range items {
		if item.IsGroupHeader && item.BaseApp == base {
			wasAdded = item.WasAdded
			isReferenced = item.IsReferenced
			checked = item.WasAdded
			wasEnabled = item.WasEnabled
		}
		if item.IsSubItem && item.BaseApp == base && appenv.AppNameToInstanceName(item.Metadata["appName"]) == "" {
			checked = item.Checked
			enabled = item.Enabled
		}
	}
	ctx := context.Background()
	envFile := filepath.Join(s.conf.ComposeDir, constants.EnvFileName)
	niceName := appenv.GetNiceName(ctx, base)
	desc := displayengine.GetPlainText(appenv.GetDescriptionFromTemplate(ctx, base, envFile))
	simpleRow := displayengine.MenuItem{
		Tag:               niceName,
		Desc:              "{{|ListItem|}}" + desc,
		Help:              appenv.StyledAppName(ctx, base),
		IsCheckbox:        true,
		Selectable:        true,
		Checked:           checked,
		WasAdded:          wasAdded,
		Enabled:           enabled,
		WasEnabled:        wasEnabled,
		ShowEnabledGutter: checked,
		IsReferenced:      isReferenced,
		BaseApp:           base,
		IsDestructive:     true,
		Metadata:          map[string]string{"appName": base, "docsURL": appenv.AppURL(base)},
	}
	newItems := make([]displayengine.MenuItem, 0, len(items))
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

// sortGroupInstances re-sorts base's subitem rows by instance appName,
// in place at their existing (contiguous) positions -- the header and
// "+ Add instance..." row, if present, keep their own positions relative to
// that block. Matches the ordering buildAllItems/expandGroup already use
// when first constructing a group, which confirmEdit's in-place
// insert/replace doesn't otherwise preserve.
func sortGroupInstances(items []displayengine.MenuItem, base string) []displayengine.MenuItem {
	var idxs []int
	for i, it := range items {
		if it.IsSubItem && it.BaseApp == base {
			idxs = append(idxs, i)
		}
	}
	if len(idxs) < 2 {
		return items
	}
	sub := make([]displayengine.MenuItem, len(idxs))
	for k, i := range idxs {
		sub[k] = items[i]
	}
	slices.SortFunc(sub, func(a, b displayengine.MenuItem) int {
		return strings.Compare(a.Metadata["appName"], b.Metadata["appName"])
	})
	for k, i := range idxs {
		items[i] = sub[k]
	}
	return items
}

func (s *AppSelectionScreen) collapseAllEmptyGroups(skipBase string) {
	cur := s.menu.GetItems()

	// Snapshot the currently selected row's identity before anything moves,
	// so it can be re-found afterward regardless of how many rows above it
	// got removed by a collapse elsewhere in the list -- selection is a raw
	// index, and any collapse before that index shifts everything after it
	// without this.
	prevIdx := s.menu.Index()
	var selAppName string
	var selBase string
	var selIsGroupHeader, selIsAddInstance bool
	haveSel := prevIdx >= 0 && prevIdx < len(cur)
	if haveSel {
		sel := cur[prevIdx]
		selAppName = sel.Metadata["appName"]
		selBase = sel.BaseApp
		selIsGroupHeader = sel.IsGroupHeader
		selIsAddInstance = sel.IsAddInstance
	}

	// Prune phantoms first
	pruned := make([]displayengine.MenuItem, 0, len(cur))
	for _, it := range cur {
		if s.isPhantom(it) {
			continue
		}
		pruned = append(pruned, it)
	}
	cur = pruned

	changed := len(cur) != len(s.menu.GetItems())

	seen := map[string]bool{}
	for _, it := range cur {
		if it.IsGroupHeader && it.BaseApp != skipBase && !seen[it.BaseApp] {
			seen[it.BaseApp] = true
		}
	}
	for base := range seen {
		if newItems, ok := s.collapseGroupIfNeeded(cur, base); ok {
			cur = newItems
			changed = true
		}
	}
	if !changed {
		return
	}
	s.menu.SetItems(cur)
	if !haveSel {
		return
	}
	// Match on appName plus role together -- a group header and its own
	// unnamed base subitem legitimately share the same appName (both copy
	// the base row's Metadata in expandGroup), so appName alone isn't a
	// unique key and would let the header steal a subitem's reselect.
	if selAppName != "" {
		for i, it := range cur {
			if it.Metadata["appName"] == selAppName && it.IsGroupHeader == selIsGroupHeader && it.IsAddInstance == selIsAddInstance {
				s.menu.Select(i)
				return
			}
		}
	}
	for i, it := range cur {
		if it.BaseApp != selBase {
			continue
		}
		if it.IsGroupHeader == selIsGroupHeader && it.IsAddInstance == selIsAddInstance {
			s.menu.Select(i)
			return
		}
	}
}

func (s *AppSelectionScreen) refreshGroupHeaders(items []displayengine.MenuItem) []displayengine.MenuItem {
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
