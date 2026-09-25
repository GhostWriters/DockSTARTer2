package screens

import (
	"fmt"
	"reflect"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// specifiedThemeDefaultFields returns the set of config.Appearance struct
// field names a theme's [defaults] table specifies (i.e. whose pointer is
// non-nil in d) -- theme.ThemeDefaults' field names match config.Appearance's 1:1 for
// every field it can suggest. Used to mark which Options rows the theme set,
// regardless of whether the resulting value actually differs from before.
func specifiedThemeDefaultFields(d theme.ThemeDefaults) map[string]bool {
	specified := make(map[string]bool)
	v := reflect.ValueOf(d)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		if !v.Field(i).IsNil() {
			specified[t.Field(i).Name] = true
		}
	}
	return specified
}

// itemConfigValue returns the config value (e.g. "user:MyTheme") for a theme menu item.
// Falls back to Tag (display name) if no Metadata entry was set.
func itemConfigValue(item displayengine.MenuItem) string {
	if cv, ok := item.Metadata["config_value"]; ok {
		return cv
	}
	return item.Tag
}

// scrollableMenus returns the settings menus that own their own scrollbars,
// as pointers so an Update's returned model can be stored back.
func (s *DisplayOptionsScreen) scrollableMenus() []**displayengine.MenuModel {
	return []**displayengine.MenuModel{&s.themeMenu, &s.optionsMenu}
}

// IsScrollbarDragging reports whether any sub-menu, or the preview panel's
// own scrollbar, is currently dragging a scrollbar thumb.
func (s *DisplayOptionsScreen) IsScrollbarDragging() bool {
	return s.themeMenu.IsScrollbarDragging() || s.optionsMenu.IsScrollbarDragging() || s.previewScroll.Drag.Dragging
}

// delegateToOuterMenu forwards msg to outerMenu generically (Tab-cycling,
// Left/Right, Space/click radio toggling, scrolling, etc. are all now
// handled by the shared section-focus framework, not a screen-local copy of
// it), then checks whether the theme list's Checked item changed as a
// result -- Space/click/middle-click on a theme item are all handled
// generically now by MenuModel's own radio logic, so this is the one
// remaining screen-specific hook: applying the newly-marked theme as the
// live preview.
func (s *DisplayOptionsScreen) delegateToOuterMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := s.outerMenu.Update(msg)
	if m, ok := updated.(*displayengine.MenuModel); ok {
		s.outerMenu = m
	}
	if s.themeMenu != nil {
		for _, it := range s.themeMenu.GetItems() {
			if it.Checked && itemConfigValue(it) != s.previewTheme {
				s.applyPreview(itemConfigValue(it))
				break
			}
		}
	}
	return s, cmd
}

func (s *DisplayOptionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Every inner menu must see its own deferred-action messages (button
	// clicks on outerMenu, item Action clicks like the Options dropdowns on
	// optionsMenu/themeMenu) before any early-return branch below can drop
	// them -- outerMenu.AbsorbMessage recurses generically through
	// appearanceLayoutRow/ContentColumn into every inner menu.
	if s.outerMenu != nil {
		if action := s.outerMenu.AbsorbMessage(msg); action != nil {
			return s, action
		}
	}

	// The "changed by theme" marker (IsNew) is transient -- clear it on the
	// next keypress or click, before that input is otherwise handled, same
	// convention as App Select's just-added/renamed marker. If this same
	// message goes on to trigger a new theme preview, applyPreview sets
	// fresh markers afterward, so they still show through this call.
	switch msg.(type) {
	case tea.KeyPressMsg, displayengine.LayerHitMsg:
		if len(s.themeChangedFields) > 0 {
			s.themeChangedFields = nil
			s.syncOptionsMenu()
		}
	}

	// Forward coalescing done-messages to whichever scrollbar owns them.
	// These messages are sent by dragDoneCmd/scrollDoneCmd after a render
	// cycle. Without forwarding, dragPending/scrollPending would be stuck
	// true permanently -- previewScroll needs this exactly like the two
	// MenuModel-owned scrollbars (a Pending stuck true is precisely why wheel
	// scroll over the preview would scroll one line and then stop).
	switch dmsg := msg.(type) {
	case displayengine.DragDoneMsg:
		var cmds []tea.Cmd
		for _, target := range s.scrollableMenus() {
			updated, uCmd := (*target).Update(dmsg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				*target = m
			}
			cmds = append(cmds, uCmd)
		}
		_, _, _ = s.previewScroll.Update(dmsg, s.previewViewport.YOffset(), s.previewViewport.TotalLineCount(), s.previewViewport.VisibleLineCount())
		return s, tea.Batch(cmds...)
	case displayengine.ScrollDoneMsg:
		var cmds []tea.Cmd
		for _, target := range s.scrollableMenus() {
			updated, uCmd := (*target).Update(dmsg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				*target = m
			}
			cmds = append(cmds, uCmd)
		}
		_, _, _ = s.previewScroll.Update(dmsg, s.previewViewport.YOffset(), s.previewViewport.TotalLineCount(), s.previewViewport.VisibleLineCount())
		return s, tea.Batch(cmds...)
	}

	// Forward raw mouse drag/release events to whichever scrollbar is
	// dragging before the type switch, so the drag continues while AppModel
	// routes events via section-2 priority. Preview's scrollbar lives on the
	// screen itself (previewScroll/previewViewport), not a MenuModel, so it's
	// driven directly rather than through target.Update.
	if s.IsScrollbarDragging() {
		if s.previewScroll.Drag.Dragging {
			switch msg.(type) {
			case tea.MouseMotionMsg, tea.MouseReleaseMsg:
				if newOff, cmd, changed := s.previewScroll.Update(msg, s.previewViewport.YOffset(), s.previewViewport.TotalLineCount(), s.previewViewport.VisibleLineCount()); changed {
					s.previewViewport.SetYOffset(newOff)
					return s, cmd
				}
				return s, nil
			}
		} else {
			target := &s.themeMenu
			for _, t := range s.scrollableMenus() {
				if (*t).IsScrollbarDragging() {
					target = t
					break
				}
			}

			switch msg.(type) {
			case tea.MouseMotionMsg, tea.MouseReleaseMsg:
				updated, uCmd := (*target).Update(msg)
				if m, ok := updated.(*displayengine.MenuModel); ok {
					*target = m
				}
				return s, uCmd
			}
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case displayengine.TitleBarRefreshMsg:
		// Title-bar [↺] icon mirrors the Reset button.
		return s, s.outerMenu.SetProcessingBtnDeferred(displayengine.IDResetButton, s.handleReset())

	case tea.MouseClickMsg:
		// Regular clicks are handled by hit regions (LayerHitMsg).
		// For cases not covered by hit regions (e.g., clicking the background to focus),
		// we rely on AppModel's hover+click focus logic which will send a ToggleFocusedMsg
		// or focus the panel.
		return s, nil

	case displayengine.LayerHitMsg:
		if i, ok := s.tabs.TabFromID(msg.ID); ok {
			return s, s.switchTab(config.ConnTypes[i], true)
		}
		if d, ok := s.tabs.ScrollFromID(msg.ID); ok {
			s.tabs.ScrollBy(d)
			if s.outerMenu != nil {
				s.outerMenu.InvalidateCache()
			}
			return s, nil
		}
		return s.delegateToOuterMenu(msg)

	case tabUnlockedMsg:
		s.unlocked = true
		return s, s.switchTab(msg.connType, msg.focusFrame)

	case tea.MouseWheelMsg, displayengine.ToggleFocusedMsg:
		return s.delegateToOuterMenu(msg)

	case tea.KeyPressMsg:
		// Title bar focus: delegate all keys to outer menu when its title bar is focused.
		if s.outerMenu != nil && s.outerMenu.TitleBarFocused() {
			updated, uCmd := s.outerMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.outerMenu = m
			}
			return s, uCmd
		}
		if key.Matches(msg, displayengine.Keys.Esc) {
			return s, s.EscapeAction()
		}
		switch {
		case key.Matches(msg, displayengine.Keys.TabStripPrev):
			return s, s.cycleTab(-1)
		case key.Matches(msg, displayengine.Keys.TabStripNext):
			return s, s.cycleTab(1)
		}
		return s.delegateToOuterMenu(msg)

	case updateDisplayOptionMsg:
		msg.update(&s.config)
		// Do NOT update baseConfig here; manual changes are staged in s.config
		// and will be lost if the user switches themes (which resets s.config to s.baseConfig).
		// This is consistent with how other options in this screen work.
		s.syncOptionsMenu()
		if s.optionsMenu != nil {
			s.optionsMenu.ClearProcessingState()
		}
		if s.themeMenu != nil {
			s.themeMenu.ClearProcessingState()
		}
		if s.outerMenu != nil {
			s.outerMenu.InvalidateCache()
		}
		return s, nil

	case toggleLoadThemeDefaultsMsg:
		s.loadThemeDefaults = !s.loadThemeDefaults
		s.syncOptionsMenu()
		if s.optionsMenu != nil {
			s.optionsMenu.ClearProcessingState()
		}
		if s.outerMenu != nil {
			s.outerMenu.InvalidateCache()
		}
		return s, nil

	case displayOptionsAbortMsg:
		s.ClearProcessingState()
		return s, nil

	case displayengine.ConfigChangedMsg:
		// Stop any in-flight spinner before rebuilding styles — spinner ticks firing
		// during the rebuild cause intermediate renders that look like a flash.
		if s.outerMenu != nil {
			s.outerMenu.ClearProcessingState()
		}
		// InitStyles (triggered by AppModel) clears the full semantic cache including "Preview_*"
		// styles. Re-establish the preview namespace so the mockup renders correctly.
		if s.previewTheme != "" {
			_, _ = theme.Load(s.previewTheme, "Preview")
			displayengine.ClearSemanticCachePrefix("Preview_")
		}
		if s.outerMenu != nil {
			s.outerMenu.InvalidateCache()
		}
		return s, nil

	}

	// Anything not matched above (notably MenuDeferredActionMsg, the
	// 50ms-delayed tick that actually runs a clicked item's Action) still
	// needs to reach outerMenu.Update -- unlike outerMenu.AbsorbMessage
	// (only checks its own instanceID/button row, never recurses into
	// contentSections), Update's own updateSections routes it correctly
	// through appearanceLayoutRow/ContentColumn to whichever inner menu
	// actually owns it. Without this, a nested item's deferred action (e.g.
	// an Options dropdown) never runs and its spinner never clears.
	return s.delegateToOuterMenu(msg)
}

func (s *DisplayOptionsScreen) applyPreview(themeName string) {
	s.previewTheme = themeName

	// Carry forward every option exactly as currently staged (whatever the
	// user has set so far, or the base config if nothing's changed yet).
	staged := s.config.Appearance
	s.config = s.baseConfig
	s.config.Appearance = staged
	s.config.Appearance.Ptr(s.editType).Theme = themeName

	// Always load to ensure tags are registered in registry
	defaults, err := theme.Load(themeName, "Preview")
	if err != nil {
		shortURI := themeName
		if strings.HasPrefix(themeName, "file:") {
			shortURI = "file:" + theme.ThemeDisplayName(themeName)
		}
		s.previewTheme = "(missing) " + shortURI
	}
	s.themeDefaults[themeName] = defaults

	// When on, the newly-focused theme's own suggested defaults overlay the
	// staged options above -- winning for whatever fields it specifies, and
	// leaving everything else (including any option the user just changed
	// by hand) as-is. When off, theme selection never touches options at all.
	s.themeChangedFields = nil
	if s.loadThemeDefaults && defaults != nil {
		theme.ApplyThemeDefaults(s.config.Appearance.Ptr(s.editType), *defaults)
		// Mark every field the theme's [defaults] table specifies, not just
		// ones whose value actually differed from what was already staged --
		// the marker means "the theme set this", not "this changed".
		s.themeChangedFields = specifiedThemeDefaultFields(*defaults)
	}

	s.syncOptionsMenu()
	if s.outerMenu != nil {
		s.outerMenu.InvalidateCache()
	}
	displayengine.ClearSemanticCachePrefix("Preview_")
}

// optionTagToUIField maps each Options row's Tag to the config.Appearance or
// config.AppearanceConfig struct field name it displays, so syncOptionsMenu can look up
// s.themeChangedFields by the same name diffUIConfigFieldSet produces.
var optionTagToUIField = map[string]string{
	"Shadows":              "Shadow",
	"Borders":              "Borders",
	"Large Buttons":        "LargeButtons",
	"Large Title Bars":     "LargeTitleBars",
	"Line Characters":      "LineCharacters",
	"Scrollbars":           "Scrollbar",
	"Menu Brackets":        "MenuBrackets",
	"Line Number Brackets": "LineNumberBrackets",
	"Shadow Level":         "ShadowLevel",
	"Border Color":         "BorderColor",
	"Dialog Title":         "DialogTitleAlign",
	"Submenu Title":        "SubmenuTitleAlign",
	"Panel Title":          "PanelTitleAlign",
	"Panel Mode":           "Panel",
	"Checkbox Brackets":    "CheckboxBrackets",
	"Radio Brackets":       "RadioBrackets",
	"Tab Layout":           "TabLayout",
	"Show Preview":         "ShowPreview",
	"Markdown Hyperlinks":  "MarkdownHyperlinks",
	"Hyperlinks":           "Hyperlinks",
}

// syncOptionsMenu refreshes the options menu's rows from the staged config.
func (s *DisplayOptionsScreen) syncOptionsMenu() {
	a := s.config.Appearance.Ptr(s.editType)
	items := s.optionsMenu.GetItems()
	for i := range items {
		items[i].IsNew = s.themeChangedFields[optionTagToUIField[items[i].Tag]]
		switch items[i].Tag {
		case "Shadows":
			items[i].Checked = s.config.Appearance.Ptr(s.editType).Shadow
		case "Borders":
			items[i].Checked = s.config.Appearance.Ptr(s.editType).Borders
		case "Large Buttons":
			items[i].Checked = s.config.Appearance.Ptr(s.editType).LargeButtons
		case "Large Title Bars":
			items[i].Checked = s.config.Appearance.Ptr(s.editType).LargeTitleBars
		case "Line Characters":
			items[i].Checked = s.config.Appearance.Ptr(s.editType).LineCharacters
		case "Scrollbars":
			items[i].Checked = s.config.Appearance.Ptr(s.editType).Scrollbar
		case "Menu Brackets":
			items[i].Checked = s.config.Appearance.Ptr(s.editType).MenuBrackets
		case "Line Number Brackets":
			items[i].Checked = s.config.Appearance.Ptr(s.editType).LineNumberBrackets
		case "Show Preview":
			items[i].Checked = a.ShowPreview
		case "Tab Layout":
			items[i].Desc = s.dropdownDesc(tabLayoutDesc(s.config.Appearance.Ptr(s.editType).TabLayout))
		case "Markdown Hyperlinks":
			items[i].Desc = s.dropdownDesc(markdownHyperlinksDesc(a.MarkdownHyperlinks))
		case "Hyperlinks":
			items[i].Desc = s.dropdownDesc(hyperlinksDesc(a.Hyperlinks))
		case "Shadow Level":
			items[i].Desc = s.dropdownDesc(s.shadowLevelToDesc(s.config.Appearance.Ptr(s.editType).ShadowLevel))
		case "Border Color":
			items[i].Desc = s.dropdownDesc(s.borderColorToDesc(s.config.Appearance.Ptr(s.editType).BorderColor))
		case "Dialog Title":
			items[i].Desc = s.dropdownDesc(titleAlignDesc(s.config.Appearance.Ptr(s.editType).DialogTitleAlign))
		case "Submenu Title":
			items[i].Desc = s.dropdownDesc(titleAlignDesc(s.config.Appearance.Ptr(s.editType).SubmenuTitleAlign))
		case "Panel Title":
			items[i].Desc = s.dropdownDesc(titleAlignDesc(s.config.Appearance.Ptr(s.editType).PanelTitleAlign))
		case "Panel Mode":
			items[i].Desc = s.dropdownDesc(s.panelModeToDesc(a.Panel))
		case "Refresh Rate":
			items[i].Desc = fmt.Sprintf("{{|OptionValue|}}%dms{{[-]}}", a.RefreshRate)
		case "Spinner Speed":
			items[i].Desc = fmt.Sprintf("{{|OptionValue|}}%dms{{[-]}}", a.SpinnerSpeed)
		case "Checkbox Brackets":
			items[i].Desc = s.dropdownDesc(bracketModeDesc(s.config.Appearance.Ptr(s.editType).CheckboxBrackets))
		case "Radio Brackets":
			items[i].Desc = s.dropdownDesc(bracketModeDesc(s.config.Appearance.Ptr(s.editType).RadioBrackets))
		}
	}
	s.optionsMenu.SetItems(items)
}

func (s *DisplayOptionsScreen) Title() string {
	return "Display Options"
}

func (s *DisplayOptionsScreen) ShortHelp() []key.Binding {
	return displayengine.Keys.ShortHelp()
}

// FullHelp adds this screen's connection type tab and preview keys to the
// standard help columns.
func (s *DisplayOptionsScreen) FullHelp() [][]key.Binding {
	return append(displayengine.Keys.FullHelp(), []key.Binding{
		displayengine.Keys.TabStripPrev,
		displayengine.Keys.TabStripNext,
		displayengine.Keys.PreviewForward,
		displayengine.Keys.PreviewBack,
	})
}

func (s *DisplayOptionsScreen) HelpText() string {
	if m := s.focusedSettingsMenu(); m != nil {
		return m.HelpText()
	}
	return "Tab to cycle panels, Enter to Apply, Esc to Cancel"
}

func (s *DisplayOptionsScreen) SetSize(width, height int) {
	s.width = width
	s.height = height

	if s.outerMenu == nil {
		return
	}

	// outerMenu.SetSize propagates to sections via calculateSectionLayout(),
	// including the settings/preview row. The preview itself is rebuilt in
	// ViewString (called every render), not here (called only on resize) --
	// see ViewString's comment for why.
	s.outerMenu.SetSize(width, height)
}

// IsMaximized reports false so model_view.go's generic centering positions
// the whole (naturally-sized) outerMenu dialog within the content area --
// settings and preview are now one dialog, not two independently-positioned
// panels needing a custom Layers() override.
func (s *DisplayOptionsScreen) IsMaximized() bool {
	return false
}

// EscapeAction implements tui.EscapeActioner: mirrors the Esc key handler.
func (s *DisplayOptionsScreen) EscapeAction() tea.Cmd {
	theme.Unload("Preview")
	if s.isRoot {
		return s.outerMenu.SetProcessingBtnDeferred(displayengine.IDExitButton, tui.ConfirmExitAction())
	}
	return s.outerMenu.SetProcessingBtnDeferred(displayengine.IDBackButton, navigateBack())
}

// ClearProcessingState clears spinner state on all inner menus.
// Called by AppModel when a dialog closes and returns focus to this screen.
func (s *DisplayOptionsScreen) ClearProcessingState() {
	if s.optionsMenu != nil {
		s.optionsMenu.ClearProcessingState()
	}
	if s.themeMenu != nil {
		s.themeMenu.ClearProcessingState()
	}
	if s.outerMenu != nil {
		s.outerMenu.ClearProcessingState()
	}
}

func (s *DisplayOptionsScreen) HasDialog() bool {
	if s.themeMenu == nil || s.optionsMenu == nil {
		return false
	}
	return s.themeMenu.HasDialog() || s.optionsMenu.HasDialog()
}

// MinHeight returns the minimum content-area height needed for the Appearance Settings
// screen to remain interactive. Used by AppModel to limit log panel expansion.
// Breakdown: outer border(2) + theme section(5) + options section(4) + bordered buttons(3) = 14.
func (s *DisplayOptionsScreen) MinHeight() int {
	return 14
}
