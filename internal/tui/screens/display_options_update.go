package screens

import (
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (s *DisplayOptionsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

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

		// 2. Component routing (menus)
		if strings.HasPrefix(msg.ID, "item-theme_list-") {
			// Theme selection logic
			s.focusedPanel = FocusThemes
			s.updateFocusStates()

			// Extract index
			var idx int
			fmt.Sscanf(msg.ID, "item-theme_list-%d", &idx)
			items := s.themeMenu.GetItems()
			if idx >= 0 && idx < len(items) {
				for j := range items {
					items[j].Checked = (idx == j)
				}
				s.themeMenu.SetItems(items)
				s.themeMenu.Select(idx)
				s.applyPreview(items[idx].Tag)
			}
			return s, nil
		} else if strings.HasPrefix(msg.ID, "item-options_list-") {
			s.focusedPanel = FocusOptions
			s.updateFocusStates()

			// Forward to options menu
			updated, uCmd := s.optionsMenu.Update(msg)
			if m, ok := updated.(*tui.MenuModel); ok {
				s.optionsMenu = m
			}
			return s, uCmd
		}

		// 3. Button actions
		switch msg.ID {
		case tui.IDApplyButton:
			s.focusedButton = 0
			return s, s.handleApply()
		case tui.IDBackButton:
			if s.isRoot {
				return s, nil // Back is not shown when root; ignore stale hits
			}
			s.focusedButton = 1
			theme.Unload("Preview")
			return s, navigateBack()
		case tui.IDExitButton:
			s.focusedButton = s.maxFocusedButton()
			theme.Unload("Preview")
			return s, tui.ConfirmExitAction()
		}

	case tui.ToggleFocusedMsg:
		// Middle click: activate the currently focused item in the hovered panel
		if s.focusedPanel == FocusThemes {
			items := s.themeMenu.GetItems()
			cursor := s.themeMenu.Index()
			if cursor >= 0 && cursor < len(items) {
				for i := range items {
					items[i].Checked = (i == cursor)
				}
				s.themeMenu.SetItems(items)
				s.applyPreview(items[cursor].Tag)
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
			s.focusedButton--
			if s.focusedButton < 0 {
				s.focusedButton = s.maxFocusedButton()
			}
			return s, nil
		}
		if key.Matches(msg, tui.Keys.Right) {
			s.focusedButton++
			if s.focusedButton > s.maxFocusedButton() {
				s.focusedButton = 0
			}
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

func (s *DisplayOptionsScreen) SetSize(width, height int) {
	s.width = width
	s.height = height

	if s.optionsMenu == nil || s.themeMenu == nil {
		return
	}

	layout := tui.GetLayout()

	previewMinWidth := 48
	minDialogWidth := 44 + layout.BorderWidth()
	previewFits := width >= minDialogWidth+layout.GutterWidth+previewMinWidth

	var dialogContentWidth int
	if previewFits {
		dialogContentWidth = width - layout.GutterWidth - previewMinWidth
	} else {
		dialogContentWidth = width
	}

	menuWidth := dialogContentWidth - layout.BorderWidth()
	if menuWidth < 40 {
		menuWidth = 40
	}

	hasShadow := tui.IsShadowEnabled()
	optionsContentHeight := layout.DialogContentHeight(height, 0, true, hasShadow)
	overhead := height - optionsContentHeight

	optionsFlowLines := s.optionsMenu.GetFlowHeight(menuWidth)
	optionsHeight := optionsFlowLines + layout.BorderHeight()

	themeHeight := s.height - optionsHeight - overhead
	if themeHeight < 4 {
		themeHeight = 4
	}

	s.themeMenu.SetSize(menuWidth, themeHeight)
	s.optionsMenu.SetSize(menuWidth, optionsHeight)
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
