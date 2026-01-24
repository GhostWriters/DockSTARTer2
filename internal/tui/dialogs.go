package tui

import (
	"DockSTARTer2/internal/theme"

	"codeberg.org/tslocum/cview"
)

// Message displays a simple info dialog with an OK button.
func Message(title, message string) {
	resultChan := make(chan struct{})

	app.QueueUpdateDraw(func() {
		btnOK := cview.NewButton("<OK>")
		btnOK.SetBackgroundColor(theme.Current.ButtonInactiveBG)
		btnOK.SetLabelColor(theme.Current.ButtonInactiveFG)
		btnOK.SetBackgroundColorFocused(theme.Current.ButtonActiveBG)
		btnOK.SetLabelColorFocused(theme.Current.ButtonActiveFG)

		btnOK.SetSelectedFunc(func() {
			panels.RemovePanel("message")
			close(resultChan)
		})

		buttonsFlex := cview.NewFlex()
		buttonsFlex.SetBackgroundColor(theme.Current.DialogBG)
		buttonsFlex.AddItem(cview.NewBox(), 0, 1, false)
		buttonsFlex.AddItem(btnOK, 10, 0, true)
		buttonsFlex.AddItem(cview.NewBox(), 0, 1, false)

		// Create a dummy content box for spacing if needed, but here msg is in WrapInDialogFrame
		content := cview.NewBox()
		content.SetBackgroundColor(theme.Current.DialogBG)

		dialog := WrapInDialogFrame(title, message, content, 40, 0, nil, buttonsFlex)
		panels.AddPanel("message", dialog, true, true)
		app.SetFocus(btnOK)
	})

	<-resultChan
}

// Success displays a success message with the appropriate title color.
func Success(title, message string) {
	Message("[_Notice_]"+title, message)
}

// Warning displays a warning message with the appropriate title color.
func Warning(title, message string) {
	Message("[_Warn_]"+title, message)
}

// Error displays an error message with the appropriate title color.
func Error(title, message string) {
	Message("[_Error_]"+title, message)
}

// Confirm displays a Yes/No dialog and returns the user's choice.
func Confirm(title, question string, defaultYes bool) bool {
	resultChan := make(chan bool)

	app.QueueUpdateDraw(func() {
		btnYes := cview.NewButton("<Yes>")
		btnYes.SetBackgroundColor(theme.Current.ButtonInactiveBG)
		btnYes.SetLabelColor(theme.Current.ButtonInactiveFG)
		btnYes.SetBackgroundColorFocused(theme.Current.ButtonActiveBG)
		btnYes.SetLabelColorFocused(theme.Current.ButtonActiveFG)

		btnNo := cview.NewButton("<No>")
		btnNo.SetBackgroundColor(theme.Current.ButtonInactiveBG)
		btnNo.SetLabelColor(theme.Current.ButtonInactiveFG)
		btnNo.SetBackgroundColorFocused(theme.Current.ButtonActiveBG)
		btnNo.SetLabelColorFocused(theme.Current.ButtonActiveFG)

		btnYes.SetSelectedFunc(func() {
			panels.RemovePanel("confirm")
			resultChan <- true
		})

		btnNo.SetSelectedFunc(func() {
			panels.RemovePanel("confirm")
			resultChan <- false
		})

		buttonsFlex := cview.NewFlex()
		buttonsFlex.SetBackgroundColor(theme.Current.DialogBG)
		buttonsFlex.AddItem(cview.NewBox(), 0, 1, false)
		buttonsFlex.AddItem(btnYes, 10, 0, true)
		buttonsFlex.AddItem(cview.NewBox(), 2, 0, false)
		buttonsFlex.AddItem(btnNo, 10, 0, true)
		buttonsFlex.AddItem(cview.NewBox(), 0, 1, false)

		content := cview.NewBox()
		content.SetBackgroundColor(theme.Current.DialogBG)

		dialog := WrapInDialogFrame("[_Notice_]"+title, question, content, 40, 0, nil, buttonsFlex)
		panels.AddPanel("confirm", dialog, true, true)

		if defaultYes {
			app.SetFocus(btnYes)
		} else {
			app.SetFocus(btnNo)
		}
	})

	return <-resultChan
}

// ProgramBox creates a view for displaying streaming output.
func NewProgramBox(title string, text string) (*cview.TextView, *cview.Button, cview.Primitive) {
	textView := cview.NewTextView()
	textView.SetDynamicColors(true)
	textView.SetBackgroundColor(theme.Current.DialogBG)
	textView.SetTextColor(theme.Current.DialogFG)
	textView.SetScrollable(true)
	textView.SetBorder(false) // Border is provided by WrapInDialogFrame's ThreeDBox

	btnOK := cview.NewButton("<OK>")
	btnOK.SetBackgroundColor(theme.Current.ButtonInactiveBG)
	btnOK.SetLabelColor(theme.Current.ButtonInactiveFG)
	btnOK.SetBackgroundColorFocused(theme.Current.ButtonActiveBG)
	btnOK.SetLabelColorFocused(theme.Current.ButtonActiveFG)

	buttonsFlex := cview.NewFlex()
	buttonsFlex.SetBackgroundColor(theme.Current.DialogBG)
	buttonsFlex.AddItem(cview.NewBox(), 0, 1, false)
	buttonsFlex.AddItem(btnOK, 10, 0, true)
	buttonsFlex.AddItem(cview.NewBox(), 0, 1, false)

	// Wrap in frame
	// Note: text here is the subtitle/body text above the programbox
	// We use contentWidth=100 for a wider log view
	frame := WrapInDialogFrame(title, text, textView, 100, 20, nil, buttonsFlex)

	return textView, btnOK, frame
}
