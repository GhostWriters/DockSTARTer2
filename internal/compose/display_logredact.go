package compose

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// redactedText replaces what redactLogLine hides.
const redactedText = "[redacted]"

// Container log text can carry credentials, and the display is copied into the log file,
// so each line passes through redactLogLine before it is stored anywhere. The sensitive
// words are matched inside names (DB_PASSWORD, apiKey, x-auth-token).
const sensitiveWords = `pass(?:word|wd|phrase)?|secret|token|key|auth|credential|cookie|session|bearer|signature`

var (
	redactPEM      = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----.*`)
	redactURLCreds = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^\s/:@]+:)[^\s/@]+@`)
	redactScheme   = regexp.MustCompile(`(?i)\b(bearer|basic)(\s+)[A-Za-z0-9._~+/=-]{6,}`)
	// name, optional closing quote, separator, then a quoted value or a run to whitespace.
	redactPair = regexp.MustCompile(`(?i)([\w.-]*(?:` + sensitiveWords + `)[\w.-]*["']?\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;&]+)`)
	// a flag and its value in the next word: --password hunter2
	redactFlag  = regexp.MustCompile(`(?i)(--?[\w-]*(?:` + sensitiveWords + `)[\w-]*\s+)([^\s-][^\s]*)`)
	redactShape = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]*|` +
		`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9_-]{16,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{10,})`)
	// a long unbroken run, kept only when it mixes letters and digits (a hash or key).
	redactOpaque = regexp.MustCompile(`[A-Za-z0-9+/_=-]{32,}`)
)

// redactLogLine hides credentials in one line of container output: private keys,
// passwords in URLs, bearer tokens, values of password/secret/token/key-named settings
// and flags, known token formats, and long opaque strings. The rest of the line stays,
// so the message can still be read and searched. Free text such as "the password is x"
// has no marker to find, which is why callers also limit how much output they show.
func redactLogLine(s string) string {
	s = redactPEM.ReplaceAllString(s, redactedText)
	s = redactURLCreds.ReplaceAllString(s, "${1}"+redactedText+"@")
	s = redactScheme.ReplaceAllString(s, "${1}${2}"+redactedText)
	s = redactPair.ReplaceAllStringFunc(s, func(m string) string {
		sub := redactPair.FindStringSubmatch(m)
		if strings.TrimSpace(sub[2]) == redactedText || strings.HasSuffix(sub[2], redactedText) {
			return m
		}
		return sub[1] + redactedText
	})
	s = redactFlag.ReplaceAllString(s, "${1}"+redactedText)
	s = redactShape.ReplaceAllString(s, redactedText)
	s = redactOpaque.ReplaceAllStringFunc(s, func(m string) string {
		if strings.ContainsAny(m, "0123456789") && strings.IndexFunc(m, isLetter) >= 0 {
			return redactedText
		}
		return m
	})
	// "Authorization: Bearer x" is hidden twice, by its scheme and by its name.
	return strings.ReplaceAll(s, redactedText+" "+redactedText, redactedText)
}

func isLetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

// cleanLogLine makes one line of container output safe to show and store: terminal
// escapes and control characters removed, whitespace collapsed, then redacted.
func cleanLogLine(s string) string {
	s = strings.Join(strings.Fields(stripControl(ansi.Strip(s))), " ")
	return redactLogLine(s)
}

// errorWords marks a line that likely says what went wrong.
var errorWords = regexp.MustCompile(`(?i)\b(error|errors|fatal|panic|failed|failure|denied|refused|cannot|can't|unable|exception|traceback|critical|invalid)\b`)

// logLinesShown is how many log lines a failed dependency shows.
const logLinesShown = 3

// pickLogLines chooses what to show from a container's last lines: the first line that
// looks like an error and the last line when it differs, since the cause is often
// above a closing hint; with no error line, the last few lines.
func pickLogLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	for i, l := range lines {
		if errorWords.MatchString(l) {
			out := []string{l}
			if last := lines[len(lines)-1]; i != len(lines)-1 {
				out = append(out, last)
			}
			return out
		}
	}
	return lines[max(len(lines)-logLinesShown, 0):]
}
