package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/tui"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

func (s *AppSelectionScreen) refreshItems() {
	ctx := context.Background()
	envFile := filepath.Join(s.conf.ComposeDir, constants.EnvFileName)

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

	enabledApps, err := appenv.ListEnabledApps(s.conf)
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

	referenced, err := appenv.ListReferencedApps(ctx, s.conf)
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

	var items []tui.MenuItem
	var lastLetter string

	for _, base := range baseApps {
		letter := ""
		if len(base) > 0 {
			letter = strings.ToUpper(base[:1])
		}
		if letter != lastLetter && letter != "" {
			if lastLetter != "" {
				items = append(items, tui.MenuItem{IsSeparator: true})
			}
			lastLetter = letter
		}

		niceName := appenv.GetNiceName(ctx, base)
		desc := tui.GetPlainText(appenv.GetDescriptionFromTemplate(ctx, base, envFile))

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

		if nonBaseCount == 0 {
			items = append(items, tui.MenuItem{
				Tag:               niceName,
				Desc:              "{{|ListApp|}}" + desc,
				Help:              fmt.Sprintf("Toggle %s. Press Ctrl/Alt+Right to manage instances.", niceName),
				Selectable:        true,
				IsCheckbox:        true,
				Checked:           addedMap[base],
				WasAdded:          addedMap[base],
				Enabled:           enabledMap[base],
				WasEnabled:        wasEnabledMap[base],
				ShowEnabledGutter: addedMap[base],
				IsReferenced:      referencedBaseSet[base],
				BaseApp:           base,
				Metadata:          map[string]string{"appName": base},
			})
		} else {
			anyEnabled := false
			for _, inst := range instances {
				if enabledMap[inst] {
					anyEnabled = true
					break
				}
			}

			items = append(items, tui.MenuItem{
				Tag:               niceName,
				Desc:              "{{|ListApp|}}" + desc,
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
				Metadata:          map[string]string{"appName": base},
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
					items = append(items, tui.MenuItem{
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
						Metadata:          map[string]string{"appName": ie.appName},
					})
				} else {
					items = append(items, tui.MenuItem{
						Tag:               displayName,
						Help:              fmt.Sprintf("Toggle %s", displayName),
						Selectable:        true,
						IsSubItem:         true,
						IsCheckbox:        true,
						Checked:           addedMap[ie.appName],
						WasAdded:          addedMap[ie.appName],
						Enabled:           enabledMap[ie.appName],
						WasEnabled:        wasEnabledMap[ie.appName],
						ShowEnabledGutter: addedMap[ie.appName],
						BaseApp:           base,
						Metadata:          map[string]string{"appName": ie.appName},
					})
				}
			}
			items = append(items, tui.MenuItem{
				Tag:           "+ Add instance\u2026",
				Help:          fmt.Sprintf("Press Space/Enter or Ctrl/Alt+Right to add another %s instance", niceName),
				IsAddInstance: true,
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
	desc := tui.GetPlainText(appenv.GetDescriptionFromTemplate(ctx, baseApp, envFile))
	groupHeader := tui.MenuItem{
		Tag:               niceName,
		Desc:              "{{|ListApp|}}" + desc,
		Help:              fmt.Sprintf("Press Ctrl/Alt+Right to manage %s instances. Ctrl/Alt+Left to collapse.", niceName),
		IsGroupHeader:     true,
		Checked:           orig.Checked,
		WasAdded:          orig.WasAdded,
		Enabled:           orig.Enabled,
		WasEnabled:        orig.WasEnabled,
		Selectable:        true,
		IsReferenced:      orig.IsReferenced,
		BaseApp:           baseApp,
		Metadata:          orig.Metadata,
	}
	baseSubRow := tui.MenuItem{
		Tag:               niceName,
		Help:              fmt.Sprintf("Toggle %s", niceName),
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
		Metadata:          orig.Metadata,
	}
	addRow := tui.MenuItem{
		Tag:           "+ Add instance\u2026",
		Help:          fmt.Sprintf("Press Space/Enter or Ctrl/Alt+Right to add a %s instance", niceName),
		IsAddInstance: true,
		Selectable:    true,
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
	s.menu.SetItems(newItems)
	s.menu.Select(simpleIdx + 1)
}

func (s *AppSelectionScreen) isPhantom(it tui.MenuItem) bool {
	if !it.IsSubItem || it.IsEditing {
		return false
	}
	// Only named instances can be phantoms (the base instance row is preserved)
	if appenv.AppNameToInstanceName(it.Metadata["appName"]) == "" {
		return false
	}
	return !it.Checked && !it.IsReferenced && !it.WasAdded
}

func (s *AppSelectionScreen) collapseGroupIfNeeded(items []tui.MenuItem, base string) ([]tui.MenuItem, bool) {
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
	desc := tui.GetPlainText(appenv.GetDescriptionFromTemplate(ctx, base, envFile))
	simpleRow := tui.MenuItem{
		Tag:               niceName,
		Desc:              "{{|ListApp|}}" + desc,
		Help:              fmt.Sprintf("Toggle %s. Press Ctrl/Alt+Right to manage instances.", niceName),
		IsCheckbox:        true,
		Selectable:        true,
		Checked:           checked,
		WasAdded:          wasAdded,
		Enabled:           enabled,
		WasEnabled:        wasEnabled,
		ShowEnabledGutter: checked,
		IsReferenced:      isReferenced,
		BaseApp:           base,
		Metadata:          map[string]string{"appName": base},
	}
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

func (s *AppSelectionScreen) collapseAllEmptyGroups(skipBase string) {
	cur := s.menu.GetItems()

	// Prune phantoms first
	pruned := make([]tui.MenuItem, 0, len(cur))
	for _, it := range cur {
		if s.isPhantom(it) {
			continue
		}
		pruned = append(pruned, it)
	}
	cur = pruned

	changed := false
	if len(cur) != len(s.menu.GetItems()) {
		changed = true
	}

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
	if changed {
		s.menu.SetItems(cur)
		if skipBase != "" {
			for i, it := range cur {
				if it.BaseApp == skipBase {
					s.menu.Select(i)
					return
				}
			}
		}
	}
}

func (s *AppSelectionScreen) refreshGroupHeaders(items []tui.MenuItem) []tui.MenuItem {
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
