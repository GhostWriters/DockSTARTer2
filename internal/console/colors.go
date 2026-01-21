package console

import (
	"os"
)

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
	// Check if stdout is a TTY
	stat, _ := os.Stdout.Stat()
	isTTY := (stat.Mode() & os.ModeCharDevice) != 0

	if isTTY {
		Colors = AppColors{
			// Base Codes
			Reset:     CodeReset,
			Bold:      CodeBold,
			Dim:       CodeDim,
			Underline: CodeUnderline,
			Blink:     CodeBlink,
			Reverse:   CodeReverse,

			// Base Colors (Foreground)
			Black:   CodeBlack,
			Red:     CodeRed,
			Green:   CodeGreen,
			Yellow:  CodeYellow,
			Blue:    CodeBlue,
			Magenta: CodeMagenta,
			Cyan:    CodeCyan,
			White:   CodeWhite,

			// Base Colors (Background)
			BlackBg:   CodeBlackBg,
			RedBg:     CodeRedBg,
			GreenBg:   CodeGreenBg,
			YellowBg:  CodeYellowBg,
			BlueBg:    CodeBlueBg,
			MagentaBg: CodeMagentaBg,
			CyanBg:    CodeCyanBg,
			WhiteBg:   CodeWhiteBg,

			// Semantic Colors
			Timestamp:              "[reset]",
			Trace:                  "[blue]",
			Debug:                  "[blue]",
			Info:                   "[blue]",
			Notice:                 "[green]",
			Warn:                   "[yellow]",
			Error:                  "[red]",
			Fatal:                  "[white:red]", // Red BG, White Text
			FatalFooter:            "[reset]",
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
	// If not TTY, fields remain empty strings (default)
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

	// These tags are automatically registered as [_FieldName_] by the parser's buildColorMap,
	// but we double-register them here as aliases to ensure they are available in the aliasMap
	// and to maintain explicit mapping for all semantic tags.

	RegisterColor("_ApplicationName_", Colors.ApplicationName)
	RegisterColor("_Version_", Colors.Version)
	RegisterColor("_UserCommand_", Colors.UserCommand)
	RegisterColor("_UserCommandError_", Colors.UserCommandError)
	RegisterColor("_UserCommandErrorMarker_", Colors.UserCommandErrorMarker)
	RegisterColor("_Yes_", Colors.Yes)
	RegisterColor("_No_", Colors.No)

	// Usage Colors
	RegisterColor("_UsageCommand_", Colors.UsageCommand)
	RegisterColor("_UsageOption_", Colors.UsageOption)
	RegisterColor("_UsageApp_", Colors.UsageApp)
	RegisterColor("_UsageBranch_", Colors.UsageBranch)
	RegisterColor("_UsageFile_", Colors.UsageFile)
	RegisterColor("_UsagePage_", Colors.UsagePage)
	RegisterColor("_UsageTheme_", Colors.UsageTheme)
	RegisterColor("_UsageVar_", Colors.UsageVar)

	// Log Level Tags (Shorthands for logger consistency)
	RegisterColor("_Timestamp_", Colors.Timestamp)
	RegisterColor("_Notice_", Colors.Notice)
	RegisterColor("_Warn_", Colors.Warn)
	RegisterColor("_Error_", Colors.Error)
	RegisterColor("_Fatal_", Colors.Fatal)
	RegisterColor("_Debug_", Colors.Debug)
	RegisterColor("_Info_", Colors.Info)
	RegisterColor("_Trace_", Colors.Trace)
}
