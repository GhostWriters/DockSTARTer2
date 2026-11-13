package compose

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"
)

func (p *consoleEventProcessor) buildSummaryLine() string {
	svcCount := 0
	for _, svcs := range p.imageServices {
		svcCount += len(svcs)
	}
	imgCount := len(p.imageOrder)
	netCount := len(p.visibleNetworkIDs()) // excludes in-use networks hidden on teardown
	volCount := len(p.volumeIDs)

	layerCount := p.layerCount()

	var parts []string
	if svcCount > 0 {
		parts = append(parts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", svcCount, plural(svcCount, "service", "services")))
	}
	if imgCount > 0 {
		parts = append(parts, fmt.Sprintf("{{|DockerImage|}}%d %s{{[-]}}", imgCount, plural(imgCount, "image", "images")))
	}
	if layerCount > 0 {
		parts = append(parts, fmt.Sprintf("{{[::D]}}%d %s{{[-]}}", layerCount, plural(layerCount, "layer", "layers")))
	}
	if netCount > 0 {
		parts = append(parts, fmt.Sprintf("{{|IPAddress|}}%d %s{{[-]}}", netCount, plural(netCount, "network", "networks")))
	}
	if volCount > 0 {
		parts = append(parts, fmt.Sprintf("{{|Folder|}}%d %s{{[-]}}", volCount, plural(volCount, "volume", "volumes")))
	}
	// Command word is bold yellow (matching the summary line's overall duration); the
	// colon uses DockerColon (matching the colons on the section/name lines below).
	summaryFmt := fmt.Sprintf("{{[yellow::B]}}%s{{[-]}}{{|DockerColon|}}:{{[-]}} %s", p.command, strings.Join(parts, ", "))
	// Prepend an overall spinner/marker (icon + space) so the summary reads as the
	// top-level rollup header, shifting the whole block 3 chars right to align under it.
	return globalIndent + p.overallRollupIcon() + " " + summaryFmt
}

// layerCount returns the number of layer rows across all images: each sha256 DiffID once
// regardless of sharing, plus each download not placed on a layer. A download another
// image placed is already counted by its DiffID.
func (p *consoleEventProcessor) layerCount() int {
	n := len(p.diffIDImageCount)
	seen := make(map[string]bool)
	for _, rawIDs := range p.imagePullOrder {
		for _, rawID := range rawIDs {
			if p.tasks[rawID] != nil && !seen[rawID] && p.diffIDImageCount[rawID] == 0 {
				seen[rawID] = true
				n++
			}
		}
	}
	return n
}

type timerStyle int

const (
	timerSection timerStyle = iota // bold white — services: header
	timerService                   // App cyan — individual service containers
	timerImage                     // DockerMarkerDone green — image rows
	timerLayer                     // unstyled — layer rows
)

type timerEntry struct {
	task  *consoleTask
	style timerStyle
}

// attachTimers right-aligns elapsed timers on lines. extraWidth lets the caller include
// the width of a line not in `lines` (e.g. the summary header) so all timers share one column.
func (p *consoleEventProcessor) attachTimers(lines []string, timers []timerEntry, extraWidth int) []string {
	// Recompute the widest line each render (not grow-only) so the timer column follows
	// the content when the terminal/viewport shrinks on resize.
	p.maxLineWidth = extraWidth
	for _, line := range lines {
		if w := utf8.RuneCountInString(semstyle.ToPlain(line)); w > p.maxLineWidth {
			p.maxLineWidth = w
		}
	}
	timerStrs := make([]string, len(timers))
	maxTimerW := 0
	for i, e := range timers {
		if e.task == nil {
			continue
		}
		s := elapsedStr(e.task)
		timerStrs[i] = s
		if w := len(s); w > maxTimerW {
			maxTimerW = w
		}
	}
	if !p.startTime.IsZero() {
		if w := len(elapsedFromTime(p.startTime)); w > maxTimerW {
			maxTimerW = w
		}
	}
	// Timer width can grow as durations tick up; keep it monotonic so the column doesn't
	// jitter narrower mid-run (a 9.9s→10.1s transition shouldn't shift everything).
	if maxTimerW > p.maxTimerWidth {
		p.maxTimerWidth = maxTimerW
	}
	maxTimerW = p.maxTimerWidth
	col := p.maxLineWidth + timerGutterW
	out := make([]string, len(lines))
	for i, line := range lines {
		e := timers[i]
		if e.task == nil {
			out[i] = line
			continue
		}
		visible := utf8.RuneCountInString(semstyle.ToPlain(line))
		pad := strutil.Repeat(" ", col-visible)
		var styleTag string
		switch e.style {
		case timerSection:
			styleTag = "{{[white::B]}}"
		case timerService:
			styleTag = "{{|App|}}"
		case timerImage:
			styleTag = "{{|DockerMarkerDone|}}"
		default:
			styleTag = "{{[::D]}}"
		}
		s := timerStrs[i]
		s = strutil.Repeat(" ", maxTimerW-len(s)) + s
		out[i] = line + pad + styleTag + s + "{{[-]}}"
	}
	return out
}

func (p *consoleEventProcessor) prependSummary(lines []string, timers []timerEntry) []string {
	summary := p.buildSummaryLine()
	// Indent all content lines under the summary header (which carries the overall
	// icon + "update:" text) so the block nests beneath it. The header itself is not
	// indented. Done before attachTimers so the timer column accounts for the shift.
	headerIndent := strutil.Repeat(" ", summaryHeaderIndentW)
	for i := range lines {
		lines[i] = headerIndent + lines[i]
	}
	// Include the summary header's width so its timer and the row timers share one column
	// (the header is often the widest line for teardown commands with no image/layer rows).
	summaryW := utf8.RuneCountInString(semstyle.ToPlain(summary))
	lines = p.attachTimers(lines, timers, summaryW)
	if vp := console.GlobalViewport; vp != nil && vp.IsActive() && !p.forceSummary {
		return lines
	}
	if summary == "" {
		return lines
	}
	return append([]string{p.withSummaryTimer(summary)}, lines...)
}

// withSummaryTimer appends a right-aligned overall elapsed timer to the summary line.
func (p *consoleEventProcessor) withSummaryTimer(summary string) string {
	if p.startTime.IsZero() {
		return summary
	}
	col := p.maxLineWidth + timerGutterW
	visible := utf8.RuneCountInString(semstyle.ToPlain(summary))
	padW := col - visible
	if padW < timerGutterW {
		padW = timerGutterW // always keep at least the gutter so the timer never glues to text
	}
	pad := strutil.Repeat(" ", padW)
	elapsed := elapsedFromTime(p.startTime)
	if w := len(elapsed); w < p.maxTimerWidth {
		elapsed = strutil.Repeat(" ", p.maxTimerWidth-w) + elapsed
	}
	return summary + pad + "{{[yellow::B]}}" + elapsed + "{{[-]}}"
}

// rowFailure is a failure's name (a service, an image no service uses, a network
// or a volume) and message.
type rowFailure struct {
	name, message string
}

// failures returns every failure that has a message, in display order. A
// service's failures are its own row's, its image's and its image's layers',
// each distinct message once; a shared image's failure is listed under every
// service using it.
func (p *consoleEventProcessor) failures() []rowFailure {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	var out []rowFailure
	failed := func(t *consoleTask) bool {
		return t != nil && t.status == api.Error && t.message != ""
	}
	listed := make(map[string]bool)
	addService := func(svc string, rows ...*consoleTask) {
		if listed[svc] {
			return
		}
		listed[svc] = true
		var msgs []string
		for _, t := range rows {
			if failed(t) && !contains(msgs, t.message) {
				msgs = append(msgs, t.message)
				out = append(out, rowFailure{svc, t.message})
			}
		}
	}
	for _, img := range p.imageOrder {
		svcs := p.serviceIDsForImage(img)
		if len(svcs) == 0 && failed(p.tasks[img]) {
			out = append(out, rowFailure{img, p.tasks[img].message})
		}
		for _, svc := range svcs {
			addService(svc, append([]*consoleTask{p.tasks[svc], p.tasks[img]}, p.layersForImage(img)...)...)
		}
	}
	// An image's pull error has the text "Error", a service status, which also
	// files the image under serviceIDs; it is already listed above.
	for _, svc := range p.serviceIDs {
		if !contains(p.imageOrder, svc) && !looksLikeImageName(svc) {
			addService(svc, p.tasks[svc])
		}
	}
	for _, id := range p.networkIDs {
		if failed(p.tasks[id]) {
			out = append(out, rowFailure{"network " + id, p.tasks[id].message})
		}
	}
	for _, id := range p.volumeIDs {
		if failed(p.tasks[id]) {
			out = append(out, rowFailure{"volume " + id, p.tasks[id].message})
		}
	}
	return out
}

// logSummary writes a structured summary to the log file only.
func (p *consoleEventProcessor) logSummary() {
	if p.logCtx == nil {
		return
	}
	ctx := logger.WithSuppressWriter(p.logCtx, logger.ConsoleWriter())
	if p.noViewport {
		if tuiW := console.GetTUIWriter(p.logCtx); tuiW != nil {
			ctx = logger.WithSuppressWriter(ctx, tuiW)
		}
	}

	const pfx = "\t{{|RunningCommand|}}compose:{{[-]}} "

	// Log uses a fixed width, not the live terminal width, so the persisted output
	// (including proportional bar sizes) is deterministic regardless of terminal size.
	const logWidth = 120

	// maxLineWidth/maxTimerWidth accumulate from the live console render (wider terminal)
	// and only grow. Reset them so the log's timer column aligns to the fixed log width
	// rather than inheriting the console's, which would pad every line out far too wide.
	savedLineW, savedTimerW := p.maxLineWidth, p.maxTimerWidth
	p.maxLineWidth, p.maxTimerWidth = 0, 0
	defer func() { p.maxLineWidth, p.maxTimerWidth = savedLineW, savedTimerW }()

	// Final log always includes layer rows, regardless of the -v console flag.
	// Passes the raw tag string, not pre-rendered ANSI -- logger.Notice defers
	// rendering to its own writer handlers, each scoped to the right session's
	// tint/profile; pre-rendering here would read semstyle's global profile
	// unscoped to any session, racing against every other concurrently
	// rendering one.
	for _, line := range p.buildLines(logWidth, true) {
		logger.Notice(ctx, pfx+line)
	}
}
