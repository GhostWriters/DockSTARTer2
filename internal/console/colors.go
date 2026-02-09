package console

import (
	"fmt"
	"strings"

	tcellColor "github.com/gdamore/tcell/v3/color"
)

// Raw ANSI Color Codes
const (
	// Reset
	CodeReset = "\033[0m"

	// Modifiers
	CodeBold          = "\033[1m"
	CodeDim           = "\033[2m"
	CodeUnderline     = "\033[4m"
	CodeBlink         = "\033[5m"
	CodeReverse       = "\033[7m"
	CodeStrikethrough = "\033[9m"

	// Modifiers (Off)
	CodeBoldOff          = "\033[22m"
	CodeDimOff           = "\033[22m"
	CodeUnderlineOff     = "\033[24m"
	CodeBlinkOff         = "\033[25m"
	CodeReverseOff       = "\033[27m"
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
}

// Colors is the global instance for application output (stdout)
var Colors AppColors

// RegisterBaseTags registers semantic tag aliases
// These map semantic names to their tview-format output values
func RegisterBaseTags() {
	// Bash-style aliases from main.sh
	RegisterSemanticTag("NC", "{{|-|}}")
	RegisterSemanticTag("BD", "{{|::B|}}")
	RegisterSemanticTag("UL", "{{|::U|}}")
	RegisterSemanticTag("DM", "{{|::D|}}")
	RegisterSemanticTag("BL", "{{|::L|}}")

	// Existing shorthands
	RegisterSemanticTag("ul", "{{|::U|}}")
	RegisterSemanticTag("blink", "{{|::L|}}")

	// Semantic tags from struct fields (auto-registered by BuildColorMap)
	// Double-register here for explicit visibility and aliasMap access
	RegisterSemanticTag("applicationname", Colors.ApplicationName)
	RegisterSemanticTag("version", Colors.Version)
	RegisterSemanticTag("branch", Colors.Branch)
	RegisterSemanticTag("usercommand", Colors.UserCommand)
	RegisterSemanticTag("usercommanderror", Colors.UserCommandError)
	RegisterSemanticTag("usercommanderrormarker", Colors.UserCommandErrorMarker)
	RegisterSemanticTag("yes", Colors.Yes)
	RegisterSemanticTag("no", Colors.No)

	// Usage Colors
	RegisterSemanticTag("usagecommand", Colors.UsageCommand)
	RegisterSemanticTag("usageoption", Colors.UsageOption)
	RegisterSemanticTag("usageapp", Colors.UsageApp)
	RegisterSemanticTag("usagebranch", Colors.UsageBranch)
	RegisterSemanticTag("usagefile", Colors.UsageFile)
	RegisterSemanticTag("usagepage", Colors.UsagePage)
	RegisterSemanticTag("usagetheme", Colors.UsageTheme)
	RegisterSemanticTag("usagevar", Colors.UsageVar)

	// Log Level Tags
	RegisterSemanticTag("timestamp", Colors.Timestamp)
	RegisterSemanticTag("notice", Colors.Notice)
	RegisterSemanticTag("warn", Colors.Warn)
	RegisterSemanticTag("error", Colors.Error)
	RegisterSemanticTag("fatal", Colors.Fatal)
	RegisterSemanticTag("debug", Colors.Debug)
	RegisterSemanticTag("info", Colors.Info)
	RegisterSemanticTag("trace", Colors.Trace)
	RegisterSemanticTag("url", Colors.URL)

	// Additional Semantic Tags
	RegisterSemanticTag("app", Colors.App)
	RegisterSemanticTag("failingcommand", Colors.FailingCommand)
	RegisterSemanticTag("file", Colors.File)
	RegisterSemanticTag("folder", Colors.Folder)
	RegisterSemanticTag("program", Colors.Program)
	RegisterSemanticTag("runningcommand", Colors.RunningCommand)
	RegisterSemanticTag("theme", Colors.Theme)
	RegisterSemanticTag("update", Colors.Update)
	RegisterSemanticTag("user", Colors.User)
	RegisterSemanticTag("var", Colors.Var)

	// Legacy Foreground Colors (F array in main.sh)
	RegisterSemanticTag("B", Colors.Blue)
	RegisterSemanticTag("C", Colors.Cyan)
	RegisterSemanticTag("G", Colors.Green)
	RegisterSemanticTag("K", Colors.Black)
	RegisterSemanticTag("M", Colors.Magenta)
	RegisterSemanticTag("R", Colors.Red)
	RegisterSemanticTag("W", Colors.White)
	RegisterSemanticTag("Y", Colors.Yellow)

	// Explicit F Array Aliases
	RegisterSemanticTag("F_B", Colors.Blue)
	RegisterSemanticTag("F_C", Colors.Cyan)
	RegisterSemanticTag("F_G", Colors.Green)
	RegisterSemanticTag("F_K", Colors.Black)
	RegisterSemanticTag("F_M", Colors.Magenta)
	RegisterSemanticTag("F_R", Colors.Red)
	RegisterSemanticTag("F_W", Colors.White)
	RegisterSemanticTag("F_Y", Colors.Yellow)

	// Legacy Background Colors (B array in main.sh)
	RegisterSemanticTag("B_B", Colors.BlueBg)
	RegisterSemanticTag("B_C", Colors.CyanBg)
	RegisterSemanticTag("B_G", Colors.GreenBg)
	RegisterSemanticTag("B_K", Colors.BlackBg)
	RegisterSemanticTag("B_M", Colors.MagentaBg)
	RegisterSemanticTag("B_R", Colors.RedBg)
	RegisterSemanticTag("B_W", Colors.WhiteBg)
	RegisterSemanticTag("B_Y", Colors.YellowBg)
	// NOTE: Theme-related tags (ThemeHostname, ThemeTitle, etc.) are registered
	// by the theme package in theme.go Default() and Apply() functions.
}
