package compose

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"DockSTARTer2/internal/strutil"

	"github.com/buger/goterm"
	"github.com/docker/compose/v5/pkg/api"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
)

// consoleEventProcessor implements api.EventProcessor with a themed live-updating
// display for terminals. Layout per image group:
//
//	svc1: Status, svc2: Status          ← service header (services sharing this image)
//	  image:tag                  Pulled  ← image row
//	    sha256:abc...  [⣿⣿⡀⠀⠀]  Done    ← layer rows (pull progress)
//
// Event classification:
//   - Tasks with no ParentID and a pull/build status text → image tasks
//   - Tasks with no ParentID and a container lifecycle status text → service tasks
//   - Tasks with a ParentID → layer tasks (children of an image task)
const (
	// Layout primitive widths — change these to adjust the entire layout.
	globalIndentW      = 1  // left margin for all lines
	iconW              = 1  // width of a spinner/status icon character
	spaceW             = 1  // single separator space
	sectionStatusW     = 13 // fixed status column width for service/image/network lines
	layerStatusW       = 11 // max width of any abbreviated layer status ("Downloading")
	sectionChildIndentW = 2  // extra indent for child entries nested under a section header
	imageLabelTextW    = 7  // visible width of "image: " ("image" + ":" + " ")
	sizeColW           = 8  // width of one fixedSize() value
	sizeSepW           = 1  // width of "/" between current/total sizes

	// Derived column positions — computed from primitives above.
	// Nesting depth (from col 0):
	//   col sectionHeaderIndent          → "services:" / "networks:" label
	//   col sectionHeaderIndent+1*child  → service name (autobrr:)
	//   col sectionHeaderIndent+2*child  → "image:" text
	//   col sectionHeaderIndent+3*child  → layer icon
	sectionHeaderIndent = globalIndentW + iconW + spaceW + sectionStatusW      // col where section labels start
	imageLabelW         = 2*sectionChildIndentW + imageLabelTextW              // indent to image: + "image: "
	imageSizesColBase   = globalIndentW + iconW + spaceW + sectionStatusW + imageLabelW
	layerPrefixW        = sectionHeaderIndent + 3*sectionChildIndentW          // spaces before layer icon
	layerSizesColBase   = layerPrefixW + iconW + spaceW
)

// Strings derived from width constants — updated automatically when constants change.
var (
	globalIndent      = strings.Repeat(" ", globalIndentW)
	sectionChildIndent = strings.Repeat(" ", sectionChildIndentW)
	layerPrefix        = strings.Repeat(" ", layerPrefixW)
)

var brailleChars = strings.Split("⠀⡀⣀⣄⣤⣦⣶⣷⣿", "")
var asciiProgressChars = []string{" ", ".", ",", ":", ";", "+", "%", "#", "█"}
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
var asciiSpinnerFrames = []string{"|", "/", "-", "\\"}

type consoleTask struct {
	id        string
	parentID  string
	text      string
	status    api.EventStatus
	current   int64
	total     int64
	percent   int
	startTime time.Time
	endTime   time.Time
}

func (t *consoleTask) completed() bool {
	return t.status == api.Done || t.status == api.Error || t.status == api.Warning
}

type consoleEventProcessor struct {
	out    io.Writer
	ctx    context.Context // set by SDK in Start(); may not have caller's context values
	logCtx context.Context // caller's original context; used for logSummary suppression
	mtx    sync.Mutex
	ticker *time.Ticker
	doneCh chan struct{}

	operation string
	ids       []string // insertion-ordered task IDs
	tasks     map[string]*consoleTask

	// imageIDs: top-level tasks that represent images (pull/build ops)
	imageIDs []string
	// serviceIDs: top-level tasks that represent container lifecycle
	serviceIDs []string
	// networkIDs: top-level tasks that represent network lifecycle (created/removed)
	networkIDs []string
	// volumeIDs: top-level tasks that represent volume lifecycle (created/removed)
	volumeIDs []string

	// command is the ds2 command name (e.g. "up", "update") used for the summary header.
	command string

	// imageServices is pre-populated from the project config: image -> []serviceName
	imageServices map[string][]string
	// imageOrder is the stable insertion order of imageServices keys (map iteration is random).
	imageOrder []string
	// containerToService maps container_name -> service name for services where they differ.
	containerToService map[string]string
	// projectName is used to strip "<project>-" prefix and "-<n>" suffix from container names.
	projectName string
	// unknownContainers maps raw container name -> assigned "<Unknown N>" id.
	unknownContainers map[string]string
	unknownCount      int
	// serviceStartTimes records the earliest wall-clock time work began for a service.
	// Set when the service's image task is first seen, before the container lifecycle starts.
	serviceStartTimes map[string]time.Time

	numLines     int // lines written in last render
	started      bool
	spinnerFrame int
	maxLineWidth int // widest visible line seen so far; grows but never shrinks
	noViewport    bool            // when true, skip GlobalViewport activation (e.g. running inside program box)
	updateFn      func([]string) // called each render tick in noViewport mode instead of writing to out
	lastSentLines []string       // last lines sent via updateFn; skip if unchanged
	asciiMode     bool           // when true, use ASCII spinners, icons, and progress bar chars
	verbose       bool           // when true, show individual layer rows under each image
}

// NewConsoleEventProcessor creates a themed live-updating EventProcessor for TTY output.
// imageServices maps image name (e.g. "lscr.io/linuxserver/plex:latest") to the list of
// service names that use it, so service headers can be shown before lifecycle events arrive.
// imageOrder is the stable key order for imageServices (caller must provide sorted/deterministic order).
func NewConsoleEventProcessor(logCtx context.Context, out io.Writer, command string, imageServices map[string][]string, imageOrder []string, containerToService map[string]string, projectName string, asciiMode bool, verbose bool, updateFn func([]string)) api.EventProcessor {
	return &consoleEventProcessor{
		out:                out,
		logCtx:             logCtx,
		doneCh:             make(chan struct{}, 1),
		tasks:              make(map[string]*consoleTask),
		command:            command,
		imageServices:      imageServices,
		imageOrder:         imageOrder,
		containerToService: containerToService,
		projectName:        projectName,
		unknownContainers:  make(map[string]string),
		serviceStartTimes:  make(map[string]time.Time),
		noViewport:         updateFn != nil,
		updateFn:           updateFn,
		asciiMode:          asciiMode,
		verbose:            verbose,
	}
}

func (p *consoleEventProcessor) Start(ctx context.Context, operation string) {
	p.ctx = ctx
	p.operation = operation

	// Activate the viewport now — enters alt screen pre-filled with recent history.
	if !p.noViewport {
		if vp := console.GlobalViewport; vp != nil {
			vp.Activate()
		}
	}

	// Summary line is built dynamically each render so network count updates as events arrive.
	// Print a placeholder now for the CLI viewport header path.
	if !p.noViewport {
		summaryLine := p.buildSummaryLine()
		if vp := console.GlobalViewport; vp != nil && vp.IsActive() {
			vp.SetHeader(summaryLine)
		} else {
			fmt.Fprintln(p.out, summaryLine)
		}
	}

	// Pre-print placeholder lines so render() can overwrite in-place.
	// Skip when the global viewport is active or running in noViewport mode.
	if !p.noViewport {
		termW := goterm.Width()
		if termW <= 0 {
			termW = 80
		}
		vpActive := func() bool { vp := console.GlobalViewport; return vp != nil && vp.IsActive() }()
		if !vpActive {
			initialLines := p.buildLines(termW)
			if len(initialLines) > 0 {
				for range initialLines {
					fmt.Fprintln(p.out)
				}
				p.numLines = len(initialLines)
				p.started = true
			}
		}
	}

	p.ticker = time.NewTicker(100 * time.Millisecond)
	go func() {
		for {
			select {
			case <-ctx.Done():
				p.ticker.Stop()
				return
			case <-p.doneCh:
				return
			case <-p.ticker.C:
				p.render()
			}
		}
	}()
}

func (p *consoleEventProcessor) Done(_ string, _ bool) {
	// Stop the ticker goroutine first, then do a final render.
	p.doneCh <- struct{}{}
	p.mtx.Lock()
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.mtx.Unlock()
	p.render()

	p.logSummary()

	// Deactivate the viewport — leaves alt screen, dumps history to main screen,
	// returns to normal inline output for anything that runs after compose.
	if !p.noViewport {
		if vp := console.GlobalViewport; vp != nil {
			// Prepend the summary to lastComposeLines so it appears in the scrollback
			// dump. The live header was shown via SetHeader; now bake it into the lines.
			termW := goterm.Width()
			if termW <= 0 {
				termW = 80
			}
			finalLines := append([]string{p.buildSummaryLine()}, p.buildLines(termW)...)
			vp.UpdateLines(finalLines)
			vp.Deactivate()
		}
	}
}

// logSummary writes a structured summary to the log file only.
// The viewport already dumped the final rendered state to the console on Deactivate,
// so we suppress the console handler here to avoid a duplicate printout.
func (p *consoleEventProcessor) logSummary() {
	if p.logCtx == nil {
		return
	}
	// Suppress the console (stderr) handler — the viewport dump already showed the
	// final state. We still want the lines in the log file and TUI panel.
	ctx := logger.WithSuppressWriter(p.logCtx, logger.ConsoleWriter())

	const pfx = "{{|RunningCommand|}}docker compose:{{[-]}} "

	termW := goterm.Width()
	if termW <= 0 {
		termW = 80
	}

	logger.Notice(ctx, pfx+p.buildSummaryLine())
	for _, line := range p.buildLines(termW) {
		logger.Notice(ctx, pfx+line)
	}
}

func (p *consoleEventProcessor) On(events ...api.Resource) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	for _, e := range events {
		if e.ID == api.ResourceCompose {
			continue
		}
		// Skip Starting/Started events for non-start commands, matching SDK TTY behaviour.
		// The last visible state before these (typically Running/Created) is more familiar
		// to users accustomed to "docker compose" output.
		if p.command != "start" && (e.Text == api.StatusStarted || e.Text == api.StatusStarting) {
			continue
		}
		p.upsert(e)
	}
}

func (p *consoleEventProcessor) upsert(e api.Resource) {
	// Normalise SDK-prefixed IDs so they match project service/image names
	isNetwork := strings.HasPrefix(e.ID, "Network ")
	isVolume := strings.HasPrefix(e.ID, "Volume ")
	id := strings.TrimPrefix(e.ID, "Container ")
	id = strings.TrimPrefix(id, "Image ")
	id = strings.TrimPrefix(id, "Network ")
	id = strings.TrimPrefix(id, "Volume ")
	parentID := strings.TrimPrefix(e.ParentID, "Container ")
	parentID = strings.TrimPrefix(parentID, "Image ")
	parentID = strings.TrimPrefix(parentID, "Network ")
	parentID = strings.TrimPrefix(parentID, "Volume ")

	// Remap container_name back to service name (e.g. "qbittorrentx" -> "qbittorrent")
	if svc, ok := p.containerToService[id]; ok {
		id = svc
	}

	// Container names from down/stop/restart use "<project>-<service>-<n>" format.
	// Strip the project prefix and instance suffix to recover the service name.
	if p.projectName != "" {
		prefix := p.projectName + "-"
		if strings.HasPrefix(id, prefix) {
			trimmed := strings.TrimPrefix(id, prefix)
			// Strip trailing "-<digits>" instance suffix
			if idx := strings.LastIndex(trimmed, "-"); idx >= 0 {
				if _, err := fmt.Sscanf(trimmed[idx+1:], "%d", new(int)); err == nil {
					trimmed = trimmed[:idx]
				}
			}
			// Only use trimmed form if it matches a known service (avoid false matches)
			if _, ok := p.containerToService[trimmed]; ok || p.isKnownServiceName(trimmed) {
				id = trimmed
			}
		}
	}

	// If this is a top-level container event that still doesn't match any known
	// service or image, assign it a stable numbered "<Unknown N>" id so it's
	// visible rather than silently dropped. Each distinct raw name gets its own slot.
	if !isNetwork && parentID == "" && isServiceStatus(e.Text) && !p.isKnownServiceName(id) && !contains(p.imageIDs, id) {
		if assigned, ok := p.unknownContainers[id]; ok {
			id = assigned
		} else {
			p.unknownCount++
			assigned = fmt.Sprintf("<Unknown %d>", p.unknownCount)
			p.unknownContainers[id] = assigned
			id = assigned
		}
	}

	t, exists := p.tasks[id]
	if !exists {
		t = &consoleTask{
			id:        id,
			parentID:  parentID,
			startTime: time.Now(),
		}
		p.tasks[id] = t
		p.ids = append(p.ids, id)

		if e.ParentID == "" {
			if isNetwork {
				p.networkIDs = append(p.networkIDs, id)
			} else if isVolume {
				p.volumeIDs = append(p.volumeIDs, id)
			} else if isServiceStatus(e.Text) && !looksLikeImageName(id) {
				p.serviceIDs = append(p.serviceIDs, id)
				// If we haven't stamped a start time yet (image may not have been seen), record now.
				if _, ok := p.serviceStartTimes[id]; !ok {
					p.serviceStartTimes[id] = t.startTime
				}
			} else {
				p.imageIDs = append(p.imageIDs, id)
				// Stamp start time for every service that uses this image.
				for _, svc := range p.imageServices[id] {
					if _, ok := p.serviceStartTimes[svc]; !ok {
						p.serviceStartTimes[svc] = t.startTime
					}
				}
			}
		}
		// layer tasks (ParentID != "") are rendered as children — no separate list needed
	}

	t.text = e.Text
	t.status = e.Status
	if e.Total > t.total {
		t.total = e.Total
	}
	if e.Current > t.current {
		t.current = e.Current
	}
	if e.Percent > t.percent {
		t.percent = e.Percent
	}
	if t.completed() && t.endTime.IsZero() {
		t.endTime = time.Now()
	}

	// Reclassify: if a top-level task initially looked like an image but later
	// receives a service lifecycle status, move it to serviceIDs.
	if !isNetwork && e.ParentID == "" && isServiceStatus(e.Text) && contains(p.imageIDs, id) {
		p.imageIDs = remove(p.imageIDs, id)
		if !contains(p.serviceIDs, id) {
			p.serviceIDs = append(p.serviceIDs, id)
		}
	}
}

func (p *consoleEventProcessor) render() {
	termW := goterm.Width()
	termH := goterm.Height()
	if termW <= 0 {
		termW = 80
	}
	if termH <= 0 {
		termH = 24
	}

	p.mtx.Lock()
	defer p.mtx.Unlock()

	if p.asciiMode {
		p.spinnerFrame = (p.spinnerFrame + 1) % len(asciiSpinnerFrames)
	} else {
		p.spinnerFrame = (p.spinnerFrame + 1) % len(spinnerFrames)
	}
	lines := p.buildLines(termW)
	if len(lines) == 0 {
		return
	}

	// When the global viewport is present, delegate rendering to it.
	// If it exists but isn't active yet (Activate() is called from Start() which
	// fires just before the first tick), skip this render and wait for next tick.
	if !p.noViewport {
		if vp := console.GlobalViewport; vp != nil {
			if vp.IsActive() {
				vp.SetHeader(p.buildSummaryLine())
				vp.UpdateLines(lines)
				p.started = true
				p.numLines = len(lines)
			}
			return
		}
	}

	// noViewport (program box) mode: call updateFn to replace lines in the TUI viewport.
	if p.noViewport {
		if p.updateFn != nil && !slices.Equal(lines, p.lastSentLines) {
			p.updateFn(lines)
			p.lastSentLines = lines
		}
		return
	}

	if !p.started {
		// First render: print blank placeholder lines so subsequent renders
		// have lines to move the cursor up into.
		for range lines {
			fmt.Fprintln(p.out)
		}
		p.numLines = len(lines)
		p.started = true
	}

	// Build the entire redraw as a single buffer to avoid partial flushes
	// between cursor movement and content (which causes visible tearing).
	var buf strings.Builder

	// Hide cursor, move up to start of previously rendered block, go to column 0
	buf.WriteString("\033[?25l") // hide cursor
	if p.numLines > 0 {
		fmt.Fprintf(&buf, "\033[%dA", p.numLines) // cursor up N lines
	}
	buf.WriteString("\r") // column 0

	rendered := 0
	for _, line := range lines {
		buf.WriteString(padOrTrunc(line, termW))
		buf.WriteString("\033[K\n") // erase to end of line, then newline
		rendered++
	}
	// Blank out leftover lines from a previous larger render
	for i := rendered; i < p.numLines; i++ {
		buf.WriteString(strings.Repeat(" ", termW))
		buf.WriteString("\033[K\n")
		rendered++
	}

	buf.WriteString("\033[?25h") // show cursor
	fmt.Fprint(p.out, buf.String())
	p.numLines = len(lines)
	_ = termH
}

// buildLines constructs the full set of display lines grouped by image.
// Layout is driven entirely by imageOrder so line count is stable across renders:
// every known image always emits a service-header + image-row + any layer rows.
// buildTeardownLines renders a flat service list for commands that don't pull images
// (down, stop, kill, pause, unpause, restart). No image column is shown.
// buildSummaryLine constructs the summary header line dynamically so network count
// updates as events arrive (networks are not known from config ahead of time).
func (p *consoleEventProcessor) buildSummaryLine() string {
	svcCount := 0
	for _, svcs := range p.imageServices {
		svcCount += len(svcs)
	}
	imgCount := len(p.imageOrder)
	netCount := len(p.networkIDs)
	volCount := len(p.volumeIDs)

	// Count layer tasks (children of image tasks).
	isImageID := make(map[string]bool, len(p.imageIDs))
	for _, id := range p.imageIDs {
		isImageID[id] = true
	}
	layerCount := 0
	for _, id := range p.ids {
		if t := p.tasks[id]; t != nil && t.parentID != "" && isImageID[t.parentID] {
			layerCount++
		}
	}

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
		parts = append(parts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", netCount, plural(netCount, "network", "networks")))
	}
	if volCount > 0 {
		parts = append(parts, fmt.Sprintf("{{|App|}}%d %s{{[-]}}", volCount, plural(volCount, "volume", "volumes")))
	}
	summaryFmt := fmt.Sprintf("{{|RunningCommand|}}%s:{{[-]}} %s", p.command, strings.Join(parts, ", "))
	return console.ToConsoleANSI(summaryFmt)
}

type timerStyle int

const (
	timerSection timerStyle = iota // bold white — services: header
	timerService                   // App cyan — individual service containers
	timerImage                     // DockerSuccess green — image rows
	timerLayer                     // unstyled — layer rows
)

type timerEntry struct {
	task  *consoleTask
	style timerStyle
}

// attachTimers right-aligns elapsed timers on lines.
// timers is parallel to lines: non-nil task entries get a timer appended.
// The column is set by the widest visible line seen so far (maxLineWidth grows, never shrinks).
func (p *consoleEventProcessor) attachTimers(lines []string, timers []timerEntry) []string {
	// Update maxLineWidth if any line is wider.
	for _, line := range lines {
		if w := utf8.RuneCountInString(console.Strip(line)); w > p.maxLineWidth {
			p.maxLineWidth = w
		}
	}
	col := p.maxLineWidth + 2
	out := make([]string, len(lines))
	for i, line := range lines {
		e := timers[i]
		if e.task == nil {
			out[i] = line
			continue
		}
		visible := utf8.RuneCountInString(console.Strip(line))
		pad := strings.Repeat(" ", col-visible)
		var styleTag string
		switch e.style {
		case timerSection:
			styleTag = "{{[white::B]}}"
		case timerService:
			styleTag = "{{|App|}}"
		case timerImage:
			styleTag = "{{|DockerSuccess|}}"
		default: // timerLayer
			styleTag = "{{[::D]}}"
		}
		timer := console.ToConsoleANSI(styleTag + elapsedStr(e.task) + "{{[-]}}")
		out[i] = line + pad + timer
	}
	return out
}

func (p *consoleEventProcessor) prependSummary(lines []string, timers []timerEntry) []string {
	lines = p.attachTimers(lines, timers)
	// When the viewport is active the summary line is shown as the header — don't
	// also prepend it to the scrollable content or it appears twice.
	if vp := console.GlobalViewport; vp != nil && vp.IsActive() {
		return lines
	}
	summary := p.buildSummaryLine()
	if summary == "" {
		return lines
	}
	return append([]string{summary}, lines...)
}

func (p *consoleEventProcessor) buildTeardownLines() []string {
	impliedText, impliedTag := p.impliedStatus()
	impliedANSI := console.ToConsoleANSI(impliedTag + impliedText + "{{[-]}}")
	ic := p.icons()
	var impliedIcon string
	if impliedTag == "{{|DockerPending|}}" {
		impliedIcon = console.ToConsoleANSI("{{|DockerPending|}}" + ic.pending + "{{[-]}}")
	} else {
		impliedIcon = console.ToConsoleANSI("{{|DockerSuccess|}}" + ic.done + "{{[-]}}")
	}

	svcRollupIDs := append([]string{}, p.serviceIDs...)
	svcImageMap := make(map[string]string)
	for _, imgName := range p.imageOrder {
		for _, s := range p.serviceIDsForImage(imgName) {
			svcImageMap[s] = imgName
			if !contains(svcRollupIDs, s) {
				svcRollupIDs = append(svcRollupIDs, s)
			}
		}
	}
	svcIcon, svcStatusANSI, svcStatusText, _ := p.sectionRollupWithPropagation(svcRollupIDs, func(id string) string { return svcImageMap[id] })
	svcStatusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(svcStatusText))
	coveredSvcs := make(map[string]bool)
	var lines []string
	var timers []timerEntry

	appendLine := func(line string, t *consoleTask, style timerStyle) {
		lines = append(lines, line)
		timers = append(timers, timerEntry{task: t, style: style})
	}

	appendLine(globalIndent+svcIcon+" "+svcStatusANSI+console.CodeReset+svcStatusPad+console.ToConsoleANSI("{{[white::B]}}services{{[-]}}{{|DockerColon|}}:{{[-]}}"), p.sectionTaskFor(svcRollupIDs), timerSection)

	for _, imgName := range p.imageOrder {
		svcs := p.serviceIDsForImage(imgName)
		for _, s := range svcs {
			coveredSvcs[s] = true
		}
		for _, svc := range svcs {
			t := p.tasks[svc]
			nameANSI := console.ToConsoleANSI("{{|App|}}" + svc + "{{[-]}}")
			var svcStatus, svcIcon, svcStatusText string
			if t != nil {
				svcStatusText = abbreviateStatus(t.text)
				svcStatus = console.ToConsoleANSI(serviceStatusTag(t.status, t.text, p.command))
				svcIcon = p.propagatedIcon(t, p.worstServiceStatus(svc, imgName))
			} else {
				svcStatusText = impliedText
				svcStatus = impliedANSI
				svcIcon = impliedIcon
			}
			statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(svcStatusText))
			appendLine(globalIndent+svcIcon+" "+svcStatus+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"), t, timerService)
		}
	}

	// Orphan services with no image group.
	for _, svc := range p.serviceIDs {
		if coveredSvcs[svc] || looksLikeImageName(svc) || contains(p.imageIDs, svc) {
			continue
		}
		t := p.tasks[svc]
		nameANSI := console.ToConsoleANSI("{{|App|}}" + svc + "{{[-]}}")
		var svcStatus, svcIcon, svcStatusText string
		if t != nil {
			svcStatusText = abbreviateStatus(t.text)
			svcStatus = console.ToConsoleANSI(serviceStatusTag(t.status, t.text, p.command))
			svcIcon = p.propagatedIcon(t, t.status)
		} else {
			svcStatusText = impliedText
			svcStatus = impliedANSI
			svcIcon = impliedIcon
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(svcStatusText))
		appendLine(globalIndent+svcIcon+" "+svcStatus+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"), t, timerService)
	}

	netLines, netTimers := p.buildNetworkLines()
	for i, line := range netLines {
		appendLine(line, netTimers[i].task, netTimers[i].style)
	}
	volLines, volTimers := p.buildVolumeLines()
	for i, line := range volLines {
		appendLine(line, volTimers[i].task, volTimers[i].style)
	}

	return p.prependSummary(lines, timers)
}

// isTeardownCommand returns true for commands that operate on containers without pulling images.
func (p *consoleEventProcessor) isTeardownCommand() bool {
	switch p.command {
	case "down", "stop", "kill", "pause", "unpause", "restart":
		return true
	}
	return false
}

func (p *consoleEventProcessor) buildLines(termW int) []string {
	if p.isTeardownCommand() {
		return p.buildTeardownLines()
	}

	svcRollupIDs := append([]string{}, p.serviceIDs...)
	svcImageMap := make(map[string]string)
	for _, imgName := range p.imageOrder {
		for _, s := range p.serviceIDsForImage(imgName) {
			svcImageMap[s] = imgName
			if !contains(svcRollupIDs, s) {
				svcRollupIDs = append(svcRollupIDs, s)
			}
		}
	}
	svcIcon, svcStatusANSI, svcStatusText, _ := p.sectionRollupWithPropagation(svcRollupIDs, func(id string) string { return svcImageMap[id] })
	svcStatusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(svcStatusText))
	var lines []string
	var timers []timerEntry

	appendLine := func(line string, t *consoleTask, style timerStyle) {
		lines = append(lines, line)
		timers = append(timers, timerEntry{task: t, style: style})
	}

	// ── services: section ──────────────────────────────────────────────────
	// Section timer spans all service start times (which are stamped when image tasks first appear).
	allSvcIDs := append(append([]string{}, p.imageOrder...), svcRollupIDs...)
	appendLine(globalIndent+svcIcon+" "+svcStatusANSI+console.CodeReset+svcStatusPad+console.ToConsoleANSI("{{[white::B]}}services{{[-]}}{{|DockerColon|}}:{{[-]}}"), p.sectionTaskFor(allSvcIDs), timerSection)

	// Pre-pass: find the widest visible image name so sizes+bar align across all image rows.
	maxImgNameW := 0
	for _, imgName := range p.imageOrder {
		if w := utf8.RuneCountInString(console.Strip(styleImage(imgName))); w > maxImgNameW {
			maxImgNameW = w
		}
	}

	coveredSvcs := make(map[string]bool)

	for _, imgName := range p.imageOrder {
		svcs := p.serviceIDsForImage(imgName)
		for _, s := range svcs {
			coveredSvcs[s] = true
		}
		img := p.tasks[imgName]

		for _, svc := range svcs {
			t := p.tasks[svc]
			nameANSI := console.ToConsoleANSI("{{|App|}}" + svc + "{{[-]}}")
			var statusText, statusANSI, icon string
			if t == nil {
				if img != nil {
					statusText = abbreviateStatus(img.text)
					statusANSI = console.ToConsoleANSI(imageStatusTag(img.status, img.text))
					icon = p.spinnerIcon(img)
				} else {
					impliedText, impliedTag := p.impliedStatus()
					statusText = impliedText
					statusANSI = console.ToConsoleANSI(impliedTag + statusText + "{{[-]}}")
					if impliedTag == "{{|DockerPending|}}" {
						icon = console.ToConsoleANSI("{{|DockerPending|}}" + p.icons().pending + "{{[-]}}")
					} else {
						icon = console.ToConsoleANSI("{{|DockerSuccess|}}" + p.icons().done + "{{[-]}}")
					}
				}
			} else {
				statusText = abbreviateStatus(t.text)
				statusANSI = console.ToConsoleANSI(serviceStatusTag(t.status, t.text, p.command))
				icon = p.propagatedIcon(t, p.worstServiceStatus(svc, imgName))
			}
			statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
			appendLine(globalIndent+icon+" "+statusANSI+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"), p.serviceTimerTask(svc), timerService)
		}

		// Collect layers for this image.
		var layers []*consoleTask
		for _, id := range p.ids {
			t := p.tasks[id]
			if t.parentID == imgName {
				layers = append(layers, t)
			}
		}

		// Image row with per-layer progress bar.
		appendLine(p.buildImageLine(imgName, img, layers, maxImgNameW, termW), img, timerImage)

		// Layer rows only in verbose mode.
		if p.verbose {
			layerLines, layerTasks := p.buildLayerLines(layers, maxImgNameW, termW)
			for i, line := range layerLines {
				appendLine(line, layerTasks[i], timerLayer)
			}
		}
	}

	// Orphan services with no image group.
	for _, svcID := range p.serviceIDs {
		if coveredSvcs[svcID] || looksLikeImageName(svcID) || contains(p.imageIDs, svcID) {
			continue
		}
		t := p.tasks[svcID]
		nameANSI := console.ToConsoleANSI("{{|App|}}" + svcID + "{{[-]}}")
		var statusText, statusANSI, icon string
		if t == nil {
			impliedText, impliedTag := p.impliedStatus()
			statusText = impliedText
			statusANSI = console.ToConsoleANSI(impliedTag + statusText + "{{[-]}}")
			if impliedTag == "{{|DockerPending|}}" {
				icon = console.ToConsoleANSI("{{|DockerPending|}}·{{[-]}}")
			} else {
				icon = console.ToConsoleANSI("{{|DockerSuccess|}}✓{{[-]}}")
			}
		} else {
			statusText = abbreviateStatus(t.text)
			statusANSI = console.ToConsoleANSI(serviceStatusTag(t.status, t.text, p.command))
			icon = p.propagatedIcon(t, t.status)
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		appendLine(globalIndent+icon+" "+statusANSI+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"), p.serviceTimerTask(svcID), timerService)
	}

	// ── networks/volumes: sections ────────────────────────────────────────
	netLines, netTimers := p.buildNetworkLines()
	for i, line := range netLines {
		appendLine(line, netTimers[i].task, netTimers[i].style)
	}
	volLines, volTimers := p.buildVolumeLines()
	for i, line := range volLines {
		appendLine(line, volTimers[i].task, volTimers[i].style)
	}

	return p.prependSummary(lines, timers)
}

// buildImageLine renders the image row with icon/status, indented to align under the service name.
// Layout: " icon Status      image: name:tag   2.5s"
// statusPad brings the cursor to the name column — no additional indent needed.
func (p *consoleEventProcessor) buildImageLine(imgName string, t *consoleTask, layers []*consoleTask, maxImgNameW int, termW int) string {
	imageLabel := strings.Repeat(" ", 2*sectionChildIndentW) + console.ToConsoleANSI("{{|DockerSuccess|}}image{{[-]}}{{|DockerColon|}}:{{[-]}} ")
	imgStr := styleImage(imgName)
	imgNameW := utf8.RuneCountInString(console.Strip(imgStr))
	imgPad := strutil.Repeat(" ", maxImgNameW-imgNameW)
	sizes, bar := p.buildImageSizesAndBar(layers, maxImgNameW, termW)
	if t == nil {
		cachedIcon := console.ToConsoleANSI("{{|DockerSuccess|}}" + p.icons().done + "{{[-]}}")
		cachedStatus := console.ToConsoleANSI("{{|DockerSuccess|}}Cached{{[-]}}")
		statusPad := strutil.Repeat(" ", sectionStatusW-len("Cached"))
		return globalIndent + cachedIcon + " " + cachedStatus + console.CodeReset + statusPad + imageLabel + imgStr + imgPad + sizes + bar
	}
	worst := p.worstImageStatus(imgName)
	icon := p.propagatedIcon(t, worst)
	statusText := abbreviateStatus(t.text)
	statusANSI := console.ToConsoleANSI(imageStatusTag(t.status, t.text))
	statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
	return globalIndent + icon + " " + statusANSI + console.CodeReset + statusPad + imageLabel + imgStr + imgPad + sizes + bar
}

// buildImageSizesAndBar computes the aggregate size column and per-layer progress bar for an image row.
// maxImgNameW is used to calculate remaining terminal space for the bar.
// Returns (sizes, bar) — both may be empty strings.
func (p *consoleEventProcessor) buildImageSizesAndBar(layers []*consoleTask, maxImgNameW int, termW int) (string, string) {
	if len(layers) == 0 {
		return "", ""
	}

	// Aggregate current/total across layers.
	var current, total int64
	for _, t := range layers {
		c := t.current
		if t.completed() && t.percent == 100 && t.total > 0 {
			c = t.total
		}
		current += c
		total += t.total
	}

	var sizes string
	if total > 0 {
		sizes = " " + console.ToConsoleANSI("{{|DockerSuccess|}}"+fixedSize(current)+"{{[-]}}"+
			"{{|DockerColon|}}/{{[-]}}"+
			"{{|DockerSuccess|}}"+fixedSize(total)+"{{[-]}}")
	} else {
		sizes = strings.Repeat(" ", 1+8+1+8)
	}

	// Fixed visible width before the bar: imageSizesColBase + maxImgNameW + sizes(space+sizeColW+sizeSepW+sizeColW)
	usedW := imageSizesColBase + maxImgNameW + spaceW + sizeColW + sizeSepW + sizeColW
	barW := len(layers)
	maxBarW := termW - usedW - 3 // 3 = space + "[" + "]"
	if maxBarW < 1 {
		return sizes, ""
	}
	if barW > maxBarW {
		barW = maxBarW
	}

	layerPcts := make([]int, barW)
	for i := range barW {
		t := layers[i]
		pct := t.percent
		if t.completed() && pct == 100 {
			pct = 100
		} else if t.total > 0 {
			pct = int(float64(t.current) / float64(t.total) * 100)
		}
		if pct > 100 {
			pct = 100
		}
		layerPcts[i] = pct
	}

	progressChars := brailleChars
	if p.asciiMode {
		progressChars = asciiProgressChars
	}
	return sizes, " " + renderProgressBarLayers(layerPcts, progressChars, "{{|DockerSuccess|}}")
}

// worstChildStatus returns the worst EventStatus among all layer children of parentID.
// Order: Error > Warning > Working > Done (i.e. any error beats everything).
func (p *consoleEventProcessor) worstChildStatus(parentID string) api.EventStatus {
	worst := api.Done
	for _, id := range p.ids {
		t := p.tasks[id]
		if t.parentID != parentID {
			continue
		}
		switch t.status {
		case api.Error:
			return api.Error // can't get worse
		case api.Warning:
			worst = api.Warning
		case api.Working:
			if worst != api.Warning {
				worst = api.Working
			}
		}
	}
	return worst
}

// worstImageStatus returns the worst status for an image, propagating from its layers.
func (p *consoleEventProcessor) worstImageStatus(imgName string) api.EventStatus {
	img := p.tasks[imgName]
	layerWorst := p.worstChildStatus(imgName)
	if img == nil {
		return layerWorst
	}
	if layerWorst == api.Error || img.status == api.Error {
		return api.Error
	}
	if layerWorst == api.Warning || img.status == api.Warning {
		return api.Warning
	}
	return img.status
}

// worstServiceStatus returns the worst status for a service, propagating from its image.
func (p *consoleEventProcessor) worstServiceStatus(svcID, imgName string) api.EventStatus {
	svc := p.tasks[svcID]
	imgWorst := p.worstImageStatus(imgName)
	if svc == nil {
		return imgWorst
	}
	if imgWorst == api.Error || svc.status == api.Error {
		return api.Error
	}
	if imgWorst == api.Warning || svc.status == api.Warning {
		return api.Warning
	}
	return svc.status
}

// propagatedIcon returns the icon for a task after considering propagated child errors.
// worstStatus should come from worstImageStatus or worstServiceStatus as appropriate.
func (p *consoleEventProcessor) propagatedIcon(t *consoleTask, worstStatus api.EventStatus) string {
	ic := p.icons()
	if worstStatus == api.Error {
		return console.ToConsoleANSI("{{|DockerFail|}}" + ic.error + "{{[-]}}")
	}
	if worstStatus == api.Warning {
		return console.ToConsoleANSI("{{|DockerWarn|}}" + ic.warn + "{{[-]}}")
	}
	return p.spinnerIcon(t)
}

// sectionRollupState is the computed rollup state for a section header.
type sectionRollupState int

const (
	rollupPending    sectionRollupState = iota
	rollupProcessing                    // at least one in progress
	rollupComplete                      // all done, no errors/warnings
	rollupWarning                       // at least one warning
	rollupError                         // at least one error
)

// sectionStatusText returns the status label and ANSI tag for a rollup state.
func sectionStatusText(s sectionRollupState) (text, statusTag, iconTag string) {
	switch s {
	case rollupError:
		return "Error", "{{|DockerFail|}}", "{{|DockerFail|}}"
	case rollupWarning:
		return "Warning", "{{|DockerWarn|}}", "{{|DockerWarn|}}"
	case rollupPending:
		return "Pending", "{{|DockerPending|}}", "{{|DockerPending|}}"
	case rollupComplete:
		return "Complete", "{{|DockerFinal|}}", "{{|DockerSuccess|}}"
	default: // rollupProcessing
		return "Processing", "{{|DockerActive|}}", "{{|DockerSpinner|}}"
	}
}

// sectionRollupWithPropagation is like sectionRollup but also checks child tasks
// (layers for image IDs, images for service IDs) for propagated errors/warnings.
func (p *consoleEventProcessor) sectionRollupWithPropagation(ids []string, imageForID func(string) string) (icon, statusANSI, statusText, labelTag string) {
	state := p.rollupState(ids, imageForID)
	text, stTag, iconTag := sectionStatusText(state)
	ic := p.icons()
	spinnerOrCheck := ic.spinner
	if state != rollupProcessing {
		spinnerOrCheck = p.sectionRollupIcon(state)
	}
	icon = console.ToConsoleANSI(iconTag + spinnerOrCheck + "{{[-]}}")
	statusANSI = console.ToConsoleANSI(stTag + text + "{{[-]}}")
	statusText = text
	labelTag = stTag
	return
}

func (p *consoleEventProcessor) sectionRollupIcon(s sectionRollupState) string {
	ic := p.icons()
	switch s {
	case rollupError:
		return ic.error
	case rollupWarning:
		return ic.warn
	case rollupComplete:
		return ic.done
	default:
		return ic.pending
	}
}

// rollupState computes the sectionRollupState for a set of IDs, optionally propagating
// through child tasks when imageForID is non-nil.
func (p *consoleEventProcessor) rollupState(ids []string, imageForID func(string) string) sectionRollupState {
	anyError := false
	anyWarning := false
	anyStarted := false
	allDone := true
	for _, id := range ids {
		t := p.tasks[id]
		var worst api.EventStatus
		if imageForID != nil {
			imgName := imageForID(id)
			if imgName != "" {
				worst = p.worstServiceStatus(id, imgName)
			} else {
				worst = p.worstImageStatus(id)
			}
		} else {
			worst = p.worstImageStatus(id)
		}
		if t != nil {
			anyStarted = true
			if !t.completed() {
				allDone = false
			}
		} else if p.isTeardownCommand() {
			// For teardown: services with no task are already in the implied final
			// state (container was already stopped/removed before we ran). Count as
			// started and done so the section shows Complete, not Pending.
			anyStarted = true
		} else {
			allDone = false
			// For startup: if the image is being worked on, the section has started
			// even though the service task doesn't exist yet (container not created).
			if imageForID != nil {
				imgName := imageForID(id)
				if imgName != "" && p.tasks[imgName] != nil {
					anyStarted = true
				}
			}
		}
		switch worst {
		case api.Error:
			anyError = true
		case api.Warning:
			anyWarning = true
		}
	}
	switch {
	case anyError:
		return rollupError
	case anyWarning:
		return rollupWarning
	case !anyStarted:
		return rollupPending
	case allDone:
		return rollupComplete
	default:
		return rollupProcessing
	}
}

// buildNetworkLines renders the networks: section header and one line per network event.
func (p *consoleEventProcessor) buildNetworkLines() ([]string, []timerEntry) {
	if len(p.networkIDs) == 0 {
		return nil, nil
	}
	netIcon, netStatusANSI, netStatusText, _ := p.sectionRollupWithPropagation(p.networkIDs, nil)
	netStatusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(netStatusText))
	lines := []string{globalIndent + netIcon + " " + netStatusANSI + console.CodeReset + netStatusPad + console.ToConsoleANSI("{{[white::B]}}networks{{[-]}}{{|DockerColon|}}:{{[-]}}")}
	timers := []timerEntry{{task: p.sectionTaskFor(p.networkIDs), style: timerSection}}
	for _, netID := range p.networkIDs {
		t := p.tasks[netID]
		nameANSI := console.ToConsoleANSI("{{|App|}}" + netID + "{{[-]}}")
		var icon, statusText, statusANSI string
		if t != nil {
			icon = p.spinnerIcon(t)
			statusText = abbreviateStatus(t.text)
			statusANSI = console.ToConsoleANSI(networkStatusTag(t.status, t.text, p.command))
		} else {
			icon = console.ToConsoleANSI("{{|DockerPending|}}·{{[-]}}")
			statusText = "Pending"
			statusANSI = console.ToConsoleANSI("{{|DockerPending|}}Pending{{[-]}}")
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		lines = append(lines, globalIndent+icon+" "+statusANSI+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"))
		timers = append(timers, timerEntry{task: t, style: timerService})
	}
	return lines, timers
}

// buildVolumeLines renders the volumes: section header and one line per volume event.
func (p *consoleEventProcessor) buildVolumeLines() ([]string, []timerEntry) {
	if len(p.volumeIDs) == 0 {
		return nil, nil
	}
	volIcon, volStatusANSI, volStatusText, _ := p.sectionRollupWithPropagation(p.volumeIDs, nil)
	volStatusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(volStatusText))
	lines := []string{globalIndent + volIcon + " " + volStatusANSI + console.CodeReset + volStatusPad + console.ToConsoleANSI("{{[white::B]}}volumes{{[-]}}{{|DockerColon|}}:{{[-]}}")}
	timers := []timerEntry{{task: p.sectionTaskFor(p.volumeIDs), style: timerSection}}
	for _, volID := range p.volumeIDs {
		t := p.tasks[volID]
		nameANSI := console.ToConsoleANSI("{{|App|}}" + volID + "{{[-]}}")
		var icon, statusText, statusANSI string
		if t != nil {
			icon = p.spinnerIcon(t)
			statusText = abbreviateStatus(t.text)
			statusANSI = console.ToConsoleANSI(volumeStatusTag(t.status, t.text, p.command))
		} else {
			icon = console.ToConsoleANSI("{{|DockerPending|}}·{{[-]}}")
			statusText = "Pending"
			statusANSI = console.ToConsoleANSI("{{|DockerPending|}}Pending{{[-]}}")
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		lines = append(lines, globalIndent+icon+" "+statusANSI+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"))
		timers = append(timers, timerEntry{task: t, style: timerService})
	}
	return lines, timers
}

// serviceIDsForImage returns the service list for an image, merging pre-populated
// config data with any service task IDs received from the SDK at runtime.
func (p *consoleEventProcessor) serviceIDsForImage(imgID string) []string {
	if svcs, ok := p.imageServices[imgID]; ok {
		return svcs
	}
	// Fallback to runtime-matched service tasks
	var result []string
	for _, svcID := range p.serviceIDs {
		if imageMatchesService(imgID, svcID) {
			result = append(result, svcID)
		}
	}
	sort.Strings(result)
	return result
}

// styleImage returns the image name with DockerImage/DockerTag styling applied,
// splitting at the last ":" to separate registry/path from the tag portion.
func styleImage(imgName string) string {
	if idx := strings.LastIndex(imgName, ":"); idx >= 0 {
		image := "{{|DockerImage|}}" + imgName[:idx] + "{{[-]}}"
		tag := "{{|DockerTag|}}:" + imgName[idx+1:] + "{{[-]}}"
		return console.ToConsoleANSI(image + tag)
	}
	return console.ToConsoleANSI("{{|DockerImage|}}" + imgName + "{{[-]}}")
}

// buildLayerLine renders a layer row with optional progress bar.
// statusW is the minimum width for the status column (0 = no padding).
// barW is the number of braille chars in the progress bar (scales with layer count).
//
// Layout: layerPrefix + icon + " " + status + statusPad + " " + id + " " + sizes + bar
// sizes is always fixed-width (fixedSize cur + "/" + fixedSize total) so the bar column aligns.
func (p *consoleEventProcessor) buildLayerLine(t *consoleTask, statusW int, maxImgNameW int, layerPcts []int) string {
	id := t.id
	if len(id) > 19 {
		id = id[:18] + "…"
	}
	idW := utf8.RuneCountInString(id)

	// The SDK sets percent=100 on Done but does not update Current to equal Total
	// (Download complete / Pull complete events carry no progress bytes).
	// Use percent=100 to render a full bar and total/total for the size column.
	current := t.current
	if t.completed() && t.percent == 100 && t.total > 0 {
		current = t.total
	}

	var sizes string
	if t.total > 0 {
		sizes = " " + console.ToConsoleANSI("{{[::D]}}"+fixedSize(current)+"{{[-]}}"+
			"{{|DockerColon|}}/{{[-]}}"+
			"{{[::D]}}"+fixedSize(t.total)+"{{[-]}}")
	} else {
		sizes = strings.Repeat(" ", 1+8+1+8)
	}

	bar := ""
	if len(layerPcts) > 0 {
		progressChars := brailleChars
		if p.asciiMode {
			progressChars = asciiProgressChars
		}
		bar = " " + renderProgressBarLayers(layerPcts, progressChars, "{{[::D]}}")
	}

	icon := p.spinnerIcon(t)
	short := abbreviateStatus(t.text)
	statusPad := ""
	if pad := statusW - utf8.RuneCountInString(short); pad > 0 {
		statusPad = strutil.Repeat(" ", pad)
	}
	statusANSI := console.ToConsoleANSI(layerStatusTag(t.status, t.text))
	// Pad id so sizes column aligns with the image row sizes column.
	// imageSizesCol = imageSizesColBase + maxImgNameW
	// layerSizesCol = layerSizesColBase + statusW + spaceW + idW
	idPad := (imageSizesColBase + maxImgNameW) - (layerSizesColBase + statusW + spaceW + idW)
	if idPad < 1 {
		idPad = 1
	}
	return layerPrefix + console.CodeDim + icon + " " + statusANSI + console.CodeReset + console.CodeDim + statusPad + " " + id + strings.Repeat(" ", idPad) + sizes + bar + console.CodeDimOff
}

// buildLayerLines renders layer rows single-column, indented under the image: line.
// Bar width scales with the number of layers (more layers = narrower bars), matching the SDK approach.
// Returns parallel (lines, tasks) slices for timer attachment.
func (p *consoleEventProcessor) buildLayerLines(layers []*consoleTask, maxImgNameW int, termW int) ([]string, []*consoleTask) {
	if len(layers) == 0 {
		return nil, nil
	}

	// SDK approach: one char per layer, no min clamp.
	// Failsafe: clamp to terminal width minus fixed prefix (~70 chars) so bar never wraps.
	barW := len(layers)
	maxBarW := termW - 70
	if maxBarW < 1 {
		maxBarW = 1
	}
	if barW > maxBarW {
		barW = maxBarW
	}

	// Pre-compute per-layer percents for the shared bar (all rows show the same bar).
	layerPcts := make([]int, len(layers))
	for i, t := range layers {
		pct := t.percent
		if t.completed() && pct == 100 {
			pct = 100
		} else if t.total > 0 {
			pct = int(float64(t.current) / float64(t.total) * 100)
		}
		if pct > 100 {
			pct = 100
		}
		layerPcts[i] = pct
	}

	var out []string
	var outTasks []*consoleTask
	for _, t := range layers {
		out = append(out, p.buildLayerLine(t, layerStatusW, maxImgNameW, layerPcts))
		outTasks = append(outTasks, t)
	}
	return out, outTasks
}


func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

func fixedSize(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	var s string
	switch {
	case b >= gb:
		s = fmt.Sprintf("%.0fGB", float64(b)/gb)
	case b >= mb:
		s = fmt.Sprintf("%.0fMB", float64(b)/mb)
	case b >= kb:
		s = fmt.Sprintf("%.1fkB", float64(b)/kb)
	default:
		s = fmt.Sprintf("%dB", b)
	}
	// Right-pad to 8 chars for consistent column width (handles up to 9999.9kB / 9999MB)
	for len(s) < 8 {
		s = " " + s
	}
	return s
}

// renderProgressBarLayers renders one char per layer, each at its own fill level — matching the SDK.
func renderProgressBarLayers(layerPcts []int, chars []string, colorTag string) string {
	levels := len(chars) - 1
	var sb strings.Builder
	for _, pct := range layerPcts {
		if pct > 100 {
			pct = 100
		}
		sb.WriteString(chars[levels*pct/100])
	}
	return "[" + console.ToConsoleANSI(colorTag+sb.String()+"{{[-]}}") + "]"
}

// spinnerIcon returns the animated spinner or a terminal icon based on task state.
// Pass nil for t to get the active spinner (used when inheriting image state).
// impliedStatus returns the status text and ANSI tag for a service that received
// no events. For teardown commands this means the container was already in the
// target state; for startup commands it's genuinely pending.
func (p *consoleEventProcessor) impliedStatus() (text, ansiTag string) {
	switch p.command {
	case "down":
		return "Removed", "{{|DockerFinal|}}"
	case "stop", "kill":
		return "Stopped", "{{|DockerFinal|}}"
	case "pause":
		return "Paused", "{{|DockerFinal|}}"
	case "unpause", "start":
		return "Running", "{{|DockerFinal|}}"
	default:
		return "Pending", "{{|DockerPending|}}"
	}
}

type iconSet struct {
	done, error, warn, pending, spinner string
}

func (p *consoleEventProcessor) icons() iconSet {
	if p.asciiMode {
		return iconSet{done: "+", error: "x", warn: "!", pending: "-", spinner: asciiSpinnerFrames[p.spinnerFrame%len(asciiSpinnerFrames)]}
	}
	return iconSet{done: "✓", error: "×", warn: "⚠", pending: "·", spinner: spinnerFrames[p.spinnerFrame]}
}

func (p *consoleEventProcessor) spinnerIcon(t *consoleTask) string {
	ic := p.icons()

	var s string
	if t == nil {
		s = console.ToConsoleANSI("{{|DockerSpinner|}}" + ic.spinner + "{{[-]}}")
	} else {
		switch t.status {
		case api.Done:
			s = console.ToConsoleANSI("{{|DockerSuccess|}}" + ic.done + "{{[-]}}")
		case api.Error:
			s = console.ToConsoleANSI("{{|DockerFail|}}" + ic.error + "{{[-]}}")
		case api.Warning:
			s = console.ToConsoleANSI("{{|DockerWarn|}}" + ic.warn + "{{[-]}}")
		default:
			if t.completed() {
				s = console.ToConsoleANSI("{{|DockerSuccess|}}" + ic.done + "{{[-]}}")
			} else {
				s = console.ToConsoleANSI("{{|DockerSpinner|}}" + ic.spinner + "{{[-]}}")
			}
		}
	}
	if s == "" {
		return " "
	}
	return s
}

// abbreviateStatus shortens verbose Docker layer/image status strings.
func abbreviateStatus(text string) string {
	switch text {
	case "Pulling fs layer":
		return "Pending"
	case api.StatusDownloadComplete, "Pull complete":
		return "Downloaded"
	case "Already exists":
		return "Cached"
	case "Verifying Checksum":
		return "Verifying"
	case "Extracting":
		return "Extracting"
	}
	return text
}

// applyStatusTag wraps short in the appropriate semantic tag based on event status and
// whether the text belongs to the final/active/success/pending category for that task type.
// finalTexts: text values that represent a stable running/done-for-good state (DockerFinal).
// activeTexts: text values that represent in-progress transitions (DockerActive).
// successTexts: text values that represent a completed transition (DockerSuccess).
// Anything else with Working status → DockerPending.
func applyStatusTag(s api.EventStatus, text string, finalTexts, activeTexts, successTexts []string) string {
	short := abbreviateStatus(text)
	switch s {
	case api.Warning:
		return "{{|DockerWarn|}}" + short + "{{[-]}}"
	case api.Error:
		return "{{|DockerFail|}}" + short + "{{[-]}}"
	case api.Done:
		if contains(finalTexts, text) {
			return "{{|DockerFinal|}}" + short + "{{[-]}}"
		}
		return "{{|DockerSuccess|}}" + short + "{{[-]}}"
	default: // Working
		if contains(finalTexts, text) {
			return "{{|DockerFinal|}}" + short + "{{[-]}}"
		}
		if contains(activeTexts, text) {
			return "{{|DockerActive|}}" + short + "{{[-]}}"
		}
		if contains(successTexts, text) {
			return "{{|DockerSuccess|}}" + short + "{{[-]}}"
		}
		return "{{|DockerPending|}}" + short + "{{[-]}}"
	}
}

// serviceStatusTag styles a service (container lifecycle) status.
// finalTexts is command-specific — see serviceFinalStatuses.
// Active: Creating, Starting, Stopping, Restarting, Killing, Removing — in-progress transitions.
// Success: remaining completed transitions not in finalTexts.
func serviceStatusTag(s api.EventStatus, text string, command string) string {
	final := serviceFinalStatuses(command)
	success := []string{api.StatusCreated, api.StatusStarted, api.StatusStopped,
		api.StatusRestarted, api.StatusKilled, api.StatusRemoved}
	// Remove any status that appears in final from the success list.
	filtered := success[:0:len(success)]
	for _, v := range success {
		if !contains(final, v) {
			filtered = append(filtered, v)
		}
	}
	return applyStatusTag(s, text,
		final,
		[]string{api.StatusCreating, api.StatusStarting, api.StatusStopping,
			api.StatusRestarting, api.StatusKilling, api.StatusRemoving},
		filtered,
	)
}

// serviceFinalStatuses returns the terminal "final" states for a given command.
func serviceFinalStatuses(command string) []string {
	switch command {
	case "down", "rm":
		return []string{api.StatusRemoved}
	case "stop":
		return []string{api.StatusStopped}
	case "restart":
		return []string{api.StatusRestarted}
	case "kill":
		return []string{api.StatusKilled}
	case "create":
		return []string{api.StatusCreated}
	default: // up, update, start
		return []string{api.StatusRunning, api.StatusHealthy, api.StatusCreated}
	}
}

// imageStatusTag styles an image-level (pull/build) status.
// Final: Pulled, Built — image fetch/build completed.
// Active: Pulling, Building — in-progress.
func imageStatusTag(s api.EventStatus, text string) string {
	return applyStatusTag(s, text,
		[]string{api.StatusPulled, api.StatusBuilt},
		[]string{api.StatusPulling, api.StatusBuilding},
		nil,
	)
}

// layerStatusTag styles a layer-level (download/extract) status.
// Final: Downloaded, Pull complete, Already exists — layer is done.
// Active: Downloading, Extracting, Verifying Checksum, Pulling fs layer — in-progress.
func layerStatusTag(s api.EventStatus, text string) string {
	return applyStatusTag(s, text,
		[]string{api.StatusDownloadComplete, "Pull complete", "Already exists"},
		[]string{api.StatusDownloading, "Extracting", "Verifying Checksum", "Pulling fs layer"},
		nil,
	)
}

// networkStatusTag styles a network lifecycle status.
// Final is command-specific: Created for up/create, Removed for down.
// Active: Creating, Removing — in-progress.
func networkStatusTag(s api.EventStatus, text string, command string) string {
	final := networkFinalStatuses(command)
	success := []string{api.StatusCreated, api.StatusRemoved}
	filtered := success[:0:len(success)]
	for _, v := range success {
		if !contains(final, v) {
			filtered = append(filtered, v)
		}
	}
	return applyStatusTag(s, text,
		final,
		[]string{api.StatusCreating, api.StatusRemoving},
		filtered,
	)
}

// networkFinalStatuses returns the terminal "final" states for a given command.
func networkFinalStatuses(command string) []string {
	switch command {
	case "down", "rm":
		return []string{api.StatusRemoved}
	default: // up, update, create
		return []string{api.StatusCreated}
	}
}

// volumeStatusTag styles a volume lifecycle status.
// Final is command-specific: Created for up/create, Removed for down -v.
func volumeStatusTag(s api.EventStatus, text string, command string) string {
	final := volumeFinalStatuses(command)
	success := []string{api.StatusCreated, api.StatusRemoved}
	filtered := success[:0:len(success)]
	for _, v := range success {
		if !contains(final, v) {
			filtered = append(filtered, v)
		}
	}
	return applyStatusTag(s, text,
		final,
		[]string{api.StatusCreating, api.StatusRemoving},
		filtered,
	)
}

// volumeFinalStatuses returns the terminal "final" states for a given command.
func volumeFinalStatuses(command string) []string {
	switch command {
	case "down", "rm":
		return []string{api.StatusRemoved}
	default: // up, update, create
		return []string{api.StatusCreated}
	}
}

func elapsedStr(t *consoleTask) string {
	end := time.Now()
	if t.completed() && !t.endTime.IsZero() {
		end = t.endTime
	}
	secs := end.Sub(t.startTime).Seconds()
	// Left-align integer part so decimal points stay aligned:
	// < 10s → " 1.2s", < 100s → "12.3s", >= 100s → "123.4s" (expands as needed)
	switch {
	case secs < 10:
		return fmt.Sprintf(" %.1fs", secs)
	default:
		return fmt.Sprintf("%.1fs", secs)
	}
}

// serviceTimerTask returns a synthetic consoleTask for a service timer:
// startTime = earliest recorded start (image pull or container start),
// endTime/status = from the service container task once it completes.
// Returns nil if work hasn't started yet.
func (p *consoleEventProcessor) serviceTimerTask(svcID string) *consoleTask {
	start, ok := p.serviceStartTimes[svcID]
	if !ok || start.IsZero() {
		return nil
	}
	synthetic := &consoleTask{startTime: start}
	if t := p.tasks[svcID]; t != nil && t.completed() && !t.endTime.IsZero() {
		synthetic.status = api.Done
		synthetic.endTime = t.endTime
	}
	return synthetic
}

// sectionTaskFor returns a synthetic consoleTask whose startTime/endTime span all tasks in ids.
// Returns nil if no tasks have started yet.
func (p *consoleEventProcessor) sectionTaskFor(ids []string) *consoleTask {
	var minStart, maxEnd time.Time
	allDone := true
	for _, id := range ids {
		t := p.tasks[id]
		if t == nil || t.startTime.IsZero() {
			allDone = false
			continue
		}
		if minStart.IsZero() || t.startTime.Before(minStart) {
			minStart = t.startTime
		}
		if !t.completed() {
			allDone = false
		} else if !t.endTime.IsZero() && t.endTime.After(maxEnd) {
			maxEnd = t.endTime
		}
	}
	if minStart.IsZero() {
		return nil
	}
	synthetic := &consoleTask{startTime: minStart}
	if allDone && !maxEnd.IsZero() {
		synthetic.status = api.Done
		synthetic.endTime = maxEnd
	}
	return synthetic
}

// looksLikeImageName returns true if id looks like a Docker image reference (contains / or :).
// Used to avoid misclassifying a failing image pull as a service when the SDK sends StatusError.
func looksLikeImageName(id string) bool {
	return strings.ContainsAny(id, "/:")
}

// isServiceStatus returns true if text is a container lifecycle status.
func isServiceStatus(text string) bool {
	switch text {
	case api.StatusCreating, api.StatusCreated,
		api.StatusStarting, api.StatusStarted,
		api.StatusStopping, api.StatusStopped,
		api.StatusRestarting, api.StatusRestarted,
		api.StatusRemoving, api.StatusRemoved,
		api.StatusRunning, api.StatusWaiting,
		api.StatusHealthy, api.StatusExited,
		api.StatusKilling, api.StatusKilled,
		api.StatusError:
		return true
	}
	return false
}

// imageMatchesService returns true if imageID's base name matches the service name.
// e.g. "Image lscr.io/linuxserver/plex:latest" matches "plex" or "plex__ota".
func imageMatchesService(imageID, serviceID string) bool {
	img := imageID
	if idx := strings.LastIndex(img, "/"); idx >= 0 {
		img = img[idx+1:]
	}
	if idx := strings.Index(img, ":"); idx >= 0 {
		img = img[:idx]
	}
	img = strings.ToLower(img)

	svc := strings.ToLower(serviceID)
	base := svc
	if idx := strings.Index(svc, "__"); idx >= 0 {
		base = svc[:idx]
	}
	return img == base || img == svc
}


// padOrTrunc ensures a line is exactly termW visible chars wide.
func padOrTrunc(line string, termW int) string {
	plain := console.Strip(line)
	visible := utf8.RuneCountInString(plain)
	if visible < termW {
		return line + strings.Repeat(" ", termW-visible)
	}
	runes := []rune(plain)
	if len(runes) > termW {
		return string(runes[:termW-1]) + "…"
	}
	return line
}

// imageBaseName returns the image name portion of a URL for sort purposes:
// the path segment after the last "/" stripped of its tag (e.g. "lscr.io/linuxserver/sonarr:latest" → "sonarr").
func imageBaseName(img string) string {
	// strip tag
	if i := strings.LastIndex(img, ":"); i >= 0 {
		img = img[:i]
	}
	// basename after last /
	if i := strings.LastIndex(img, "/"); i >= 0 {
		return img[i+1:]
	}
	return img
}

// isKnownServiceName returns true if name appears in any imageServices service list or serviceIDs.
func (p *consoleEventProcessor) isKnownServiceName(name string) bool {
	for _, svcs := range p.imageServices {
		if contains(svcs, name) {
			return true
		}
	}
	return contains(p.serviceIDs, name)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func remove(s []string, v string) []string {
	out := s[:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
