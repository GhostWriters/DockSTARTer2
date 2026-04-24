package screens

import (
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// itemConfigValue returns the config value (e.g. "user:MyTheme") for a theme menu item.
// Falls back to Tag (display name) if no Metadata entry was set.
func itemConfigValue(item tui.MenuItem) string {
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
	var cmd tea.Cmd

	// Forward coalescing done-messages to whichever inner menu owns them.
	// These messages are sent by inner menus' dragDoneCmd/scrollDoneCmd after a render cycle.
	// Without forwarding, dragPending/scrollPending would be stuck true permanently on inner menus.
	switch dmsg := msg.(type) {
	case tui.DragDoneMsg:
		updated, uCmd := s.themeMenu.Update(dmsg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.themeMenu = m
		}
		updated, uCmd2 := s.optionsMenu.Update(dmsg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.optionsMenu = m
		}
		return s, tea.Batch(uCmd, uCmd2)
	case tui.ScrollDoneMsg:
		updated, uCmd := s.themeMenu.Update(dmsg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.themeMenu = m
		}
		updated, uCmd2 := s.optionsMenu.Update(dmsg)
		if m, ok := updated.(*tui.MenuModel); ok {
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
			if m, ok := updated.(*tui.MenuModel); ok {
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
			if m, ok := updated.(*tui.MenuModel); ok {
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
		case FocusThemes:
			updated, uCmd := s.themeMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.themeMenu = m
			}
			return s, uCmd
		case FocusOptions:
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
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

	case tui.LayerHitMsg:
		// 1. Focus routing via panel hit
		switch msg.ID {
		case tui.IDThemePanel:
			s.focusedPanel = FocusThemes
			s.updateFocusStates()
			return s, nil
		case tui.IDOptionsPanel:
			s.focusedPanel = FocusOptions
			s.updateFocusStates()
			return s, nil
		case tui.IDButtonPanel:
			s.focusedPanel = FocusButtons
			s.updateFocusStates()
			return s, nil
		}

		// 2. Button actions (global buttons not belonging to a sub-menu)
		if tui.ButtonIDMatches(msg.ID, tui.IDApplyButton) {
			if msg.Button == tea.MouseLeft {
				s.focusedPanel = FocusButtons
				s.focusedButton = 0
				s.updateFocusStates()
				return s, s.handleApply()
			}
			if msg.Button == tui.HoverButton {
				s.focusedPanel = FocusButtons
				s.focusedButton = 0
				s.updateFocusStates()
				return s, nil
			}
		} else if tui.ButtonIDMatches(msg.ID, tui.IDBackButton) {
			if msg.Button == tea.MouseLeft {
				s.focusedPanel = FocusButtons
				s.focusedButton = 1
				s.updateFocusStates()
				if s.isRoot {
					return s, nil
				}
				theme.Unload("Preview")
				return s, navigateBack()
			}
			if msg.Button == tui.HoverButton {
				s.focusedPanel = FocusButtons
				s.focusedButton = 1
				s.updateFocusStates()
				return s, nil
			}
		} else if tui.ButtonIDMatches(msg.ID, tui.IDExitButton) {
			if msg.Button == tea.MouseLeft {
				s.focusedPanel = FocusButtons
				s.focusedButton = s.maxFocusedButton()
				s.updateFocusStates()
				theme.Unload("Preview")
				return s, tui.ConfirmExitAction()
			}
			if msg.Button == tui.HoverButton {
				s.focusedPanel = FocusButtons
				s.focusedButton = s.maxFocusedButton()
				s.updateFocusStates()
				return s, nil
			}
		}

		// 3. Delegation to sub-menus (handles items and internal buttons)
		if strings.Contains(msg.ID, tui.IDThemePanel) {
			s.focusedPanel = FocusThemes
			s.updateFocusStates()
			updated, uCmd := s.themeMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.themeMenu = m
			}
			// Hook for theme preview: if theme changed (Left Click), apply it
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
		} else if strings.Contains(msg.ID, tui.IDOptionsPanel) {
			s.focusedPanel = FocusOptions
			s.updateFocusStates()
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		}

	case tui.ToggleFocusedMsg:
		// Middle click: activate the currently focused item in the hovered panel
		switch s.focusedPanel {
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
			if m, ok := updated.(*tui.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		case FocusButtons:
			return s.execFocusedButton()
		}
		return s, nil

	case tea.KeyPressMsg:
		// 1. Panel Cycling (Tab / Shift-Tab) - Themes <-> Options only
		if key.Matches(msg, tui.Keys.CycleTab) || key.Matches(msg, tui.Keys.CycleShiftTab) {
			if s.focusedPanel == FocusThemes {
				s.focusedPanel = FocusOptions
			} else {
				s.focusedPanel = FocusThemes
			}
			s.updateFocusStates()
			return s, nil
		}

		// 2. Strict Navigation (Workstation Model)

		// Left/Right: Cycle buttons globally
		if key.Matches(msg, tui.Keys.Left) {
			s.focusedPanel = FocusButtons
			s.focusedButton--
			if s.focusedButton < 0 {
				s.focusedButton = s.maxFocusedButton()
			}
			s.updateFocusStates()
			return s, nil
		}
		if key.Matches(msg, tui.Keys.Right) {
			s.focusedPanel = FocusButtons
			s.focusedButton++
			if s.focusedButton > s.maxFocusedButton() {
				s.focusedButton = 0
			}
			s.updateFocusStates()
			return s, nil
		}

		if key.Matches(msg, tui.Keys.Enter) {
			return s.execFocusedButton()
		}

		// Esc: Cancel — navigate back or quit if root
		if key.Matches(msg, tui.Keys.Esc) {
			theme.Unload("Preview")
			if s.isRoot {
				return s, tui.ConfirmExitAction()
			}
			return s, navigateBack()
		}

		// 3. Up/Down/Space: Routed to focused panel
		switch s.focusedPanel {
		case FocusThemes:
			// Specific radio logic for Space on theme list
			if key.Matches(msg, tui.Keys.Space) {
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
			if m, ok := updated.(*tui.MenuModel); ok {
				s.themeMenu = m
			}
			return s, uCmd
		case FocusOptions:
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
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
		if s.outerMenu != nil {
			s.outerMenu.InvalidateCache()
		}
		return s, nil

	case tui.ConfigChangedMsg:
		// InitStyles (triggered by AppModel) clears the full semantic cache including "Preview_*"
		// styles. Re-establish the preview namespace so the mockup renders correctly.
		if s.previewTheme != "" {
			_, _ = theme.Load(s.previewTheme, "Preview")
			tui.ClearSemanticCachePrefix("Preview_")
		}
		if s.outerMenu != nil {
			s.outerMenu.InvalidateCache()
		}
		return s, nil

	case tui.LockStateChangedMsg:
		updated, uCmd := s.themeMenu.Update(msg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.themeMenu = m
		}
		updated, uCmd2 := s.optionsMenu.Update(msg)
		if m, ok := updated.(*tui.MenuModel); ok {
			s.optionsMenu = m
		}
		return s, tea.Batch(uCmd, uCmd2)
	}

	return s, cmd
}

func (s *DisplayOptionsScreen) applyPreview(themeName string) {
	s.previewTheme = themeName

	// Preserve all staged UI options the user has changed interactively.
	// We only reset s.config to baseConfig so that theme defaults get a clean
	// base, then we re-apply the user's staged choices on top.
	staged := s.config.UI

	// Reset to base configs
	s.config = s.baseConfig

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

	if defaults != nil {
		theme.ApplyThemeDefaults(&s.config, *defaults)
	}

	// Re-apply staged UI options on top of theme defaults.
	// This preserves choices like PanelLocal/PanelRemote, Borders, Shadow, etc.
	// that the user has changed since opening this screen.
	s.config.UI.Borders = staged.Borders
	s.config.UI.ButtonBorders = staged.ButtonBorders
	s.config.UI.LineCharacters = staged.LineCharacters
	s.config.UI.Shadow = staged.Shadow
	s.config.UI.ShadowLevel = staged.ShadowLevel
	s.config.UI.Scrollbar = staged.Scrollbar
	s.config.UI.BorderColor = staged.BorderColor
	s.config.UI.DialogTitleAlign = staged.DialogTitleAlign
	s.config.UI.SubmenuTitleAlign = staged.SubmenuTitleAlign
	s.config.UI.PanelTitleAlign = staged.PanelTitleAlign
	s.config.UI.PanelLocal = staged.PanelLocal
	s.config.UI.PanelRemote = staged.PanelRemote

	s.syncOptionsMenu()
	if s.outerMenu != nil {
		s.outerMenu.InvalidateCache()
	}
	tui.ClearSemanticCachePrefix("Preview_")
}

func (s *DisplayOptionsScreen) syncOptionsMenu() {
	items := s.optionsMenu.GetItems()
	for i := range items {
		switch items[i].Tag {
		case "Borders":
			items[i].Checked = s.config.UI.Borders
		case "Button Borders":
			items[i].Checked = s.config.UI.ButtonBorders
		case "Line Characters":
			items[i].Checked = s.config.UI.LineCharacters
		case "Shadow":
			items[i].Checked = s.config.UI.Shadow
		case "Scrollbar":
			items[i].Checked = s.config.UI.Scrollbar
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
		}
	}
	s.optionsMenu.SetItems(items)
}

func (s *DisplayOptionsScreen) Title() string {
	return "Display Options"
}

func (s *DisplayOptionsScreen) HelpText() string {
	if s.themeMenu == nil || s.optionsMenu == nil {
		return ""
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
	layout := tui.GetLayout()
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

func (s *DisplayOptionsScreen) IsMaximized() bool {
	return true
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
