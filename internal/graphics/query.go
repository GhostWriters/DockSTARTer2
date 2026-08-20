package graphics

import (
	"io"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/input"
	"golang.org/x/term"
)

// sixelDA1Param is the Primary Device Attributes parameter DEC terminals use
// to advertise Sixel graphics support.
const sixelDA1Param = 4

// HasSixelSupport reports whether a Primary Device Attributes (DA1) reply
// advertises Sixel graphics support. Shared by QuerySixelSupport (the
// blocking pre-Program query) and AppModel.Update's handling of the async
// tea.Raw-driven query, so both interpret the same response the same way.
func HasSixelSupport(da1 input.PrimaryDeviceAttributesEvent) bool {
	for _, p := range da1 {
		if p == sixelDA1Param {
			return true
		}
	}
	return false
}

// DefaultQueryTimeout is the default bound on how long QuerySixelSupport
// waits for a Primary Device Attributes reply before assuming the terminal
// doesn't support (or won't answer) the query. A terminal that doesn't
// understand DA1 at all may never respond, so "no response within this
// window" has to mean "no" -- there's no way to distinguish that from "slow
// to respond". 500ms comfortably covers a real terminal's round trip even
// over a slow SSH link; callers on a known-high-latency connection can pass
// a longer value instead.
const DefaultQueryTimeout = 500 * time.Millisecond

// QuerySixelSupport queries a real, local terminal for Sixel graphics
// support via a Primary Device Attributes (DA1) request, used for the
// standalone `ds2 --man` CLI path where no Bubble Tea Program is running
// yet to own input.
//
// Not safe to call while a Bubble Tea Program already owns in/out (e.g.
// `--man` run from the TUI's console panel) -- both would race to read the
// same stream. That path instead reuses the owning session's own
// already-resolved capability (see PanelModel.GraphicsSupported): the same
// DA1 query runs once via tea.Raw from AppModel.Init(), through the
// Program's own input loop rather than a second competing reader, and by
// the time a user types --man into the console panel the reply has long
// since arrived. The interactive TUI's help dialog reads that same resolved
// value directly, for the same reason.
func QuerySixelSupport(inFd int, in io.Reader, out io.Writer, timeout time.Duration) bool {
	if !term.IsTerminal(inFd) {
		// Not a real terminal (piped output, redirected input, etc) --
		// nothing to query, and raw mode wouldn't make sense here anyway.
		return false
	}
	state, err := term.MakeRaw(inFd)
	if err != nil {
		return false
	}
	defer func() { _ = term.Restore(inFd, state) }()

	rd, err := input.NewReader(in, "", 0)
	if err != nil {
		return false
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

	if _, err := io.WriteString(out, ansi.RequestPrimaryDeviceAttributes); err != nil {
		return false
	}

	for {
		events, err := rd.ReadEvents()
		if err != nil {
			// Includes the Cancel() case above (timeout elapsed).
			return false
		}
		for _, e := range events {
			if da1, ok := e.(input.PrimaryDeviceAttributesEvent); ok {
				return HasSixelSupport(da1)
			}
		}
	}
}
