package tui

import "charm.land/lipgloss/v2"

// nextButtonFocus moves focus right through buttons, clamping at the last button (no wrap).
func (m *MenuModel) nextButtonFocus() FocusItem {
	if m.focusedItem == FocusList {
		m.focusedBtnIndex = 0
		return FocusBtn
	}
	// FocusBtn / FocusSelectBtn: advance index, clamp at end
	if m.focusedBtnIndex < len(m.buttons)-1 {
		m.focusedBtnIndex++
	}
	return FocusBtn
}

// prevButtonFocus moves focus left through buttons, clamping at Select (no wrap).
func (m *MenuModel) prevButtonFocus() FocusItem {
	if m.focusedItem == FocusBtn && m.focusedBtnIndex > 0 {
		m.focusedBtnIndex--
		return FocusBtn
	}
	m.focusedBtnIndex = 0
	return FocusSelectBtn
}

// getButtonSpecs returns the current button configuration based on state
func (m *MenuModel) getButtonSpecs() []ButtonSpec {
	if !m.showButtons {
		return nil
	}
	var specs []ButtonSpec
	for i, btn := range m.buttons {
		active := m.focusedItem == FocusBtn && m.focusedBtnIndex == i
		specs = append(specs, ButtonSpec{
			Text:         btn.Label,
			Active:       active,
			Locked:       btn.Locked,
			Spinning:     m.processingBtnID == btn.ZoneID,
			SpinnerFrame: m.spinnerFrame,
			ZoneID:       btn.ZoneID,
			Help:         btn.Help,
		})
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
