package console

func init() {
	// Initialize color definitions in tview tag format
	// These are the OUTPUT format values that ToTview() will produce
	Colors = AppColors{
		// Base Codes (Standardized format)
		Reset:     "{{|-|}}",
		Bold:      "{{|::B|}}",
		Dim:       "{{|::D|}}",
		Underline: "{{|::U|}}",
		Blink:     "{{|::L|}}",
		Reverse:   "{{|::R|}}",

		// Base Colors (Foreground)
		Black:   "{{|black|}}",
		Red:     "{{|red|}}",
		Green:   "{{|green|}}",
		Yellow:  "{{|yellow|}}",
		Blue:    "{{|blue|}}",
		Magenta: "{{|magenta|}}",
		Cyan:    "{{|cyan|}}",
		White:   "{{|white|}}",

		// Base Colors (Background)
		BlackBg:   "{{|:black|}}",
		RedBg:     "{{|:red|}}",
		GreenBg:   "{{|:green|}}",
		YellowBg:  "{{|:yellow|}}",
		BlueBg:    "{{|:blue|}}",
		MagentaBg: "{{|:magenta|}}",
		CyanBg:    "{{|:cyan|}}",
		WhiteBg:   "{{|:white|}}",

		// Semantic Colors (Standard DockSTARTer mappings)
		Timestamp:              "{{|-|}}",
		Trace:                  "{{|-|}}{{|blue|}}",
		Debug:                  "{{|-|}}{{|blue|}}",
		Info:                   "{{|-|}}{{|blue|}}",
		Notice:                 "{{|-|}}{{|green|}}",
		Warn:                   "{{|-|}}{{|yellow|}}",
		Error:                  "{{|-|}}{{|red|}}",
		Fatal:                  "{{|-|}}{{|white:red|}}",
		FatalFooter:            "{{|-|}}",
		TraceHeader:            "{{|-|}}{{|red|}}",
		TraceFooter:            "{{|-|}}{{|red|}}",
		TraceFrameNumber:       "{{|-|}}{{|red|}}",
		TraceFrameLines:        "{{|-|}}{{|red|}}",
		TraceSourceFile:        "{{|-|}}{{|cyan::B|}}",
		TraceLineNumber:        "{{|-|}}{{|yellow::B|}}",
		TraceFunction:          "{{|-|}}{{|green::B|}}",
		TraceCmd:               "{{|-|}}{{|green::B|}}",
		TraceCmdArgs:           "{{|-|}}{{|green|}}",
		UnitTestPass:           "{{|-|}}{{|green|}}",
		UnitTestFail:           "{{|-|}}{{|red|}}",
		UnitTestFailArrow:      "{{|-|}}{{|red|}}",
		App:                    "{{|-|}}{{|cyan|}}",
		ApplicationName:        "{{|-|}}{{|cyan::B|}}",
		Branch:                 "{{|-|}}{{|cyan|}}",
		FailingCommand:         "{{|-|}}{{|red|}}",
		File:                   "{{|-|}}{{|cyan::B|}}",
		Folder:                 "{{|-|}}{{|cyan::B|}}",
		Program:                "{{|-|}}{{|cyan|}}",
		RunningCommand:         "{{|-|}}{{|green::B|}}",
		Theme:                  "{{|-|}}{{|cyan|}}",
		Update:                 "{{|-|}}{{|green|}}",
		User:                   "{{|-|}}{{|cyan|}}",
		URL:                    "{{|-|}}{{|cyan::U|}}",
		UserCommand:            "{{|-|}}{{|yellow::B|}}",
		UserCommandError:       "{{|-|}}{{|red::U|}}",
		UserCommandErrorMarker: "{{|-|}}{{|red|}}",
		Var:                    "{{|-|}}{{|magenta|}}",
		Version:                "{{|-|}}{{|cyan|}}",
		Yes:                    "{{|-|}}{{|green|}}",
		No:                     "{{|-|}}{{|red|}}",

		// Usage Colors
		UsageCommand: "{{|-|}}{{|yellow::B|}}",
		UsageOption:  "{{|-|}}{{|yellow|}}",
		UsageApp:     "{{|-|}}{{|cyan|}}",
		UsageBranch:  "{{|-|}}{{|cyan|}}",
		UsageFile:    "{{|-|}}{{|cyan::B|}}",
		UsagePage:    "{{|-|}}{{|cyan::B|}}",
		UsageTheme:   "{{|-|}}{{|cyan|}}",
		UsageVar:     "{{|-|}}{{|magenta|}}",
	}
	RegisterBaseTags()
}
