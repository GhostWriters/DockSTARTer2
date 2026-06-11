package compose

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
	globalIndent  = " "   // 1-space left margin for all lines
	layerIndent   = "    " // additional indent for layer rows (after globalIndent+icon)
	progressWidth = 10
)

var brailleChars = strings.Split("⠀⡀⣀⣄⣤⣦⣶⣷⣿", "")
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

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
	ctx    context.Context
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

	// command is the ds2 command name (e.g. "up", "update") used for the summary header.
	command string

	// imageServices is pre-populated from the project config: image -> []serviceName
	imageServices map[string][]string
	// imageOrder is the stable insertion order of imageServices keys (map iteration is random).
	imageOrder []string
	// containerToService maps container_name -> service name for services where they differ.
	containerToService map[string]string

	numLines     int // lines written in last render
	started      bool
	spinnerFrame int
}

// NewConsoleEventProcessor creates a themed live-updating EventProcessor for TTY output.
// imageServices maps image name (e.g. "lscr.io/linuxserver/plex:latest") to the list of
// service names that use it, so service headers can be shown before lifecycle events arrive.
// imageOrder is the stable key order for imageServices (caller must provide sorted/deterministic order).
func NewConsoleEventProcessor(out io.Writer, command string, imageServices map[string][]string, imageOrder []string, containerToService map[string]string) api.EventProcessor {
	return &consoleEventProcessor{
		out:                out,
		doneCh:             make(chan struct{}, 1),
		tasks:              make(map[string]*consoleTask),
		command:            command,
		imageServices:      imageServices,
		imageOrder:         imageOrder,
		containerToService: containerToService,
	}
}

func (p *consoleEventProcessor) Start(ctx context.Context, operation string) {
	p.ctx = ctx
	p.operation = operation

	// Print a one-line summary header showing service and image counts.
	svcCount := 0
	for _, svcs := range p.imageServices {
		svcCount += len(svcs)
	}
	imgCount := len(p.imageOrder)
	summary := console.ToConsoleANSI(fmt.Sprintf(
		"{{|RunningCommand|}}%s{{[-]}}  {{|DockerPending|}}%d service%s{{[-]}}  {{|DockerPending|}}%d image%s{{[-]}}\n",
		p.command,
		svcCount, pluralS(svcCount),
		imgCount, pluralS(imgCount),
	))
	fmt.Fprint(p.out, summary)

	// Pre-print one blank line per line buildLines() will produce from the initial
	// state (no tasks yet, but all imageOrder/imageServices entries known).
	// This fixes the stable height — render() will overwrite these in-place.
	// We call buildLines() with a wide terminal to avoid two-column compression
	// affecting the reserved count (layer columns only appear during active pulls).
	termW := goterm.Width()
	if termW <= 0 {
		termW = 80
	}
	initialLines := p.buildLines(termW)
	if len(initialLines) > 0 {
		for range initialLines {
			fmt.Fprintln(p.out)
		}
		p.numLines = len(initialLines)
		p.started = true
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
}

// logSummary writes a plain-text summary to the log file — one layer per line,
// no ANSI codes, no cursor movement. Uses WithSuppressConsole so it goes to
// the log file and TUI panel only, not the terminal (which has the live display).
func (p *consoleEventProcessor) logSummary() {
	if p.ctx == nil {
		return
	}
	ctx := logger.WithSuppressConsole(p.ctx)

	const pfx = "{{|RunningCommand|}}docker compose:{{[-]}} "
	svcCount := 0
	for _, svcs := range p.imageServices {
		svcCount += len(svcs)
	}
	imgCount := len(p.imageOrder)
	logger.Info(ctx, pfx+"{{|RunningCommand|}}%s{{[-]}}  {{|DockerPending|}}%d service%s{{[-]}}  {{|DockerPending|}}%d image%s{{[-]}}",
		p.command,
		svcCount, pluralS(svcCount),
		imgCount, pluralS(imgCount),
	)

	for _, imgName := range p.imageOrder {
		svcs := p.serviceIDsForImage(imgName)
		img := p.tasks[imgName]

		for _, svc := range svcs {
			t := p.tasks[svc]
			var svcStatus string
			if t != nil {
				svcStatus = statusTag(t.status, t.text)
			} else if img != nil {
				svcStatus = statusTag(img.status, img.text)
			} else {
				svcStatus = "{{|DockerPending|}}Pending{{[-]}}"
			}
			logger.Info(ctx, pfx+"  {{|App|}}%s{{[-]}}: %s", svc, svcStatus)
		}

		if img != nil {
			logger.Info(ctx, pfx+"    %s  %s  %s",
				styleImage(imgName),
				statusTag(img.status, img.text),
				"{{|DockerPending|}}"+elapsedStr(img)+"{{[-]}}")
		} else {
			logger.Info(ctx, pfx+"    %s  {{|DockerSuccess|}}Cached{{[-]}}", styleImage(imgName))
		}

		for _, id := range p.ids {
			t := p.tasks[id]
			if t.parentID != imgName {
				continue
			}
			if t.total > 0 {
				logger.Info(ctx, pfx+"      {{|DockerPending|}}%s{{[-]}}  %s/%s  %s",
					t.id,
					strings.TrimSpace(fixedSize(t.current)),
					strings.TrimSpace(fixedSize(t.total)),
					statusTag(t.status, t.text))
			} else {
				logger.Info(ctx, pfx+"      {{|DockerPending|}}%s{{[-]}}  %s", t.id, statusTag(t.status, t.text))
			}
		}
	}
}

func (p *consoleEventProcessor) On(events ...api.Resource) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	for _, e := range events {
		if e.ID == api.ResourceCompose {
			continue
		}
		p.upsert(e)
	}
}

func (p *consoleEventProcessor) upsert(e api.Resource) {
	// Normalise SDK-prefixed IDs so they match project service/image names
	id := strings.TrimPrefix(e.ID, "Container ")
	id = strings.TrimPrefix(id, "Image ")
	parentID := strings.TrimPrefix(e.ParentID, "Container ")
	parentID = strings.TrimPrefix(parentID, "Image ")

	// Remap container_name back to service name (e.g. "qbittorrentx" -> "qbittorrent")
	if svc, ok := p.containerToService[id]; ok {
		id = svc
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
			if isServiceStatus(e.Text) {
				p.serviceIDs = append(p.serviceIDs, id)
			} else {
				p.imageIDs = append(p.imageIDs, id)
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
	if e.ParentID == "" && isServiceStatus(e.Text) && contains(p.imageIDs, id) {
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
	_ = termH

	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.spinnerFrame = (p.spinnerFrame + 1) % len(spinnerFrames)
	lines := p.buildLines(termW)
	if len(lines) == 0 {
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
		buf.WriteString(fmt.Sprintf("\033[%dA", p.numLines)) // cursor up N lines
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
}

// buildLines constructs the full set of display lines grouped by image.
// Layout is driven entirely by imageOrder so line count is stable across renders:
// every known image always emits a service-header + image-row + any layer rows.
func (p *consoleEventProcessor) buildLines(termW int) []string {
	var lines []string

	// Pre-compute global alignment widths across all service groups.
	// Floor is the longest possible abbreviated status word so the image column
	// never shifts as statuses change during the operation.
	globalNameW := 0
	globalStatusW := len("Downloading")
	globalImageW := 0
	for _, imgName := range p.imageOrder {
		if n := utf8.RuneCountInString(imgName); n > globalImageW {
			globalImageW = n
		}
		for _, svc := range p.serviceIDsForImage(imgName) {
			if n := utf8.RuneCountInString(svc); n > globalNameW {
				globalNameW = n
			}
			if t := p.tasks[svc]; t != nil {
				if s := utf8.RuneCountInString(abbreviateStatus(t.text)); s > globalStatusW {
					globalStatusW = s
				}
			}
		}
	}
	// Also account for orphan services.
	for _, svcID := range p.serviceIDs {
		if n := utf8.RuneCountInString(svcID); n > globalNameW {
			globalNameW = n
		}
		if t := p.tasks[svcID]; t != nil {
			if s := utf8.RuneCountInString(abbreviateStatus(t.text)); s > globalStatusW {
				globalStatusW = s
			}
		}
	}

	coveredSvcs := make(map[string]bool)

	for _, imgName := range p.imageOrder {
		// Service header — use live task IDs if available, else config names
		svcs := p.serviceIDsForImage(imgName)
		for _, s := range svcs {
			coveredSvcs[s] = true
		}
		// One line per service; image suffix on the last one, columns aligned.
		// Layout: "name: " padded to maxNameW+2, status padded to maxStatusW, then image.
		img := p.tasks[imgName]

		for i, svc := range svcs {
			t := p.tasks[svc]
			nameW := utf8.RuneCountInString(svc)
			namePad := strings.Repeat(" ", globalNameW-nameW)
			nameANSI := console.ToConsoleANSI("{{|App|}}" + svc + "{{[-]}}")

			var statusText, statusANSI, icon string
			if t == nil {
				if img != nil {
					statusText = abbreviateStatus(img.text)
					statusANSI = console.ToConsoleANSI(statusTag(img.status, img.text))
					icon = p.spinnerIcon(nil) // active — inheriting image pull
				} else {
					statusText = "Pending"
					statusANSI = console.ToConsoleANSI("{{|DockerPending|}}" + statusText + "{{[-]}}")
					icon = console.ToConsoleANSI("{{|DockerPending|}}·{{[-]}}")
				}
			} else {
				statusText = abbreviateStatus(t.text)
				statusANSI = console.ToConsoleANSI(statusTag(t.status, t.text))
				icon = p.spinnerIcon(t)
			}
			statusPad := strings.Repeat(" ", globalStatusW-utf8.RuneCountInString(statusText))
			line := globalIndent + icon + " " + nameANSI + ":" + namePad + " " + statusANSI + console.CodeReset + statusPad

			if i == len(svcs)-1 {
				line += "  " + p.buildImageSuffix(imgName, img, globalImageW, globalStatusW)
			}
			lines = append(lines, line)
		}

		// Layer rows for this image — two columns if terminal is wide enough.
		var layers []*consoleTask
		for _, id := range p.ids {
			t := p.tasks[id]
			if t.parentID == imgName {
				layers = append(layers, t)
			}
		}
		lines = append(lines, p.buildLayerLines(layers, termW)...)
	}

	// Runtime service events with no image group (restart/stop/start) — one line each.
	for _, svcID := range p.serviceIDs {
		if coveredSvcs[svcID] {
			continue
		}
		t := p.tasks[svcID]
		nameW := utf8.RuneCountInString(svcID)
		namePad := strings.Repeat(" ", globalNameW-nameW)
		nameANSI := console.ToConsoleANSI("{{|App|}}" + svcID + "{{[-]}}")
		var statusText, statusANSI, icon string
		if t == nil {
			statusText = "Pending"
			statusANSI = console.ToConsoleANSI("{{|DockerPending|}}" + statusText + "{{[-]}}")
			icon = console.ToConsoleANSI("{{|DockerPending|}}·{{[-]}}")
		} else {
			statusText = abbreviateStatus(t.text)
			statusANSI = console.ToConsoleANSI(statusTag(t.status, t.text))
			icon = p.spinnerIcon(t)
		}
		statusPad := strings.Repeat(" ", globalStatusW-utf8.RuneCountInString(statusText))
		lines = append(lines, globalIndent+icon+" "+nameANSI+":"+namePad+" "+statusANSI+console.CodeReset+statusPad)
	}

	return lines
}

// buildServiceHeader renders "svc1: Status, svc2: Status"
// If a service has no task yet (no lifecycle event received), it shows with no status.
func (p *consoleEventProcessor) buildServiceHeader(svcIDs []string) string {
	if len(svcIDs) == 0 {
		return ""
	}
	var parts []string
	for _, id := range svcIDs {
		t := p.tasks[id]
		name := console.ToConsoleANSI("{{|App|}}" + id + "{{[-]}}")
		if t == nil {
			parts = append(parts, name)
			continue
		}
		statusANSI := console.ToConsoleANSI(statusTag(t.status, t.text))
		parts = append(parts, name+": "+statusANSI+console.CodeReset)
	}
	return strings.Join(parts, ", ")
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

// buildImageSuffix returns the parenthesised image info appended to the service header line.
// e.g. " (lscr.io/.../radarr:develop  Pulling  [⣿⣿⡀⠀⠀]  18.1s)"
// If no task yet (image not yet being pulled), returns just the image name in parens.
func (p *consoleEventProcessor) buildImageSuffix(imgName string, t *consoleTask, imageW, statusW int) string {
	// Pad image name to imageW so the status column aligns across all groups.
	imagePad := strings.Repeat(" ", imageW-utf8.RuneCountInString(imgName))
	imageANSI := styleImage(imgName) + imagePad
	if t == nil {
		// No pull event means the image was already cached — show as done.
		cachedIcon := console.ToConsoleANSI("{{|DockerSuccess|}}✔{{[-]}}")
		return cachedIcon + " " + imageANSI
	}
	icon := p.spinnerIcon(t)
	statusText := abbreviateStatus(t.text)
	statusPad := strings.Repeat(" ", statusW-utf8.RuneCountInString(statusText))
	statusANSI := console.ToConsoleANSI(statusTag(t.status, t.text)) + statusPad
	timerANSI := console.ToConsoleANSI("{{|DockerPending|}}" + elapsedStr(t) + "{{[-]}}")
	return icon + " " + imageANSI + "  " + statusANSI + console.CodeReset + "  " + timerANSI
}

// buildLayerLine renders a layer row with optional progress bar.
func (p *consoleEventProcessor) buildLayerLine(t *consoleTask) string {
	id := t.id
	if len(id) > 19 {
		id = id[:18] + "…"
	}

	// barWidth = 1(space) + 2(brackets)+progressWidth + 1(space) + 7(cur) + 1(/) + 7(total)
	const barWidth = 1 + (2 + progressWidth) + 1 + 7 + 1 + 7
	bar := ""
	if t.total > 0 {
		bar = " " + renderProgressBar(t.current, t.total, progressWidth) +
			" " + fixedSize(t.current) + "/" + fixedSize(t.total)
	} else if t.percent > 0 {
		bar = " " + renderProgressBarPct(t.percent, progressWidth) + strings.Repeat(" ", barWidth-(1+2+progressWidth))
	} else {
		bar = strings.Repeat(" ", barWidth)
	}

	icon := p.spinnerIcon(t)
	statusANSI := console.ToConsoleANSI(statusTag(t.status, t.text))
	return globalIndent + layerIndent + icon + " " + id + bar + "  " + statusANSI + console.CodeReset
}

const (
	minColWidth  = 60 // minimum visible width for a layer column
	layerGutter  = 2  // chars between left and right layer columns
)

// buildLayerLines lays out layer rows in one or two columns depending on terminal width.
func (p *consoleEventProcessor) buildLayerLines(layers []*consoleTask, termW int) []string {
	if len(layers) == 0 {
		return nil
	}
	// Account for globalIndent + icon + space prefix (3 chars) already on each line.
	usableW := termW - len(globalIndent) - 2 // 2 = icon + space
	colW := (usableW - layerGutter) / 2
	twoCol := colW >= minColWidth && len(layers) > 1

	if !twoCol {
		var out []string
		for _, t := range layers {
			out = append(out, p.buildLayerLine(t))
		}
		return out
	}

	gutter := strings.Repeat(" ", layerGutter)
	half := (len(layers) + 1) / 2

	// Pre-render all left-column lines and find the widest visible width,
	// so we pad to content width rather than half the terminal.
	leftLines := make([]string, half)
	maxLeftW := 0
	for i := 0; i < half; i++ {
		leftLines[i] = p.buildLayerLine(layers[i])
		if w := utf8.RuneCountInString(console.Strip(leftLines[i])); w > maxLeftW {
			maxLeftW = w
		}
	}

	var out []string
	for i := 0; i < half; i++ {
		left := padOrTruncN(leftLines[i], maxLeftW)
		right := ""
		if i+half < len(layers) {
			right = p.buildLayerLine(layers[i+half])
		}
		out = append(out, left+gutter+right)
	}
	return out
}

// fixedSize formats a byte count as a fixed-width 7-char string (e.g. " 80.6MB", "  1.0KB").
// MB and above are whole numbers; KB gets one decimal. Always right-aligned in 7 chars.
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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
	// Right-pad to 7 chars for consistent column width
	for len(s) < 7 {
		s = " " + s
	}
	return s
}

func renderProgressBar(current, total int64, width int) string {
	if total == 0 {
		return "[" + console.ToConsoleANSI("{{|DockerProgressBar|}}" + strings.Repeat(brailleChars[0], width) + "{{[-]}}") + "]"
	}
	filled := int(float64(current) / float64(total) * float64(width*8))
	var sb strings.Builder
	for i := range width {
		cell := filled - i*8
		switch {
		case cell >= 8:
			sb.WriteString(brailleChars[8])
		case cell > 0:
			sb.WriteString(brailleChars[cell])
		default:
			sb.WriteString(brailleChars[0])
		}
	}
	return "[" + console.ToConsoleANSI("{{|DockerProgressBar|}}" + sb.String() + "{{[-]}}") + "]"
}

func renderProgressBarPct(pct, width int) string {
	return renderProgressBar(int64(pct), 100, width)
}

// spinnerIcon returns the animated spinner or a terminal icon based on task state.
// Pass nil for t to get the active spinner (used when inheriting image state).
func (p *consoleEventProcessor) spinnerIcon(t *consoleTask) string {
	if t == nil {
		return console.ToConsoleANSI("{{|DockerActive|}}" + spinnerFrames[p.spinnerFrame] + "{{[-]}}")
	}
	switch t.status {
	case api.Done:
		return console.ToConsoleANSI("{{|DockerSuccess|}}✔{{[-]}}")
	case api.Error:
		return console.ToConsoleANSI("{{|DockerFail|}}✘{{[-]}}")
	case api.Warning:
		return console.ToConsoleANSI("{{|DockerWarn|}}⚠{{[-]}}")
	}
	if t.completed() {
		return console.ToConsoleANSI("{{|DockerSuccess|}}✔{{[-]}}")
	}
	return console.ToConsoleANSI("{{|DockerActive|}}" + spinnerFrames[p.spinnerFrame] + "{{[-]}}")
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

func statusTag(s api.EventStatus, text string) string {
	short := abbreviateStatus(text)
	switch s {
	case api.Done:
		return "{{|DockerSuccess|}}" + short + "{{[-]}}"
	case api.Warning:
		return "{{|DockerWarn|}}" + short + "{{[-]}}"
	case api.Error:
		return "{{|DockerFail|}}" + short + "{{[-]}}"
	default:
		// Distinguish active (in-progress) from pending (waiting)
		switch text {
		case api.StatusDownloading, api.StatusPulling, "Extracting", "Verifying Checksum":
			return "{{|DockerActive|}}" + short + "{{[-]}}"
		default:
			return "{{|DockerPending|}}" + short + "{{[-]}}"
		}
	}
}

func elapsedStr(t *consoleTask) string {
	end := time.Now()
	if t.completed() && !t.endTime.IsZero() {
		end = t.endTime
	}
	return fmt.Sprintf("%.1fs", end.Sub(t.startTime).Seconds())
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

// padOrTruncN ensures a line is exactly w visible chars wide.
func padOrTruncN(line string, w int) string {
	plain := console.Strip(line)
	visible := utf8.RuneCountInString(plain)
	if visible < w {
		return line + strings.Repeat(" ", w-visible)
	}
	runes := []rune(plain)
	if len(runes) > w {
		return string(runes[:w-1]) + "…"
	}
	return line
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
