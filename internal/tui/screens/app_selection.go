package screens

import (
	"context"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const appSelectionLegend = "| " +
	"{{|MarkerAdded|}}+{{[-]}} Added | " +
	"{{|MarkerDeleted|}}-{{[-]}} Removed | " +
	"{{|MarkerModified|}}r{{[-]}} Referenced | " +
	"{{|MarkerAdded|}}R{{[-]}} Referenced \u0026 Added | " +
	"{{|MarkerAdded|}}E{{[-]}} Enabled | " +
	"{{|MarkerDeleted|}}D{{[-]}} Disabled |"

// AppSelectionScreen wraps MenuModel to provide a custom Legend help panel.
type AppSelectionScreen struct {
	menu *tui.MenuModel
	conf config.AppConfig

	// Inline editing state
	isEditing               bool
	editingBaseApp          string
	editingIdx              int
	editContent             string
	editError               string
	isRenaming              bool
	renamingOriginal        tui.MenuItem
	convertedFromSimple     bool
	convertedSimpleOriginal tui.MenuItem
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
	return s.menu.HelpContext(maxWidth)
}

// NewAppSelectionScreen creates the app selection screen.
func NewAppSelectionScreen(conf config.AppConfig, isRoot bool) *AppSelectionScreen {
	s := &AppSelectionScreen{
		conf: conf,
	}

	var backAction tea.Cmd
	if !isRoot {
		backAction = func() tea.Msg {
			cs := s.computeChanges()
			msg := s.buildChangeSummary(cs)
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
		"Choose which apps you would like to install:\nUse {{|KeyCap|}}[up]{{[-]}}/{{|KeyCap|}}[down]{{[-]}} and {{|KeyCap|}}[space]{{[-]}} to select; {{|KeyCap|}}[ctrl+←/→]{{[-]}} to move between Add/Enable columns.",
		nil,
		backAction,
	)
	s.menu = menu

	menu.SetMenuName("app-select")
	menu.SetHelpItemPrefix("App")
	menu.SetItemHelpFunc(func(item tui.MenuItem) (itemTitle, itemText string) {
		if item.BaseApp == "" || item.IsSeparator {
			return "", ""
		}
		ctx := context.Background()
		appMeta, _ := appenv.LoadAppMeta(ctx, item.BaseApp)
		var parts []string
		if appMeta != nil && appMeta.App.HelpText != "" {
			parts = append(parts, appMeta.App.HelpText)
		} else if desc := appenv.GetDescriptionFromTemplate(ctx, item.BaseApp, ""); desc != "" {
			parts = append(parts, desc)
		}
		if appMeta != nil && appMeta.App.Website != "" {
			parts = append(parts, "Website: {{|url|}}"+appMeta.App.Website+"{{[-]}}")
		}
		if appenv.IsAppDeprecated(ctx, item.BaseApp) {
			parts = append(parts, "{{|TitleError|}}⚠ This app is deprecated.{{[-]}}")
		}
		if len(parts) == 0 {
			return "", ""
		}
		return item.Tag, strings.Join(parts, "\n\n")
	})
	menu.SetButtonLabels("Done", "Back", "Exit")
	menu.SetShowExit(true)
	menu.SetGroupedMode(true)
	menu.SetVariableHeight(true)
	menu.SetMaximized(true)
	menu.SetSubMenuMode(false)
	menu.SetFocusedItem(tui.FocusSelectBtn)

	menu.SetEnterAction(func() tea.Msg {
		return s.handleSave()
	})

	menu.SetUpdateInterceptor(s.updateInterceptor)

	menu.SetHelpLegend(appSelectionLegend)
	s.refreshItems()

	return s
}

func (s *AppSelectionScreen) toggleItem(idx int) {
	items := s.menu.GetItems()
	if idx < 0 || idx >= len(items) {
		return
	}
	item := items[idx]
	if item.IsSeparator || item.IsEditing {
		return
	}
	if item.IsAddInstance {
		s.startEditing(item.BaseApp)
		return
	}

	col := s.menu.ActiveColumn()

	if col == tui.ColAdd {
		item.Checked = !item.Checked
		if item.Checked {
			item.Enabled = true // Auto-enable when adding
		} else {
			item.Enabled = false // Auto-disable when removing
		}
		item.ShowEnabledGutter = item.Checked
	} else if col == tui.ColEnable {
		item.Enabled = !item.Enabled
		if item.Enabled {
			item.Checked = true // Auto-add if user enables
			item.ShowEnabledGutter = true
		}
	}

	items[idx] = item
	if item.IsSubItem {
		items = s.refreshGroupHeaders(items)
	}
	s.menu.SetItems(items)
}

func (s *AppSelectionScreen) updateInterceptor(msg tea.Msg, m *tui.MenuModel) (tea.Cmd, bool) {
	isSubRow := func(it tui.MenuItem) bool {
		return it.IsSubItem || it.IsAddInstance || it.IsEditing
	}

	if hitMsg, ok := msg.(tui.LayerHitMsg); ok && hitMsg.Button == tea.MouseLeft {
		if s.isEditing {
			return nil, true
		}
		if idx, ok := tui.ParseMenuItemIndex(hitMsg.ID, m.ID()); ok {
			items := m.GetItems()
			if idx >= 0 && idx < len(items) {
				item := items[idx]
				m.Select(idx)
				if item.IsSeparator || item.IsEditing {
					return nil, true
				}

				var addCols, enableCols [2]int
				var nameStart int

				if item.IsSubItem || item.IsAddInstance || item.IsEditing {
					addCols = [2]int{15, 18}
					enableCols = [2]int{19, 22}
					nameStart = 23
				} else {
					addCols = [2]int{5, 8}
					enableCols = [2]int{9, 12}
					nameStart = 14
				}

				if item.IsAddInstance {
					s.startEditing(item.BaseApp)
					return nil, true
				}
				if hitMsg.X >= addCols[0] && hitMsg.X <= addCols[1] && !item.IsGroupHeader {
					m.SetActiveColumn(tui.ColAdd)
					s.toggleItem(idx)
					return nil, true
				}
				if hitMsg.X >= enableCols[0] && hitMsg.X <= enableCols[1] && !item.IsGroupHeader {
					m.SetActiveColumn(tui.ColEnable)
					s.toggleItem(idx)
					return nil, true
				}
				if hitMsg.X >= nameStart {
					base := item.BaseApp
					m.Select(idx)
					if item.IsSubItem {
						if !item.IsReferenced && !item.WasAdded {
							s.startRenaming(idx)
						}
					} else if item.IsGroupHeader {
						if ni, ok := s.collapseGroupIfNeeded(m.GetItems(), base); ok {
							m.SetItems(ni)
							for i, it := range ni {
								if it.BaseApp == base {
									m.Select(i)
									break
								}
							}
						}
						s.collapseAllEmptyGroups(base)
					} else {
						s.expandGroup(base)
						s.collapseAllEmptyGroups(base)
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

	if s.isEditing {
		switch keyMsg.String() {
		case "esc":
			s.cancelEdit()
		case "enter":
			s.confirmEdit()
		case "backspace", "ctrl+h":
			if len(s.editContent) > 0 {
				runes := []rune(s.editContent)
				s.editContent = string(runes[:len(runes)-1])
				s.editError = ""
				s.refreshEditRow()
			}
		case "up", "down", "left", "right", "ctrl+left", "ctrl+right", "alt+left", "alt+right", "tab", "shift+tab":
		default:
			if keyMsg.Text != "" {
				s.editContent += strings.ToUpper(keyMsg.Text)
				s.editError = ""
				s.refreshEditRow()
			} else {
				return nil, false
			}
		}
		return nil, true
	}

	items := m.GetItems()
	idx := m.Index()
	if idx < 0 || idx >= len(items) {
		return nil, false
	}
	item := items[idx]

	navUp := func() {
		if idx <= 0 {
			return
		}
		prevBase := item.BaseApp
		if isSubRow(item) {
			// Constrain to this group
			if idx > 0 && items[idx-1].BaseApp == prevBase && !items[idx-1].IsSeparator {
				// Don't go back into header from sub-row via Up
				if items[idx-1].IsGroupHeader {
					return
				}
				m.Select(idx - 1)
			}
			return
		}
		// Main list: skip all sub-items and go to previous Header
		for i := idx - 1; i >= 0; i-- {
			if !items[i].IsSeparator && !isSubRow(items[i]) {
				m.Select(i)
				// Smart Collapse: collapse the group we just left
				if ni, ok := s.collapseGroupIfNeeded(m.GetItems(), prevBase); ok {
					m.SetItems(ni)
					for j, it := range ni {
						if it.BaseApp == items[i].BaseApp {
							m.Select(j)
							break
						}
					}
				}
				break
			}
		}
	}

	navDown := func() {
		if idx >= len(items)-1 {
			return
		}
		prevBase := item.BaseApp
		if isSubRow(item) {
			// Constrain to this group
			if idx+1 < len(items) && items[idx+1].BaseApp == prevBase && !items[idx+1].IsSeparator {
				m.Select(idx + 1)
			}
			return
		}
		// Main list: skip all sub-items and go to next Header
		for i := idx + 1; i < len(items); i++ {
			if !items[i].IsSeparator && !isSubRow(items[i]) {
				m.Select(i)
				// Smart Collapse: collapse the group we just left
				if ni, ok := s.collapseGroupIfNeeded(m.GetItems(), prevBase); ok {
					m.SetItems(ni)
					for j, it := range ni {
						if it.BaseApp == items[i].BaseApp {
							m.Select(j)
							break
						}
					}
				}
				break
			}
		}
	}

	switch keyMsg.String() {
	case "up":
		navUp()
		return nil, true
	case "down":
		navDown()
		return nil, true
	case "pgup", "ctrl+b", "ctrl+up", "ctrl+u":
		if isSubRow(item) {
			first := idx
			for i := idx - 1; i >= 0; i-- {
				if items[i].BaseApp != item.BaseApp {
					break
				}
				first = i
			}
			m.Select(first)
		} else {
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
	case "pgdown", "ctrl+f", "ctrl+down", "ctrl+d":
		if isSubRow(item) {
			last := idx
			for i := idx + 1; i < len(items); i++ {
				if items[i].BaseApp != item.BaseApp {
					break
				}
				last = i
			}
			m.Select(last)
		} else {
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
				if items[i].BaseApp != item.BaseApp {
					break
				}
				first = i
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
				if items[i].BaseApp != item.BaseApp {
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
	case "ctrl+left":
		if isSubRow(item) && m.ActiveColumn() == tui.ColAdd {
			for i := idx - 1; i >= 0; i-- {
				if items[i].BaseApp == item.BaseApp && items[i].IsGroupHeader {
					m.Select(i)
					return nil, true
				}
				if items[i].BaseApp != item.BaseApp {
					break
				}
			}
		}
		if m.ActiveColumn() == tui.ColEnable {
			m.SetActiveColumn(tui.ColAdd)
		}
		return nil, true
	case "ctrl+right":
		if m.ActiveColumn() == tui.ColEnable {
			if item.IsGroupHeader {
				// Already expanded header? Just jump to first sub-item
				m.Select(idx + 1)
				m.SetActiveColumn(tui.ColAdd)
				return nil, true
			}
			if !item.IsSubItem && !item.IsSeparator && !item.IsEditing && item.IsCheckbox {
				// Simple row -> Expand and jump
				s.expandGroup(item.BaseApp)
				m.SetActiveColumn(tui.ColAdd)
				return nil, true
			}
		}
		if m.ActiveColumn() == tui.ColAdd {
			m.SetActiveColumn(tui.ColEnable)
		}
		return nil, true
	case "space":
		s.toggleItem(idx)
		return nil, true
	case "f2":
		if item.IsSubItem {
			s.startRenaming(idx)
			return nil, true
		}
		if item.IsAddInstance {
			s.startEditing(item.BaseApp)
			return nil, true
		}
	}

	return nil, false
}
