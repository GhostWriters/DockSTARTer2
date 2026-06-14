package strutil

import (
	"fmt"
	"strings"
	"time"
)

// FormatDuration formats a duration using a layout string that mirrors Go's time.Format convention.
// Reference tokens (matched in order, longest first):
//
//	Minutes: "04" zero-pad, "_4" space-pad, "4" no-pad
//	Seconds: "05" zero-pad, "_5" space-pad, "5" no-pad
//	Fractional seconds (appended to seconds): ".000" ms, ".00" hundredths, ".0" tenths
//
// Example layouts:
//
//	"4m05.0s"  → "1m02.3s"
//	"_5.0s"    → " 9.1s" or "42.7s"
//	"05.0s"    → "09.1s"
func FormatDuration(d time.Duration, layout string) string {
	if d < 0 {
		d = 0
	}
	// Work in integer milliseconds to avoid float drift.
	totalMs := int64(d.Milliseconds())
	mins := int(totalMs / 60000)
	remMs := totalMs - int64(mins)*60000

	// If layout has no minutes token, use total seconds rather than remainder.
	hasMinutes := strings.ContainsAny(layout, "4")
	var wholeS, ms int
	if hasMinutes {
		wholeS = int(remMs / 1000)
		ms = int(remMs % 1000)
	} else {
		wholeS = int(totalMs / 1000)
		ms = int(totalMs % 1000)
	}

	// Use placeholders to avoid substituted digits being re-matched by later tokens.
	// Each sentinel is a unique null-byte-prefixed string that cannot appear in layout.
	const (
		pMins = "\x00M"
		pSecs = "\x00S"
		pFrac = "\x00F"
	)

	result := layout

	// Minutes tokens (longest first)
	result = strings.ReplaceAll(result, "04", fmt.Sprintf("%s%02d", pMins, mins))
	result = strings.ReplaceAll(result, "_4", fmt.Sprintf("%s%2d", pMins, mins))
	result = strings.ReplaceAll(result, "4", fmt.Sprintf("%s%d", pMins, mins))

	// Seconds tokens (longest first)
	result = strings.ReplaceAll(result, "05", fmt.Sprintf("%s%02d", pSecs, wholeS))
	result = strings.ReplaceAll(result, "_5", fmt.Sprintf("%s%2d", pSecs, wholeS))
	result = strings.ReplaceAll(result, "5", fmt.Sprintf("%s%d", pSecs, wholeS))

	// Fractional second suffix: find '.0+' in the original layout positions (before sentinels
	// have shifted indices), then replace with the formatted fractional value.
	if i := strings.Index(result, "."); i >= 0 {
		j := i + 1
		for j < len(result) && result[j] == '0' {
			j++
		}
		digits := j - (i + 1)
		if digits > 0 {
			divisor := 1
			for k := digits; k < 3; k++ {
				divisor *= 10
			}
			result = result[:i] + pFrac + fmt.Sprintf("%0*d", digits, ms/divisor) + result[j:]
		}
	}

	// Resolve all sentinels
	result = strings.ReplaceAll(result, pMins, "")
	result = strings.ReplaceAll(result, pSecs, "")
	result = strings.ReplaceAll(result, pFrac, ".")

	return result
}
