package console

// Raw ANSI Color Codes
const (
	// Reset
	CodeReset = "\033[0m"

	// Modifiers
	CodeBold      = "\033[1m"
	CodeDim       = "\033[2m"
	CodeUnderline = "\033[4m"
	CodeBlink     = "\033[5m"
	CodeReverse   = "\033[7m"

	// Foreground
	CodeBlack   = "\033[30m"
	CodeRed     = "\033[31m"
	CodeGreen   = "\033[32m"
	CodeYellow  = "\033[33m"
	CodeBlue    = "\033[34m"
	CodeMagenta = "\033[35m"
	CodeCyan    = "\033[36m"
	CodeWhite   = "\033[37m"

	// Background
	CodeBlackBg   = "\033[40m"
	CodeRedBg     = "\033[41m"
	CodeGreenBg   = "\033[42m"
	CodeYellowBg  = "\033[43m"
	CodeBlueBg    = "\033[44m"
	CodeMagentaBg = "\033[45m"
	CodeCyanBg    = "\033[46m"
	CodeWhiteBg   = "\033[47m"
)

// AppColors defines the struct for program-wide colors/styles
type AppColors struct {
	// Base Codes
	Reset     string
	Bold      string
	Dim       string
	Underline string
	Blink     string
	Reverse   string

	// Base Colors (Foreground)
	Black   string
	Red     string
	Green   string
	Yellow  string
	Blue    string
	Magenta string
	Cyan    string
	White   string

	// Base Colors (Background)
	BlackBg   string
	RedBg     string
	GreenBg   string
	YellowBg  string
	BlueBg    string
	MagentaBg string
	CyanBg    string
	WhiteBg   string

	// Semantic Colors
	Timestamp              string
	Trace                  string
	Debug                  string
	Info                   string
	Notice                 string
	Warn                   string
	Error                  string
	Fatal                  string
	FatalFooter            string
	TraceHeader            string
	TraceFooter            string
	TraceFrameNumber       string
	TraceFrameLines        string
	TraceSourceFile        string
	TraceLineNumber        string
	TraceFunction          string
	TraceCmd               string
	TraceCmdArgs           string
	UnitTestPass           string
	UnitTestFail           string
	UnitTestFailArrow      string
	App                    string
	ApplicationName        string
	Branch                 string
	FailingCommand         string
	File                   string
	Folder                 string
	Program                string
	RunningCommand         string
	Theme                  string
	Update                 string
	User                   string
	URL                    string
	UserCommand            string
	UserCommandError       string
	UserCommandErrorMarker string
	Var                    string
	Version                string
	Yes                    string
	No                     string

	// Usage Colors
	UsageCommand string
	UsageOption  string
	UsageApp     string
	UsageBranch  string
	UsageFile    string
	UsagePage    string
	UsageTheme   string
	UsageVar     string
}

// Colors is the global instance for application output (stdout)
var Colors AppColors

func init() {
	// IMPORTANT: ALWAYS initialize color definitions, regardless of TTY status
	// The TTY check should only affect whether Parse outputs ANSI codes, not whether colors are defined
	// This is crucial for cross-compilation (e.g., building on Windows for Linux)
	Colors = AppColors{
		// Base Codes (Mapped to cview-like tags for parsing)
		Reset:     "[-]",
		Bold:      "[::b]",
		Dim:       "[::d]",
		Underline: "[::u]",
		Blink:     "[::l]",
		Reverse:   "[::r]",

		// Base Colors (Foreground - standard names)
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
		Fatal:                  "[white:red]", // Red BG, White Text
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

// RegisterBaseTags registers all the semantic shorthands and aliases
// that are used throughout the application.
func RegisterBaseTags() {
	// Bash-style aliases from main.sh
	RegisterColor("_NC_", "[-]")
	RegisterColor("_BD_", "[::b]")
	RegisterColor("_UL_", "[::u]")
	RegisterColor("_DM_", "[::d]")
	RegisterColor("_BL_", "[::l]")

	// Existing shorthands
	RegisterColor("_ul_", "[::u]")
	RegisterColor("_blink_", "[::l]")

	// These tags are automatically registered as [_fieldname_] by the parser's buildColorMap,
	// but we double-register them here as aliases to ensure they are available in the aliasMap
	// and to maintain explicit mapping for all semantic tags.
	// IMPORTANT: Use lowercase to match BuildColorMap's strings.ToLower(field.Name) conversion!

	RegisterColor("_applicationname_", Colors.ApplicationName)
	RegisterColor("_version_", Colors.Version)
	RegisterColor("_branch_", Colors.Branch)
	RegisterColor("_usercommand_", Colors.UserCommand)
	RegisterColor("_usercommanderror_", Colors.UserCommandError)
	RegisterColor("_usercommanderrormarker_", Colors.UserCommandErrorMarker)
	RegisterColor("_yes_", Colors.Yes)
	RegisterColor("_no_", Colors.No)

	// Usage Colors
	RegisterColor("_usagecommand_", Colors.UsageCommand)
	RegisterColor("_usageoption_", Colors.UsageOption)
	RegisterColor("_usageapp_", Colors.UsageApp)
	RegisterColor("_usagebranch_", Colors.UsageBranch)
	RegisterColor("_usagefile_", Colors.UsageFile)
	RegisterColor("_usagepage_", Colors.UsagePage)
	RegisterColor("_usagetheme_", Colors.UsageTheme)
	RegisterColor("_usagevar_", Colors.UsageVar)

	// Log Level Tags (Shorthands for logger consistency)
	RegisterColor("_timestamp_", Colors.Timestamp)
	RegisterColor("_notice_", Colors.Notice)
	RegisterColor("_warn_", Colors.Warn)
	RegisterColor("_error_", Colors.Error)
	RegisterColor("_fatal_", Colors.Fatal)
	RegisterColor("_debug_", Colors.Debug)
	RegisterColor("_info_", Colors.Info)
	RegisterColor("_trace_", Colors.Trace)
	RegisterColor("_url_", Colors.URL)

	// Missing Semantic Tags from main.sh
	RegisterColor("_app_", Colors.App)
	RegisterColor("_failingcommand_", Colors.FailingCommand)
	RegisterColor("_file_", Colors.File)
	RegisterColor("_folder_", Colors.Folder)
	RegisterColor("_program_", Colors.Program)
	RegisterColor("_runningcommand_", Colors.RunningCommand)
	RegisterColor("_theme_", Colors.Theme)
	RegisterColor("_update_", Colors.Update)
	RegisterColor("_user_", Colors.User)
	RegisterColor("_var_", Colors.Var)

	// Legacy Foreground Colors (F array in main.sh)
	RegisterColor("_B_", Colors.Blue)
	RegisterColor("_C_", Colors.Cyan)
	RegisterColor("_G_", Colors.Green)
	RegisterColor("_K_", Colors.Black)
	RegisterColor("_M_", Colors.Magenta)
	RegisterColor("_R_", Colors.Red)
	RegisterColor("_W_", Colors.White)
	RegisterColor("_Y_", Colors.Yellow)

	// Explicit F Array Aliases
	RegisterColor("_F_B_", Colors.Blue)
	RegisterColor("_F_C_", Colors.Cyan)
	RegisterColor("_F_G_", Colors.Green)
	RegisterColor("_F_K_", Colors.Black)
	RegisterColor("_F_M_", Colors.Magenta)
	RegisterColor("_F_R_", Colors.Red)
	RegisterColor("_F_W_", Colors.White)
	RegisterColor("_F_Y_", Colors.Yellow)

	// Legacy Background Colors (B array in main.sh)
	RegisterColor("_B_B_", Colors.BlueBg)
	RegisterColor("_B_C_", Colors.CyanBg)
	RegisterColor("_B_G_", Colors.GreenBg)
	RegisterColor("_B_K_", Colors.BlackBg)
	RegisterColor("_B_M_", Colors.MagentaBg)
	RegisterColor("_B_R_", Colors.RedBg)
	RegisterColor("_B_W_", Colors.WhiteBg)
	RegisterColor("_B_Y_", Colors.YellowBg)
}
