package console

import (
	"fmt"
	"strings"

	tcellColor "github.com/gdamore/tcell/v3/color"
)

// Raw ANSI Color Codes
const (
	// Reset
	CodeReset   = "\033[0m"
	CodeFGReset = "\033[39m" // Reset foreground to default
	CodeBGReset = "\033[49m" // Reset background to default

	// Hard reset sequences — bypass MaintainBackground interception.
	// MaintainBackground only matches single-parameter SGR sequences (\x1b[0m, \x1b[39m, \x1b[49m).
	// These multi-parameter equivalents have the same terminal effect but are not intercepted.
	CodeHardReset   = "\033[0;39;49m" // Full reset to terminal defaults
	CodeHardFGReset = "\033[39;39m"   // FG reset to terminal default
	CodeHardBGReset = "\033[49;49m"   // BG reset to terminal default

	// Modifiers
	CodeBold          = "\033[1m"
	CodeDim           = "\033[2m"
	CodeUnderline     = "\033[4m"
	CodeBlink         = "\033[5m"
	CodeReverse       = "\033[7m"
	CodeItalic        = "\033[3m"
	CodeStrikethrough = "\033[9m"

	// Modifiers (Off)
	CodeBoldOff          = "\033[22m"
	CodeDimOff           = "\033[22m"
	CodeUnderlineOff     = "\033[24m"
	CodeBlinkOff         = "\033[25m"
	CodeReverseOff       = "\033[27m"
	CodeItalicOff        = "\033[23m"
	CodeStrikethroughOff = "\033[29m"

	// Foreground
	CodeBlack   = "\033[30m"
	CodeRed     = "\033[31m"
	CodeGreen   = "\033[32m"
	CodeYellow  = "\033[33m"
	CodeBlue    = "\033[34m"
	CodeMagenta = "\033[35m"
	CodeCyan    = "\033[36m"
	CodeWhite   = "\033[37m"

	// Foreground (Bright)
	CodeBrightBlack   = "\033[90m"
	CodeBrightRed     = "\033[91m"
	CodeBrightGreen   = "\033[92m"
	CodeBrightYellow  = "\033[93m"
	CodeBrightBlue    = "\033[94m"
	CodeBrightMagenta = "\033[95m"
	CodeBrightCyan    = "\033[96m"
	CodeBrightWhite   = "\033[97m"

	// Background
	CodeBlackBg   = "\033[40m"
	CodeRedBg     = "\033[41m"
	CodeGreenBg   = "\033[42m"
	CodeYellowBg  = "\033[43m"
	CodeBlueBg    = "\033[44m"
	CodeMagentaBg = "\033[45m"
	CodeCyanBg    = "\033[46m"
	CodeWhiteBg   = "\033[47m"

	// Background (Bright)
	CodeBrightBlackBg   = "\033[100m"
	CodeBrightRedBg     = "\033[101m"
	CodeBrightGreenBg   = "\033[102m"
	CodeBrightYellowBg  = "\033[103m"
	CodeBrightBlueBg    = "\033[104m"
	CodeBrightMagentaBg = "\033[105m"
	CodeBrightCyanBg    = "\033[106m"
	CodeBrightWhiteBg   = "\033[107m"
)

var colorAliases map[string]string

func init() {
	colorAliases = make(map[string]string)

	// Register common aliases for tcell compatibility
	colorAliases["cyan"] = "aqua"
	colorAliases["magenta"] = "fuchsia"

	// Register "bright-" variants
	colorAliases["bright-red"] = "red"
	colorAliases["bright-green"] = "lime"
	colorAliases["bright-blue"] = "blue"
	colorAliases["bright-yellow"] = "yellow"
	colorAliases["bright-magenta"] = "fuchsia"
	colorAliases["bright-cyan"] = "aqua"
	colorAliases["bright-white"] = "white"
	colorAliases["bright-black"] = "gray"

	// Initialize color definitions in tview tag format
	// These are the OUTPUT format values that ToANSI() will handle
	Colors = AppColors{
		// Base Codes (Standardized format)
		Reset:     "{{[-]}}",
		Bold:      "{{[::B]}}",
		Dim:       "{{[::D]}}",
		Underline: "{{[::U]}}",
		Blink:     "{{[::L]}}",
		Reverse:   "{{[::R]}}",

		// Base Colors (Foreground)
		Black:   "{{[black]}}",
		Red:     "{{[red]}}",
		Green:   "{{[green]}}",
		Yellow:  "{{[yellow]}}",
		Blue:    "{{[blue]}}",
		Magenta: "{{[magenta]}}",
		Cyan:    "{{[cyan]}}",
		White:   "{{[white]}}",

		// Base Colors (Background)
		BlackBg:   "{{[:black]}}",
		RedBg:     "{{[:red]}}",
		GreenBg:   "{{[:green]}}",
		YellowBg:  "{{[:yellow]}}",
		BlueBg:    "{{[:blue]}}",
		MagentaBg: "{{[:magenta]}}",
		CyanBg:    "{{[:cyan]}}",
		WhiteBg:   "{{[:white]}}",

		// Semantic Colors (Standard DockSTARTer mappings)
		Timestamp:              "{{[-]}}{{[gray::D]}}",
		Trace:                  "{{[-]}}{{[blue]}}",
		Debug:                  "{{[-]}}{{[blue]}}",
		Info:                   "{{[-]}}{{[blue]}}",
		Notice:                 "{{[-]}}{{[green]}}",
		Warn:                   "{{[-]}}{{[yellow]}}",
		Error:                  "{{[-]}}{{[red]}}",
		Fatal:                  "{{[-]}}{{[white:red]}}",
		FatalFooter:            "{{[-]}}",
		TraceHeader:            "{{[-]}}{{[red]}}",
		TraceFooter:            "{{[-]}}{{[red]}}",
		TraceFrameNumber:       "{{[-]}}{{[red]}}",
		TraceFrameLines:        "{{[-]}}{{[red]}}",
		TraceSourceFile:        "{{[-]}}{{[cyan::B]}}",
		TraceLineNumber:        "{{[-]}}{{[yellow::B]}}",
		TraceFunction:          "{{[-]}}{{[green::B]}}",
		TraceCmd:               "{{[-]}}{{[green::B]}}",
		TraceCmdArgs:           "{{[-]}}{{[green]}}",
		UnitTestPass:           "{{[-]}}{{[green]}}",
		UnitTestFail:           "{{[-]}}{{[red]}}",
		UnitTestFailArrow:      "{{[-]}}{{[red]}}",
		App:                    "{{[-]}}{{[cyan]}}",
		ApplicationName:        "{{[-]}}{{[cyan::B]}}",
		Branch:                 "{{[-]}}{{[cyan]}}",
		FailingCommand:         "{{[-]}}{{[red]}}",
		File:                   "{{[-]}}{{[cyan::B]}}",
		Folder:                 "{{[-]}}{{[cyan::B]}}",
		Program:                "{{[-]}}{{[cyan]}}",
		RunningCommand:         "{{[-]}}{{[green::B]}}",
		Theme:                  "{{[-]}}{{[cyan]}}",
		Update:                 "{{[-]}}{{[green]}}",
		User:                   "{{[-]}}{{[cyan]}}",
		URL:                    "{{[-]}}{{[cyan::U]}}",
		UserCommand:            "{{[-]}}{{[yellow::B]}}",
		UserCommandError:       "{{[-]}}{{[red::U]}}",
		UserCommandErrorMarker: "{{[-]}}{{[red]}}",
		Var:                    "{{[-]}}{{[magenta]}}",
		Version:                "{{[-]}}{{[cyan]}}",
		Yes:                    "{{[-]}}{{[green]}}",
		No:                     "{{[-]}}{{[red]}}",

		// Usage Colors
		UsageCommand: "{{[-]}}{{[yellow::B]}}",
		UsageOption:  "{{[-]}}{{[yellow]}}",
		UsageApp:     "{{[-]}}{{[cyan]}}",
		UsageBranch:  "{{[-]}}{{[cyan]}}",
		UsageFile:    "{{[-]}}{{[cyan::B]}}",
		UsagePage:    "{{[-]}}{{[cyan::B]}}",
		UsageTheme:   "{{[-]}}{{[cyan]}}",
		UsageVar:     "{{[-]}}{{[magenta]}}",

		// Viewport Colors
		ProgramBox: "{{[-]}}{{[white:black]}}",
		LogBox:     "{{[-]}}{{[white:black]}}",
	}

	// Register base tags once Colors is populated
	RegisterBaseTags()
}

// ResolveTcellColor attempts to resolve a color name using local aliases first, then tcell
func ResolveTcellColor(name string) tcellColor.Color {
	name = strings.ToLower(name)
	if alias, ok := colorAliases[name]; ok {
		name = alias
	}
	return tcellColor.GetColor(name)
}

// GetHexForColor resolves a color name (including aliases) to a Hex string.
// Returns empty string if not found or invalid.
func GetHexForColor(name string) string {
	tc := ResolveTcellColor(name)
	if tc != tcellColor.Default {
		if h := tc.Hex(); h >= 0 {
			return fmt.Sprintf("#%06x", h)
		}
	}
	// Also check if it's already a hex string
	if strings.HasPrefix(name, "#") {
		return name
	}
	return ""
}

// ColorToHexMap has been removed in favor of tcell/v3/color parsing in internal/theme

// AppColors defines the struct for program-wide colors/styles
// Values are stored in tview tag format (e.g., "[cyan::b]")
type AppColors struct {
	// Base Codes
	Reset         string
	Bold          string
	Dim           string
	Underline     string
	Blink         string
	Reverse       string
	Strikethrough string
	HighIntensity string

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

	// Viewport Colors
	ProgramBox string
	LogBox     string
}

// Colors is the global instance for application output (stdout)
var Colors AppColors

// RegisterBaseTags registers semantic tag aliases
// These map semantic names to their tview-format output values
func RegisterBaseTags() {
	// Bash-style aliases from main.sh
	RegisterConsoleTag("NC", "{{[-]}}")
	RegisterConsoleTag("BD", "{{[::B]}}")
	RegisterConsoleTag("UL", "{{[::U]}}")
	RegisterConsoleTag("DM", "{{[::D]}}")
	RegisterConsoleTag("BL", "{{[::L]}}")

	// Existing shorthands
	RegisterConsoleTag("ul", "{{[::U]}}")
	RegisterConsoleTag("blink", "{{[::L]}}")

	// Semantic tags from struct fields (auto-registered by BuildColorMap)
	// Double-register here for explicit visibility and aliasMap access
	RegisterConsoleTag("applicationname", Colors.ApplicationName)
	RegisterConsoleTag("version", Colors.Version)
	RegisterConsoleTag("branch", Colors.Branch)
	RegisterConsoleTag("usercommand", Colors.UserCommand)
	RegisterConsoleTag("usercommanderror", Colors.UserCommandError)
	RegisterConsoleTag("usercommanderrormarker", Colors.UserCommandErrorMarker)
	RegisterConsoleTag("yes", Colors.Yes)
	RegisterConsoleTag("no", Colors.No)

	// Usage Colors
	RegisterConsoleTag("usagecommand", Colors.UsageCommand)
	RegisterConsoleTag("usageoption", Colors.UsageOption)
	RegisterConsoleTag("usageapp", Colors.UsageApp)
	RegisterConsoleTag("usagebranch", Colors.UsageBranch)
	RegisterConsoleTag("usagefile", Colors.UsageFile)
	RegisterConsoleTag("usagepage", Colors.UsagePage)
	RegisterConsoleTag("usagetheme", Colors.UsageTheme)
	RegisterConsoleTag("usagevar", Colors.UsageVar)

	// Viewport Tags
	RegisterConsoleTag("programbox", Colors.ProgramBox)
	RegisterConsoleTag("logbox", Colors.LogBox)

	// Log Level Tags
	RegisterConsoleTag("timestamp", Colors.Timestamp)
	RegisterConsoleTag("notice", Colors.Notice)
	RegisterConsoleTag("warn", Colors.Warn)
	RegisterConsoleTag("error", Colors.Error)
	RegisterConsoleTag("fatal", Colors.Fatal)
	RegisterConsoleTag("debug", Colors.Debug)
	RegisterConsoleTag("info", Colors.Info)
	RegisterConsoleTag("trace", Colors.Trace)
	RegisterConsoleTag("url", Colors.URL)

	// Additional Semantic Tags
	RegisterConsoleTag("app", Colors.App)
	RegisterConsoleTag("failingcommand", Colors.FailingCommand)
	RegisterConsoleTag("file", Colors.File)
	RegisterConsoleTag("folder", Colors.Folder)
	RegisterConsoleTag("program", Colors.Program)
	RegisterConsoleTag("runningcommand", Colors.RunningCommand)
	RegisterConsoleTag("theme", Colors.Theme)
	RegisterConsoleTag("update", Colors.Update)
	RegisterConsoleTag("user", Colors.User)
	RegisterConsoleTag("var", Colors.Var)

	// Legacy Foreground Colors (F array in main.sh)
	RegisterConsoleTag("B", Colors.Blue)
	RegisterConsoleTag("C", Colors.Cyan)
	RegisterConsoleTag("G", Colors.Green)
	RegisterConsoleTag("K", Colors.Black)
	RegisterConsoleTag("M", Colors.Magenta)
	RegisterConsoleTag("R", Colors.Red)
	RegisterConsoleTag("W", Colors.White)
	RegisterConsoleTag("Y", Colors.Yellow)

	// Explicit F Array Aliases
	RegisterConsoleTag("F_B", Colors.Blue)
	RegisterConsoleTag("F_C", Colors.Cyan)
	RegisterConsoleTag("F_G", Colors.Green)
	RegisterConsoleTag("F_K", Colors.Black)
	RegisterConsoleTag("F_M", Colors.Magenta)
	RegisterConsoleTag("F_R", Colors.Red)
	RegisterConsoleTag("F_W", Colors.White)
	RegisterConsoleTag("F_Y", Colors.Yellow)

	// Legacy Background Colors (B array in main.sh)
	RegisterConsoleTag("B_B", Colors.BlueBg)
	RegisterConsoleTag("B_C", Colors.CyanBg)
	RegisterConsoleTag("B_G", Colors.GreenBg)
	RegisterConsoleTag("B_K", Colors.BlackBg)
	RegisterConsoleTag("B_M", Colors.MagentaBg)
	RegisterConsoleTag("B_R", Colors.RedBg)
	RegisterConsoleTag("B_W", Colors.WhiteBg)
	RegisterConsoleTag("B_Y", Colors.YellowBg)
	// NOTE: Theme-related tags (ThemeHostname, ThemeTitle, etc.) are registered
	// by the theme package in theme.go Default() and Apply() functions.
}
