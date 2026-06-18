package semstyle

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
	colorAliases = map[string]string{
		// tcell compatibility
		"cyan":    "aqua",
		"magenta": "fuchsia",
		// bright- variants
		"bright-red":     "red",
		"bright-green":   "lime",
		"bright-blue":    "blue",
		"bright-yellow":  "yellow",
		"bright-magenta": "fuchsia",
		"bright-cyan":    "aqua",
		"bright-white":   "white",
		"bright-black":   "gray",
	}
}

// ResolveTcellColor resolves a color name using local aliases first, then tcell.
func ResolveTcellColor(name string) tcellColor.Color {
	name = strings.ToLower(name)
	if alias, ok := colorAliases[name]; ok {
		name = alias
	}
	return tcellColor.GetColor(name)
}

// GetHexForColor resolves a color name (including aliases) to a hex string.
// Returns empty string if not found or invalid.
func GetHexForColor(name string) string {
	tc := ResolveTcellColor(name)
	if tc != tcellColor.Default {
		if h := tc.Hex(); h >= 0 {
			return fmt.Sprintf("#%06x", h)
		}
	}
	if strings.HasPrefix(name, "#") {
		return name
	}
	return ""
}
