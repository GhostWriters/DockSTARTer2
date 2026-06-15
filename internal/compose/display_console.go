package compose

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"

	"github.com/buger/goterm"
	"github.com/docker/compose/v5/pkg/api"
	dockerclient "github.com/moby/moby/client"
)

// imageInspector is the subset of the Docker client API used for post-pull layer inspection.
type imageInspector interface {
	ImageInspect(ctx context.Context, imageID string, opts ...dockerclient.ImageInspectOption) (dockerclient.ImageInspectResult, error)
}

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
	// Layout constants from dockerlayout — change there to affect all Docker output.
	globalIndentW        = dockerlayout.GlobalIndentW
	iconW                = dockerlayout.IconW
	spaceW               = dockerlayout.SpaceW
	sectionStatusTextW   = dockerlayout.SectionStatusTextW
	sectionStatusGutterW = dockerlayout.SectionStatusGutterW
	sectionStatusW       = dockerlayout.SectionStatusW
	sectionChildIndentW  = dockerlayout.SectionChildIndentW
	imageLabelTextW      = dockerlayout.ImageLabelTextW
	timerGutterW         = dockerlayout.TimerGutterW
	sectionHeaderIndent  = dockerlayout.SectionHeaderIndent
	imageLabelW          = dockerlayout.ImageLabelW
	layerPrefixW         = dockerlayout.LayerPrefixW

	// Compose-specific constants.
	layerStatusW      = dockerlayout.LayerStatusW // max width of any abbreviated layer status ("Downloading")
	sizeColW          = 8  // width of one fixedSize() value
	sizeSepW          = 1  // width of "/" between current/total sizes
	imageSizesColBase = globalIndentW + iconW + spaceW + sectionStatusW + imageLabelW
	layerSizesColBase = layerPrefixW + iconW + spaceW
)

// Strings derived from width constants — updated automatically when constants change.
var (
	globalIndent       = dockerlayout.GlobalIndent
	sectionChildIndent = dockerlayout.SectionChildIndent
	layerPrefix        = dockerlayout.LayerPrefix
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
	if t.status == api.Done || t.status == api.Error || t.status == api.Warning {
		return true
	}
	// SDK drops "Pull complete" (jm.Progress == nil guard), so "Extracting" is the
	// terminal state for extracted layers — treat it as completed.
	return t.text == "Extracting"
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

	startTime     time.Time       // set on first event; used for overall elapsed timer in summary
	numLines      int             // lines written in last render
	started       bool
	spinnerFrame  int
	maxLineWidth  int             // widest visible line seen so far; grows but never shrinks
	maxTimerWidth int             // widest timer string seen so far; grows but never shrinks
	noViewport    bool            // when true, skip GlobalViewport activation (e.g. running inside program box)
	updateFn      func([]string)  // called each render tick in noViewport mode instead of writing to out
	lastSentLines []string        // last lines sent via updateFn; skip if unchanged
	asciiMode     bool            // when true, use ASCII spinners, icons, and progress bar chars
	verbose       bool            // when true, show individual layer rows under each image

	dockerClient imageInspector // Docker client for pre-flight and post-pull ImageInspect

	// Pre-flight layer data (populated before pull starts via ImageInspect on local images).
	// imageLayerDiffIDs[imgRef] = ordered RootFS.Layers sha256s from pre-flight inspect.
	// diffIDImageCount[sha256]  = number of images that contain this layer (for sharing).
	// diffIDGroupNum[sha256]    = badge group number (1-based); 0 = unique.
	imageLayerDiffIDs map[string][]string // imgRef → ordered sha256 DiffIDs
	diffIDImageCount  map[string]int      // sha256 → count of images containing it
	diffIDGroupNum    map[string]int      // sha256 → sharing group number (assigned in first-seen order)

	// Pull-event → DiffID positional mapping.
	// Docker sends MANY events per layer (Pulling fs layer → Downloading → Download
	// complete → Extracting → Pull complete), all sharing the same pull-event ID. Layers
	// first appear in RootFS.DiffIDs order, so we assign each NEW pull-event ID to the next
	// DiffID slot and remember that assignment for all subsequent events of that same ID.
	// imagePullIdx[imgRef] = index of next unassigned DiffID slot for that image.
	// pullIDToDiffID[imgRef+":"+pullEventID] = assigned sha256 DiffID (stable across events).
	imagePullIdx   map[string]int    // imgRef → next DiffID slot index
	pullIDToDiffID map[string]string // imgRef+":"+pullEventID → sha256 DiffID

	// Cold-pull layer identity (when pre-flight had no DiffIDs, e.g. after a prune).
	// Layer tasks are keyed by the pull-event rawID during the pull. rawIDs are NOT stable
	// across images for the same layer, so we can't detect cross-image sharing from them.
	// Instead we record each image's rawID arrival order, then AFTER the image finishes
	// pulling we inspect it (now on disk) to get its RootFS DiffIDs and positionally remap
	// each rawID task to its sha256 DiffID — unifying identity with warm images so shared
	// layers collapse onto one sha task and badges/counts work across warm+cold images.
	imagePullOrder map[string][]string // imgRef → rawIDs in first-seen pull order
	rawIDSeen      map[string]bool      // imgRef+":"+rawID → already recorded in order

	// Post-pull inspect (for images not already local at start, or to confirm).
	inspectWG       sync.WaitGroup
	inspectedImages map[string]bool
}

// NewConsoleEventProcessor creates a themed live-updating EventProcessor for TTY output.
// imageServices maps image name (e.g. "lscr.io/linuxserver/plex:latest") to the list of
// service names that use it, so service headers can be shown before lifecycle events arrive.
// imageOrder is the stable key order for imageServices (caller must provide sorted/deterministic order).
func NewConsoleEventProcessor(logCtx context.Context, out io.Writer, command string, imageServices map[string][]string, imageOrder []string, containerToService map[string]string, projectName string, asciiMode bool, verbose bool, updateFn func([]string), dockerClient imageInspector) api.EventProcessor {
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
		startTime:          time.Now(),
		dockerClient:      dockerClient,
		imageLayerDiffIDs: make(map[string][]string),
		diffIDImageCount:  make(map[string]int),
		diffIDGroupNum:    make(map[string]int),
		imagePullIdx:      make(map[string]int),
		pullIDToDiffID:    make(map[string]string),
		inspectedImages:   make(map[string]bool),
		imagePullOrder:    make(map[string][]string),
		rawIDSeen:         make(map[string]bool),
	}
}

func (p *consoleEventProcessor) Start(ctx context.Context, operation string) {
	p.ctx = ctx
	p.operation = operation

	// Pre-flight: inspect all images to get RootFS DiffIDs before pull events arrive.
	p.preflight(ctx)

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
			vp.SetHeader(p.withSummaryTimer(summaryLine))
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

	tickInterval := time.Duration(console.SpinnerSpeed) * time.Millisecond
	if tickInterval <= 0 {
		tickInterval = 100 * time.Millisecond
	}
	p.ticker = time.NewTicker(tickInterval)
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
	// Stop the ticker goroutine first.
	p.doneCh <- struct{}{}
	p.mtx.Lock()
	if p.ticker != nil {
		p.ticker.Stop()
	}
	p.mtx.Unlock()

	// Wait for all post-pull ImageInspect goroutines to complete, then do positional
	// mapping of layer task IDs → true sha256s now that all layer events have arrived.
	p.inspectWG.Wait()
	p.mtx.Lock()
	p.mtx.Unlock()
	p.render()

	// Deactivate the viewport first — leaves alt screen, dumps history to main screen.
	// logSummary must run after so buildLines sees the viewport as inactive and
	// prependSummary includes the summary header the same way in all paths.
	if !p.noViewport {
		if vp := console.GlobalViewport; vp != nil {
			// Prepend the summary to lastComposeLines so it appears in the scrollback
			// dump. The live header was shown via SetHeader; now bake it into the lines.
			termW := goterm.Width()
			if termW <= 0 {
				termW = 80
			}
			finalLines := append([]string{p.withSummaryTimer(p.buildSummaryLine())}, p.buildLines(termW)...)
			vp.UpdateLines(finalLines)
			vp.Deactivate()
		}
	}

	p.logSummary()
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
			assigned = fmt.Sprintf("<Unknown%d>", p.unknownCount)
			p.unknownContainers[id] = assigned
			id = assigned
		}
	}

	// Positional mapping: remap pull-event layer IDs to sha256 DiffID task keys.
	// A single layer produces many events sharing one pull-event ID. Assign each NEW
	// pull-event ID to the next DiffID slot (layers first appear in RootFS order) and
	// reuse that assignment for all later events of the same ID, so a layer's whole
	// progression lands on one DiffID row instead of being smeared across several.
	// Shared layers (same id across images) intentionally map to ONE task and show under
	// each image — see the regression guard below for late re-pull events from a 2nd image.
	if parentID != "" {
		diffIDs := p.imageLayerDiffIDs[parentID]
		if len(diffIDs) > 0 {
			mapKey := parentID + ":" + id
			if sha, ok := p.pullIDToDiffID[mapKey]; ok {
				id = sha // already assigned this pull-event ID
			} else {
				idx := p.imagePullIdx[parentID]
				if idx < len(diffIDs) {
					sha := diffIDs[idx]
					p.imagePullIdx[parentID] = idx + 1
					p.pullIDToDiffID[mapKey] = sha
					id = sha
				}
			}
		} else {
			// Cold-pull path: no pre-flight DiffIDs. Record this rawID's first-seen position
			// for this image so post-pull inspect can remap it to its sha256 DiffID. id is rawID.
			seenKey := parentID + ":" + id
			if !p.rawIDSeen[seenKey] {
				p.rawIDSeen[seenKey] = true
				p.imagePullOrder[parentID] = append(p.imagePullOrder[parentID], id)
			}
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

	// Update task state.
	// Text/status always reflect the latest event — a shared layer may legitimately move
	// through phases again for another image. But progress (sizes/bar) is NEVER cleared:
	// total/current/percent only ever advance, so a sizeless re-pull event from a second
	// image can't blank a row that already showed download progress.
	if e.Text != "" {
		t.text = e.Text
		t.status = e.Status
	}
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

	// Post-pull inspect for images where pre-flight had no local data.
	if t.completed() && contains(p.imageIDs, id) && p.dockerClient != nil && !p.inspectedImages[id] {
		p.inspectedImages[id] = true
		p.inspectWG.Add(1)
		go func(imgRef string) {
			defer p.inspectWG.Done()
			info, err := p.dockerClient.ImageInspect(context.Background(), imgRef)
			if err != nil {
				return
			}
			p.mtx.Lock()
			p.applyInspectResult(imgRef, info.RootFS.Layers)
			p.mtx.Unlock()
		}(id)
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
				vp.SetHeader(p.withSummaryTimer(p.buildSummaryLine()))
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
		buf.WriteString(strutil.Repeat(" ", termW))
		buf.WriteString("\033[K\n")
		rendered++
	}

	buf.WriteString("\033[?25h") // show cursor
	fmt.Fprint(p.out, buf.String())
	p.numLines = len(lines)
	_ = termH
}

// preflight calls ImageInspect on all known images concurrently before the pull starts,
// populating imageLayerDiffIDs, diffIDImageCount, and diffIDGroupNum so layer rows and
// sharing badges are available from the first render.
func (p *consoleEventProcessor) preflight(ctx context.Context) {
	if p.dockerClient == nil {
		return
	}
	type result struct {
		imgRef string
		layers []string
	}
	ch := make(chan result, len(p.imageOrder))
	for _, imgRef := range p.imageOrder {
		go func(ref string) {
			info, err := p.dockerClient.ImageInspect(ctx, ref)
			if err != nil {
				ch <- result{ref, nil}
				return
			}
			ch <- result{ref, info.RootFS.Layers}
		}(imgRef)
	}
	for range p.imageOrder {
		r := <-ch
		if len(r.layers) > 0 {
			p.imageLayerDiffIDs[r.imgRef] = r.layers
			for _, sha := range r.layers {
				p.diffIDImageCount[sha]++
			}
		}
	}
	// Assign group numbers to shared DiffIDs in imageOrder × layer order.
	nextGroup := 1
	for _, imgRef := range p.imageOrder {
		for _, sha := range p.imageLayerDiffIDs[imgRef] {
			if p.diffIDImageCount[sha] > 1 {
				if _, seen := p.diffIDGroupNum[sha]; !seen {
					p.diffIDGroupNum[sha] = nextGroup
					nextGroup++
				}
			}
		}
	}
	// Pre-populate layer tasks from DiffIDs so rows appear before pull events arrive.
	// Keyed by sha256 — no parentID needed; display iterates imageLayerDiffIDs per image.
	// Default to Already exists/Done — safe assumption since layer updates always apply.
	for _, imgRef := range p.imageOrder {
		for _, sha := range p.imageLayerDiffIDs[imgRef] {
			if _, exists := p.tasks[sha]; !exists {
				p.tasks[sha] = &consoleTask{
					id:        sha,
					startTime: time.Now(),
					text:      "Already exists",
					status:    api.Done,
					percent:   100,
				}
				p.ids = append(p.ids, sha)
			}
		}
	}
}

// applyInspectResult runs after a cold-pull image finishes pulling (it's now on disk).
// It positionally remaps that image's rawID-keyed layer tasks to their sha256 DiffIDs,
// unifying identity with warm images so shared layers collapse onto one sha task. This is
// how dup badges/counts work across mixed warm+cold images: rawIDs differ per image for
// the same layer, but DiffIDs match. Existing progress is preserved (merged into the sha
// task, or the rawID task is rekeyed in place).
func (p *consoleEventProcessor) applyInspectResult(imgRef string, layers []string) {
	if len(layers) == 0 || len(p.imageLayerDiffIDs[imgRef]) > 0 {
		return // warm image (already had DiffIDs) or nothing to do
	}
	order := p.imagePullOrder[imgRef]
	p.imageLayerDiffIDs[imgRef] = layers

	// Alignment: the SDK drops "Already exists" events for cached layers, so `order` (the
	// rawIDs we actually saw download) only contains this image's NON-cached layers, while
	// `layers` is the full RootFS DiffID list. The cached layers are exactly the DiffIDs that
	// already have a sha task (owned by a warm image that shares them). So the downloaded
	// rawIDs map, in order, to the subsequence of DiffIDs that have NO existing sha task.
	var downloadSlots []string // DiffIDs (sha) not yet owned — these were downloaded
	for _, sha := range layers {
		if _, ok := p.tasks[sha]; !ok {
			downloadSlots = append(downloadSlots, sha)
		}
	}
	n := len(order)
	if len(downloadSlots) < n {
		n = len(downloadSlots)
	}
	for i := 0; i < n; i++ {
		rawID, sha := order[i], downloadSlots[i]
		rawTask := p.tasks[rawID]
		if rawTask == nil {
			continue
		}
		// Rekey the downloaded rawID task in place to its sha DiffID.
		rawTask.id = sha
		delete(p.tasks, rawID)
		p.tasks[sha] = rawTask
		for j, x := range p.ids {
			if x == rawID {
				p.ids[j] = sha
				break
			}
		}
	}

	// Recompute sharing counts + badge group numbers from unified sha identity.
	p.recomputeDiffIDSharing()
}

// recomputeDiffIDSharing rebuilds diffIDImageCount and diffIDGroupNum from imageLayerDiffIDs,
// so dup counts and badge numbers reflect all images currently known (warm + remapped cold).
func (p *consoleEventProcessor) recomputeDiffIDSharing() {
	p.diffIDImageCount = make(map[string]int)
	for _, imgRef := range p.imageOrder {
		for _, sha := range p.imageLayerDiffIDs[imgRef] {
			p.diffIDImageCount[sha]++
		}
	}
	p.diffIDGroupNum = make(map[string]int)
	nextGroup := 1
	for _, imgRef := range p.imageOrder {
		for _, sha := range p.imageLayerDiffIDs[imgRef] {
			if p.diffIDImageCount[sha] > 1 {
				if _, seen := p.diffIDGroupNum[sha]; !seen {
					p.diffIDGroupNum[sha] = nextGroup
					nextGroup++
				}
			}
		}
	}
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
