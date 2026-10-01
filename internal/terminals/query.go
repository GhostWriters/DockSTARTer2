package terminals

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/input"
	"golang.org/x/term"
)

// Debugf, when set, receives a line per query and per event read.
var Debugf func(format string, args ...any)

func debugf(format string, args ...any) {
	if Debugf != nil {
		Debugf(format, args...)
	}
}

// DefaultQueryTimeout bounds each blocking query when the terminal doesn't
// answer DA1 either. Terminals answer in order, so a DA1 reply ends the wait
// early whether or not the query before it was answered.
const DefaultQueryTimeout = 500 * time.Millisecond

// HasTrueColorCap reports whether an XTGETTCAP reply advertises direct color
// (RGB or Tc). A reply may carry a value, e.g. "RGB=8/8/8".
func HasTrueColorCap(reply string) bool {
	for _, c := range strings.Split(reply, ";") {
		name, _, _ := strings.Cut(c, "=")
		if name == "RGB" || name == "Tc" {
			return true
		}
	}
	return false
}

// Upgrade returns the higher of base and p, except that a base below ANSI
// (no TTY, NO_COLOR) is returned as is.
func Upgrade(base, p colorprofile.Profile) colorprofile.Profile {
	if base < colorprofile.ANSI || p <= base {
		return base
	}
	return p
}

// DetectProfile queries a real, local terminal and returns base raised by
// what it reports: the terminals.toml entry for its XTVERSION reply, then an
// XTGETTCAP RGB/Tc reply unless that entry already gives truecolor. A
// terminal that ignores XTVERSION is matched by its DA1 reply instead and is
// never sent XTGETTCAP, since some such terminals print it as text. Returns
// base without querying when it is already TrueColor or below ANSI, or inFd
// isn't a terminal.
//
// Not safe while a Bubble Tea Program owns in/out.
func DetectProfile(base colorprofile.Profile, inFd int, in io.Reader, out io.Writer, timeout time.Duration) (colorprofile.Profile, Info) {
	if base < colorprofile.ANSI || base == colorprofile.TrueColor || !term.IsTerminal(inFd) {
		debugf("terminal query skipped: profile %s, stdin terminal %v", base, term.IsTerminal(inFd))
		return base, Info{}
	}
	state, err := term.MakeRaw(inFd)
	if err != nil {
		debugf("terminal query skipped: raw mode: %v", err)
		return base, Info{}
	}
	defer func() { _ = term.Restore(inFd, state) }()

	xtv, da1 := queryUntilDA1(in, out, ansi.RequestNameVersion, timeout, func(e input.Event) (string, bool) {
		v, ok := e.(input.TerminalVersionEvent)
		return string(v), ok
	})
	if xtv == "" {
		info, ok := LookupDA1(da1)
		if !ok {
			return base, Info{}
		}
		return Upgrade(base, info.Profile), info
	}
	info, ok := Lookup(xtv)
	if !ok {
		info = Info{Name: xtv}
	}
	p := Upgrade(base, info.Profile)
	if p == colorprofile.TrueColor {
		return p, info
	}
	// Separate requests: some terminals reject a multi-capability query
	// outright when they don't know one of the names.
	tc, _ := queryUntilDA1(in, out, ansi.RequestTermcap("RGB")+ansi.RequestTermcap("Tc"), timeout, func(e input.Event) (string, bool) {
		c, ok := e.(input.CapabilityEvent)
		return string(c), ok && HasTrueColorCap(string(c))
	})
	if tc != "" {
		return Upgrade(p, colorprofile.TrueColor), info
	}
	return p, info
}

// queryUntilDA1 writes req followed by a DA1 request and returns the first
// reply pick accepts ("" if the DA1 reply or the timeout comes first), plus
// the DA1 reply's parameters joined with ";".
func queryUntilDA1(in io.Reader, out io.Writer, req string, timeout time.Duration, pick func(input.Event) (string, bool)) (got, da1 string) {
	rd, err := input.NewReader(in, "", 0)
	if err != nil {
		return "", ""
	}
	defer func() { _ = rd.Close() }()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
		case <-time.After(timeout):
			rd.Cancel()
		}
	}()

	if _, err := io.WriteString(out, req+ansi.RequestPrimaryDeviceAttributes); err != nil {
		return "", ""
	}

	debugf("terminal query %q", req)
	for {
		events, err := rd.ReadEvents()
		if err != nil {
			debugf("terminal query ended: %v", err)
			return got, ""
		}
		for _, e := range events {
			debugf("terminal query event %T %q", e, fmt.Sprint(e))
			if v, ok := pick(e); ok && got == "" {
				got = v
			}
			if p, ok := e.(input.PrimaryDeviceAttributesEvent); ok {
				return got, joinParams(p)
			}
		}
	}
}

func joinParams(p []int) string {
	s := make([]string, len(p))
	for i, v := range p {
		s[i] = strconv.Itoa(v)
	}
	return strings.Join(s, ";")
}
