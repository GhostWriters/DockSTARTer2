package tui

import tea "charm.land/bubbletea/v2"

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

// ConfirmExitAction returns a tea.Cmd that prompts the user to confirm exiting.
// If confirmed, it returns tea.QuitMsg to gracefully terminate the application.
func ConfirmExitAction() tea.Cmd {
	return func() tea.Msg {
		if Confirm("Exit DockSTARTer", "Do you want to exit DockSTARTer?", true) {
			return tea.Quit() // Returns tea.QuitMsg{}
		}
		return nil
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
