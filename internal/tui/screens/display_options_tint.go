package screens

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// tintElementLabels names each tint element.
var tintElementLabels = map[string]string{"menu": "Menu", "programbox": "ProgramBox", "cli": "CLI"}

// tintElementsFor returns the elements connType's tint can be set for: cli
// only applies to Local.
func tintElementsFor(connType string) []string {
	if connType == "local" {
		return []string{"menu", "programbox", "cli"}
	}
	return []string{"menu", "programbox"}
}

// tintElementMsg shows element's tint settings.
type tintElementMsg struct{ element string }

// tintPickMsg stages ref ("" for none) as the shown element's tint.
type tintPickMsg struct{ ref string }

// tintRepoDownloadMsg starts downloading the tinted-theming schemes.
type tintRepoDownloadMsg struct{}

// tintRepoDownloadedMsg reports the download finished.
type tintRepoDownloadedMsg struct{ err error }

// stagedTint returns the shown tab's staged settings for the shown element.
func (s *DisplayOptionsScreen) stagedTint() *config.AnsiElementColors {
	return s.config.Appearance.Ptr(s.editType).AnsiColors.ElementPtr(s.tintElement)
}

// tintElementIndex returns element's position in elements (0 if absent).
func tintElementIndex(elements []string, element string) int {
	for i, e := range elements {
		if e == element {
			return i
		}
	}
	return 0
}

// tintFrameFocused reports whether focus is inside the Tint pane.
func (s *DisplayOptionsScreen) tintFrameFocused() bool {
	switch s.focusedSettingsLeaf() {
	case s.tintSearchMenu, s.tintMenu:
		return true
	}
	return false
}

// baseTint returns the shown tab's saved settings for the shown element.
func (s *DisplayOptionsScreen) baseTint() config.AnsiElementColors {
	return s.baseConfig.Appearance.ForConnType(s.editType).AnsiColors.Element(s.tintElement)
}

// tintPart and overridePart split an element's settings into what the Tint
// pane edits (the scheme and its switch) and what the Overrides pane edits
// (the color slots and their switch).
func tintPart(e config.AnsiElementColors) config.AnsiElementColors {
	return config.AnsiElementColors{Tint: e.Tint, TintEnabled: e.TintEnabled}
}

func overridePart(e config.AnsiElementColors) config.AnsiElementColors {
	e.Tint, e.TintEnabled = "", false
	return e
}

// elementPartChanged reports whether part of element's staged settings
// differs from the saved ones on the shown tab; element "" means any.
func (s *DisplayOptionsScreen) elementPartChanged(element string, part func(config.AnsiElementColors) config.AnsiElementColors) bool {
	staged := s.config.Appearance.ForConnType(s.editType).AnsiColors
	base := s.baseConfig.Appearance.ForConnType(s.editType).AnsiColors
	elements := []string{element}
	if element == "" {
		elements = tintElementsFor(s.editType)
	}
	for _, e := range elements {
		if part(staged.Element(e)) != part(base.Element(e)) {
			return true
		}
	}
	return false
}

// shownTintElement keeps tintElement valid for the shown tab: only Menu
// without Elements.
func (s *DisplayOptionsScreen) shownTintElement() {
	if !s.showElements {
		s.tintElement = "menu"
		return
	}
	for _, e := range tintElementsFor(s.editType) {
		if e == s.tintElement {
			return
		}
	}
	s.tintElement = "menu"
}

// newElementStrip builds the element tabs, marking the elements with
// changed settings.
func (s *DisplayOptionsScreen) newElementStrip() *displayengine.TabStrip {
	elements := tintElementsFor(s.editType)
	labels := make([]string, len(elements))
	for i, e := range elements {
		labels[i] = tintElementLabels[e]
	}
	strip := &displayengine.TabStrip{ID: "appearance_elements", Labels: labels, Active: tintElementIndex(elements, s.tintElement)}
	strip.Changed = func(i int) bool {
		return s.elementPartChanged(elements[i], func(e config.AnsiElementColors) config.AnsiElementColors { return e })
	}
	return strip
}

// themePaneShown reports whether the Theme pane shows: for the Menu
// element, since ProgramBox and CLI have no theme.
func (s *DisplayOptionsScreen) themePaneShown() bool {
	return s.tintElement == "menu"
}

// panesFocused reports whether focus is inside the Theme/Tint/Overrides
// panes.
func (s *DisplayOptionsScreen) panesFocused() bool {
	leaf := s.focusedSettingsLeaf()
	return leaf != nil && leaf != s.optionsMenu
}

// elementStripHit handles a click on the element tabs.
func (s *DisplayOptionsScreen) elementStripHit(id string) (tea.Cmd, bool) {
	strip := s.elementStrip
	if strip == nil {
		return nil, false
	}
	if i, ok := strip.TabFromID(id); ok {
		element := tintElementsFor(s.editType)[i]
		return func() tea.Msg { return tintElementMsg{element: element} }, true
	}
	if d, ok := strip.ScrollFromID(id); ok {
		strip.ScrollBy(d)
		if s.outerMenu != nil {
			s.outerMenu.InvalidateCache()
		}
		return nil, true
	}
	return nil, false
}

// tintPaneColumn and overridePaneColumn build the Tint and Overrides panes'
// contents: the shown element's list. The Find box, when expanded, sits
// below the Tint list's rows, sharing its border.
func (s *DisplayOptionsScreen) tintPaneColumn() *displayengine.ContentColumn {
	s.tintFilterList = displayengine.NewFooteredList(s.tintSearchMenu, s.tintMenu)
	s.tintFilterList.SetSectionShown(s.tintFilterShown)
	return displayengine.NewContentColumn(s.tintFilterList)
}

// toggleTintFilter expands or collapses the Tint list's Find box.
func (s *DisplayOptionsScreen) toggleTintFilter() tea.Cmd {
	return s.toggleFind(&s.tintFilterShown, s.tintFilterList, s.tintSearchMenu, s.tintMenu, func() {
		s.syncTintMenus()
		s.selectCheckedTint()
	})
}

func (s *DisplayOptionsScreen) overridePaneColumn() *displayengine.ContentColumn {
	return displayengine.NewContentColumn(s.overrideMenu)
}

// toggleTintEnabled and toggleOverrideEnabled flip the shown element's tint
// and override switches.
func (s *DisplayOptionsScreen) toggleTintEnabled() tea.Cmd {
	return func() tea.Msg {
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			e := cfg.Appearance.Ptr(s.editType).AnsiColors.ElementPtr(s.tintElement)
			e.TintEnabled = !e.TintEnabled
		}}
	}
}

func (s *DisplayOptionsScreen) toggleOverrideEnabled() tea.Cmd {
	return func() tea.Msg {
		return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
			e := cfg.Appearance.Ptr(s.editType).AnsiColors.ElementPtr(s.tintElement)
			e.OverrideEnabled = !e.OverrideEnabled
		}}
	}
}

// tintListItems returns the scheme list: None, then the schemes matching
// the search (see tintSearch) grouped by source (see groupedItems), with
// the element's staged scheme checked.
func (s *DisplayOptionsScreen) tintListItems() []displayengine.MenuItem {
	current := s.stagedTint().Tint
	saved := s.baseTint().Tint
	// changedAt marks the saved scheme's row while another is staged.
	changedAt := func(selects func(string) bool) bool { return current != saved && selects(saved) }
	matches, searchErr := s.tintSearch().Matcher()
	pick := func(ref string) func() tea.Msg {
		return func() tea.Msg { return tintPickMsg{ref: ref} }
	}
	none := displayengine.MenuItem{
		Tag:           "None",
		Desc:          "{{|ItemList|}}No tint",
		Help:          "Use the terminal's own colors for this element.",
		IsRadioButton: true,
		Selectable:    true,
		Checked:       current == "",
		Changed:       changedAt(func(ref string) bool { return ref == "" }),
		SpaceAction:   pick(""),
		Metadata:      map[string]string{"config_value": ""},
	}

	groups := map[string]*listGroup{
		"embedded": {Label: "Bundled"},
		"user":     {Label: "User"},
		"repo":     {Label: "Repo (tinted-theming)"},
	}
	found := current == ""
	s.tintFound = 0
	for _, e := range s.tintCatalog {
		if e.Selects(current) {
			found = true
		}
		if searchErr != nil || !matches(e) || s.tintSource != "" && e.Source != s.tintSource ||
			s.tintHues != "" && e.Kind != s.tintHues ||
			len(s.tintHueColors) > 0 && !slices.ContainsFunc(s.tintHueColors, func(h string) bool { return slices.Contains(e.Hues, h) }) {
			continue
		}
		s.tintFound++
		name := e.Name
		if name == "" {
			name = e.Slug
		}
		descTag := "{{|ItemList|}}"
		if e.Source == "user" {
			descTag = "{{|ItemListUserDefined|}}"
		}
		author := ""
		if a := authorName(e.Author); a != "" {
			author = " [by " + a + "]"
		}
		// A row picks the file the Base filter names. A scheme with both
		// files, one of them applied, gets a row for each with no Base
		// filter, so the one in use shows and picking again can't switch
		// files. A single row with no Base filter is checked for either.
		systems := []string{s.tintSystem}
		split := s.tintSystem == "" && e.HasBase16 && e.HasBase24 && e.Selects(saved)
		if split {
			systems = []string{"base24", "base16"}
		}
		for _, system := range systems {
			ref := e.RefFor(system)
			label := ""
			if len(systems) > 1 {
				label = " (" + system + ")"
			}
			markup := descTag + name
			// A repo scheme's name links to its upstream file.
			if url := e.SourceURLFor(system); url != "" {
				tag := strings.TrimSuffix(descTag, "|}}")
				markup = tag + "::::" + url + "|}}" + name + "{{[-]}}" + descTag
			}
			g := groups[e.Source]
			g.Items = append(g.Items, displayengine.MenuItem{
				Tag:           e.Slug,
				Desc:          markup + label + author,
				Help:          name + label + author,
				IsRadioButton: true,
				Selectable:    true,
				Checked:       current == ref || !split && s.tintSystem == "" && e.Selects(current),
				Changed:       changedAt(func(r string) bool { return r == ref }),
				IsUserDefined: e.Source == "user",
				SpaceAction:   pick(ref),
				Metadata:      map[string]string{"config_value": ref},
			})
		}
	}
	if !commands.TintRepoCloned() && !s.tintNarrowed() && (s.tintSource == "" || s.tintSource == "repo") {
		desc := "Adds several hundred schemes from github.com/tinted-theming/schemes"
		if s.tintDownloading {
			desc = "Downloading..."
		}
		groups["repo"].Items = append(groups["repo"].Items, displayengine.MenuItem{
			Tag:        "Download schemes",
			Desc:       "{{|ItemList|}}" + desc,
			Help:       "Download the tinted-theming scheme collection (Enter).",
			Selectable: true,
			Action:     func() tea.Msg { return tintRepoDownloadMsg{} },
		})
	}

	var currentGroup []displayengine.MenuItem
	if !found {
		currentGroup = append(currentGroup, displayengine.MenuItem{
			Tag:           current,
			Desc:          "{{|ItemListUserDefined|}}Not in the list above",
			Help:          "The configured tint isn't one of the listed schemes.",
			IsRadioButton: true,
			Selectable:    true,
			Checked:       true,
			IsUserDefined: true,
			SpaceAction:   pick(current),
			Metadata:      map[string]string{"config_value": current},
		})
	}
	items := []displayengine.MenuItem{none}
	grouped := groupedItems([]listGroup{
		{Label: "Current", Items: currentGroup},
		*groups["embedded"],
		*groups["user"],
		*groups["repo"],
	})
	switch {
	case searchErr != nil:
		items = append(items, displayengine.MenuItem{Tag: "Search: " + searchErr.Error(), IsSeparator: true})
	case len(grouped) == 0 && s.tintSearching():
		items = append(items, displayengine.MenuItem{Tag: "No matching schemes", IsSeparator: true})
	}
	return append(items, grouped...)
}

// buildTintMenus builds the Tint and Overrides panes' menus: the element
// tabs (used in advanced mode), each pane's list, and the Tint pane's search
// box.
func (s *DisplayOptionsScreen) buildTintMenus() {
	s.shownTintElement()
	s.elementStrip = s.newElementStrip()
	s.elementFrame = &tabFrame{strip: s.elementStrip, focused: s.panesFocused}

	list := displayengine.NewMenuModel(displayengine.IDTintPanel, config.ConnTypeLabel(s.editType)+" Tint", "", s.tintListItems())
	s.tintSearchMenu, s.tintSearchInput = newFindBox(tintSearchID, s.tintQuery, "Show only schemes whose name, slug, variant, or author contains every word typed, comma-separated. \"base16\" or \"base24\" shows only that format -- the same search --tint-list and --tint-table use. Word matches whole words only; turn it off to match part of a word. Turning Find off (Alt+F, or its checkbox in the list's title) stops matching the typed words; Variant, Base, Hues, and Source, which narrow to light or dark, to base16 or base24, to monochrome or multi-color or some hues, and to bundled, user, or repo schemes, apply either way.",
		&s.tintPartial, s.applyTintSearch)

	list.SetHelpItemPrefix("Tint")
	list.SetFooterBar(&displayengine.FooterBar{
		Status: func() (string, string) { return foundStatus(s.tintFound) },
		Controls: func() []displayengine.TitleControl {
			return []displayengine.TitleControl{
				{Label: "Variant", Key: 'a', Value: func() string { return searchOptionLabel(s.tintVariant) }, Help: "Show all, light, or dark schemes"},
				{Label: "Base", Key: 'b', Value: func() string { return searchOptionLabel(s.tintSystem) }, Help: "Show all, base16, or base24 schemes"},
				{Label: "Hues", Key: 'u', Value: func() string { return huesLabel(s.tintHues, s.tintHueColors) }, Help: "Show all, monochrome, or multi-color schemes, or schemes known by some hues"},
				{Label: "Source", Key: 's', Value: func() string { return searchOptionLabel(s.tintSource) }, Help: "Show all, bundled, user, or repo schemes"},
			}
		},
	})
	list.SetTitleControls([]displayengine.TitleControl{
		{Label: "Find", Key: 'f', Checked: func() bool { return s.tintFilterShown }, Help: "Show or hide the scheme search box"},
		{Label: "Enabled", Key: 'e', Checked: func() bool { return s.stagedTint().TintEnabled },
			Changed: func() bool { return s.stagedTint().TintEnabled != s.baseTint().TintEnabled }, Help: "Turn Enabled on or off"},
	})
	list.SetTitleChanged(func() bool { return s.stagedTint().Tint != s.baseTint().Tint })
	list.SetTitleIcons(sectionResetIcons(func() bool { return s.elementPartChanged(s.tintElement, tintPart) }))
	list.SetMinTagWidth(s.tintTagWidth())
	list.SetItemHelpFunc(s.buildTintItemHelp)
	list.SetHelpPageText("Choose an ANSI color scheme to tint this element with.")
	list.SetSubMenuMode(true)
	list.SetVariableHeight(true)
	list.SetIsDialog(false)
	list.SetButtons([]displayengine.ButtonDef{})
	list.SetMaximized(true)
	list.SetShowLockGutter(false)
	list.SetNoLeftMargin(true)
	s.tintMenu = list
	s.selectCheckedTint()

	overrides := displayengine.NewMenuModel(displayengine.IDOverridePanel, config.ConnTypeLabel(s.editType)+" Overrides", "", s.overrideItems())
	overrides.SetHelpItemPrefix("Override")
	overrides.SetTitleCheckbox("Enabled", 'e', func() bool { return s.stagedTint().OverrideEnabled },
		func() bool { return s.stagedTint().OverrideEnabled != s.baseTint().OverrideEnabled })
	overrides.SetTitleIcons(sectionResetIcons(func() bool { return s.elementPartChanged(s.tintElement, overridePart) }))
	overrides.SetTitleChanged(func() bool {
		staged, base := overridePart(*s.stagedTint()), overridePart(s.baseTint())
		staged.OverrideEnabled, base.OverrideEnabled = false, false
		return staged != base
	})
	overrides.SetHelpPageText("Set individual colors for this element, applied on top of its tint. Each row shows the color without an override (the tint's, or the terminal's own without a tint), then the override that replaces it. Enter to edit a color; clear it for no override.")
	overrides.SetSubMenuMode(true)
	overrides.SetVariableHeight(true)
	overrides.SetIsDialog(false)
	overrides.SetButtons([]displayengine.ButtonDef{})
	overrides.SetMaximized(true)
	overrides.SetShowLockGutter(false)
	overrides.SetNoLeftMargin(true)
	s.overrideMenu = overrides
}

// buildTintItemHelp returns a scheme's full details for the help page.
func (s *DisplayOptionsScreen) buildTintItemHelp(item displayengine.MenuItem) (itemTitle, itemText string) {
	ref, ok := item.Metadata["config_value"]
	if !ok || ref == "" {
		return "", ""
	}
	for _, e := range s.tintCatalog {
		if !e.Selects(ref) {
			continue
		}
		// The name, then the details one per line.
		var parts []string
		if e.Variant != "" {
			parts = append(parts, "Variant: "+e.Variant)
		}
		var systems []string
		if e.HasBase16 {
			systems = append(systems, "base16")
		}
		if e.HasBase24 {
			systems = append(systems, "base24")
		}
		if len(systems) > 0 {
			parts = append(parts, "Formats: "+strings.Join(systems, ", "))
		}
		if len(e.Hues) > 0 {
			hues := "Hues: " + strings.Join(e.Hues, ", ")
			if e.Kind == commands.TintKindMonochrome {
				hues += " (monochrome)"
			}
			parts = append(parts, hues)
		}
		if e.Author != "" {
			parts = append(parts, "By: "+e.Author)
		}
		parts = append(parts, "Source: "+tintSourceLabel(e.Source), "Reference: "+ref)
		text := strings.Join(parts, "\n")
		if e.Name != "" {
			text = e.Name + "\n\n" + text
		}
		return item.Tag, text
	}
	return item.Tag, "Reference: " + ref
}

// tintSourceLabel names a tint source.
func tintSourceLabel(source string) string {
	switch source {
	case "embedded":
		return "Bundled with DS2"
	case "user":
		return "User tint folder"
	case "repo":
		return "tinted-theming schemes collection"
	}
	return source
}

// selectCheckedTint moves the scheme list's cursor to the checked scheme,
// or to the top when it isn't listed.
func (s *DisplayOptionsScreen) selectCheckedTint() {
	for i, it := range s.tintMenu.GetItems() {
		if it.Checked && !it.IsSeparator {
			s.tintMenu.Select(i)
			return
		}
	}
	s.tintMenu.Select(0)
}

// applyTintSearch re-filters the scheme list when the search text changed,
// in the same update as the keystroke so it draws once.
func (s *DisplayOptionsScreen) applyTintSearch() {
	if s.tintSearchInput != nil && s.tintSearchInput.Value() != s.tintQuery {
		s.tintQuery = s.tintSearchInput.Value()
		s.syncTintMenus()
		s.selectCheckedTint()
	}
}

// tintSearch returns the scheme search the Tint pane's Find makes: its
// text and Word while expanded, Variant and Base always.
func (s *DisplayOptionsScreen) tintSearch() commands.TintSearch {
	search := commands.TintSearch{Variant: s.tintVariant, System: s.tintSystem}
	if s.tintFilterShown {
		search.Query, search.Partial = s.tintQuery, s.tintPartial
	}
	return search
}

// tintSearching reports whether Find or a filter narrows the scheme list.
func (s *DisplayOptionsScreen) tintSearching() bool {
	return s.tintNarrowed() || s.tintSource != ""
}

// tintNarrowed reports whether Find, Variant, Base, or Hues narrows the
// scheme list: anything but Source.
func (s *DisplayOptionsScreen) tintNarrowed() bool {
	return s.tintFilterShown && s.tintQuery != "" || s.tintVariant != "" || s.tintSystem != "" ||
		s.tintHues != "" || len(s.tintHueColors) > 0
}

// searchOptionLabel names a search option's value, "" being All.
func searchOptionLabel(v string) string {
	switch v {
	case "":
		return "All"
	case "base16":
		return "Base16"
	case "base24":
		return "Base24"
	case "embedded":
		return "Bundled"
	}
	return strings.ToUpper(v[:1]) + v[1:]
}

// tintSearchOptionMsg sets a search option: whole words or partial, the
// variant, or the base.
type tintSearchOptionMsg struct{ apply func(*DisplayOptionsScreen) }

// toggleTintWholeWords switches the search between whole words and partial.
func (s *DisplayOptionsScreen) toggleTintWholeWords() tea.Cmd {
	return func() tea.Msg {
		return tintSearchOptionMsg{func(s *DisplayOptionsScreen) { s.tintPartial = !s.tintPartial }}
	}
}

// showTintSearchPicker opens a picker for one scheme search option (see
// showSearchPicker).
func (s *DisplayOptionsScreen) showTintSearchPicker(id, title, current string, values []string, set func(*DisplayOptionsScreen, string)) tea.Cmd {
	return showSearchPicker(id, title, "schemes", current, values, func(v string) tea.Msg {
		return tintSearchOptionMsg{func(s *DisplayOptionsScreen) { set(s, v) }}
	})
}

// showSearchPicker opens a picker for one list filter: title names it,
// noun what the list holds, values its choices ("" for All), and picked
// returns the message storing the one picked.
func showSearchPicker(id, title, noun, current string, values []string, picked func(string) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		items := make([]displayengine.MenuItem, len(values))
		applyFuncs := make([]tea.Cmd, len(values))
		sel := 0
		for i, v := range values {
			items[i] = displayengine.MenuItem{Tag: searchOptionLabel(v), Help: "Show " + strings.ToLower(searchOptionLabel(v)) + " " + noun, IsRadioButton: true, Selectable: true, Checked: v == current}
			if v == current {
				sel = i
			}
			applyFuncs[i] = tui.CloseDialogThen(func() tea.Msg { return picked(v) })
		}
		menu := displayengine.NewMenuModel(id, title, "Show only these "+noun, items)
		menu.SetUpdateInterceptor(tui.RadioGroupInterceptor(id))
		menu.SetButtons([]displayengine.ButtonDef{
			{Label: "Done", ZoneID: "btn-select", Action: radioMenuSelectAction(menu, applyFuncs), Help: "Confirm the marked choice."},
			{Label: "Cancel", ZoneID: "btn-cancel", Action: func() tea.Msg { return displayengine.CloseDialogMsg{} }, Help: "Cancel and close."},
		})
		menu.Select(sel)
		return displayengine.ShowDialogMsg{Dialog: menu}
	}
}

// showTintVariantPicker and showTintBasePicker open the search's Variant
// and Base pickers.
func (s *DisplayOptionsScreen) showTintVariantPicker() tea.Cmd {
	return s.showTintSearchPicker("tint_search_variant", "Variant", s.tintVariant, []string{"", "light", "dark"},
		func(s *DisplayOptionsScreen, v string) { s.tintVariant = v })
}

// showTintHuesPicker opens the Tint list's Hues picker, a scheme showing
// when it's known by any checked hue.
func (s *DisplayOptionsScreen) showTintHuesPicker() tea.Cmd {
	return showHuesPicker("tint_search_hues", "schemes", "known by",
		[]string{"", commands.TintKindMonochrome, commands.TintKindMultiColor}, commands.TintHueNames,
		s.tintHues, s.tintHueColors, func(kind string, colors []string) tea.Msg {
			return tintSearchOptionMsg{func(s *DisplayOptionsScreen) { s.tintHues, s.tintHueColors = kind, colors }}
		})
}

func (s *DisplayOptionsScreen) showTintSourcePicker() tea.Cmd {
	return s.showTintSearchPicker("tint_search_source", "Source", s.tintSource, []string{"", "embedded", "user", "repo"},
		func(s *DisplayOptionsScreen, v string) { s.tintSource = v })
}

func (s *DisplayOptionsScreen) showTintBasePicker() tea.Cmd {
	return s.showTintSearchPicker("tint_search_base", "Base", s.tintSystem, []string{"", "base16", "base24"},
		func(s *DisplayOptionsScreen, v string) { s.tintSystem = v })
}

// tintSearchID is the scheme search box's section ID.
const tintSearchID = "tint_search_input"

// syncTintMenus refreshes the Tint and Overrides panes from the staged
// config.
func (s *DisplayOptionsScreen) syncTintMenus() {
	if s.tintMenu == nil || s.overrideMenu == nil {
		return
	}
	s.shownTintElement()
	active := tintElementIndex(tintElementsFor(s.editType), s.tintElement)
	s.elementStrip.Active = active
	overrideCursor := s.overrideMenu.Index()
	s.overrideMenu.SetItems(s.overrideItems())
	s.overrideMenu.Select(overrideCursor)
	cursor := s.tintMenu.Index()
	var focused displayengine.MenuItem
	if items := s.tintMenu.GetItems(); cursor >= 0 && cursor < len(items) {
		focused = items[cursor]
	}
	s.tintMenu.SetMinTagWidth(s.tintTagWidth())
	s.tintMenu.SetItems(s.tintListItems())
	s.tintMenu.Select(tintCursorFor(s.tintMenu.GetItems(), focused, cursor))
}

// tintCursorFor returns the index in items of the row focused was, since
// rows above it can split or merge as the applied scheme changes: the row
// picking the same tint, else the same scheme's, else cursor.
func tintCursorFor(items []displayengine.MenuItem, focused displayengine.MenuItem, cursor int) int {
	ref, ok := focused.Metadata["config_value"]
	if !ok {
		return cursor
	}
	sameScheme := -1
	for i, it := range items {
		if it.IsSeparator {
			continue
		}
		if it.Metadata["config_value"] == ref {
			return i
		}
		if sameScheme < 0 && ref != "" && it.Tag == focused.Tag {
			sameScheme = i
		}
	}
	if sameScheme >= 0 {
		return sameScheme
	}
	return cursor
}

// tintTagWidth returns the widest scheme name in the catalog, so the scheme
// list's label column stays put as the search narrows the list.
func (s *DisplayOptionsScreen) tintTagWidth() int {
	w := 0
	for _, e := range s.tintCatalog {
		w = max(w, lipgloss.Width(e.Slug))
	}
	return w
}

// loadTintCatalog reads the schemes the Tint list offers.
func (s *DisplayOptionsScreen) loadTintCatalog() {
	s.tintCatalog, _ = commands.TintCatalog(context.Background())
}

// downloadTintRepo downloads the tinted-theming schemes in the background.
func (s *DisplayOptionsScreen) downloadTintRepo() tea.Cmd {
	if s.tintDownloading || commands.TintRepoCloned() {
		return nil
	}
	s.tintDownloading = true
	s.syncTintMenus()
	return func() tea.Msg {
		return tintRepoDownloadedMsg{err: commands.DownloadTintRepo(context.Background())}
	}
}

// authorName returns a scheme author's name without the "(url)" or
// "<email>" that often follows it.
func authorName(author string) string {
	if i := strings.IndexAny(author, "(<"); i >= 0 {
		author = author[:i]
	}
	return strings.TrimSpace(author)
}

// tintRepoDownloadFailed describes a failed download.
func tintRepoDownloadFailed(err error) string {
	return fmt.Sprintf("Could not download the tinted-theming schemes: %v", err)
}
