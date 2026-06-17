package semstyle

import (
	"fmt"
	"reflect"
	"strings"

	tcellColor "github.com/gdamore/tcell/v3/color"
)

// Raw ANSI Color Codes
const (
	// Reset
	CodeReset   = "\033[0m"
	CodeFGReset = "\033[39m" // Reset foreground to default
	CodeBGReset = "\033[49m" // Reset background to default

	// Hard reset sequences: multi-parameter SGR variants with the same terminal effect as
	// the single-parameter resets above. Useful when a compositor or filter intercepts the
	// single-parameter forms (\x1b[0m, \x1b[39m, \x1b[49m) but should let a true reset through.
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
		IPAddress:              "{{[-]}}{{[cyan]}}",
		URL:                    "{{[-]}}{{[cyan::U]}}",
		UserCommand:            "{{[-]}}{{[yellow::B]}}",
		UserCommandError:       "{{[-]}}{{[red::U]}}",
		UserCommandErrorMarker: "{{[-]}}{{[red]}}",
		MenuPage:               "{{[-]}}{{[cyan]}}",
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
		ConsoleBox: "{{[-]}}{{[white:black]}}",

		// Docker Compose progress colors — markers (icons, labels, decorations)
		DockerMarkerDone:  "{{[-]}}{{[green]}}",
		DockerMarkerError: "{{[-]}}{{[red]}}",
		DockerMarkerWarn:  "{{[-]}}{{[yellow]}}",
		DockerColon:       "{{[-]}}{{[gray::D]}}",
		DockerImage:       "{{[-]}}{{[magenta]}}",
		DockerTag:         "{{[-]}}{{[magenta::D]}}",
		DockerSpinner:     "{{[-]}}{{[yellow]}}",
		DockerBar:         "{{[-]}}{{[cyan]}}",
		DockerSharedLayer: "{{[-]}}{{[yellow]}}",
		// Docker Compose progress colors — status text
		DockerStatusSuccess: "{{[-]}}{{[cyan]}}",
		DockerStatusFinal:   "{{[-]}}{{[green::B]}}",
		DockerStatusFail:    "{{[-]}}{{[red]}}",
		DockerStatusWarn:    "{{[-]}}{{[yellow]}}",
		DockerStatusPending: "{{[-]}}{{[gray::D]}}",
		DockerStatusActive:  "{{[-]}}{{[yellow]}}",
	}

	// Re-register base tags onto Default now that Colors is populated.
	//
	// Ordering note: Default = New() (a var initializer) runs before this init() and already
	// calls RegisterBaseTags, but at that point Colors is still its zero value (Colors is set
	// here in init, not at declaration). This second call re-registers from the populated
	// Colors so Default is correct before any application code runs. Any Styler created via
	// New() *after* package init (the normal case) sees the populated Colors directly.
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
	IPAddress              string
	URL                    string
	UserCommand            string
	UserCommandError       string
	UserCommandErrorMarker string
	MenuPage               string
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
	ConsoleBox string

	// Docker Compose progress colors — markers (icons, labels, decorations)
	DockerMarkerDone  string
	DockerMarkerError string
	DockerMarkerWarn  string
	DockerColon       string
	DockerImage       string
	DockerTag         string
	DockerSpinner     string
	DockerBar         string
	DockerSharedLayer string
	// Docker Compose progress colors — status text
	DockerStatusSuccess string
	DockerStatusFinal   string
	DockerStatusFail    string
	DockerStatusWarn    string
	DockerStatusPending string
	DockerStatusActive  string
}

// Colors is the global instance for application output (stdout)
var Colors AppColors

// RegisterBaseTags registers semantic tag aliases from AppColors struct fields
// and a small set of static aliases not covered by the struct.
func (st *Styler) RegisterBaseTags() {
	// Auto-register all AppColors struct fields by lowercased field name.
	v := reflect.ValueOf(Colors)
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		val := v.Field(i).String()
		if val != "" {
			st.RegisterConsoleTag(strings.ToLower(field.Name), val)
		}
	}

	// Bash-style aliases from main.sh
	st.RegisterConsoleTag("NC", "{{[-]}}")
	st.RegisterConsoleTag("BD", "{{[::B]}}")
	st.RegisterConsoleTag("UL", "{{[::U]}}")
	st.RegisterConsoleTag("DM", "{{[::D]}}")
	st.RegisterConsoleTag("BL", "{{[::L]}}")

	// Existing shorthands
	st.RegisterConsoleTag("ul", "{{[::U]}}")
	st.RegisterConsoleTag("blink", "{{[::L]}}")

	// Legacy single-letter foreground aliases (F array in main.sh)
	st.RegisterConsoleTag("B", Colors.Blue)
	st.RegisterConsoleTag("C", Colors.Cyan)
	st.RegisterConsoleTag("G", Colors.Green)
	st.RegisterConsoleTag("K", Colors.Black)
	st.RegisterConsoleTag("M", Colors.Magenta)
	st.RegisterConsoleTag("R", Colors.Red)
	st.RegisterConsoleTag("W", Colors.White)
	st.RegisterConsoleTag("Y", Colors.Yellow)

	// Explicit F_ aliases
	st.RegisterConsoleTag("F_B", Colors.Blue)
	st.RegisterConsoleTag("F_C", Colors.Cyan)
	st.RegisterConsoleTag("F_G", Colors.Green)
	st.RegisterConsoleTag("F_K", Colors.Black)
	st.RegisterConsoleTag("F_M", Colors.Magenta)
	st.RegisterConsoleTag("F_R", Colors.Red)
	st.RegisterConsoleTag("F_W", Colors.White)
	st.RegisterConsoleTag("F_Y", Colors.Yellow)

	// Legacy background aliases (B array in main.sh)
	st.RegisterConsoleTag("B_B", Colors.BlueBg)
	st.RegisterConsoleTag("B_C", Colors.CyanBg)
	st.RegisterConsoleTag("B_G", Colors.GreenBg)
	st.RegisterConsoleTag("B_K", Colors.BlackBg)
	st.RegisterConsoleTag("B_M", Colors.MagentaBg)
	st.RegisterConsoleTag("B_R", Colors.RedBg)
	st.RegisterConsoleTag("B_W", Colors.WhiteBg)
	st.RegisterConsoleTag("B_Y", Colors.YellowBg)

	// NOTE: Theme-related tags (ThemeHostname, ThemeTitle, etc.) are registered
	// by the theme package in theme.go Default() and Apply() functions.
}

// --- package-level delegators to Default ---
func RegisterBaseTags() {
	Default.RegisterBaseTags()
}
