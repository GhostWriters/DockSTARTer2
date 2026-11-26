package compose

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"
)

// countLabel returns n of a kind of row (n services) in its kind's colour.
func countLabel(n int, singular, pluralNoun string) string {
	return fmt.Sprintf("{{|%s|}}%d %s{{[-]}}", dockerlayout.KindStyle(pluralNoun), n, plural(n, singular, pluralNoun))
}

func (p *consoleEventProcessor) buildSummaryLine() string {
	if p.containerVerb != "" {
		// docker start, stop or restart acts on containers, whichever service they run.
		n := len(p.containers)
		return globalIndent + p.overallRollupIcon() + " " + fmt.Sprintf("{{[yellow::B]}}%s{{[-]}}{{|DockerColon|}}:{{[-]}} %s",
			p.containerVerb, countLabel(n, "container", "containers"))
	}
	svcCount := 0
	for _, svcs := range p.imageServices {
		svcCount += len(svcs)
	}
	imgCount := len(p.imageOrder)
	// The networks the project's services use, like its services and images, whether
	// or not the run changed them, on the commands that bring services up; the others
	// count those they acted on (a teardown, those it removed, excluding the in-use
	// ones hidden there).
	netCount := len(p.visibleNetworkIDs())
	switch p.command {
	case "up", "update", "create", "run":
		netCount = max(netCount, p.projectNetworks)
	}
	volCount := len(p.volumeIDs)

	layerCount := p.layerCount()

	// A count of a kind of row (services, networks, volumes, models, jobs, orphans) takes
	// the section headings' style; images and layers, lines under the services,
	// go in brackets after them in those lines' styles.
	var imageParts []string
	if imgCount > 0 {
		imageParts = append(imageParts, fmt.Sprintf("{{|DockerImage|}}%d %s{{[-]}}", imgCount, plural(imgCount, "image", "images")))
	}
	if layerCount > 0 {
		imageParts = append(imageParts, fmt.Sprintf("{{[::D]}}%d %s{{[-]}}", layerCount, plural(layerCount, "layer", "layers")))
	}
	var parts []string
	switch {
	case svcCount > 0 && len(imageParts) > 0:
		parts = append(parts, countLabel(svcCount, "service", "services")+" ("+strings.Join(imageParts, ", ")+")")
	case svcCount > 0:
		parts = append(parts, countLabel(svcCount, "service", "services"))
	default:
		parts = append(parts, imageParts...) // images with no service, e.g. a pull of one
	}
	if netCount > 0 {
		parts = append(parts, countLabel(netCount, "network", "networks"))
	}
	if volCount > 0 {
		parts = append(parts, countLabel(volCount, "volume", "volumes"))
	}
	if modelCount := len(p.modelRows()); modelCount > 0 {
		parts = append(parts, countLabel(modelCount, "model", "models"))
	}
	if jobCount := len(p.jobsActedOn()); jobCount > 0 {
		parts = append(parts, countLabel(jobCount, "job", "jobs"))
	}
	if orphanCount := len(p.orphanIDs); orphanCount > 0 {
		parts = append(parts, countLabel(orphanCount, "orphan", "orphans"))
	}
	// The docker command is bold yellow (matching the summary line's overall duration);
	// the colon uses DockerColon (matching the colons on the section/name lines below).
	summaryFmt := fmt.Sprintf("{{[yellow::B]}}%s{{[-]}}{{|DockerColon|}}:{{[-]}} %s", composeHeading(p.command), strings.Join(parts, ", "))
	// Prepend an overall spinner/marker (icon + space) so the summary reads as the
	// top-level rollup header, shifting the whole block 3 chars right to align under it.
	return globalIndent + p.overallRollupIcon() + " " + summaryFmt
}

// composeHeading returns the docker command DS2 ran for command, as the summary
// heading names it: update is up --pull always, the one flag that sets it apart (the
// flags DS2 always passes are in the Running: line above).
func composeHeading(command string) string {
	if command == "update" {
		return "docker compose up --pull always"
	}
	return "docker compose " + command
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
	timerSection timerStyle = iota // bright cyan bold — a kind heading's title style
	timerService                   // App cyan — individual service containers
	timerImage                     // bright magenta bold — the image: title style
	timerLayer                     // unstyled — layer rows
	timerGroup                     // bright green bold — the changed: and unchanged: title style
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
			styleTag = "{{|DockerTitle|}}"
		case timerGroup:
			styleTag = "{{|DockerGroupHeading|}}"
		case timerService:
			styleTag = "{{|App|}}"
		case timerImage:
			styleTag = "{{|DockerTitleImage|}}"
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
	// icon + the docker command) so the block nests beneath it. The header itself is not
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
// or a volume), that name styled as the display shows it, and its message.
type rowFailure struct {
	name, label, message string
}

// failures returns every failure that has a message, in display order. A
// service's failures are its own row's, its image's and its image's layers',
// each distinct message once; a shared image's failure is listed under every
// service using it.
func (p *consoleEventProcessor) failures() []rowFailure {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	return p.rowMessages(func(t *consoleTask) bool { return t.status == api.Error })
}

// interruptedGroup is the rows compose gave up on for one reason: their names styled
// as the display shows them, and the reason.
type interruptedGroup struct {
	labels  []string
	message string
}

// interruptions returns the rows compose gave up on because another row failed,
// grouped by reason in display order.
func (p *consoleEventProcessor) interruptions() []interruptedGroup {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	var out []interruptedGroup
	for _, r := range p.rowMessages(func(t *consoleTask) bool { return t.interrupted }) {
		i := slices.IndexFunc(out, func(g interruptedGroup) bool { return g.message == r.message })
		if i < 0 {
			out = append(out, interruptedGroup{message: r.message})
			i = len(out) - 1
		}
		out[i].labels = append(out[i].labels, r.label)
	}
	return out
}

// styleReason returns an interruption's reason with the container that failed, which
// compose names last ("… of Container azuracast"), styled as its service when it is
// one of the project's.
func (p *consoleEventProcessor) styleReason(reason string) string {
	const kind = "Container "
	i := strings.LastIndex(reason, kind)
	if i < 0 {
		return reason
	}
	name := reason[i+len(kind):]
	if name == "" || strings.ContainsAny(name, " \t") {
		return reason
	}
	p.mtx.Lock()
	svc := p.serviceForContainer(name)
	known := p.isKnownServiceName(svc)
	p.mtx.Unlock()
	if !known {
		return reason
	}
	return reason[:i+len(kind)] + dockerlayout.StyleServiceName(svc)
}

// rowMessages returns the message of every row match accepts, in display order, as
// failures describes. Callers hold p.mtx.
func (p *consoleEventProcessor) rowMessages(match func(*consoleTask) bool) []rowFailure {
	var out []rowFailure
	failed := func(t *consoleTask) bool {
		return t != nil && (t.message != "" || t.summary != "") && match(t)
	}
	summaryOf := func(t *consoleTask) string {
		if t.summary != "" {
			return t.summary
		}
		return t.message
	}
	listed := make(map[string]bool)
	addService := func(svc string, rows ...*consoleTask) {
		if listed[svc] {
			return
		}
		listed[svc] = true
		var msgs []string
		for _, t := range rows {
			if !failed(t) {
				continue
			}
			if msg := shortMessage(t.message); !contains(msgs, msg) {
				msgs = append(msgs, msg)
				out = append(out, rowFailure{svc, dockerlayout.StyleServiceName(svc), msg})
			}
		}
	}
	for _, img := range p.imageOrder {
		svcs := p.serviceIDsForImage(img)
		if len(svcs) == 0 && failed(p.tasks[img]) {
			out = append(out, rowFailure{img, styleImage(img), p.tasks[img].message})
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
			out = append(out, rowFailure{"network " + id, "network {{|IPAddress|}}" + id + "{{[-]}}", p.tasks[id].message})
		}
	}
	for _, id := range p.volumeIDs {
		if failed(p.tasks[id]) {
			out = append(out, rowFailure{"volume " + id, "volume {{|Folder|}}" + id + "{{[-]}}", p.tasks[id].message})
		}
	}
	for _, name := range p.jobIDs {
		if t := p.tasks[jobTaskID(name)]; failed(t) {
			out = append(out, rowFailure{"job " + name, "job " + dockerlayout.StyleServiceName(name), summaryOf(t)})
		}
	}
	for _, name := range p.modelIDs {
		if t := p.tasks[modelTaskID(name)]; failed(t) {
			out = append(out, rowFailure{"model " + name, "model " + dockerlayout.StyleServiceName(name), summaryOf(t)})
		}
	}
	for _, name := range p.orphanIDs {
		if t := p.tasks[orphanTaskID(name)]; failed(t) {
			out = append(out, rowFailure{"orphan " + name, "orphan " + dockerlayout.StyleServiceName(name), summaryOf(t)})
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
	// Final log always includes layer rows, regardless of the -v console flag. The
	// lines are built under p.mtx, as every render is, and written after it's released.
	p.mtx.Lock()
	savedLineW, savedTimerW := p.maxLineWidth, p.maxTimerWidth
	p.maxLineWidth, p.maxTimerWidth = 0, 0
	p.forLog = true
	lines := p.buildLines(logWidth, dockerlayout.VerboseLayers)
	p.forLog = false
	p.maxLineWidth, p.maxTimerWidth = savedLineW, savedTimerW
	p.mtx.Unlock()

	// Passes the raw tag string, not pre-rendered ANSI -- logger.Notice defers
	// rendering to its own writer handlers, each scoped to the right session's
	// tint/profile; pre-rendering here would read semstyle's global profile
	// unscoped to any session, racing against every other concurrently
	// rendering one.
	for _, line := range lines {
		// A blank display line is the prefix alone, with no trailing space.
		if line == "" {
			logger.Notice(ctx, strings.TrimRight(pfx, " "))
			continue
		}
		logger.Notice(ctx, pfx+line)
	}
}
