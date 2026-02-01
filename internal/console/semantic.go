package console

func init() {
	// Initialize color definitions in tview tag format
	// These are the OUTPUT format values that ToTview() will produce
	Colors = AppColors{
		// Base Codes (tview format)
		Reset:     "[-]",
		Bold:      "[::b]",
		Dim:       "[::d]",
		Underline: "[::u]",
		Blink:     "[::l]",
		Reverse:   "[::r]",

		// Base Colors (Foreground)
		Black:   "[black]",
		Red:     "[red]",
		Green:   "[green]",
		Yellow:  "[yellow]",
		Blue:    "[blue]",
		Magenta: "[magenta]",
		Cyan:    "[cyan]",
		White:   "[white]",

		// Base Colors (Background)
		BlackBg:   "[:black]",
		RedBg:     "[:red]",
		GreenBg:   "[:green]",
		YellowBg:  "[:yellow]",
		BlueBg:    "[:blue]",
		MagentaBg: "[:magenta]",
		CyanBg:    "[:cyan]",
		WhiteBg:   "[:white]",

		// Semantic Colors (Standard DockSTARTer mappings)
		Timestamp:              "[-]",
		Trace:                  "[blue]",
		Debug:                  "[blue]",
		Info:                   "[blue]",
		Notice:                 "[green]",
		Warn:                   "[yellow]",
		Error:                  "[red]",
		Fatal:                  "[white:red]",
		FatalFooter:            "[-]",
		TraceHeader:            "[red]",
		TraceFooter:            "[red]",
		TraceFrameNumber:       "[red]",
		TraceFrameLines:        "[red]",
		TraceSourceFile:        "[cyan::b]",
		TraceLineNumber:        "[yellow::b]",
		TraceFunction:          "[green::b]",
		TraceCmd:               "[green::b]",
		TraceCmdArgs:           "[green]",
		UnitTestPass:           "[green]",
		UnitTestFail:           "[red]",
		UnitTestFailArrow:      "[red]",
		App:                    "[cyan]",
		ApplicationName:        "[cyan::b]",
		Branch:                 "[cyan]",
		FailingCommand:         "[red]",
		File:                   "[cyan::b]",
		Folder:                 "[cyan::b]",
		Program:                "[cyan]",
		RunningCommand:         "[green::b]",
		Theme:                  "[cyan]",
		Update:                 "[green]",
		User:                   "[cyan]",
		URL:                    "[cyan::u]",
		UserCommand:            "[yellow::b]",
		UserCommandError:       "[red::u]",
		UserCommandErrorMarker: "[red]",
		Var:                    "[magenta]",
		Version:                "[cyan]",
		Yes:                    "[green]",
		No:                     "[red]",

		// Usage Colors
		UsageCommand: "[yellow::b]",
		UsageOption:  "[yellow]",
		UsageApp:     "[cyan]",
		UsageBranch:  "[cyan]",
		UsageFile:    "[cyan::b]",
		UsagePage:    "[cyan::b]",
		UsageTheme:   "[cyan]",
		UsageVar:     "[magenta]",
	}
	RegisterBaseTags()
}
