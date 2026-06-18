package console

// DS2-specific semantic tag definitions and base-tag registration.
//
// AppColors holds the default style values for every named semantic tag used by DS2.
// RegisterBaseTags registers them (and the legacy Bash-style aliases) into the Default
// semstyle Styler. Call it once at startup and again after a theme load to restore any
// base tags that the theme overrode.

import (
	"reflect"
	"strings"

	"github.com/GhostWriters/semstyle"
)

// AppColors holds DS2's named semantic styles.
// Each field value is a tagged style string (e.g. "{{[cyan::B]}}").
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

// Colors is DS2's global style instance. RegisterBaseTags reads from it.
var Colors = AppColors{
	// Base Codes
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

	// Semantic Colors
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

	// Docker Compose progress colors — markers
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

func init() {
	RegisterBaseTags()
}

// RegisterBaseTags registers DS2's semantic tags and legacy Bash-style aliases into the
// Default semstyle Styler. Call after theme load to restore base tags.
func RegisterBaseTags() {
	// Auto-register all AppColors struct fields by lowercased field name.
	v := reflect.ValueOf(Colors)
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		val := v.Field(i).String()
		if val != "" {
			semstyle.RegisterConsoleTag(strings.ToLower(field.Name), val)
		}
	}

	// Bash-style aliases from main.sh
	semstyle.RegisterConsoleTag("NC", "{{[-]}}")
	semstyle.RegisterConsoleTag("BD", "{{[::B]}}")
	semstyle.RegisterConsoleTag("UL", "{{[::U]}}")
	semstyle.RegisterConsoleTag("DM", "{{[::D]}}")
	semstyle.RegisterConsoleTag("BL", "{{[::L]}}")

	// Existing shorthands
	semstyle.RegisterConsoleTag("ul", "{{[::U]}}")
	semstyle.RegisterConsoleTag("blink", "{{[::L]}}")

	// Legacy single-letter foreground aliases (F array in main.sh)
	semstyle.RegisterConsoleTag("B", Colors.Blue)
	semstyle.RegisterConsoleTag("C", Colors.Cyan)
	semstyle.RegisterConsoleTag("G", Colors.Green)
	semstyle.RegisterConsoleTag("K", Colors.Black)
	semstyle.RegisterConsoleTag("M", Colors.Magenta)
	semstyle.RegisterConsoleTag("R", Colors.Red)
	semstyle.RegisterConsoleTag("W", Colors.White)
	semstyle.RegisterConsoleTag("Y", Colors.Yellow)

	// Explicit F_ aliases
	semstyle.RegisterConsoleTag("F_B", Colors.Blue)
	semstyle.RegisterConsoleTag("F_C", Colors.Cyan)
	semstyle.RegisterConsoleTag("F_G", Colors.Green)
	semstyle.RegisterConsoleTag("F_K", Colors.Black)
	semstyle.RegisterConsoleTag("F_M", Colors.Magenta)
	semstyle.RegisterConsoleTag("F_R", Colors.Red)
	semstyle.RegisterConsoleTag("F_W", Colors.White)
	semstyle.RegisterConsoleTag("F_Y", Colors.Yellow)

	// Legacy background aliases (B array in main.sh)
	semstyle.RegisterConsoleTag("B_B", Colors.BlueBg)
	semstyle.RegisterConsoleTag("B_C", Colors.CyanBg)
	semstyle.RegisterConsoleTag("B_G", Colors.GreenBg)
	semstyle.RegisterConsoleTag("B_K", Colors.BlackBg)
	semstyle.RegisterConsoleTag("B_M", Colors.MagentaBg)
	semstyle.RegisterConsoleTag("B_R", Colors.RedBg)
	semstyle.RegisterConsoleTag("B_W", Colors.WhiteBg)
	semstyle.RegisterConsoleTag("B_Y", Colors.YellowBg)
}
