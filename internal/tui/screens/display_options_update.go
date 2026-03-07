package screens

import (
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (s *DisplayOptionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if s.outerMenu != nil {
		s.outerMenu.InvalidateCache()
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case tea.MouseWheelMsg:
		// ONLY interact with the focused panel, no mouse-over fallback
		if s.focusedPanel == FocusThemes {
			updated, uCmd := s.themeMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.themeMenu = m
			}
			return s, uCmd
		} else if s.focusedPanel == FocusOptions {
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		} else if s.focusedPanel == FocusButtons {
			// Scroll wheel cycles the focused button (up=left, down=right) — clamps, no wrap.
			maxBtn := s.maxFocusedButton()
			if msg.Button == tea.MouseWheelUp {
				if s.focusedButton > 0 {
					s.focusedButton--
				}
			} else if msg.Button == tea.MouseWheelDown {
				if s.focusedButton < maxBtn {
					s.focusedButton++
				}
			}
			return s, nil
		}
		return s, nil

	case tui.LayerHitMsg:
		// 1. Focus routing via panel hit
		if msg.ID == tui.IDThemePanel {
			s.focusedPanel = FocusThemes
			s.updateFocusStates()
			return s, nil
		} else if msg.ID == tui.IDOptionsPanel {
			s.focusedPanel = FocusOptions
			s.updateFocusStates()
			return s, nil
		} else if msg.ID == tui.IDButtonPanel {
			s.focusedPanel = FocusButtons
			s.updateFocusStates()
			return s, nil
		}

		// 2. Button actions (global buttons not belonging to a sub-menu)
		switch msg.ID {
		case tui.IDApplyButton:
			s.focusedPanel = FocusButtons
			s.focusedButton = 0
			s.updateFocusStates()
			if msg.Button == tui.HoverButton {
				return s, nil
			}
			return s, s.handleApply()
		case tui.IDBackButton:
			s.focusedPanel = FocusButtons
			s.focusedButton = 1
			s.updateFocusStates()
			if msg.Button == tui.HoverButton {
				return s, nil
			}
			if s.isRoot {
				return s, nil
			}
			theme.Unload("Preview")
			return s, navigateBack()
		case tui.IDExitButton:
			s.focusedPanel = FocusButtons
			s.focusedButton = s.maxFocusedButton()
			s.updateFocusStates()
			if msg.Button == tui.HoverButton {
				return s, nil
			}
			theme.Unload("Preview")
			return s, tui.ConfirmExitAction()
		}

		// 3. Delegation to sub-menus (handles items and internal buttons)
		if strings.Contains(msg.ID, tui.IDThemePanel) {
			s.focusedPanel = FocusThemes
			s.updateFocusStates()
			updated, uCmd := s.themeMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.themeMenu = m
			}
			// Hook for theme preview: if theme changed, apply it
			if strings.HasPrefix(msg.ID, "item-") {
				idx := s.themeMenu.Index()
				items := s.themeMenu.GetItems()
				if idx >= 0 && idx < len(items) {
					// Update radio button states
					for i := range items {
						items[i].Checked = (i == idx)
					}
					s.themeMenu.SetItems(items)
					s.applyPreview(items[idx].Tag)
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
		if s.focusedPanel == FocusThemes {
			// Activate radio item
			idx := s.themeMenu.Index()
			items := s.themeMenu.GetItems()
			if idx >= 0 && idx < len(items) {
				for i := range items {
					items[i].Checked = (i == idx)
				}
				s.themeMenu.SetItems(items)
				s.applyPreview(items[idx].Tag)
			}
			return s, nil
		} else if s.focusedPanel == FocusOptions {
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		} else if s.focusedPanel == FocusButtons {
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
		if s.focusedPanel == FocusThemes {
			// Specific radio logic for Space on theme list
			if key.Matches(msg, tui.Keys.Space) {
				items := s.themeMenu.GetItems()
				cursor := s.themeMenu.Index()
				if cursor >= 0 && cursor < len(items) {
					for i := range items {
						items[i].Checked = (i == cursor)
					}
					s.themeMenu.SetItems(items)
					s.applyPreview(items[cursor].Tag)
					return s, nil
				}
			}

			updated, uCmd := s.themeMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.themeMenu = m
			}
			return s, uCmd
		} else if s.focusedPanel == FocusOptions {
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		}

	case updateDisplayOptionMsg:
		msg.update(&s.config)
		msg.update(&s.baseConfig) // User actively changed an option, save it to base config
		s.syncOptionsMenu()
		return s, nil
	}

	return s, cmd
}

func (s *DisplayOptionsScreen) applyPreview(themeName string) {
	s.previewTheme = themeName

	// Reset to base configs
	s.config = s.baseConfig

	// Always load to ensure tags are registered in registry
	defaults, err := theme.Load(themeName, "Preview")
	if err != nil {
		s.previewTheme = "ERR: " + err.Error()
	}
	s.themeDefaults[themeName] = defaults

	if defaults != nil {
		theme.ApplyThemeDefaults(&s.config, *defaults)
	}
	s.syncOptionsMenu()
	tui.ClearSemanticCachePrefix("Preview_Theme_")
}

func (s *DisplayOptionsScreen) syncOptionsMenu() {
	items := s.optionsMenu.GetItems()
	items[0].Checked = s.config.UI.Borders
	items[1].Checked = s.config.UI.LineCharacters
	items[2].Checked = s.config.UI.Shadow
	// Update dropdown descriptions
	items[3].Desc = s.dropdownDesc(s.shadowLevelToDesc(s.config.UI.ShadowLevel))
	items[4].Desc = s.dropdownDesc(s.borderColorToDesc(s.config.UI.BorderColor))
	items[5].Desc = s.dropdownDesc(titleAlignDesc(s.config.UI.DialogTitleAlign))
	items[6].Desc = s.dropdownDesc(titleAlignDesc(s.config.UI.SubmenuTitleAlign))
	items[7].Desc = s.dropdownDesc(titleAlignDesc(s.config.UI.LogTitleAlign))
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

func (s *DisplayOptionsScreen) MenuName() string {
	return "appearance"
}
