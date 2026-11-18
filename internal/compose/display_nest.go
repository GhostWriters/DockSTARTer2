package compose

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"DockSTARTer2/internal/strutil"
)

// nestW is how far a group heading's rows sit further in, past the status column.
const nestW = sectionChildIndentW

// nestLine returns line with its content, everything past the status column (the
// row's name, an image or info line, a message), nestW further in, so it sits under a
// group heading. The status column stays put. Tags count no width.
func nestLine(line string) string {
	col := globalIndentW + iconW + spaceW + sectionStatusW
	seen := 0
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "{{") {
			if end := strings.Index(line[i:], "}}"); end >= 0 {
				i += end + 2
				continue
			}
		}
		if seen == col {
			return line[:i] + strutil.Repeat(" ", nestW) + line[i:]
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		i += size
		seen++
	}
	return line // shorter than the status column: nothing to move
}

// groupHeading returns a group heading: its rows' rolled-up state in the status
// column, as a section heading's, then how many rows of noun it holds, adjective
// saying which (changed, unchanged).
func groupHeading(icon, statusTag, statusText string, count int, adjective, singular, plural string) string {
	noun := singular
	if count != 1 {
		noun = plural
	}
	return globalIndent + icon + " " + statusTag + strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText)) +
		sectionChildIndent + "{{|DockerDetailLabel|}}" + strconv.Itoa(count) + " " + adjective + " " + noun + "{{[-]}}{{|DockerColon|}}:{{[-]}}"
}
