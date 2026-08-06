//go:build !windows

package console

import (
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// queryENQ sends ENQ (Ctrl+E / \005) and reads the terminal's answerback string.
// PuTTY responds with "PuTTY" by default. Returns "" if unsupported or timed out.
func queryENQ() string {
	fd := int(os.Stdout.Fd())
	if !term.IsTerminal(fd) {
		return ""
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return ""
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	if _, err := os.Stdout.WriteString("\005"); err != nil {
		return ""
	}

	tty, err := os.Open("/dev/tty")
	if err != nil {
		return ""
	}

	ch := make(chan string, 1)
	go func() {
		defer close(ch)
		buf := make([]byte, 64)
		var collected []byte
		for len(collected) < 64 {
			n, readErr := tty.Read(buf)
			if n > 0 {
				collected = append(collected, buf[:n]...)
			}
			if readErr != nil {
				break
			}
			// ENQ responses are short and don't have a terminator — stop on any pause
			if len(collected) > 0 {
				break
			}
		}
		ch <- strings.TrimSpace(string(collected))
	}()

	select {
	case r, ok := <-ch:
		tty.Close()
		if !ok {
			return ""
		}
		return r
	case <-time.After(150 * time.Millisecond):
		tty.Close()
		return ""
	}
}

// IsPuTTY returns true if the terminal is PuTTY or a PuTTY-based client,
// detected via env vars or the ENQ answerback string.
func IsPuTTY() bool {
	// Env var checks — work even without a live query
	if strings.HasPrefix(strings.ToLower(os.Getenv("TERM")), "putty") {
		return true
	}
	if os.Getenv("PUTTY_VERSION") != "" {
		return true
	}
	// ENQ answerback — works through SSH when env vars are not forwarded
	if enq := queryENQ(); strings.Contains(strings.ToLower(enq), "putty") {
		return true
	}
	return false
}

// ApplyPuTTYFixes emits ESC sequences that correct PuTTY's line-drawing
// character set so Unicode box-drawing chars render correctly.
//
//	ESC ( B  — designate G0 as ASCII (prevents VT100 line-drawing mode)
//	ESC ) B  — designate G1 as ASCII
func ApplyPuTTYFixes(out io.Writer) {
	if out == nil {
		out = os.Stdout
	}
	_, _ = io.WriteString(out, "\033(B\033)B")
}
