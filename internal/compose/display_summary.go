package compose

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/strutil"

	"github.com/buger/goterm"
)

func (p *consoleEventProcessor) buildSummaryLine() string {
	svcCount := 0
	for _, svcs := range p.imageServices {
		svcCount += len(svcs)
	}
	imgCount := len(p.imageOrder)
	netCount := len(p.networkIDs)
	volCount := len(p.volumeIDs)

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
	timerImage                     // DockerMarkerDone green — image rows
	timerLayer                     // unstyled — layer rows
)

type timerEntry struct {
	task  *consoleTask
	style timerStyle
}

// attachTimers right-aligns elapsed timers on lines.
func (p *consoleEventProcessor) attachTimers(lines []string, timers []timerEntry) []string {
	for _, line := range lines {
		if w := utf8.RuneCountInString(console.Strip(line)); w > p.maxLineWidth {
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
		visible := utf8.RuneCountInString(console.Strip(line))
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
		timer := console.ToConsoleANSI(styleTag + s + "{{[-]}}")
		out[i] = line + pad + timer
	}
	return out
}

func (p *consoleEventProcessor) prependSummary(lines []string, timers []timerEntry) []string {
	summary := p.buildSummaryLine()
	if w := utf8.RuneCountInString(console.Strip(summary)); w > p.maxLineWidth {
		p.maxLineWidth = w
	}
	lines = p.attachTimers(lines, timers)
	if vp := console.GlobalViewport; vp != nil && vp.IsActive() {
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
	visible := utf8.RuneCountInString(console.Strip(summary))
	pad := strutil.Repeat(" ", col-visible)
	elapsed := elapsedFromTime(p.startTime)
	if w := len(elapsed); w < p.maxTimerWidth {
		elapsed = strutil.Repeat(" ", p.maxTimerWidth-w) + elapsed
	}
	timer := console.ToConsoleANSI("{{[yellow::B]}}" + elapsed + "{{[-]}}")
	return summary + pad + timer
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

	const pfx = "{{|RunningCommand|}}docker compose:{{[-]}} "

	termW := goterm.Width()
	if termW <= 0 {
		termW = 80
	}

	for _, line := range p.buildLines(termW) {
		logger.Notice(ctx, pfx+line)
	}
}

