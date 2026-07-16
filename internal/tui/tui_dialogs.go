package tui

import tea "charm.land/bubbletea/v2"

// sessionConfirmFunc returns a confirm callback bound to p specifically,
// instead of the global program var -- used to give each session's
// PanelModel its own callback (see PanelModel.SetConfirmFunc) so a console
// command's confirm prompt reaches the session that issued it even if
// another session has since become the current global program.
func sessionConfirmFunc(p *tea.Program) func(title, question string, defaultYes bool) bool {
	return func(title, question string, defaultYes bool) bool {
		if p == nil {
			return defaultYes
		}
		resultChan := make(chan bool)
		p.Send(UniversalPromptMsg{
			Title:      title,
			Question:   question,
			DefaultYes: defaultYes,
			ResultChan: resultChan,
			Type:       PromptTypeConfirm,
		})
		return <-resultChan
	}
}

// Confirm shows a confirmation dialog and returns the user's choice.
// If a program is already running, it sends a message to the active program.
func Confirm(title, question string, defaultYes bool) bool {
	if program != nil {
		resultChan := make(chan bool)
		program.Send(UniversalPromptMsg{
			Title:      title,
			Question:   question,
			DefaultYes: defaultYes,
			ResultChan: resultChan,
			Type:       PromptTypeConfirm,
		})
		return <-resultChan
	}
	return ShowConfirmDialog(title, question, defaultYes)
}

// ConfirmExitAction returns a tea.Cmd that shows an exit confirmation dialog.
// Returns ConfirmQuitMsg which AppModel handles without blocking the event loop.
func ConfirmExitAction() tea.Cmd {
	return func() tea.Msg {
		return ConfirmQuitMsg{}
	}
}

// Message shows an info message dialog
func Message(title, message string) {
	ShowInfoDialog(title, message)
}

// Success shows a success message dialog
func Success(title, message string) {
	ShowSuccessDialog(title, message)
}

// Warning shows a warning message dialog
func Warning(title, message string) {
	ShowWarningDialog(title, message)
}

// Error shows an error message dialog
func Error(title, message string) {
	ShowErrorDialog(title, message)
}

// PromptConfirm displays a blocking confirmation dialog.
// It is used by the console package via callback.
func PromptConfirm(title, question string, defaultYes bool) bool {
	return Confirm(title, question, defaultYes)
}

// PromptChoice displays a blocking multi-choice sub-dialog over the active ProgramBox.
// choices are the button labels. Returns the chosen index (0-based), or -1 on cancel/Esc.
func PromptChoice(title, question string, choices ...string) int {
	if program == nil {
		return -1
	}
	ch := make(chan int)
	dialog := newChoiceDialog(title, question, choices)
	dialog.onResult = func(i int) tea.Msg {
		return SubDialogResultMsg{Result: i}
	}
	program.Send(SubDialogMsg{
		Model: dialog,
		Chan:  ch,
	})
	return <-ch
}
