package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/displayengine"
	"context"
	"fmt"
)

func (s *AppSelectionScreen) editingTag(niceName, content, errMsg string) string {
	var display string
	if content == "" {
		display = niceName
	} else {
		display = niceName + "__" + appenv.CapitalizeFirstLetter(content)
	}
	if errMsg != "" {
		display += "  {{|TitleError|}}" + errMsg + "{{[-]}}"
	}
	return display
}

func (s *AppSelectionScreen) refreshEditRow() {
	items := s.menu.GetItems()
	if s.editingIdx < 0 || s.editingIdx >= len(items) {
		return
	}
	ctx := context.Background()
	niceName := appenv.GetNiceName(ctx, s.editingBaseApp)
	updated := items[s.editingIdx]
	updated.Tag = s.editingTag(niceName, s.editContent, s.editError)
	s.menu.SetItem(s.editingIdx, updated)
}

func (s *AppSelectionScreen) startEditing(baseApp string) {
	if s.isEditing {
		return
	}
	items := s.menu.GetItems()
	insertAt := -1
	simpleIdx := -1
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
				insertAt = i
				break
			} else {
				break
			}
		}
		if !item.IsGroupHeader && !item.IsSubItem && item.IsCheckbox && item.BaseApp == baseApp {
			simpleIdx = i
			insertAt = i + 1
			break
		}
	}
	if insertAt < 0 {
		return
	}

	editRow := displayengine.MenuItem{
		Tag:        s.editingTag(appenv.GetNiceName(context.Background(), baseApp), "", ""),
		Help:       "Type name of new instance of " + appenv.StyledAppName(context.Background(), baseApp) + ". Press {{|KeyCap|}}[Enter]{{[-]}} to confirm, {{|KeyCap|}}[Esc]{{[-]}} to cancel.",
		IsEditing:  true,
		IsSubItem:  true,
		IsCheckbox: true,
		Selectable: true,
		Checked:    true,
		BaseApp:    baseApp,
	}
	workItems := items
	if simpleIdx >= 0 {
		ctx := context.Background()
		niceName := appenv.GetNiceName(ctx, baseApp)
		orig := items[simpleIdx]
		s.convertedSimpleOriginal = orig
		promoted := displayengine.MenuItem{
			Tag:           orig.Tag,
			Desc:          orig.Desc,
			Help:          fmt.Sprintf("Press Ctrl/Alt+Right to manage %s instances", niceName),
			IsGroupHeader: true,
			Checked:       orig.Checked,
			IsReferenced:  orig.IsReferenced,
			BaseApp:       baseApp,
			Metadata:      orig.Metadata,
		}
		workItems = make([]displayengine.MenuItem, len(items))
		copy(workItems, items)
		workItems[simpleIdx] = promoted
		s.convertedFromSimple = true
	}

	newItems := make([]displayengine.MenuItem, 0, len(workItems)+1)
	newItems = append(newItems, workItems[:insertAt]...)
	newItems = append(newItems, editRow)
	newItems = append(newItems, workItems[insertAt:]...)

	s.isEditing = true
	s.editingBaseApp = baseApp
	s.editingIdx = insertAt
	s.editContent = ""
	s.editError = ""

	s.menu.SetItems(newItems)
	s.menu.Select(insertAt)
}

func (s *AppSelectionScreen) startRenaming(subIdx int) {
	if s.isEditing {
		return
	}
	items := s.menu.GetItems()
	if subIdx < 0 || subIdx >= len(items) || !items[subIdx].IsSubItem {
		return
	}
	if items[subIdx].IsReferenced || items[subIdx].WasAdded {
		return
	}
	subItem := items[subIdx]
	suffix := appenv.AppNameToInstanceName(subItem.Metadata["appName"])

	editRow := displayengine.MenuItem{
		Tag:        s.editingTag(appenv.GetNiceName(context.Background(), subItem.BaseApp), suffix, ""),
		Help:       "Rename instance " + appenv.StyledInstanceName(context.Background(), subItem.Metadata["appName"]) + ". Press {{|KeyCap|}}[Enter]{{[-]}} to confirm, {{|KeyCap|}}[Esc]{{[-]}} to cancel.",
		IsEditing:  true,
		IsSubItem:  true,
		IsCheckbox: subItem.IsCheckbox,
		Selectable: true,
		Checked:    subItem.Checked,
		BaseApp:    subItem.BaseApp,
	}

	newItems := make([]displayengine.MenuItem, len(items))
	copy(newItems, items)
	newItems[subIdx] = editRow

	s.isEditing = true
	s.isRenaming = true
	s.renamingOriginal = subItem
	s.editingBaseApp = subItem.BaseApp
	s.editingIdx = subIdx
	s.editContent = suffix
	s.editError = ""

	s.menu.SetItems(newItems)
	s.menu.Select(subIdx)
	// Renaming a name is conceptually "on" that name, which for sub-items sits
	// right after Enable (there's no dedicated Name column for sub-items the
	// way the top-level Add/Enable/Expand/Name row has one) -- so landing back
	// on Enable when Esc cancels the edit matches that ordering, regardless of
	// which column was active before editing started (e.g. a direct click on
	// the name itself, which doesn't otherwise touch the active column).
	s.menu.SetActiveColumn(displayengine.ColEnable)
}

func (s *AppSelectionScreen) cancelEdit() {
	items := s.menu.GetItems()
	newItems := make([]displayengine.MenuItem, 0, len(items))
	if s.isRenaming {
		for i, item := range items {
			if i == s.editingIdx {
				newItems = append(newItems, s.renamingOriginal)
			} else {
				newItems = append(newItems, item)
			}
		}
	} else if s.convertedFromSimple {
		for i, item := range items {
			if item.IsGroupHeader && item.BaseApp == s.editingBaseApp {
				newItems = append(newItems, s.convertedSimpleOriginal)
			} else if i == s.editingIdx {
				// skip
			} else {
				newItems = append(newItems, item)
			}
		}
	} else {
		for i, item := range items {
			if i != s.editingIdx {
				newItems = append(newItems, item)
			}
		}
	}
	s.isEditing = false
	s.isRenaming = false
	s.convertedFromSimple = false
	s.convertedSimpleOriginal = displayengine.MenuItem{}
	s.renamingOriginal = displayengine.MenuItem{}
	s.editingBaseApp = ""
	s.editContent = ""
	s.editError = ""
	s.editingIdx = -1
	s.menu.SetItems(newItems)
}

func (s *AppSelectionScreen) confirmEdit() {
	base := s.editingBaseApp
	suffix := s.editContent
	var newAppName string
	if suffix == "" {
		newAppName = base
	} else {
		newAppName = base + "__" + suffix
	}
	if s.isRenaming && newAppName == s.renamingOriginal.Metadata["appName"] {
		s.cancelEdit()
		return
	}
	if suffix != "" && !appenv.InstanceNameIsValid(suffix) {
		s.editError = "reserved name"
		s.refreshEditRow()
		return
	}
	if !appenv.IsAppNameValid(newAppName) {
		s.editError = "invalid name"
		s.refreshEditRow()
		return
	}
	items := s.menu.GetItems()
	for _, item := range items {
		if item.IsSubItem && item.BaseApp == base && item.Metadata["appName"] == newAppName {
			s.editError = "already exists"
			s.refreshEditRow()
			return
		}
	}
	ctx := context.Background()
	baseNiceName := appenv.GetNiceName(ctx, base)
	displayName := appenv.InstanceDisplayName(baseNiceName, newAppName)
	checkedState := true
	enabledState := true
	if s.isRenaming {
		checkedState = s.renamingOriginal.Checked
		enabledState = s.renamingOriginal.Enabled
	}
	newSubRow := displayengine.MenuItem{
		Tag:               displayName,
		Help:              fmt.Sprintf("Toggle %s", displayName),
		IsSubItem:         true,
		IsCheckbox:        true,
		Selectable:        true,
		IsNew:             true,
		Checked:           checkedState,
		Enabled:           enabledState && checkedState,
		ShowEnabledGutter: checkedState,
		BaseApp:           base,
		Metadata:          map[string]string{"appName": newAppName},
	}
	newItems := make([]displayengine.MenuItem, 0, len(items))
	for i, item := range items {
		if i == s.editingIdx {
			newItems = append(newItems, newSubRow)
		} else {
			newItems = append(newItems, item)
		}
	}
	newItems = s.refreshGroupHeaders(newItems)
	if suffix != "" && !s.isRenaming {
		cleaned := newItems[:0:len(newItems)]
		for _, it := range newItems {
			if it.IsSubItem && it.BaseApp == base && it.Metadata["appName"] == base && !it.Checked && !it.IsReferenced {
				continue
			}
			cleaned = append(cleaned, it)
		}
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
	s.isEditing = false
	s.isRenaming = false
	s.convertedFromSimple = false
	s.convertedSimpleOriginal = displayengine.MenuItem{}
	s.renamingOriginal = displayengine.MenuItem{}
	s.editingBaseApp = ""
	s.editContent = ""
	s.editError = ""
	s.editingIdx = -1
	s.menu.SetItems(newItems)
}
