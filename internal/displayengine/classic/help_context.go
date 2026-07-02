package classic

// HelpContext defines the two contextual help panels.
type HelpContext struct {
	ScreenName string // e.g., "Main Menu" — used in the title bar: "Help: Main Menu"
	PageTitle  string // title for the page context box (e.g. "Description")
	PageText   string // body text for the page context box
	Legend     string // multi-line legend (newline-separated); rendered centered at the bottom of each page in its own "Legend" box
	ItemTitle  string // e.g., variable name or menu item Tag
	ItemText   string

	DocMarkdown string // Markdown documentation content
	DocAppName  string // Name of the application for the documentation
}

// HelpContextProvider is implemented by models that can provide structured help content.
type HelpContextProvider interface {
	HelpContext(maxWidth int) HelpContext
}

// HelpContextWidth returns the content width the help dialog will use for word-wrapping,
// given the current terminal dimensions. Mirrors the calculation in showHelpCmd.
func HelpContextWidth(termW, termH int) int {
	availW, _ := GetAvailableDialogSize(termW, termH, true)
	w := availW - 8
	if w < 30 {
		w = 30
	}
	if w > 120 {
		w = 120
	}
	return w
}
