package screens

import (
	"context"
	"fmt"
	"strings"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
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

// tintSearchMsg reports that the scheme search text may have changed.
type tintSearchMsg struct{}

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
	case s.tintStripSection, s.tintSearchMenu, s.tintMenu:
		return true
	}
	return false
}

// overrideFrameFocused reports whether focus is inside the Overrides pane.
func (s *DisplayOptionsScreen) overrideFrameFocused() bool {
	switch s.focusedSettingsLeaf() {
	case s.overrideStripSection, s.overrideMenu:
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
// outside advanced mode.
func (s *DisplayOptionsScreen) shownTintElement() {
	if !s.advanced {
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

// newElementFrame builds a frame with the element tabs in its top edge,
// marking the elements whose part of the settings changed.
func (s *DisplayOptionsScreen) newElementFrame(id string, part func(config.AnsiElementColors) config.AnsiElementColors, focused func() bool) (*tabFrame, *tabStripSection) {
	elements := tintElementsFor(s.editType)
	labels := make([]string, len(elements))
	for i, e := range elements {
		labels[i] = tintElementLabels[e]
	}
	strip := &displayengine.TabStrip{ID: id, Labels: labels, Active: tintElementIndex(elements, s.tintElement)}
	strip.Changed = func(i int) bool { return s.elementPartChanged(elements[i], part) }
	frame := &tabFrame{strip: strip, focused: focused}
	return frame, newTabStripSection(frame, func(tab int) tea.Cmd {
		return func() tea.Msg { return tintElementMsg{element: elements[tab]} }
	})
}

// tintPaneColumn and overridePaneColumn build the Tint and Overrides panes'
// contents: in advanced mode framed by the element tabs, with Tab walking
// every element; otherwise just the Menu element's sections.
func (s *DisplayOptionsScreen) tintPaneColumn() *displayengine.ContentColumn {
	if !s.advanced {
		return displayengine.NewContentColumn(s.tintSearchMenu, s.tintMenu)
	}
	return displayengine.NewContentColumn(s.perElement(displayengine.NewContentColumn(s.tintStripSection,
		newTabFrameSection(s.tintSearchMenu, s.tintFrame, false, false),
		newTabFrameSection(s.tintMenu, s.tintFrame, false, true))))
}

func (s *DisplayOptionsScreen) overridePaneColumn() *displayengine.ContentColumn {
	if !s.advanced {
		return displayengine.NewContentColumn(s.overrideMenu)
	}
	return displayengine.NewContentColumn(s.perElement(displayengine.NewContentColumn(s.overrideStripSection,
		newTabFrameSection(s.overrideMenu, s.overrideFrame, false, true))))
}

// perElement repeats column's Tab stops once per element tab (see
// displayengine.RepeatedStops), so Tab walks through every element.
func (s *DisplayOptionsScreen) perElement(column *displayengine.ContentColumn) *displayengine.RepeatedStops {
	return displayengine.NewRepeatedStops(column,
		func() int { return len(tintElementsFor(s.editType)) },
		func() int { return tintElementIndex(tintElementsFor(s.editType), s.tintElement) },
		func(tab int) {
			s.tintElement = tintElementsFor(s.editType)[tab]
			s.syncTintMenus()
			s.selectCheckedTint()
		})
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
// the search (see commands.TintMatcher) grouped by source (see
// groupedItems), with the element's staged scheme checked.
func (s *DisplayOptionsScreen) tintListItems() []displayengine.MenuItem {
	current := s.stagedTint().Tint
	saved := s.baseTint().Tint
	// changedAt marks the saved scheme's row while another is staged.
	changedAt := func(ref string) bool { return current != saved && ref == saved }
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
		Changed:       changedAt(""),
		SpaceAction:   pick(""),
		Metadata:      map[string]string{"config_value": ""},
	}

	groups := map[string]*listGroup{
		"embedded": {Label: "Bundled"},
		"user":     {Label: "User"},
		"repo":     {Label: "Repo (tinted-theming)"},
	}
	found := current == ""
	for _, e := range s.tintCatalog {
		ref := e.Ref()
		if ref == current {
			found = true
		}
		if searchErr != nil || !matches(e) {
			continue
		}
		name := e.Name
		if name == "" {
			name = e.Slug
		}
		desc := name
		if author := authorName(e.Author); author != "" {
			desc += " [by " + author + "]"
		}
		descTag := "{{|ItemList|}}"
		if e.Source == "user" {
			descTag = "{{|ItemListUserDefined|}}"
		}
		g := groups[e.Source]
		g.Items = append(g.Items, displayengine.MenuItem{
			Tag:           e.Slug,
			Desc:          descTag + desc,
			Help:          desc,
			IsRadioButton: true,
			Selectable:    true,
			Checked:       ref == current,
			Changed:       changedAt(ref),
			IsUserDefined: e.Source == "user",
			SpaceAction:   pick(ref),
			Metadata:      map[string]string{"config_value": ref},
		})
	}
	if !commands.TintRepoCloned() && !s.tintSearching() {
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

// buildTintMenus builds the Tint and Overrides panes: each has the element
// tabs framing its list, and the Tint pane a search box too.
func (s *DisplayOptionsScreen) buildTintMenus() {
	s.shownTintElement()
	s.tintFrame, s.tintStripSection = s.newElementFrame("appearance_tint_elements", tintPart, s.tintFrameFocused)
	s.overrideFrame, s.overrideStripSection = s.newElementFrame("appearance_override_elements", overridePart, s.overrideFrameFocused)

	list := displayengine.NewMenuModel(displayengine.IDTintPanel, config.ConnTypeLabel(s.editType)+" Tint", "", s.tintListItems())
	s.tintSearchMenu, s.tintSearchInput = displayengine.NewSinputSection(tintSearchID, "Search", s.tintQuery)
	s.tintSearchMenu.SetHelpPageText("Show only schemes whose name, slug, variant, or author contains every word typed, comma-separated. \"base16\" or \"base24\" shows only that format -- the same search --tint-list and --tint-table use. Word matches whole words only; turn it off to match part of a word. Variant and Base narrow to light or dark, and to base16 or base24, schemes.")
	s.tintSearchMenu.SetTitleControls([]displayengine.TitleControl{
		{Label: "Word", Key: 'o', Checked: func() bool { return !s.tintPartial }, Help: "Match whole words only, or part of a word"},
		{Label: "Variant", Key: 'r', Value: func() string { return searchOptionLabel(s.tintVariant) }, Help: "Show all, light, or dark schemes"},
		{Label: "Base", Key: 'b', Value: func() string { return searchOptionLabel(s.tintSystem) }, Help: "Show all, base16, or base24 schemes"},
	})
	prev := s.tintSearchMenu.Interceptor
	s.tintSearchMenu.SetUpdateInterceptor(func(msg tea.Msg, menu *displayengine.MenuModel) (tea.Cmd, bool) {
		cmd, handled := prev(msg, menu)
		if handled && s.tintSearchInput.Value() != s.tintQuery {
			return tea.Batch(cmd, func() tea.Msg { return tintSearchMsg{} }), true
		}
		return cmd, handled
	})

	list.SetHelpItemPrefix("Tint")
	list.SetTitleCheckbox("Enabled", 'e', func() bool { return s.stagedTint().TintEnabled },
		func() bool { return s.stagedTint().TintEnabled != s.baseTint().TintEnabled })
	list.SetTitleChanged(func() bool { return s.stagedTint().Tint != s.baseTint().Tint })
	list.SetRowCache(true)
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
	overrides.SetTitleChanged(func() bool {
		staged, base := overridePart(*s.stagedTint()), overridePart(s.baseTint())
		staged.OverrideEnabled, base.OverrideEnabled = false, false
		return staged != base
	})
	overrides.SetHelpPageText("Set individual colors for this element, applied on top of its tint. Enter to edit a color; clear it to use the tint's.")
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
		if e.Ref() != ref {
			continue
		}
		var parts []string
		if e.Name != "" {
			parts = append(parts, e.Name)
		}
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
		if e.Author != "" {
			parts = append(parts, "By: "+e.Author)
		}
		parts = append(parts, "Source: "+tintSourceLabel(e.Source), "Reference: "+ref)
		return item.Tag, strings.Join(parts, "\n\n")
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

// tintSearch returns the scheme search the Tint pane's search box and its
// options make.
func (s *DisplayOptionsScreen) tintSearch() commands.TintSearch {
	return commands.TintSearch{Query: s.tintQuery, Partial: s.tintPartial, Variant: s.tintVariant, System: s.tintSystem}
}

// tintSearching reports whether the search narrows the scheme list.
func (s *DisplayOptionsScreen) tintSearching() bool {
	return s.tintQuery != "" || s.tintVariant != "" || s.tintSystem != ""
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

// showTintSearchPicker opens a picker for one search option: title names
// it, values are its choices ("" for All), and set stores the one picked.
func (s *DisplayOptionsScreen) showTintSearchPicker(id, title, current string, values []string, set func(*DisplayOptionsScreen, string)) tea.Cmd {
	return func() tea.Msg {
		items := make([]displayengine.MenuItem, len(values))
		applyFuncs := make([]tea.Cmd, len(values))
		sel := 0
		for i, v := range values {
			items[i] = displayengine.MenuItem{Tag: searchOptionLabel(v), Help: "Show " + strings.ToLower(searchOptionLabel(v)) + " schemes", IsRadioButton: true, Selectable: true, Checked: v == current}
			if v == current {
				sel = i
			}
			applyFuncs[i] = func() tea.Msg {
				return tea.Batch(
					func() tea.Msg { return tintSearchOptionMsg{func(s *DisplayOptionsScreen) { set(s, v) }} },
					tui.CloseDialog(),
				)()
			}
		}
		menu := displayengine.NewMenuModel(id, title, "Show only these schemes", items)
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
	s.tintFrame.strip.Active, s.overrideFrame.strip.Active = active, active
	overrideCursor := s.overrideMenu.Index()
	s.overrideMenu.SetItems(s.overrideItems())
	s.overrideMenu.Select(overrideCursor)
	cursor := s.tintMenu.Index()
	s.tintMenu.SetItems(s.tintListItems())
	s.tintMenu.Select(cursor)
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
