package graphics

import (
	"context"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/input"
	"golang.org/x/term"
)

// sixelDA1Param is the Primary Device Attributes parameter DEC terminals use
// to advertise Sixel graphics support.
const sixelDA1Param = 4

// HasSixelSupport reports whether a Primary Device Attributes (DA1) reply
// advertises Sixel graphics support. Shared by QueryProtocol (the
// blocking pre-Program query) and AppModel.Update's handling of the async
// tea.Raw-driven query, so both interpret the same response the same way.
func HasSixelSupport(da1 []int) bool {
	for _, p := range da1 {
		if p == sixelDA1Param {
			return true
		}
	}
	return false
}

// DefaultQueryTimeout is the default bound on how long QueryProtocol
// waits for a Primary Device Attributes reply before assuming the terminal
// doesn't support (or won't answer) the query. A terminal that doesn't
// understand DA1 at all may never respond, so "no response within this
// window" has to mean "no" -- there's no way to distinguish that from "slow
// to respond". 500ms comfortably covers a real terminal's round trip even
// over a slow SSH link; callers on a known-high-latency connection can pass
// a longer value instead.
const DefaultQueryTimeout = 500 * time.Millisecond

// Protocol is the inline graphics protocol a terminal supports.
type Protocol int

const (
	ProtocolNone Protocol = iota
	ProtocolSixel
	ProtocolKitty
)

// daGrace is how long QueryProtocol keeps waiting for the Kitty reply once the
// DA1 reply is in.
const daGrace = 150 * time.Millisecond

type protocolKey struct{}

// WithProtocol attaches an already-resolved terminal protocol to ctx, for a
// command that runs after the terminal was queried but can't query it
// itself (e.g. under -g, where a Bubble Tea program owns the terminal).
func WithProtocol(ctx context.Context, p Protocol) context.Context {
	return context.WithValue(ctx, protocolKey{}, p)
}

// ProtocolFromContext returns the protocol attached via WithProtocol.
func ProtocolFromContext(ctx context.Context) (Protocol, bool) {
	p, ok := ctx.Value(protocolKey{}).(Protocol)
	return p, ok
}

// kittyProbeID tags the Kitty graphics probe so its reply is recognised.
const kittyProbeID = 31

// KittyProbe asks the terminal whether it supports the Kitty graphics
// protocol (a query action, so nothing is drawn). A supporting terminal
// answers with a Kitty graphics reply, see IsKittyProbeReply; others ignore it.
var KittyProbe = ansi.KittyGraphics([]byte("AAAA"), "i="+strconv.Itoa(kittyProbeID), "s=1", "v=1", "a=q", "t=d", "f=24")

// IsKittyProbeReply reports whether a Kitty graphics reply (its image id and
// payload, from either x/input's or ultraviolet's KittyGraphicsEvent) is a
// supporting terminal's reply to KittyProbe.
func IsKittyProbeReply(id int, payload []byte) bool {
	return id == kittyProbeID && strings.HasPrefix(string(payload), "OK")
}

// QueryProtocol queries a real, local terminal for its inline graphics
// protocol, used for the standalone `ds2 --man` CLI path where no Bubble Tea
// Program is running yet to own input. It sends KittyProbe and a Primary
// Device Attributes (DA1) request: a Kitty reply means Kitty graphics,
// otherwise a DA1 reply listing Sixel means Sixel, else none.
//
// Not safe to call while a Bubble Tea Program already owns in/out (e.g.
// `--man` run from the TUI's console panel) -- both would race to read the
// same stream. That path instead reuses the owning session's own
// already-resolved capability (see PanelModel.GraphicsSupported), which comes
// from the DA1 query run once via tea.Raw from AppModel.Init().
func QueryProtocol(inFd int, in io.Reader, out io.Writer, timeout time.Duration) Protocol {
	if !term.IsTerminal(inFd) {
		// Not a real terminal (piped output, redirected input, etc) --
		// nothing to query, and raw mode wouldn't make sense here anyway.
		return ProtocolNone
	}
	state, err := term.MakeRaw(inFd)
	if err != nil {
		return ProtocolNone
	}
	defer func() { _ = term.Restore(inFd, state) }()

	rd, err := input.NewReader(in, "", 0)
	if err != nil {
		return ProtocolNone
	}
	defer func() { _ = rd.Close() }()

	// A terminal's replies are not guaranteed to arrive in request order,
	// so DA1 does not end the wait: after it, the Kitty reply gets daGrace
	// to show up.
	done := make(chan struct{})
	defer close(done)
	gotDA1 := make(chan struct{})
	go func() {
		select {
		case <-done:
			return
		case <-time.After(timeout):
		case <-gotDA1:
			select {
			case <-done:
				return
			case <-time.After(daGrace):
			}
		}
		rd.Cancel()
	}()

	if _, err := io.WriteString(out, KittyProbe+ansi.RequestPrimaryDeviceAttributes); err != nil {
		return ProtocolNone
	}

	var (
		sixel   bool
		haveDA1 bool
	)
	for {
		events, err := rd.ReadEvents()
		if err != nil {
			// Includes the Cancel() case above (timeout or grace elapsed).
			if haveDA1 && sixel {
				return ProtocolSixel
			}
			return ProtocolNone
		}
		for _, e := range events {
			switch ev := e.(type) {
			case input.KittyGraphicsEvent:
				if IsKittyProbeReply(ev.Options.ID, ev.Payload) {
					return ProtocolKitty
				}
			case input.PrimaryDeviceAttributesEvent:
				sixel = HasSixelSupport(ev)
				if !haveDA1 {
					haveDA1 = true
					close(gotDA1)
				}
			}
		}
	}
}
