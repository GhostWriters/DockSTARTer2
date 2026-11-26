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
	UserApp                string
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
	DockerMarkerDone    string
	DockerMarkerError   string
	DockerMarkerWarn    string
	DockerMarkerCause   string // a warning row that caused another row's failure
	DockerColon         string
	DockerImage         string
	DockerTag           string
	DockerSpinner       string
	DockerBar           string
	DockerSharedLayer   string
	DockerSection       string // the final status word on a section or kind heading's line
	DockerTitle         string // a kind heading's title (services:, N networks:), its items' colour at high intensity
	DockerTitleImage    string // the same for images
	DockerGroupHeading  string // the changed and unchanged headings and their timers
	DockerGroupStatus   string // the final status word on those headings' lines (Complete, or the word their rows share)
	DockerSeparator     string // the rule of the changed and unchanged headings, in the group's hue
	DockerInfo          string // an image's info lines: their marker, Info status and label
	DockerStatusSuccess string
	DockerStatusFail    string
	DockerStatusWarn    string
	DockerStatusPending string
	DockerStatusActive  string
	// Docker Compose progress colors — healthcheck line status
	DockerHealthChecking  string
	DockerHealthUnhealthy string
	DockerHealthNoResult  string
	// Docker Compose progress colors — detail line labels (healthcheck, depends_on)
	DockerDetailLabel string
	// Docker Compose progress colors — dependency names, by whether their condition is met
	DockerDependsWaiting string
	DockerDependsMet     string
	DockerDependsFailed  string
}

// The hues the Docker display's kinds share: what the compose file defines (services,
// networks, volumes, models, jobs, with the containers and orphans beside them) is
// itemHue, images are imageHue; groupHue is the changed and unchanged groups', as if
// they had items of their own. A kind's name, final status and counts are its hue; its
// title and timer are the bright version, bold. Change a hue here and they all follow.
const (
	itemHue  = "cyan"
	imageHue = "magenta"
	groupHue = "green"
)

// hueStyle returns the style of hue with attrs (B bold, U underline, D dim).
func hueStyle(hue, attrs string) string {
	if attrs == "" {
		return "{{[-]}}{{[" + hue + "]}}"
	}
	return "{{[-]}}{{[" + hue + "::" + attrs + "]}}"
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
	App:                    hueStyle(itemHue, "B"),
	UserApp:                hueStyle(itemHue, ""),
	ApplicationName:        "{{[-]}}{{[cyan::B]}}",
	Branch:                 "{{[-]}}{{[cyan]}}",
	FailingCommand:         "{{[-]}}{{[red]}}",
	File:                   "{{[-]}}{{[cyan::B]}}",
	Folder:                 hueStyle(itemHue, "B"),
	Program:                "{{[-]}}{{[cyan]}}",
	RunningCommand:         "{{[-]}}{{[green::B]}}",
	Theme:                  "{{[-]}}{{[cyan]}}",
	Update:                 "{{[-]}}{{[green]}}",
	User:                   "{{[-]}}{{[cyan]}}",
	IPAddress:              hueStyle(itemHue, ""),
	URL:                    hueStyle(itemHue, "U"),
	UserCommand:            "{{[-]}}{{[yellow::B]}}",
	UserCommandError:       "{{[-]}}{{[red::U]}}",
	UserCommandErrorMarker: "{{[-]}}{{[red]}}",
	MenuPage:               "{{[-]}}{{[cyan]}}",
	Var:                    "{{[-]}}{{[magenta]}}",
	Version:                hueStyle(itemHue, ""),
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
	DockerMarkerDone:   "{{[-]}}{{[green]}}",
	DockerMarkerError:  "{{[-]}}{{[red]}}",
	DockerMarkerWarn:   "{{[-]}}{{[yellow]}}",
	DockerMarkerCause:  "{{[-]}}{{[red]}}",
	DockerColon:        "{{[-]}}{{[gray::D]}}",
	DockerImage:        hueStyle(imageHue, ""),
	DockerTag:          hueStyle(imageHue, "D"),
	DockerSpinner:      "{{[-]}}{{[yellow]}}",
	DockerBar:          hueStyle(itemHue, ""),
	DockerSharedLayer:  "{{[-]}}{{[yellow]}}",
	DockerSection:      hueStyle(itemHue, "B"),
	DockerTitle:        hueStyle("bright-"+itemHue, "B"),
	DockerTitleImage:   hueStyle("bright-"+imageHue, "B"),
	DockerGroupHeading: hueStyle("bright-"+groupHue, "B"),
	DockerGroupStatus:  hueStyle(groupHue, "B"),
	DockerSeparator:    hueStyle(groupHue, ""),
	DockerInfo:         "{{[-]}}{{[bright-blue::B]}}",
	// Docker Compose progress colors — status text
	DockerStatusSuccess: hueStyle(itemHue, ""),
	DockerStatusFail:    "{{[-]}}{{[red]}}",
	DockerStatusWarn:    "{{[-]}}{{[yellow]}}",
	DockerStatusPending: "{{[-]}}{{[gray::D]}}",
	DockerStatusActive:  "{{[-]}}{{[yellow]}}",
	// Docker Compose progress colors — healthcheck line status
	DockerHealthChecking:  "{{[-]}}{{[yellow]}}",
	DockerHealthUnhealthy: "{{[-]}}{{[red]}}",
	DockerHealthNoResult:  "{{[-]}}{{[yellow]}}",
	// Docker Compose progress colors — detail line labels
	DockerDetailLabel: "{{[-]}}{{[white]}}",
	// Docker Compose progress colors — dependency names
	DockerDependsWaiting: "{{[-]}}{{[yellow]}}",
	DockerDependsMet:     "{{[-]}}{{[green]}}",
	DockerDependsFailed:  "{{[-]}}{{[red]}}",
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
