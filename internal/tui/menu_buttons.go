package tui

import "charm.land/lipgloss/v2"

// nextButtonFocus moves focus right through buttons, clamping at the last button (no wrap).
func (m *MenuModel) nextButtonFocus() FocusItem {
	switch m.focusedItem {
	case FocusList:
		return FocusSelectBtn
	case FocusSelectBtn:
		if m.backAction != nil {
			return FocusBackBtn
		}
		if m.showExit {
			return FocusExitBtn
		}
		return FocusSelectBtn // only one button, stay
	case FocusBackBtn:
		if m.showExit {
			return FocusExitBtn
		}
		return FocusBackBtn // rightmost, clamp
	case FocusExitBtn:
		return FocusExitBtn // already rightmost, stay
	}
	return FocusSelectBtn
}

// prevButtonFocus moves focus left through buttons, clamping at Select (no wrap).
func (m *MenuModel) prevButtonFocus() FocusItem {
	switch m.focusedItem {
	case FocusList, FocusSelectBtn:
		return FocusSelectBtn // already leftmost, stay
	case FocusExitBtn:
		if m.backAction != nil {
			return FocusBackBtn
		}
		return FocusSelectBtn
	case FocusBackBtn:
		return FocusSelectBtn
	}
	return FocusSelectBtn
}

// getButtonSpecs returns the current button configuration based on state
func (m *MenuModel) getButtonSpecs() []ButtonSpec {
	if !m.showButtons {
		return nil
	}
	var specs []ButtonSpec

	// Select Button
	label := m.selectLabel
	if label == "" {
		label = "Select"
	}
	specs = append(specs, ButtonSpec{Text: label, Active: m.focusedItem == FocusSelectBtn, ZoneID: "btn-select", Help: "Confirm and execute the selected action."})

	// Back Button
	if m.backAction != nil {
		label := m.backLabel
		if label == "" {
			label = "Back"
		}
		specs = append(specs, ButtonSpec{Text: label, Active: m.focusedItem == FocusBackBtn, ZoneID: "btn-back", Help: "Return to the previous screen."})
	}

	// Exit Button
	if m.showExit {
		label := m.exitLabel
		if label == "" {
			label = "Exit"
		}
		specs = append(specs, ButtonSpec{Text: label, Active: m.focusedItem == FocusExitBtn, ZoneID: "btn-exit", Help: "Exit the application immediately."})
	}

	return specs
}

// renderSimpleButtons creates a button row with evenly spaced sections.
// Uses the border decision already stored in m.layout.ButtonHeight to avoid
// re-evaluating width (which would ignore height constraints).
func (m *MenuModel) renderSimpleButtons(contentWidth int) string {
	specs := m.getButtonSpecs()
	useBorders := m.layout.ButtonHeight == DialogButtonHeight
	return renderCenteredButtonsImpl(contentWidth, useBorders, GetActiveContext(), specs...)
}


func (m *MenuModel) renderButtonBox(buttons string, contentWidth int) string {
	styles := GetStyles()

	// Center buttons in content width
	centeredButtons := lipgloss.NewStyle().
		Width(contentWidth).
		Align(lipgloss.Center).
		Background(styles.Dialog.GetBackground()).
		Render(buttons)

	// Add padding for spacing (no border since buttons have their own)
	boxStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, 1)

	return boxStyle.Render(centeredButtons)
}
