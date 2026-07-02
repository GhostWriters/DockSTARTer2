package tui

import (
	"strings"
	"time"

	semstyle "github.com/GhostWriters/semstyle/lg"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"DockSTARTer2/internal/displayengine"
)

// choiceDialogModel is a multi-button choice dialog.
// onResult receives the chosen button index (0-based), or -1 if cancelled via Esc.
type choiceDialogModel struct {
	displayengine.BaseDialogModel
	title    string
	question string
	choices  []string
	focused  int
	onResult func(int) tea.Msg
	buttons  *displayengine.ButtonRow
}

func newChoiceDialog(title, question string, choices []string) *choiceDialogModel {
	defs := make([]displayengine.ButtonDef, len(choices))
	for i, c := range choices {
		defs[i] = displayengine.ButtonDef{Label: c, ZoneID: c}
	}
	m := &choiceDialogModel{
		BaseDialogModel: displayengine.BaseDialogModel{ID: "choice_dialog", Focused: true},
		title:           title,
		question:        question,
		choices:         choices,
		focused:         0,
		onResult:        func(i int) tea.Msg { return displayengine.CloseDialogMsg{Result: i} },
		buttons:         displayengine.NewButtonRow(defs),
	}
	return m
}

func (m *choiceDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if tickCmd, ok := m.buttons.Update(msg); ok {
		return m, tickCmd
	}

	submit := func(idx int) tea.Cmd {
		return func() tea.Msg { return m.onResult(idx) }
	}
	submitWithSpinner := func(idx int) tea.Cmd {
		if idx >= 0 && idx < len(m.choices) {
			return m.buttons.SetProcessing(m.choices[idx], submit(idx))
		}
		return submit(idx)
	}

	if m.HandleWidgetClearPress(msg) {
		return m, nil
	}

	n := len(m.choices)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.CalculateLayout()
		return m, nil

	case tea.KeyPressMsg:
		if handled, cmd := m.HandleTitleBarKey(msg, nil); handled {
			m.focused = len(m.choices) - 1
			return m, tea.Batch(cmd, submitWithSpinner(m.focused))
		}
		switch {
		case key.Matches(msg, displayengine.Keys.Esc):
			return m, submitWithSpinner(len(m.choices) - 1)
		case key.Matches(msg, displayengine.Keys.Enter):
			return m, submitWithSpinner(m.focused)
		case key.Matches(msg, displayengine.Keys.Left), key.Matches(msg, displayengine.Keys.Up):
			if m.focused > 0 {
				m.focused--
			}
		case key.Matches(msg, displayengine.Keys.Right), key.Matches(msg, displayengine.Keys.Down):
			if m.focused < n-1 {
				m.focused++
			}
		case key.Matches(msg, displayengine.Keys.Tab):
			m.focused = (m.focused + 1) % n
		case key.Matches(msg, displayengine.Keys.ShiftTab):
			m.focused = (m.focused + n - 1) % n
		default:
			// First letter of each choice as hotkey
			k := strings.ToLower(msg.String())
			for i, choice := range m.choices {
				if len(choice) > 0 && strings.ToLower(string(choice[0])) == k {
					return m, submitWithSpinner(i)
				}
			}
		}

	case displayengine.LayerHitMsg:
		if handled, cmd := m.HandleTitleBarHit(msg, nil); handled {
			m.focused = len(m.choices) - 1
			return m, tea.Batch(cmd, submitWithSpinner(m.focused))
		}
		if msg.Button == tea.MouseLeft {
			for i, choice := range m.choices {
				if displayengine.ButtonIDMatches(msg.ID, choice) {
					return m, submitWithSpinner(i)
				}
			}
		}
	}

	if _, ok := msg.(displayengine.ToggleFocusedMsg); ok {
		return m, submitWithSpinner(m.focused)
	}

	if wheelMsg, ok := msg.(tea.MouseWheelMsg); ok {
		if wheelMsg.Button == tea.MouseWheelUp && m.focused > 0 {
			m.focused--
		} else if wheelMsg.Button == tea.MouseWheelDown && m.focused < n-1 {
			m.focused++
		}
		return m, nil
	}

	return m, nil
}

func (m *choiceDialogModel) contentWidth() int {
	maxAllowed := m.Layout.Width - 2
	w := displayengine.MaxLineWidth(m.question) + displayengine.DialogBodyPadH

	// Minimum to fit all buttons with spacing
	minBtn := 4
	for _, c := range m.choices {
		minBtn += lipgloss.Width(c) + 4
	}
	if minBtn > w {
		w = minBtn
	}
	if tw := lipgloss.Width(displayengine.GetPlainText(m.title)) + 6; tw > w {
		w = tw
	}
	ctx := displayengine.GetActiveContext()
	w = displayengine.MinWidthForWidgets(w, displayengine.GetPlainText(m.title), ctx.DialogTitleAlign, displayengine.BuildInactiveTitleWidgets(ctx))
	if w > maxAllowed {
		w = maxAllowed
	}
	return w
}

func (m *choiceDialogModel) ViewString() string {
	if m.Width == 0 {
		return ""
	}

	ctx := displayengine.GetActiveContext()
	ctx.LargeTitleBars = m.Layout.LargeTitleBar
	borderBG := ctx.Dialog.GetBackground()
	contentWidth := m.contentWidth()

	questionStyle := ctx.Dialog.Padding(1, 2).Width(contentWidth)
	questionText := strings.TrimRight(questionStyle.Render(semstyle.Sprintf("%s", m.question)), "\n")

	specs := make([]displayengine.ButtonSpec, len(m.choices))
	for i, c := range m.choices {
		specs[i] = displayengine.ButtonSpec{Text: c, Active: i == m.focused || m.buttons.IsProcessingID(c), ZoneID: c}
	}
	specs = m.buttons.ApplySpinner(specs)
	buttonRow := strings.TrimRight(displayengine.RenderCenteredButtonsCtx(contentWidth, ctx, specs...), "\n")

	spacer := lipgloss.NewStyle().Width(contentWidth).Background(borderBG).Render("")
	fullContent := lipgloss.JoinVertical(lipgloss.Left, questionText, spacer, buttonRow)

	return displayengine.RenderDialogWithTypeAndWidgets(m.title, fullContent, m.Focused || m.TitleBarFocused(), 0, displayengine.DialogTypeConfirm, m.State())
}

func (m *choiceDialogModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

func (m *choiceDialogModel) Layers() []*lipgloss.Layer { return m.BaseDialogModel.Layers(m.ViewString) }

func (m *choiceDialogModel) GetHitRegions(offsetX, offsetY int) []displayengine.HitRegion {
	ctx := displayengine.GetActiveContext()
	contentWidth := m.contentWidth()

	questionStyle := ctx.Dialog.Padding(1, 2).Width(contentWidth)
	questionHeight := lipgloss.Height(questionStyle.Render(semstyle.Sprintf("%s", m.question)))
	buttonY := 1 + questionHeight + 1
	if m.Layout.LargeTitleBar {
		buttonY += displayengine.LargeTitleBarOverhead
	}

	btnSpecs := make([]displayengine.ButtonSpec, len(m.choices))
	for i, c := range m.choices {
		btnSpecs[i] = displayengine.ButtonSpec{Text: c, ZoneID: c, Help: "Select this option."}
	}

	var regions []displayengine.HitRegion
	regions = append(regions, displayengine.GetButtonHitRegions(
		displayengine.HelpContext{ScreenName: m.title, PageTitle: "Question", PageText: m.question},
		m.ID, offsetX+1, offsetY+buttonY, contentWidth, displayengine.ZDialog+20,
		btnSpecs...,
	)...)

	// Dialog background
	regions = append(regions, displayengine.HitRegion{
		ID:     m.ID,
		X:      offsetX,
		Y:      offsetY,
		Width:  contentWidth + 2,
		Height: buttonY + 2, // buttonRow (1) + border (1 more)
		ZOrder: displayengine.ZDialog,
		Label:  "Choice",
		Help: &displayengine.HelpContext{
			ScreenName: m.title,
			PageTitle:  "Question",
			PageText:   m.question,
		},
	})

	regions = append(regions, m.TitleBarHitRegions(offsetX, offsetY, contentWidth, displayengine.ZDialog)...)
	return regions
}

func (m *choiceDialogModel) AdvanceSpinners(now time.Time) bool {
	return m.buttons.AdvanceSpinner(now)
}
