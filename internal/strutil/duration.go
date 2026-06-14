package strutil

import (
	"fmt"
	"strings"
	"time"
)

// FormatDuration formats a duration using a layout string that mirrors Go's time.Format convention.
// Reference tokens for minutes and seconds (when minutes are present) are handled by
// time.Time.Format. Fractional seconds and the no-minutes total-seconds case are handled
// on top, as time.Format does not support them for durations.
//
//	Minutes: "04" zero-pad, "_4" space-pad, "4" no-pad
//	Seconds: "05" zero-pad, "_5" space-pad, "5" no-pad
//	Fractional: ".0" tenths, ".00" hundredths, ".000" milliseconds
//
// If the layout contains no minutes token ("4"), seconds reflect the total duration
// (e.g. 62.3s → "62.3s" not "2.3s") and are handled without time.Format.
//
// Examples:
//
//	"4m05.0s"  → "1m02.3s"
//	"5.0s"     → "9.1s" / "42.7s" / "200.3s"
//	"05.0s"    → "09.1s"
func FormatDuration(d time.Duration, layout string) string {
	if d < 0 {
		d = 0
	}
	totalMs := int64(d.Milliseconds())
	mins := int(totalMs / 60000)
	remMs := totalMs - int64(mins)*60000
	remS := int(remMs / 1000)
	ms := int(remMs % 1000)

	hasMinutes := strings.ContainsAny(layout, "4")

	// Build fractional string and strip it from the layout, replacing with a placeholder.
	const fracPlaceholder = "\x00F"
	fracStr := ""
	if fracIdx := strings.Index(layout, "."); fracIdx >= 0 {
		j := fracIdx + 1
		for j < len(layout) && layout[j] == '0' {
			j++
		}
		if digits := j - (fracIdx + 1); digits > 0 {
			divisor := 1
			for k := digits; k < 3; k++ {
				divisor *= 10
			}
			fracStr = "." + fmt.Sprintf("%0*d", digits, ms/divisor)
			layout = layout[:fracIdx] + fracPlaceholder + layout[j:]
		}
	}

	var result string
	if hasMinutes {
		// time.Format handles 4/04 (minutes) and 5/05 (seconds) natively.
		// _4/_5 (space-pad) are not time.Format tokens — substitute them first.
		layout = strings.ReplaceAll(layout, "_4", fmt.Sprintf("%2d", mins))
		layout = strings.ReplaceAll(layout, "_5", fmt.Sprintf("%2d", remS))
		t := time.Date(0, 1, 1, 0, mins, remS, 0, time.UTC)
		result = t.Format(layout)
	} else {
		// No minutes token — use total seconds, handle all tokens ourselves.
		totalS := int(totalMs / 1000)
		layout = strings.ReplaceAll(layout, "05", fmt.Sprintf("%02d", totalS))
		layout = strings.ReplaceAll(layout, "_5", fmt.Sprintf("%2d", totalS))
		layout = strings.ReplaceAll(layout, "5", fmt.Sprintf("%d", totalS))
		result = layout
	}

	result = strings.ReplaceAll(result, fracPlaceholder, fracStr)
	return result
}
