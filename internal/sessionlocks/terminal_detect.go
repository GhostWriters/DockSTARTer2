package sessionlocks

import (
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// queryXTVersion sends the XTVERSION escape sequence and reads the response.
// Returns the terminal name (e.g. "WezTerm 20240203") or "" if unsupported.
// Only works when stdout is a real TTY.
func queryXTVersion() string {
	fd := int(os.Stdout.Fd())
	if !term.IsTerminal(fd) {
		return ""
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return ""
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	// Send XTVERSION query.
	if _, err := os.Stdout.WriteString("\033[>q"); err != nil {
		return ""
	}

	// Read response with a short timeout using a goroutine.
	type result struct{ s string }
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 128)
		// Response format: \033P>|TerminalName Version\033\\
		var collected []byte
		stdin := os.Stdin
		_ = stdin.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		defer func() { _ = stdin.SetReadDeadline(time.Time{}) }()
		for {
			n, err := stdin.Read(buf)
			if n > 0 {
				collected = append(collected, buf[:n]...)
			}
			if err != nil || len(collected) > 100 {
				break
			}
			// Stop once we see the ST terminator \033\\
			if len(collected) >= 2 {
				s := string(collected)
				if strings.Contains(s, "\033\\") || strings.Contains(s, "\a") {
					break
				}
			}
		}
		ch <- result{string(collected)}
	}()

	select {
	case r := <-ch:
		return parseXTVersion(r.s)
	case <-time.After(250 * time.Millisecond):
		return ""
	}
}

// parseXTVersion extracts the terminal name from a DCS response.
// Input: \033P>|TerminalName Version\033\\ → "TerminalName Version"
func parseXTVersion(response string) string {
	// Find >| marker
	idx := strings.Index(response, ">|")
	if idx < 0 {
		return ""
	}
	s := response[idx+2:]
	// Strip ST terminator \033\\ or \a
	if i := strings.Index(s, "\033\\"); i >= 0 {
		s = s[:i]
	} else if i := strings.Index(s, "\a"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// DetectTerminal returns a terminal identifier string in the form
// "TerminalName/TERM" using XTVERSION query, TERM_PROGRAM, and TERM env vars.
func DetectTerminal() string {
	term := os.Getenv("TERM")

	// Try XTVERSION first for precise terminal name.
	if xtv := queryXTVersion(); xtv != "" {
		if term != "" {
			return xtv + "/" + term
		}
		return xtv
	}

	// Fall back to TERM_PROGRAM.
	if termProgram := os.Getenv("TERM_PROGRAM"); termProgram != "" {
		if term != "" {
			return termProgram + "/" + term
		}
		return termProgram
	}

	return term
}
