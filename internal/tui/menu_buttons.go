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
	specs = append(specs, ButtonSpec{Text: label, Active: m.focusedItem == FocusSelectBtn, ZoneID: "btn-select"})

	// Back Button
	if m.backAction != nil {
		label := m.backLabel
		if label == "" {
			label = "Back"
		}
		specs = append(specs, ButtonSpec{Text: label, Active: m.focusedItem == FocusBackBtn, ZoneID: "btn-back"})
	}

	// Exit Button
	if m.showExit {
		label := m.exitLabel
		if label == "" {
			label = "Exit"
		}
		specs = append(specs, ButtonSpec{Text: label, Active: m.focusedItem == FocusExitBtn, ZoneID: "btn-exit"})
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

func (m *MenuModel) renderButtons(contentWidth int) string {
	styles := GetStyles()

	// Select button
	selectStyle := styles.ButtonInactive
	if m.focusedItem == FocusSelectBtn {
		selectStyle = styles.ButtonActive
	}
	selectBtn := selectStyle.Render("<Select>")

	// Back button (optional)
	var backBtn string
	if m.backAction != nil {
		backStyle := styles.ButtonInactive
		if m.focusedItem == FocusBackBtn {
			backStyle = styles.ButtonActive
		}
		backBtn = backStyle.Render("<Back>")
	}

	// Exit button
	exitStyle := styles.ButtonInactive
	if m.focusedItem == FocusExitBtn {
		exitStyle = styles.ButtonActive
	}
	exitBtn := exitStyle.Render("<Exit>")

	// Collect all buttons
	var buttonStrs []string
	buttonStrs = append(buttonStrs, selectBtn)
	if m.backAction != nil {
		buttonStrs = append(buttonStrs, backBtn)
	}
	buttonStrs = append(buttonStrs, exitBtn)

	// Divide available width into equal sections (one per button)
	numButtons := len(buttonStrs)
	sectionWidth := contentWidth / numButtons

	// Center each button in its section
	var sections []string
	for _, btn := range buttonStrs {
		centeredBtn := lipgloss.NewStyle().
			Width(sectionWidth).
			Align(lipgloss.Center).
			Background(styles.Dialog.GetBackground()).
			Render(btn)
		sections = append(sections, centeredBtn)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, sections...)
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
