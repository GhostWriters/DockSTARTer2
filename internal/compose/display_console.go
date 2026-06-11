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
	"github.com/docker/go-units"
	"github.com/morikuni/aec"

	"DockSTARTer2/internal/console"
)

// consoleEventProcessor implements api.EventProcessor with a themed live-updating
// display for terminals. Layout per image group:
//
//	svc1: Status, svc2: Status          ← service header (shared image)
//	  image:tag                  Pulled  ← image row
//	    sha256:abc...  [⣿⣿⣿⡀⠀]  Done    ← layer rows
const (
	layerIndent   = "    "
	imageIndent   = "  "
	progressWidth = 10 // number of braille cells in progress bar
)

// braille progress chars — same as docker compose upstream
var brailleChars = strings.Split("⠀⡀⣀⣄⣤⣦⣶⣷⣿", "")

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

func (t *consoleTask) done() bool {
	return t.status == api.Done || t.status == api.Error || t.status == api.Warning
}

// consoleEventProcessor groups tasks by image and renders them live.
type consoleEventProcessor struct {
	out       io.Writer
	mtx       sync.Mutex
	ticker    *time.Ticker
	doneCh    chan struct{}
	operation string

	// ids preserves insertion order
	ids   []string
	tasks map[string]*consoleTask

	// services maps service name -> status text (no parentID)
	serviceIDs []string

	// imageIDs are tasks that are parents of layer tasks
	imageIDs []string

	numLines int
	repeated bool
}

// NewConsoleEventProcessor creates a themed live-updating EventProcessor for TTY output.
func NewConsoleEventProcessor(out io.Writer) api.EventProcessor {
	return &consoleEventProcessor{
		out:    out,
		doneCh: make(chan struct{}),
		tasks:  make(map[string]*consoleTask),
	}
}

func (p *consoleEventProcessor) Start(ctx context.Context, operation string) {
	p.operation = operation
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
	p.render()
	p.doneCh <- struct{}{}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if p.ticker != nil {
		p.ticker.Stop()
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
	t, exists := p.tasks[e.ID]
	if !exists {
		t = &consoleTask{
			id:        e.ID,
			parentID:  e.ParentID,
			startTime: time.Now(),
		}
		p.tasks[e.ID] = t
		p.ids = append(p.ids, e.ID)

		// Classify on first appearance
		if e.ParentID == "" {
			// Could be a service or a top-level image — decide by text
			if isServiceStatus(e.Text) {
				p.serviceIDs = append(p.serviceIDs, e.ID)
			} else {
				p.imageIDs = append(p.imageIDs, e.ID)
			}
		} else {
			// Has a parent — check if parent needs to be promoted to imageID
			if _, seen := p.tasks[e.ParentID]; !seen {
				// Parent hasn't appeared yet; it will self-classify when it arrives
			} else if !contains(p.imageIDs, e.ParentID) && !contains(p.serviceIDs, e.ParentID) {
				p.imageIDs = append(p.imageIDs, e.ParentID)
			}
		}
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
	if t.done() && t.endTime.IsZero() {
		t.endTime = time.Now()
	}

	// When a service status arrives and the ID was previously classified as image, reclassify
	if e.ParentID == "" && isServiceStatus(e.Text) && contains(p.imageIDs, e.ID) {
		p.imageIDs = remove(p.imageIDs, e.ID)
		if !contains(p.serviceIDs, e.ID) {
			p.serviceIDs = append(p.serviceIDs, e.ID)
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

	lines := p.buildLines(termW)

	up := p.numLines + 1
	if !p.repeated {
		up--
		p.repeated = true
	}

	b := aec.NewBuilder(aec.Hide, aec.Up(uint(up)), aec.Column(0))
	fmt.Fprint(p.out, b.ANSI)
	defer fmt.Fprint(p.out, aec.Show)

	rendered := 0
	for _, line := range lines {
		padded := padOrTrunc(line, termW)
		fmt.Fprintln(p.out, padded)
		rendered++
	}
	// Clear leftover lines from previous render
	for i := rendered; i < p.numLines; i++ {
		fmt.Fprintln(p.out, strings.Repeat(" ", termW))
		rendered++
	}
	p.numLines = rendered
}

// buildLines constructs the full set of display lines, grouped by image.
func (p *consoleEventProcessor) buildLines(termW int) []string {
	var lines []string

	// Build a map: imageID -> []serviceIDs that reference this image
	// A service "references" an image if the image task shares the same base name
	// or if no image tasks exist yet (service-only operations like restart/stop).
	imageToServices := p.mapImagesToServices()

	// Render each image group
	for _, imgID := range p.imageIDs {
		img := p.tasks[imgID]
		if img == nil {
			continue
		}

		// Header line: comma-separated "svc: Status" entries
		svcs := imageToServices[imgID]
		headerLine := p.buildServiceHeader(svcs, termW)
		if headerLine != "" {
			lines = append(lines, headerLine)
		}

		// Image row
		lines = append(lines, p.buildImageLine(img, termW))

		// Layer rows (children of this image)
		for _, id := range p.ids {
			t := p.tasks[id]
			if t.parentID != imgID {
				continue
			}
			lines = append(lines, p.buildLayerLine(t, termW))
		}
	}

	// Services with no associated image (e.g. restart, stop, start)
	coveredSvcs := make(map[string]bool)
	for _, svcs := range imageToServices {
		for _, s := range svcs {
			coveredSvcs[s] = true
		}
	}
	var orphanSvcs []string
	for _, svcID := range p.serviceIDs {
		if !coveredSvcs[svcID] {
			orphanSvcs = append(orphanSvcs, svcID)
		}
	}
	if len(orphanSvcs) > 0 {
		lines = append(lines, p.buildServiceHeader(orphanSvcs, termW))
	}

	return lines
}

// mapImagesToServices returns a map of imageID -> []serviceID.
// Heuristic: service name matches the prefix of the image ID (e.g. "plex" matches "lscr.io/.../plex:latest").
func (p *consoleEventProcessor) mapImagesToServices() map[string][]string {
	result := make(map[string][]string)
	for _, imgID := range p.imageIDs {
		for _, svcID := range p.serviceIDs {
			if imageMatchesService(imgID, svcID) {
				result[imgID] = append(result[imgID], svcID)
			}
		}
		if len(result[imgID]) == 0 {
			// No match — still show image with no service header
			result[imgID] = nil
		}
	}
	return result
}

// buildServiceHeader renders the comma-separated "svc: Status, svc2: Status" line.
func (p *consoleEventProcessor) buildServiceHeader(svcIDs []string, termW int) string {
	if len(svcIDs) == 0 {
		return ""
	}
	var parts []string
	for _, id := range svcIDs {
		t := p.tasks[id]
		if t == nil {
			parts = append(parts, id)
			continue
		}
		statusANSI := console.ToConsoleANSI(statusTag(t.status, t.text))
		parts = append(parts, id+": "+statusANSI+console.CodeReset)
	}
	return strings.Join(parts, ", ")
}

// buildImageLine renders the image row: "  image:tag   Status   1.2s"
func (p *consoleEventProcessor) buildImageLine(t *consoleTask, termW int) string {
	statusANSI := console.ToConsoleANSI(statusTag(t.status, t.text))
	timer := elapsed(t)
	timerANSI := console.ToConsoleANSI("{{|Info|}}" + timer + "{{[-]}}")
	return imageIndent + t.id + "  " + statusANSI + console.CodeReset + "  " + timerANSI
}

// buildLayerLine renders a layer row with progress bar.
func (p *consoleEventProcessor) buildLayerLine(t *consoleTask, termW int) string {
	bar := ""
	if t.total > 0 {
		bar = " " + renderProgressBar(t.current, t.total, progressWidth) + " " +
			units.HumanSize(float64(t.current)) + " / " + units.HumanSize(float64(t.total))
	} else if t.percent > 0 {
		bar = " " + renderProgressBarPct(t.percent, progressWidth)
	}

	id := t.id
	if len(id) > 20 {
		id = id[:19] + "…"
	}

	statusANSI := console.ToConsoleANSI(statusTag(t.status, t.text))
	return layerIndent + id + bar + "  " + statusANSI + console.CodeReset
}

// renderProgressBar builds a braille progress bar from byte counts.
func renderProgressBar(current, total int64, width int) string {
	if total == 0 {
		return "[" + strings.Repeat(brailleChars[0], width) + "]"
	}
	filled := int(float64(current) / float64(total) * float64(width*8))
	var sb strings.Builder
	sb.WriteString("[")
	for i := range width {
		cell := filled - i*8
		if cell >= 8 {
			sb.WriteString(brailleChars[8])
		} else if cell > 0 {
			sb.WriteString(brailleChars[cell])
		} else {
			sb.WriteString(brailleChars[0])
		}
	}
	sb.WriteString("]")
	return console.ToConsoleANSI("{{|Notice|}}" + sb.String() + "{{[-]}}")
}

// renderProgressBarPct builds a braille progress bar from a percentage.
func renderProgressBarPct(pct, width int) string {
	return renderProgressBar(int64(pct), 100, width)
}

// statusTag returns a themed inline tag string for a status.
func statusTag(s api.EventStatus, text string) string {
	switch s {
	case api.Done:
		return "{{|Success|}}" + text + "{{[-]}}"
	case api.Warning:
		return "{{|Warn|}}" + text + "{{[-]}}"
	case api.Error:
		return "{{|Error|}}" + text + "{{[-]}}"
	default:
		return "{{|Notice|}}" + text + "{{[-]}}"
	}
}

// elapsed returns a formatted elapsed time string for a task.
func elapsed(t *consoleTask) string {
	end := time.Now()
	if t.done() && !t.endTime.IsZero() {
		end = t.endTime
	}
	return fmt.Sprintf("%.1fs", end.Sub(t.startTime).Seconds())
}

// isServiceStatus returns true if the text looks like a container lifecycle status.
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

// imageMatchesService returns true if the image ID appears to belong to a service.
// e.g. "lscr.io/linuxserver/plex:latest" matches service "plex" or "plex__ota".
func imageMatchesService(imageID, serviceID string) bool {
	// Extract the image name without registry/tag
	img := imageID
	if idx := strings.LastIndex(img, "/"); idx >= 0 {
		img = img[idx+1:]
	}
	if idx := strings.Index(img, ":"); idx >= 0 {
		img = img[:idx]
	}
	img = strings.ToLower(img)

	// Service may be "plex" or "plex__ota" — base name is before "__"
	svc := strings.ToLower(serviceID)
	base := svc
	if idx := strings.Index(svc, "__"); idx >= 0 {
		base = svc[:idx]
	}
	return img == base || img == svc
}

// padOrTrunc pads or truncates a line (ANSI-aware) to exactly termW visible chars.
func padOrTrunc(line string, termW int) string {
	visible := utf8.RuneCountInString(console.Strip(line))
	if visible < termW {
		return line + strings.Repeat(" ", termW-visible)
	}
	// Truncate visible chars — crude but avoids splitting ANSI sequences mid-sequence
	// by stripping first, truncating, then returning plain.
	plain := console.Strip(line)
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
