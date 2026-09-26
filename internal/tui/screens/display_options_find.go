package screens

import (
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui/components/sinput"

	tea "charm.land/bubbletea/v2"
)

// newFindBox builds a list's Find box (see displayengine.NewFooteredList):
// a search input with id starting with query, its Word option showing
// partial, calling changed after each message it handles.
func newFindBox(id, query, help string, partial *bool, changed func()) (*displayengine.MenuModel, *sinput.Model) {
	box, input := displayengine.NewSinputSection(id, "Find", query)
	box.SetHelpPageText(help)
	box.SetDarkBorder(true)
	box.SetInputControls(func() []displayengine.TitleControl { return []displayengine.TitleControl{wordControl(partial)} })
	box.SetInputPrompt("Find> ")
	prev := box.Interceptor
	box.SetUpdateInterceptor(func(msg tea.Msg, menu *displayengine.MenuModel) (tea.Cmd, bool) {
		cmd, handled := prev(msg, menu)
		if handled {
			changed()
		}
		return cmd, handled
	})
	return box, input
}

// wordControl is a Find box's Word option: whole words, or part of a word
// while partial.
func wordControl(partial *bool) displayengine.TitleControl {
	return displayengine.TitleControl{Label: "Word", Key: 'o', Checked: func() bool { return !*partial },
		Help: "Match whole words only, or part of a word"}
}

// toggleFind expands or collapses a list's Find box, refreshing the list
// with sync, and focuses the box when expanded or the list when collapsed
// from the box.
func (s *DisplayOptionsScreen) toggleFind(shown *bool, list *displayengine.HeaderedList, box, menu *displayengine.MenuModel, sync func()) tea.Cmd {
	onBox := s.focusedSettingsLeaf() == box
	*shown = !*shown
	list.SetSectionShown(*shown)
	sync()
	s.SetSize(s.width, s.height)
	switch {
	case *shown:
		return s.focusSettingsStop(box.ID())
	case onBox:
		return s.focusSettingsStop(menu.ID())
	}
	return nil
}

// focusSettingsStop focuses the settings Tab stop that id belongs to.
func (s *DisplayOptionsScreen) focusSettingsStop(id string) tea.Cmd {
	if s.layoutRow == nil || s.outerMenu == nil {
		return nil
	}
	for i, leaf := range s.layoutRow.settings.Items() {
		if leaf.MatchesID(id) {
			s.layoutRow.settings.SetSubFocusIndex(i)
			return s.focusFrame()
		}
	}
	return nil
}

// focusedFindBox returns the Find box holding focus and its input, if any.
func (s *DisplayOptionsScreen) focusedFindBox() (*displayengine.MenuModel, *sinput.Model) {
	switch leaf := s.focusedSettingsLeaf(); {
	case leaf == nil:
	case s.tintSearchMenu != nil && leaf == s.tintSearchMenu:
		return s.tintSearchMenu, s.tintSearchInput
	case s.themeFindMenu != nil && leaf == s.themeFindMenu:
		return s.themeFindMenu, s.themeFindInput
	}
	return nil, nil
}

// themeFindID is the Theme list's Find box section ID.
const themeFindID = "theme_find_input"

// themePaneColumn builds the Theme pane's contents: the theme list, with its
// Find box below its rows when expanded.
func (s *DisplayOptionsScreen) themePaneColumn() *displayengine.ContentColumn {
	s.themeFindList = displayengine.NewFooteredList(s.themeFindMenu, s.themeMenu)
	s.themeFindList.SetSectionShown(s.themeFindShown)
	return displayengine.NewContentColumn(s.themeFindList)
}

// themeFrameFocused reports whether focus is inside the Theme pane.
func (s *DisplayOptionsScreen) themeFrameFocused() bool {
	leaf := s.focusedSettingsLeaf()
	return leaf != nil && (leaf == s.themeMenu || leaf == s.themeFindMenu)
}

// toggleThemeFind expands or collapses the Theme list's Find box.
func (s *DisplayOptionsScreen) toggleThemeFind() tea.Cmd {
	return s.toggleFind(&s.themeFindShown, s.themeFindList, s.themeFindMenu, s.themeMenu, s.syncThemeList)
}

// toggleThemeWholeWords flips the Theme Find box's Word option.
func (s *DisplayOptionsScreen) toggleThemeWholeWords() tea.Cmd {
	s.themePartial = !s.themePartial
	s.syncThemeList()
	s.themeFindMenu.InvalidateCache()
	return nil
}

// applyThemeFind re-filters the theme list when the Find text changed.
func (s *DisplayOptionsScreen) applyThemeFind() {
	if s.themeFindInput != nil && s.themeFindInput.Value() != s.themeQuery {
		s.themeQuery = s.themeFindInput.Value()
		s.syncThemeList()
	}
}

// syncThemeList rebuilds the theme list from the staged theme and the Find
// box, keeping the cursor on the checked theme.
func (s *DisplayOptionsScreen) syncThemeList() {
	if s.themeMenu == nil {
		return
	}
	s.themeMenu.SetItems(s.themeListItems(s.config.Appearance.ForConnType(s.editType).Theme))
	s.themeMenu.Select(0)
	for i, it := range s.themeMenu.GetItems() {
		if it.Checked && !it.IsSeparator {
			s.themeMenu.Select(i)
			break
		}
	}
	s.markThemeList()
	s.themeMenu.InvalidateCache()
}

// findBoxFocused reports whether a Find box holds focus.
func (s *DisplayOptionsScreen) findBoxFocused() bool {
	box, _ := s.focusedFindBox()
	return box != nil
}
