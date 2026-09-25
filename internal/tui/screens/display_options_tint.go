package screens

import (
	"context"
	"fmt"
	"strings"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/displayengine"

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

// tintRepoDownloadMsg starts downloading the tinted-theming schemes.
type tintRepoDownloadMsg struct{}

// tintRepoDownloadedMsg reports the download finished.
type tintRepoDownloadedMsg struct{ err error }

// stagedTint returns the shown tab's staged settings for the shown element.
func (s *DisplayOptionsScreen) stagedTint() *config.AnsiElementColors {
	return s.config.Appearance.Ptr(s.editType).AnsiColors.ElementPtr(s.tintElement)
}

// shownTintElement keeps tintElement valid for the shown tab.
func (s *DisplayOptionsScreen) shownTintElement() {
	for _, e := range tintElementsFor(s.editType) {
		if e == s.tintElement {
			return
		}
	}
	s.tintElement = "menu"
}

// tintControlItems returns the element choice and the element's on/off
// switches.
func (s *DisplayOptionsScreen) tintControlItems() []displayengine.MenuItem {
	var items []displayengine.MenuItem
	for _, e := range tintElementsFor(s.editType) {
		element := e
		items = append(items, displayengine.MenuItem{
			Tag:           tintElementLabels[element],
			Help:          "Show the " + tintElementLabels[element] + " tint (Space to select)",
			IsRadioButton: true,
			Selectable:    true,
			Checked:       element == s.tintElement,
			SpaceAction:   func() tea.Msg { return tintElementMsg{element: element} },
		})
	}
	el := s.stagedTint()
	items = append(items,
		displayengine.MenuItem{
			Tag:        "Tint",
			Help:       "Apply the chosen scheme to this element (Space to toggle)",
			IsCheckbox: true,
			Selectable: true,
			Checked:    el.TintEnabled,
			SpaceAction: func() tea.Msg {
				return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
					e := cfg.Appearance.Ptr(s.editType).AnsiColors.ElementPtr(s.tintElement)
					e.TintEnabled = !e.TintEnabled
				}}
			},
		},
		displayengine.MenuItem{
			Tag:        "Color Overrides",
			Help:       "Apply this element's individual color overrides, set with --ansi-override (Space to toggle)",
			IsCheckbox: true,
			Selectable: true,
			Checked:    el.OverrideEnabled,
			SpaceAction: func() tea.Msg {
				return updateDisplayOptionMsg{func(cfg *config.AppConfig) {
					e := cfg.Appearance.Ptr(s.editType).AnsiColors.ElementPtr(s.tintElement)
					e.OverrideEnabled = !e.OverrideEnabled
				}}
			},
		},
	)
	return items
}

// tintListItems returns the scheme list: None, then the schemes grouped by
// source (see groupedItems), with the element's staged scheme checked.
func (s *DisplayOptionsScreen) tintListItems() []displayengine.MenuItem {
	current := s.stagedTint().Tint
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
		if ref == current {
			found = true
		}
		g := groups[e.Source]
		g.Items = append(g.Items, displayengine.MenuItem{
			Tag:           e.Slug,
			Desc:          descTag + desc,
			Help:          desc,
			IsRadioButton: true,
			Selectable:    true,
			Checked:       ref == current,
			IsUserDefined: e.Source == "user",
			SpaceAction:   pick(ref),
			Metadata:      map[string]string{"config_value": ref},
		})
	}
	if !commands.TintRepoCloned() {
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
	return append([]displayengine.MenuItem{none}, groupedItems([]listGroup{
		{Label: "Current", Items: currentGroup},
		*groups["embedded"],
		*groups["user"],
		*groups["repo"],
	})...)
}

// buildTintMenus builds the Tint pane's controls and scheme list.
func (s *DisplayOptionsScreen) buildTintMenus() {
	s.shownTintElement()

	controls := displayengine.NewMenuModel(displayengine.IDTintControlsPanel, "", "", s.tintControlItems())
	controls.SetHelpItemPrefix("Tint")
	controls.SetHelpPageText("Choose which part of the screen to tint, and turn its tint and color overrides on or off.")
	controls.SetSubMenuMode(true)
	controls.SetIsDialog(false)
	controls.SetButtons([]displayengine.ButtonDef{})
	controls.SetFlowMode(true)
	controls.SetMaximized(true)
	controls.SetShowLockGutter(false)
	s.tintControlsMenu = controls

	list := displayengine.NewMenuModel(displayengine.IDTintPanel, config.ConnTypeLabel(s.editType)+" Tint", "", s.tintListItems())
	list.SetHelpItemPrefix("Tint")
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

// selectCheckedTint moves the scheme list's cursor to the checked scheme.
func (s *DisplayOptionsScreen) selectCheckedTint() {
	for i, it := range s.tintMenu.GetItems() {
		if it.Checked && !it.IsSeparator {
			s.tintMenu.Select(i)
			return
		}
	}
}

// syncTintMenus refreshes the Tint pane from the staged config.
func (s *DisplayOptionsScreen) syncTintMenus() {
	if s.tintControlsMenu == nil || s.tintMenu == nil {
		return
	}
	s.shownTintElement()
	s.tintControlsMenu.SetItems(s.tintControlItems())
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
