package screens

import (
	"reflect"
	"strings"

	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// specifiedThemeDefaultFields returns the set of config.UIConfig struct field
// names a theme's [defaults] table specifies (i.e. whose pointer is non-nil
// in d) -- theme.ThemeDefaults' field names match config.UIConfig's 1:1 for
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

// IsScrollbarDragging reports whether any sub-menu is currently dragging a scrollbar thumb.
func (s *DisplayOptionsScreen) IsScrollbarDragging() bool {
	return s.themeMenu.IsScrollbarDragging() || s.optionsMenu.IsScrollbarDragging()
}

func (s *DisplayOptionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Every inner menu must see its own deferred-action messages (button
	// clicks on outerMenu, item Action clicks like the Options dropdowns on
	// optionsMenu/themeMenu) before any early-return branch below can drop
	// them -- each menu's menuDeferredActionMsg is scoped to its own
	// instanceID, so only that menu can absorb it.
	if s.outerMenu != nil {
		if action := s.outerMenu.AbsorbMessage(msg); action != nil {
			return s, action
		}
	}
	if s.optionsMenu != nil {
		if action := s.optionsMenu.AbsorbMessage(msg); action != nil {
			return s, action
		}
	}
	if s.themeMenu != nil {
		if action := s.themeMenu.AbsorbMessage(msg); action != nil {
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

	var cmd tea.Cmd

	// Forward coalescing done-messages to whichever inner menu owns them.
	// These messages are sent by inner menus' dragDoneCmd/scrollDoneCmd after a render cycle.
	// Without forwarding, dragPending/scrollPending would be stuck true permanently on inner menus.
	switch dmsg := msg.(type) {
	case displayengine.DragDoneMsg:
		updated, uCmd := s.themeMenu.Update(dmsg)
		if m, ok := updated.(*displayengine.MenuModel); ok {
			s.themeMenu = m
		}
		updated, uCmd2 := s.optionsMenu.Update(dmsg)
		if m, ok := updated.(*displayengine.MenuModel); ok {
			s.optionsMenu = m
		}
		return s, tea.Batch(uCmd, uCmd2)
	case displayengine.ScrollDoneMsg:
		updated, uCmd := s.themeMenu.Update(dmsg)
		if m, ok := updated.(*displayengine.MenuModel); ok {
			s.themeMenu = m
		}
		updated, uCmd2 := s.optionsMenu.Update(dmsg)
		if m, ok := updated.(*displayengine.MenuModel); ok {
			s.optionsMenu = m
		}
		return s, tea.Batch(uCmd, uCmd2)
	}

	// Forward raw mouse drag/release events to the dragging sub-menu before the type switch
	// so the drag continues while AppModel routes events via section-2 priority.
	if s.IsScrollbarDragging() {
		target := s.themeMenu
		if s.optionsMenu.IsScrollbarDragging() {
			target = s.optionsMenu
		}

		if _, ok := msg.(tea.MouseMotionMsg); ok {
			updated, uCmd := target.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				if target == s.themeMenu {
					s.themeMenu = m
				} else {
					s.optionsMenu = m
				}
			}
			return s, uCmd
		}
		if _, ok := msg.(tea.MouseReleaseMsg); ok {
			updated, uCmd := target.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				if target == s.themeMenu {
					s.themeMenu = m
				} else {
					s.optionsMenu = m
				}
			}
			return s, uCmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case tea.MouseClickMsg:
		// Regular clicks are handled by hit regions (LayerHitMsg).
		// For cases not covered by hit regions (e.g., clicking the background to focus),
		// we rely on AppModel's hover+click focus logic which will send a ToggleFocusedMsg
		// or focus the panel.
		return s, nil

	case tea.MouseWheelMsg:
		// ONLY interact with the focused panel, no mouse-over fallback
		switch s.focusedPanel {
		case FocusLoadDefaults:
			updated, uCmd := s.loadDefaultsMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.loadDefaultsMenu = m
			}
			return s, uCmd
		case FocusThemes:
			updated, uCmd := s.themeMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.themeMenu = m
			}
			return s, uCmd
		case FocusOptions:
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		case FocusButtons:
			// Scroll wheel cycles the focused button (up=left, down=right) — clamps, no wrap.
			maxBtn := s.maxFocusedButton()
			switch msg.Button {
			case tea.MouseWheelUp:
				if s.focusedButton > 0 {
					s.focusedButton--
				}
			case tea.MouseWheelDown:
				if s.focusedButton < maxBtn {
					s.focusedButton++
				}
			}
			return s, nil
		}
		return s, nil

	case displayengine.LayerHitMsg:
		// 1. Focus routing via panel hit
		switch msg.ID {
		case displayengine.IDLoadDefaultsPanel:
			s.buttonFocused = false
			s.focusedPanel = FocusLoadDefaults
			s.updateFocusStates()
			return s, nil
		case displayengine.IDThemePanel:
			s.buttonFocused = false
			s.focusedPanel = FocusThemes
			s.updateFocusStates()
			return s, nil
		case displayengine.IDOptionsPanel:
			s.buttonFocused = false
			s.focusedPanel = FocusOptions
			s.updateFocusStates()
			return s, nil
		case displayengine.IDButtonPanel:
			s.buttonFocused = false
			s.focusedPanel = FocusButtons
			s.updateFocusStates()
			return s, nil
		}

		// 2. Button actions (global buttons not belonging to a sub-menu)
		if displayengine.ButtonIDMatches(msg.ID, displayengine.IDApplyButton) {
			if msg.Button == tea.MouseLeft {
				s.focusedPanel = FocusButtons
				s.focusedButton = 0
				s.updateFocusStates()
				return s, s.outerMenu.SetProcessingBtnDeferred(displayengine.IDApplyButton, s.handleApply())
			}
			if msg.Button == displayengine.HoverButton {
				s.focusedPanel = FocusButtons
				s.focusedButton = 0
				s.updateFocusStates()
				return s, nil
			}
		} else if displayengine.ButtonIDMatches(msg.ID, displayengine.IDBackButton) {
			if msg.Button == tea.MouseLeft {
				s.focusedPanel = FocusButtons
				s.focusedButton = 1
				s.updateFocusStates()
				if s.isRoot {
					return s, nil
				}
				theme.Unload("Preview")
				return s, s.outerMenu.SetProcessingBtnDeferred(displayengine.IDBackButton, navigateBack())
			}
			if msg.Button == displayengine.HoverButton {
				s.focusedPanel = FocusButtons
				s.focusedButton = 1
				s.updateFocusStates()
				return s, nil
			}
		} else if displayengine.ButtonIDMatches(msg.ID, displayengine.IDExitButton) {
			if msg.Button == tea.MouseLeft {
				s.focusedPanel = FocusButtons
				s.focusedButton = s.maxFocusedButton()
				s.updateFocusStates()
				theme.Unload("Preview")
				return s, s.outerMenu.SetProcessingBtnDeferred(displayengine.IDExitButton, tui.ConfirmExitAction())
			}
			if msg.Button == displayengine.HoverButton {
				s.focusedPanel = FocusButtons
				s.focusedButton = s.maxFocusedButton()
				s.updateFocusStates()
				return s, nil
			}
		}

		// 3. Delegation to sub-menus (handles items and internal buttons)
		if strings.Contains(msg.ID, displayengine.IDLoadDefaultsPanel) {
			s.focusedPanel = FocusLoadDefaults
			s.updateFocusStates()
			updated, uCmd := s.loadDefaultsMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.loadDefaultsMenu = m
			}
			return s, uCmd
		} else if strings.Contains(msg.ID, displayengine.IDThemePanel) {
			s.focusedPanel = FocusThemes
			s.updateFocusStates()
			updated, uCmd := s.themeMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.themeMenu = m
			}
			// Hook for theme preview: if theme changed (Left Click), apply it
			// and update the radio Checked states.
			if msg.Button == tea.MouseLeft && strings.HasPrefix(msg.ID, "item-") {
				idx := s.themeMenu.Index()
				items := s.themeMenu.GetItems()
				if idx >= 0 && idx < len(items) {
					// Update radio button states
					for i := range items {
						items[i].Checked = (i == idx)
					}
					s.themeMenu.SetItems(items)
					s.applyPreview(itemConfigValue(items[idx]))
				}
			}
			return s, uCmd
		} else if strings.Contains(msg.ID, displayengine.IDOptionsPanel) {
			s.focusedPanel = FocusOptions
			s.updateFocusStates()
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		}

		// Title widget clicks — delegate to outerMenu
		if s.outerMenu != nil && displayengine.IsTitleWidgetID(msg.ID) {
			updated, uCmd := s.outerMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.outerMenu = m
			}
			return s, uCmd
		}

	case displayengine.ToggleFocusedMsg:
		// Middle click: activate the currently focused item in the hovered panel
		switch s.focusedPanel {
		case FocusLoadDefaults:
			updated, uCmd := s.loadDefaultsMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.loadDefaultsMenu = m
			}
			return s, uCmd
		case FocusThemes:
			// Activate radio item
			idx := s.themeMenu.Index()
			items := s.themeMenu.GetItems()
			if idx >= 0 && idx < len(items) {
				for i := range items {
					items[i].Checked = (i == idx)
				}
				s.themeMenu.SetItems(items)
				s.applyPreview(itemConfigValue(items[idx]))
			}
			return s, nil
		case FocusOptions:
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		case FocusButtons:
			return s.execFocusedButton()
		}
		return s, nil

	case tea.KeyPressMsg:
		// Title bar focus: delegate all keys to outer menu when its title bar is focused.
		if s.outerMenu != nil && s.outerMenu.TitleBarFocused() {
			updated, uCmd := s.outerMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.outerMenu = m
			}
			return s, uCmd
		}

		// 1. Panel Cycling (Tab / Shift-Tab) - LoadDefaults -> Themes -> Options -> LoadDefaults
		if key.Matches(msg, displayengine.Keys.CycleTab) {
			s.buttonFocused = false
			switch s.focusedPanel {
			case FocusLoadDefaults:
				s.focusedPanel = FocusThemes
			case FocusThemes:
				s.focusedPanel = FocusOptions
			default:
				s.focusedPanel = FocusLoadDefaults
			}
			s.updateFocusStates()
			return s, nil
		}
		if key.Matches(msg, displayengine.Keys.CycleShiftTab) {
			s.buttonFocused = false
			switch s.focusedPanel {
			case FocusOptions:
				s.focusedPanel = FocusThemes
			case FocusThemes:
				s.focusedPanel = FocusLoadDefaults
			default:
				s.focusedPanel = FocusOptions
			}
			s.updateFocusStates()
			return s, nil
		}

		// 2. Strict Navigation (Workstation Model)

		// Left/Right: cycle buttons; when on a submenu, keep it focused too (buttonFocused).
		if key.Matches(msg, displayengine.Keys.Left) {
			if s.focusedPanel == FocusButtons || s.buttonFocused {
				s.buttonFocused = s.focusedPanel != FocusButtons
				s.focusedButton--
				if s.focusedButton < 0 {
					s.focusedButton = s.maxFocusedButton()
				}
			} else {
				s.buttonFocused = true
				s.focusedButton = s.maxFocusedButton()
			}
			s.updateFocusStates()
			return s, nil
		}
		if key.Matches(msg, displayengine.Keys.Right) {
			if s.focusedPanel == FocusButtons || s.buttonFocused {
				s.buttonFocused = s.focusedPanel != FocusButtons
				s.focusedButton++
				if s.focusedButton > s.maxFocusedButton() {
					s.focusedButton = 0
				}
			} else {
				s.buttonFocused = true
				s.focusedButton = 0
			}
			s.updateFocusStates()
			return s, nil
		}

		if key.Matches(msg, displayengine.Keys.Enter) {
			if s.buttonFocused || s.focusedPanel == FocusButtons {
				s.buttonFocused = false
				return s.execFocusedButton()
			}
		}

		// Esc: Cancel — same as clicking the close widget
		if key.Matches(msg, displayengine.Keys.Esc) {
			if s.buttonFocused {
				s.buttonFocused = false
				s.updateFocusStates()
			}
			return s, s.EscapeAction()
		}

		// 3. Up/Down/Space: Routed to focused panel; clear button highlight when navigating submenus.
		if key.Matches(msg, displayengine.Keys.Up) || key.Matches(msg, displayengine.Keys.Down) {
			s.buttonFocused = false
			s.updateFocusStates()
		}
		switch s.focusedPanel {
		case FocusLoadDefaults:
			updated, uCmd := s.loadDefaultsMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.loadDefaultsMenu = m
			}
			return s, uCmd
		case FocusThemes:
			// Specific radio logic for Space on theme list
			if key.Matches(msg, displayengine.Keys.Space) {
				items := s.themeMenu.GetItems()
				cursor := s.themeMenu.Index()
				if cursor >= 0 && cursor < len(items) {
					for i := range items {
						items[i].Checked = (i == cursor)
					}
					s.themeMenu.SetItems(items)
					s.applyPreview(itemConfigValue(items[cursor]))
					return s, nil
				}
			}

			updated, uCmd := s.themeMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.themeMenu = m
			}
			return s, uCmd
		case FocusOptions:
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*displayengine.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		}

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

	return s, cmd
}

func (s *DisplayOptionsScreen) applyPreview(themeName string) {
	s.previewTheme = themeName

	// Carry forward every option exactly as currently staged (whatever the
	// user has set so far, or the base config if nothing's changed yet).
	staged := s.config.UI
	s.config = s.baseConfig
	s.config.UI = staged

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
		theme.ApplyThemeDefaults(&s.config, *defaults)
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

// optionTagToUIField maps each Options row's Tag to the config.UIConfig
// struct field name it displays, so syncOptionsMenu can look up
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
	"Log Title":            "PanelTitleAlign",
	"Local Panel Mode":     "PanelLocal",
	"Remote Panel Mode":    "PanelRemote",
	"Checkbox Brackets":    "CheckboxBrackets",
	"Radio Brackets":       "RadioBrackets",
}

func (s *DisplayOptionsScreen) syncOptionsMenu() {
	items := s.optionsMenu.GetItems()
	for i := range items {
		items[i].IsNew = s.themeChangedFields[optionTagToUIField[items[i].Tag]]
		switch items[i].Tag {
		case "Shadows":
			items[i].Checked = s.config.UI.Shadow
		case "Borders":
			items[i].Checked = s.config.UI.Borders
		case "Large Buttons":
			items[i].Checked = s.config.UI.LargeButtons
		case "Large Title Bars":
			items[i].Checked = s.config.UI.LargeTitleBars
		case "Line Characters":
			items[i].Checked = s.config.UI.LineCharacters
		case "Scrollbars":
			items[i].Checked = s.config.UI.Scrollbar
		case "Menu Brackets":
			items[i].Checked = s.config.UI.MenuBrackets
		case "Line Number Brackets":
			items[i].Checked = s.config.UI.LineNumberBrackets
		case "Shadow Level":
			items[i].Desc = s.dropdownDesc(s.shadowLevelToDesc(s.config.UI.ShadowLevel))
		case "Border Color":
			items[i].Desc = s.dropdownDesc(s.borderColorToDesc(s.config.UI.BorderColor))
		case "Dialog Title":
			items[i].Desc = s.dropdownDesc(titleAlignDesc(s.config.UI.DialogTitleAlign))
		case "Submenu Title":
			items[i].Desc = s.dropdownDesc(titleAlignDesc(s.config.UI.SubmenuTitleAlign))
		case "Log Title":
			items[i].Desc = s.dropdownDesc(titleAlignDesc(s.config.UI.PanelTitleAlign))
		case "Local Panel Mode":
			items[i].Desc = s.dropdownDesc(s.panelModeToDesc(s.config.UI.PanelLocal))
		case "Remote Panel Mode":
			items[i].Desc = s.dropdownDesc(s.panelModeToDesc(s.config.UI.PanelRemote))
		case "Checkbox Brackets":
			items[i].Desc = s.dropdownDesc(bracketModeDesc(s.config.UI.CheckboxBrackets))
		case "Radio Brackets":
			items[i].Desc = s.dropdownDesc(bracketModeDesc(s.config.UI.RadioBrackets))
		}
	}
	s.optionsMenu.SetItems(items)
}

func (s *DisplayOptionsScreen) Title() string {
	return "Display Options"
}

func (s *DisplayOptionsScreen) HelpText() string {
	if s.loadDefaultsMenu == nil || s.themeMenu == nil || s.optionsMenu == nil {
		return ""
	}
	if s.focusedPanel == FocusLoadDefaults {
		return s.loadDefaultsMenu.HelpText()
	}
	if s.focusedPanel == FocusThemes {
		return s.themeMenu.HelpText()
	}
	if s.focusedPanel == FocusOptions {
		return s.optionsMenu.HelpText()
	}
	return "Tab to cycle panels, Enter to Apply, Esc to Cancel"
}

// panelLayout holds the computed split-panel sizing for DisplayOptionsScreen.
// Used by SetSize, ViewString, Layers, and GetHitRegions to guarantee consistent calculations.
type panelLayout struct {
	previewFits         bool
	settingsDialogWidth int // outer width passed to renderSettingsDialog
	menuWidth           int // inner content width for sub-menus, buttons, and hit regions
}

// Panel layout constants — single definition used by all rendering paths.
const (
	displayPreviewInnerWidth = 44 // preview inner content width (border added via BorderWidth)
	displayPreviewMinWidth   = 50 // minimum preview panel width
	displayMinMenuWidth      = 40 // minimum settings menu inner content width
)

// computePanelLayout is the single source of truth for the split-panel layout.
// All rendering paths (SetSize, ViewString, Layers, GetHitRegions) delegate here.
func (s *DisplayOptionsScreen) computePanelLayout(width int) panelLayout {
	layout := displayengine.GetLayout()
	gutter := layout.VisualGutter(tui.IsShadowEnabled())

	fullPreviewW := displayPreviewInnerWidth + layout.BorderWidth()
	minSettingsOuterW := displayMinMenuWidth + layout.BorderWidth()
	previewFits := width >= minSettingsOuterW+gutter+displayPreviewMinWidth

	var settingsDialogWidth int
	if previewFits {
		// Subtract shadow space (= BorderWidth) so outer dialog + shadow fits in settingsW.
		settingsW := (width - fullPreviewW) - gutter
		settingsDialogWidth = settingsW - layout.BorderWidth()
	} else {
		// Reserve shadow space on the right for manual composition in ViewString.
		settingsDialogWidth = width - layout.BorderWidth()
	}
	if settingsDialogWidth < minSettingsOuterW {
		settingsDialogWidth = minSettingsOuterW
	}

	menuWidth := settingsDialogWidth - layout.BorderWidth()
	if menuWidth < displayMinMenuWidth {
		menuWidth = displayMinMenuWidth
	}

	return panelLayout{
		previewFits:         previewFits,
		settingsDialogWidth: settingsDialogWidth,
		menuWidth:           menuWidth,
	}
}

func (s *DisplayOptionsScreen) SetSize(width, height int) {
	s.width = width
	s.height = height

	if s.outerMenu == nil {
		return
	}

	dl := s.computePanelLayout(width)
	// outerMenu.SetSize propagates to sections via calculateSectionLayout().
	s.outerMenu.SetSize(dl.settingsDialogWidth, height)
}

// IsMaximized reports true so model_view.go's generic outer centering (one
// shared offset applied uniformly to every layer) is skipped -- Layers()
// computes each panel's own Y independently instead, so the settings dialog
// and the preview mockup can differ in height without one shifting the
// other's position.
func (s *DisplayOptionsScreen) IsMaximized() bool {
	return true
}

// EscapeAction implements tui.EscapeActioner: mirrors the Esc key handler.
func (s *DisplayOptionsScreen) EscapeAction() tea.Cmd {
	theme.Unload("Preview")
	if s.isRoot {
		s.focusedPanel = FocusButtons
		s.focusedButton = s.maxFocusedButton()
		s.updateFocusStates()
		return s.outerMenu.SetProcessingBtnDeferred(displayengine.IDExitButton, tui.ConfirmExitAction())
	}
	s.focusedPanel = FocusButtons
	s.focusedButton = 1 // Back
	s.updateFocusStates()
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
