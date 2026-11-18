package compose

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

func (p *consoleEventProcessor) isTeardownCommand() bool {
	switch p.command {
	case "down", "stop", "kill", "pause", "unpause", "restart":
		return true
	}
	return false
}

// buildLines renders the full output at verbosity level: below
// dockerlayout.VerboseDetails, a service's health and dependency lines show only while
// pending or with a problem, and an image's info lines only what a pull changed; at
// VerboseDetails, all of them; at VerboseLayers, the layers too. The console passes
// p.verbosity, the final log VerboseLayers, so the log has everything regardless of -v.
func (p *consoleEventProcessor) buildLines(termW int, level int) []string {
	p.buildLevel = level
	if p.isTeardownCommand() {
		return p.buildTeardownLines(termW, level)
	}
	showLayers := level >= dockerlayout.VerboseLayers

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
	svcIcon, svcStatusTag, svcStatusText, _ := p.sectionRollupWithPropagation(svcRollupIDs, func(id string) string { return svcImageMap[id] })
	svcStatusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(svcStatusText))
	var lines []string
	var timers []timerEntry

	appendLine := func(line string, t *consoleTask, style timerStyle) {
		lines = append(lines, line)
		timers = append(timers, timerEntry{task: t, style: style})
	}
	appendError := func(t *consoleTask) {
		if line := p.errorLine(t); line != "" {
			appendLine(line, nil, timerLayer)
		}
	}

	// ── services: section ──────────────────────────────────────────────────
	// A job -c run runs with no service dependencies has none to show. Grouped, the
	// changed: and unchanged: headings take the sections' place.
	grouped := p.groupsRows()
	if !grouped && (len(svcRollupIDs) > 0 || p.runJob == "") {
		appendLine(globalIndent+svcIcon+" "+svcStatusTag+"{{[-]}}"+svcStatusPad+"{{|DockerSection|}}services{{[-]}}{{|DockerColon|}}:{{[-]}}", p.serviceSectionTask(svcRollupIDs), timerSection)
	}

	maxImgNameW := 0
	for _, imgName := range p.imageOrder {
		if w := utf8.RuneCountInString(imgName); w > maxImgNameW {
			maxImgNameW = w
		}
	}
	// Widest layer-count suffix (e.g. " [9/2]") so the size/bar columns stay aligned
	// when the count is appended directly to each image URL.
	maxCountW := 0
	for _, imgName := range p.imageOrder {
		if w := p.layerCountWidth(imgName); w > maxCountW {
			maxCountW = w
		}
	}
	maxImgNameW += maxCountW

	// Each image's block (its services, then the image) or service with no image goes
	// to the changed rows when something happened to it, or else to the group of its
	// state, the groups following in groupOrder. Folded, a group is one line of names;
	// open (-v), its blocks.
	open := level >= dockerlayout.VerboseDetails
	type block struct {
		img  string   // "" for a service with no image
		svcs []string // its services
	}
	var changed []block
	groups := map[string][]block{} // group word → its blocks

	renderBlock := func(b block) {
		if b.img == "" {
			p.appendOrphanRow(b.svcs[0], termW, level, appendLine, appendError)
			return
		}
		imgName := b.img
		img := p.tasks[imgName]
		// Each service's row: its status, then its error and detail lines.
		var rows []serviceRow
		for _, svc := range b.svcs {
			t := p.tasks[svc]
			var statusText, statusTag, icon string
			if t == nil {
				if img != nil && p.allLayersAlreadyExist(imgName) {
					statusText, statusTag, icon = p.cachedStatus(isPullingText(img.text), serviceFinalTag)
				} else if img != nil {
					// The row shows its image's status, styled as a service row's.
					statusText = abbreviateStatus(img.text)
					statusTag = serviceStatusTag(img.status, img.text, p.command)
					icon = p.spinnerIcon(img)
				} else if p.command == "pull" && p.imageFetched(imgName) {
					statusText, statusTag, icon = p.cachedStatus(false, serviceFinalTag)
				} else {
					impliedText, impliedTag := p.impliedStatus()
					statusText = impliedText
					statusTag = impliedTag + statusText + "{{[-]}}"
					icon = p.impliedIcon(impliedTag)
				}
			} else {
				statusText, statusTag, icon = p.serviceRowStatus(svc, imgName, t)
			}
			var lines []string
			if line := p.errorLine(t); line != "" {
				lines = append(lines, line)
			}
			lines = append(lines, p.serviceDetailLines(svc, termW, level, false)...)
			rows = append(rows, serviceRow{svc: svc, statusText: statusText, statusTag: statusTag, icon: icon, lines: lines})
		}
		p.appendServiceRows(rows, level, termW, appendLine)

		layers := p.layersForImage(imgName)

		appendLine(p.buildImageLine(imgName, img, layers, maxImgNameW, termW), img, timerImage)
		appendError(img)
		for _, line := range p.imageInfoLines(imgName, termW, level >= dockerlayout.VerboseDetails) {
			appendLine(line, nil, timerLayer)
		}

		if showLayers {
			if len(layers) > 0 {
				appendLine(p.layersLine(layers), nil, timerLayer)
			}
			layerLines, layerTasks := p.buildLayerLines(layers, maxImgNameW, termW)
			for i, line := range layerLines {
				appendLine(line, layerTasks[i], timerLayer)
				appendError(layerTasks[i])
			}
		}
	}

	// A block moves only as a whole: to its own rows if any of its services has, else
	// to the earliest group (in groupOrder) of its services'.
	place := func(b block) {
		if !grouped {
			renderBlock(b) // every row in its place
			return
		}
		best := len(groupOrder)
		for _, svc := range b.svcs {
			word := p.serviceGroupWord(svc, b.img, termW)
			if word == "" {
				changed = append(changed, b)
				return
			}
			best = min(best, slices.Index(groupOrder, word))
		}
		groups[groupOrder[best]] = append(groups[groupOrder[best]], b)
	}
	coveredSvcs := make(map[string]bool)
	for _, imgName := range p.imageOrder {
		svcs := p.serviceIDsForImage(imgName)
		for _, s := range svcs {
			coveredSvcs[s] = true
		}
		place(block{img: imgName, svcs: svcs})
	}
	for _, svcID := range p.serviceIDs {
		if !coveredSvcs[svcID] && !looksLikeImageName(svcID) && !contains(p.imageIDs, svcID) {
			place(block{svcs: []string{svcID}})
		}
	}
	if grouped {
		// The services' changed set: the changed blocks; their unchanged set: the rest in
		// groupOrder, each group one folded line of names, or with -v their blocks.
		var unchanged []block
		for _, word := range groupOrder {
			unchanged = append(unchanged, groups[word]...)
		}
		// capture returns the lines f appends, taking them back off.
		capture := func(f func()) ([]string, []timerEntry) {
			start := len(lines)
			f()
			l, t := slices.Clone(lines[start:]), slices.Clone(timers[start:])
			lines, timers = lines[:start], timers[:start]
			return l, t
		}
		svcSet := func(blocks []block, render func()) groupSet {
			var ids []string
			for _, b := range blocks {
				ids = append(ids, b.svcs...)
			}
			set := groupSet{count: len(ids)}
			if len(ids) == 0 {
				return set
			}
			set.state = p.rollupState(ids, func(id string) string { return svcImageMap[id] })
			set.timer = p.serviceSectionTask(ids)
			set.lines, set.timers = capture(render)
			return set
		}
		renderAll := func(blocks []block) func() {
			return func() {
				for _, b := range blocks {
					renderBlock(b)
				}
			}
		}
		services := kindGroups{singular: "service", plural: "services"}
		services.changed = svcSet(changed, renderAll(changed))
		if open {
			services.unchanged = svcSet(unchanged, renderAll(unchanged))
		} else {
			services.unchanged = svcSet(unchanged, func() {
				for _, word := range groupOrder {
					var names []string
					for _, b := range groups[word] {
						names = append(names, b.svcs...)
					}
					if len(names) == 0 {
						continue
					}
					slices.Sort(names)
					notRunning := word != api.StatusRunning && word != cachedText
					// Services with possible port conflicts fold apart, one group per
					// identical set of them, each with its unconfirmed ports line.
					plain, possible := p.splitPossible(names)
					if len(plain) > 0 {
						for _, line := range p.groupLines(word, notRunning, plain, "service", "services", termW) {
							appendLine(line, nil, timerLayer)
						}
					}
					for _, g := range possible {
						for _, line := range p.groupLines(word, notRunning, g.members, "service", "services", termW) {
							appendLine(line, nil, timerLayer)
						}
						for _, line := range p.possibleGroupLine(g, termW) {
							appendLine(line, nil, timerLayer)
						}
					}
				}
			})
		}
		kinds := []kindGroups{services}
		if p.groupsResources() {
			kinds = append(kinds,
				p.resourceGroups(networkKind, p.networks, p.networkIDs, termW, level),
				p.resourceGroups(volumeKind, p.volumes, p.volumeIDs, termW, level))
		}
		kinds = append(kinds, p.kindGroups(p.modelKind(), termW, level), p.jobGroups(termW, level), p.kindGroups(p.orphanKind(), termW, level))
		groupLines, groupTimers := p.groupedLines(kinds)
		for i, line := range groupLines {
			appendLine(line, groupTimers[i].task, groupTimers[i].style)
		}
	}

	var sections [][]string
	var sectionTimers [][]timerEntry
	if !p.groupsResources() {
		netLines, netTimers := p.buildNetworkLines(termW)
		volLines, volTimers := p.buildVolumeLines(termW)
		sections = append(sections, netLines, volLines)
		sectionTimers = append(sectionTimers, netTimers, volTimers)
	}
	if !grouped {
		modelLines, modelTimers := p.kindSection(p.modelKind(), termW, level)
		jobLines, jobTimers := p.buildJobLines(termW, level)
		orphanLines, orphanTimers := p.kindSection(p.orphanKind(), termW, level)
		sections = append(sections, modelLines, jobLines, orphanLines)
		sectionTimers = append(sectionTimers, modelTimers, jobTimers, orphanTimers)
	}
	for s, section := range sections {
		for i, line := range section {
			appendLine(line, sectionTimers[s][i].task, sectionTimers[s][i].style)
		}
	}

	lines, timers = p.placeErrorLines(lines, timers, termW)
	out := p.prependSummary(lines, timers)
	return append(out, p.jobOutputLines(len(out))...)
}

// appendOrphanRow adds the row of a service with no image block, its error and its
// detail lines.
func (p *consoleEventProcessor) appendOrphanRow(svcID string, termW, level int, appendLine func(string, *consoleTask, timerStyle), appendError func(*consoleTask)) {
	t := p.tasks[svcID]
	nameTag := dockerlayout.StyleServiceName(svcID)
	var statusText, statusTag, icon string
	if t == nil {
		impliedText, impliedTag := p.impliedStatus()
		statusText = impliedText
		statusTag = impliedTag + statusText + "{{[-]}}"
		icon = p.impliedIcon(impliedTag)
	} else {
		statusText, statusTag = p.actionStatus(t)
		icon = p.serviceIcon(t, t.status)
	}
	statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
	appendLine(globalIndent+icon+" "+statusTag+"{{[-]}}"+statusPad+sectionChildIndent+nameTag+"{{|DockerColon|}}:{{[-]}}", p.serviceTimerTask(svcID), timerService)
	appendError(t)
	for _, line := range p.serviceDetailLines(svcID, termW, level, false) {
		appendLine(line, nil, timerLayer)
	}
}

// serviceDetailLines returns the lines under svc's row: its condition lines, its
// possible port conflicts (which, unlike a condition line, don't make the row a
// change), then its model lines, each model's variables only at
// dockerlayout.VerboseDetails.
func (p *consoleEventProcessor) serviceDetailLines(svc string, termW, level int, neededBy bool) []string {
	lines := append(p.serviceConditionLines(svc, termW, level, neededBy), p.predictedLines(svc, termW)...)
	return append(lines, p.modelLines(svc, termW, level >= dockerlayout.VerboseDetails)...)
}

// serviceConditionLines returns svc's healthcheck line, its dependency lines, its ports
// line in a port conflict and, with neededBy, its needed-by line. Below
// dockerlayout.VerboseDetails, only the ones still pending or with a problem.
func (p *consoleEventProcessor) serviceConditionLines(svc string, termW, level int, neededBy bool) []string {
	all := level >= dockerlayout.VerboseDetails
	var lines []string
	if line := p.healthLine(svc, termW); line != "" && (all || p.serviceHealth(svc) != healthHealthy) {
		lines = append(lines, line)
	}
	lines = append(lines, p.dependsLines(svc, termW, all)...)
	lines = append(lines, p.portsLines(svc, termW)...)
	if neededBy {
		lines = append(lines, p.neededByLines(svc, termW, all)...)
	}
	return lines
}

// errorIndentW returns the column of a row's error message: two past the service name column.
func (p *consoleEventProcessor) errorIndentW() int {
	return globalIndentW + iconW + spaceW + sectionStatusW + sectionChildIndentW + 2
}

// errorLinePrefixes start every line errorLine returns, so placeErrorLines can find
// them: a failed row's message, then a warning row's (interrupted or skipped).
func (p *consoleEventProcessor) errorLinePrefixes() []string {
	indent := strutil.Repeat(" ", p.errorIndentW())
	return []string{indent + "{{|DockerStatusFail|}}", indent + "{{|DockerStatusWarn|}}"}
}

// errorLine returns the line under a failed or warning row showing its whole message on
// one line, without Docker's "Error response from daemon: " prefix (the row already
// shows its status), or "" when t has no message. placeErrorLines later fits it to the
// table.
func (p *consoleEventProcessor) errorLine(t *consoleTask) string {
	if t == nil || t.message == "" || t.status != api.Error && t.status != api.Warning {
		return ""
	}
	if p.unhealthyHealthErr(t) {
		return "" // its healthcheck line and its dependents' needs lines say it all
	}
	prefix := p.errorLinePrefixes()[0]
	if t.status == api.Warning {
		prefix = p.errorLinePrefixes()[1]
	}
	return prefix + shortMessage(t.message) + "{{[-]}}"
}

// cutRunes returns s cut to at most maxW runes (at least 1), ending in "…" when cut.
func cutRunes(s string, maxW int) string {
	maxW = max(maxW, 1)
	r := []rune(s)
	if len(r) <= maxW {
		return s
	}
	return string(append(r[:maxW-1], '…'))
}

// isPullingText reports whether text is an image fetch still in progress.
func isPullingText(text string) bool {
	return text == api.StatusPulling || text == api.StatusBuilding
}

// cachedStatus returns the status for a row whose image needed no download:
// active while the SDK is still checking for a newer image, final once it is done
// or the call has returned, in finalTag, its row's final style.
func (p *consoleEventProcessor) cachedStatus(checking bool, finalTag string) (text, tag, icon string) {
	if checking && !p.finished {
		return "Cached", "{{|DockerStatusActive|}}Cached{{[-]}}", p.activeSpinnerTag(p.icons().spinner)
	}
	return "Cached", finalTag + "Cached{{[-]}}", "{{|DockerMarkerDone|}}" + p.icons().done + "{{[-]}}"
}

// serviceRowStatus returns a service row's status, showing Cached in place of
// Pulling or Pulled when its image needed no download, and its container's health
// after its action.
func (p *consoleEventProcessor) serviceRowStatus(svc, imgName string, t *consoleTask) (text, tag, icon string) {
	if p.allLayersAlreadyExist(imgName) {
		switch t.text {
		case api.StatusPulling, api.StatusBuilding:
			return p.cachedStatus(true, serviceFinalTag)
		case api.StatusPulled:
			return p.cachedStatus(false, serviceFinalTag)
		}
	}
	text, tag = p.actionStatus(t)
	return text, tag, p.serviceIcon(t, p.worstServiceStatus(svc, imgName))
}

// layersForImage returns the ordered layer tasks for an image: its pre-flight/remapped
// DiffIDs, then its downloads not yet remapped (an update's new layers while it pulls).
func (p *consoleEventProcessor) layersForImage(imgName string) []*consoleTask {
	var layers []*consoleTask
	for _, sha := range p.imageLayerDiffIDs[imgName] {
		if t := p.tasks[sha]; t != nil {
			layers = append(layers, t)
		}
	}
	for _, rawID := range p.imagePullOrder[imgName] {
		if t := p.tasks[rawID]; t != nil {
			layers = append(layers, t)
		}
	}
	return layers
}

// layerCountText returns the inner text of the layer-count suffix (e.g. "9"),
// or "" when there are no layers. Shared layers are indicated by per-layer badges,
// so the image suffix is just the total count.
func (p *consoleEventProcessor) layerCountText(layers []*consoleTask) string {
	total := len(layers)
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("%d", total)
}

// layerCountWidth returns the visible width of an image's layer-count suffix, including
// the leading space and brackets (e.g. " [9]" -> 4), or 0 when there are no layers.
func (p *consoleEventProcessor) layerCountWidth(imgName string) int {
	inner := p.layerCountText(p.layersForImage(imgName))
	if inner == "" {
		return 0
	}
	return 1 + 1 + len(inner) + 1 // " " + "[" + inner + "]"
}

func (p *consoleEventProcessor) buildImageLine(imgName string, t *consoleTask, layers []*consoleTask, maxImgNameW int, termW int) string {
	imageLabel := strutil.Repeat(" ", 2*sectionChildIndentW) + "{{|DockerMarkerDone|}}image{{[-]}}{{|DockerColon|}}:{{[-]}} "
	// Layer count is appended directly to the image URL (e.g. "radarr:latest [9/2]").
	// Brackets use the image-tag color (DockerTag); the count interior is dim.
	layerCount := ""
	countW := 0
	if inner := p.layerCountText(layers); inner != "" {
		layerCount = " {{|DockerTag|}}[{{[-]}}{{[::D]}}" + inner + "{{[-]}}{{|DockerTag|}}]{{[-]}}"
		countW = 1 + 1 + len(inner) + 1 // " " + "[" + inner + "]"
	}
	imgStr := styleImage(imgName)
	imgNameW := utf8.RuneCountInString(imgName)
	// maxImgNameW already includes the widest count suffix; pad by what this row lacks
	// so the size/bar columns stay aligned across images.
	imgPad := strutil.Repeat(" ", maxImgNameW-imgNameW-countW)
	urlWithCount := imgStr + layerCount

	sizes, bar := p.buildImageSizesAndBar(layers, maxImgNameW, termW)

	allLayersCached := len(layers) > 0
	for _, l := range layers {
		if l.text != "Already exists" {
			allLayersCached = false
			break
		}
	}

	if t == nil || allLayersCached {
		statusText, statusTag, icon := p.cachedStatus(t != nil && isPullingText(t.text), imageFinalTag)
		statusPad := strutil.Repeat(" ", sectionStatusW-len(statusText))
		return globalIndent + icon + " " + statusTag + "{{[-]}}" + statusPad + imageLabel + urlWithCount + imgPad + sizes + bar
	}
	worst := p.worstImageStatus(imgName)
	icon := p.propagatedIcon(t, worst)
	statusText := abbreviateStatus(t.text)
	statusTag := imageStatusTag(t.status, t.text, p.command)
	statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
	return globalIndent + icon + " " + statusTag + "{{[-]}}" + statusPad + imageLabel + urlWithCount + imgPad + sizes + bar
}

func (p *consoleEventProcessor) buildImageSizesAndBar(layers []*consoleTask, maxImgNameW int, termW int) (string, string) {
	if len(layers) == 0 {
		return "", ""
	}

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
		sizes = " " + "{{|DockerMarkerDone|}}" + fixedSize(current) + "{{[-]}}" +
			"{{|DockerColon|}}/{{[-]}}" +
			"{{|DockerMarkerDone|}}" + fixedSize(total) + "{{[-]}}"
	} else {
		sizes = strutil.Repeat(" ", 1+8+1+8)
	}

	usedW := imageSizesColBase + maxImgNameW + spaceW + sizeColW + sizeSepW + sizeColW
	barW := len(layers)
	maxBarW := termW - usedW - timerReserveW - summaryHeaderIndentW
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
	return sizes, " " + renderProgressBarLayers(layerPcts, progressChars, "{{|DockerMarkerDone|}}")
}

func (p *consoleEventProcessor) buildNetworkLines(termW int) ([]string, []timerEntry) {
	// Only surface networks we actually act on — in-use Warnings on teardown are hidden.
	netIDs := p.visibleNetworkIDs()
	if len(netIDs) == 0 {
		return nil, nil
	}
	netIcon, netStatusTag, netStatusText, _ := p.sectionRollupWithPropagation(netIDs, nil)
	netStatusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(netStatusText))
	lines := []string{globalIndent + netIcon + " " + netStatusTag + "{{[-]}}" + netStatusPad + "{{|DockerSection|}}networks{{[-]}}{{|DockerColon|}}:{{[-]}}"}
	timers := []timerEntry{{task: p.sectionTaskFor(netIDs), style: timerSection}}
	for _, netID := range netIDs {
		t := p.tasks[netID]
		nameTag := "{{|IPAddress|}}" + netID + "{{[-]}}"
		var icon, statusText, statusTag string
		if t != nil {
			icon = p.spinnerIcon(t)
			statusText = abbreviateStatus(t.text)
			statusTag = networkStatusTag(t.status, t.text, p.command)
		} else {
			icon = "{{|DockerStatusPending|}}·{{[-]}}"
			statusText = "Pending"
			statusTag = "{{|DockerStatusPending|}}Pending{{[-]}}"
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		lines = append(lines, globalIndent+icon+" "+statusTag+"{{[-]}}"+statusPad+sectionChildIndent+nameTag+"{{|DockerColon|}}:{{[-]}}")
		timers = append(timers, timerEntry{task: t, style: timerService})
		if line := p.errorLine(t); line != "" {
			lines = append(lines, line)
			timers = append(timers, timerEntry{})
		}
	}
	return lines, timers
}

func (p *consoleEventProcessor) buildVolumeLines(termW int) ([]string, []timerEntry) {
	if len(p.volumeIDs) == 0 {
		return nil, nil
	}
	volIcon, volStatusTag, volStatusText, _ := p.sectionRollupWithPropagation(p.volumeIDs, nil)
	volStatusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(volStatusText))
	lines := []string{globalIndent + volIcon + " " + volStatusTag + "{{[-]}}" + volStatusPad + "{{|DockerSection|}}volumes{{[-]}}{{|DockerColon|}}:{{[-]}}"}
	timers := []timerEntry{{task: p.sectionTaskFor(p.volumeIDs), style: timerSection}}
	for _, volID := range p.volumeIDs {
		t := p.tasks[volID]
		nameTag := "{{|Folder|}}" + volID + "{{[-]}}"
		var icon, statusText, statusTag string
		if t != nil {
			icon = p.spinnerIcon(t)
			statusText = abbreviateStatus(t.text)
			statusTag = volumeStatusTag(t.status, t.text, p.command)
		} else {
			icon = "{{|DockerStatusPending|}}·{{[-]}}"
			statusText = "Pending"
			statusTag = "{{|DockerStatusPending|}}Pending{{[-]}}"
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		lines = append(lines, globalIndent+icon+" "+statusTag+"{{[-]}}"+statusPad+sectionChildIndent+nameTag+"{{|DockerColon|}}:{{[-]}}")
		timers = append(timers, timerEntry{task: t, style: timerService})
		if line := p.errorLine(t); line != "" {
			lines = append(lines, line)
			timers = append(timers, timerEntry{})
		}
	}
	return lines, timers
}

func (p *consoleEventProcessor) buildLayerLine(t *consoleTask, statusW int, maxImgNameW int, layerPcts []int, groupNum int) string {
	// DiffIDs are "sha256:<64hex>"; show first 12 hex chars to match Docker convention.
	id := strings.TrimPrefix(t.id, "sha256:")
	if len(id) > 12 {
		id = id[:12]
	}
	idW := utf8.RuneCountInString(id)

	current := t.current
	if t.completed() && t.percent == 100 && t.total > 0 {
		current = t.total
	}

	var sizes string
	if t.total > 0 {
		sizes = " " + "{{[::D]}}" + fixedSize(current) + "{{[-]}}" +
			"{{|DockerColon|}}/{{[-]}}" +
			"{{[::D]}}" + fixedSize(t.total) + "{{[-]}}"
	} else {
		sizes = strutil.Repeat(" ", 1+8+1+8)
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
	displayText := t.text
	// SDK drops "Pull complete" events, so "Extracting" is the last event we receive.
	// While the parent image is still pulling it's genuinely in progress; once the
	// image completes, relabel it as "Extracted" to reflect the finished state.
	if t.text == "Extracting" && t.completed() {
		displayText = "Extracted"
	}
	short := abbreviateStatus(displayText)
	statusPad := ""
	if pad := statusW - utf8.RuneCountInString(short); pad > 0 {
		statusPad = strutil.Repeat(" ", pad)
	}
	statusTag := layerStatusTag(t.status, displayText)

	// Shared-layer badge: [N] in yellow immediately after the layer ID.
	// Shared-layer badge uses parens "(N)" to stay distinct from the image line's "[N]"
	// layer-count suffix. The number is colored DockerSharedLayer (yellow); parens are dim.
	badge := ""
	badgeW := 0
	if groupNum > 0 {
		badge = fmt.Sprintf("{{[::D]}}({{[-]}}{{|DockerSharedLayer|}}%d{{[-]}}{{[::D]}}){{[-]}}", groupNum)
		badgeW = 2 + len(fmt.Sprintf("%d", groupNum)) // "(" + digits + ")"
	}

	// The status shares the main status column; the ID sits one step in under layers:.
	idCol := globalIndentW + iconW + spaceW + sectionStatusW + layerIDIndentW
	idPad := max(imageSizesColBase+maxImgNameW-(idCol+idW+badgeW), 1)
	barReset := ""
	if bar != "" {
		barReset = "{{[-]}}"
	}

	return globalIndent + icon + " " + statusTag + "{{[-]}}" + statusPad + strutil.Repeat(" ", layerIDIndentW) +
		"{{[::D]}}" + id + badge + strutil.Repeat(" ", idPad) + sizes + barReset + bar + "{{[-]}}"
}

// layerIDIndentW is how far a layer's ID sits past the status column: one step in from
// the layers: line under its image.
const layerIDIndentW = 4 * sectionChildIndentW

func (p *consoleEventProcessor) buildLayerLines(layers []*consoleTask, maxImgNameW int, termW int) ([]string, []*consoleTask) {
	if len(layers) == 0 {
		return nil, nil
	}

	// Bar starts after the size column; cap its width so the line stays within termW,
	// leaving timerReserveW chars for the timer column.
	usedW := imageSizesColBase + maxImgNameW + spaceW + sizeColW + sizeSepW + sizeColW
	maxBarW := termW - usedW - timerReserveW - summaryHeaderIndentW
	if maxBarW < 1 {
		maxBarW = 1
	}

	// Per-layer bar WIDTH is proportional to the layer's byte size relative to the
	// largest layer in this image (so a big layer gets a wide bar, a small one a short
	// bar). Each bar's cells are all filled to that layer's own download percent.
	var maxTotal int64
	for _, t := range layers {
		if t.total > maxTotal {
			maxTotal = t.total
		}
	}

	var out []string
	var outTasks []*consoleTask
	for _, t := range layers {
		pct := t.percent
		if t.completed() && pct == 100 {
			pct = 100
		} else if t.total > 0 {
			pct = int(float64(t.current) / float64(t.total) * 100)
		}
		if pct > 100 {
			pct = 100
		}

		// Scale this layer's bar width by its size relative to the largest layer.
		barW := 1
		if maxTotal > 0 && t.total > 0 {
			barW = int(float64(maxBarW) * float64(t.total) / float64(maxTotal))
			if barW < 1 {
				barW = 1
			}
		}
		layerPcts := make([]int, barW)
		for i := range layerPcts {
			layerPcts[i] = pct
		}

		out = append(out, p.buildLayerLine(t, sectionStatusW, maxImgNameW, layerPcts, p.diffIDGroupNum[t.id]))
		if t.text == "Already exists" && t.total == 0 {
			outTasks = append(outTasks, nil) // never downloaded, so no time to show
		} else {
			outTasks = append(outTasks, t)
		}
	}
	return out, outTasks
}

func (p *consoleEventProcessor) buildTeardownLines(termW int, level int) []string {
	impliedText, impliedTag := p.impliedStatus()
	impliedIcon := p.impliedIcon(impliedTag)

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
	svcIcon, svcStatusTag, svcStatusText, _ := p.sectionRollupWithPropagation(svcRollupIDs, func(id string) string { return svcImageMap[id] })
	svcStatusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(svcStatusText))

	var lines []string
	var timers []timerEntry
	appendLine := func(line string, t *consoleTask, style timerStyle) {
		lines = append(lines, line)
		timers = append(timers, timerEntry{task: t, style: style})
	}
	appendError := func(t *consoleTask) {
		if line := p.errorLine(t); line != "" {
			appendLine(line, nil, timerLayer)
		}
	}

	// The services in display order: by image, then those with none.
	var svcs []string
	seenSvcs := make(map[string]bool)
	for _, imgName := range p.imageOrder {
		for _, svc := range p.serviceIDsForImage(imgName) {
			if !seenSvcs[svc] {
				seenSvcs[svc] = true
				svcs = append(svcs, svc)
			}
		}
	}
	for _, svcID := range p.serviceIDs {
		if !seenSvcs[svcID] {
			seenSvcs[svcID] = true
			svcs = append(svcs, svcID)
		}
	}

	// renderRow adds svc's row, its error and its detail lines; word, when not "", is
	// the status of a row left unchanged, its group's: still to be acted on when pending.
	rowStatus := func(svc, word string, pending bool) (icon, statusTag, statusText string) {
		t := p.tasks[svc]
		imgName := svcImageMap[svc]
		switch {
		case word != "" && pending:
			icon, statusTag, statusText = "{{|DockerStatusPending|}}"+p.icons().pending+"{{[-]}}", "{{|DockerStatusPending|}}"+word+"{{[-]}}", word
		case word != "":
			icon, statusTag, statusText = "{{|DockerMarkerDone|}}"+p.icons().done+"{{[-]}}", serviceFinalTag+word+"{{[-]}}", word
		case t == nil:
			icon, statusTag, statusText = impliedIcon, impliedTag+impliedText+"{{[-]}}", impliedText
		case imgName != "":
			statusText, statusTag, icon = p.serviceRowStatus(svc, imgName, t)
		default:
			statusText, statusTag = p.actionStatus(t)
			icon = p.serviceIcon(t, t.status)
		}
		return icon, statusTag, statusText
	}
	renderRow := func(svc, word string, pending bool) {
		icon, statusTag, statusText := rowStatus(svc, word, pending)
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		appendLine(globalIndent+icon+" "+statusTag+"{{[-]}}"+statusPad+sectionChildIndent+dockerlayout.StyleServiceName(svc)+"{{|DockerColon|}}:{{[-]}}", p.serviceTimerTask(svc), timerService)
		appendError(p.tasks[svc])
		for _, line := range p.serviceDetailLines(svc, termW, level, true) {
			appendLine(line, nil, timerLayer)
		}
	}
	// renderChanged draws the changed rows, folding those that show the same thing
	// (appendServiceRows).
	renderChanged := func(svcs []string) {
		rows := make([]serviceRow, 0, len(svcs))
		for _, svc := range svcs {
			icon, statusTag, statusText := rowStatus(svc, "", false)
			var lines []string
			if line := p.errorLine(p.tasks[svc]); line != "" {
				lines = append(lines, line)
			}
			lines = append(lines, p.serviceDetailLines(svc, termW, level, true)...)
			rows = append(rows, serviceRow{svc, icon, statusTag, statusText, lines})
		}
		p.appendServiceRows(rows, level, termW, appendLine)
	}

	if p.groupsRows() {
		lines, timers = p.teardownGroupedLines(svcs, svcImageMap, renderRow, renderChanged, &lines, &timers, termW, level)
	} else {
		appendLine(globalIndent+svcIcon+" "+svcStatusTag+"{{[-]}}"+svcStatusPad+"{{|DockerSection|}}services{{[-]}}{{|DockerColon|}}:{{[-]}}", p.serviceSectionTask(svcRollupIDs), timerSection)
		for _, svc := range svcs {
			word := "" // a service the command left alone keeps its state's word
			if state, seen := p.preRun[svc]; p.tasks[svc] == nil {
				word = teardownSkip(p.command, state, seen)
			}
			renderRow(svc, word, false)
		}
		netLines, netTimers := p.buildNetworkLines(termW)
		volLines, volTimers := p.buildVolumeLines(termW)
		modelLines, modelTimers := p.kindSection(p.modelKind(), termW, level)
		jobLines, jobTimers := p.buildJobLines(termW, level)
		orphanLines, orphanTimers := p.kindSection(p.orphanKind(), termW, level)
		for s, section := range [][]string{netLines, volLines, modelLines, jobLines, orphanLines} {
			sectionTimers := [][]timerEntry{netTimers, volTimers, modelTimers, jobTimers, orphanTimers}[s]
			for i, line := range section {
				appendLine(line, sectionTimers[i].task, sectionTimers[i].style)
			}
		}
	}

	lines, timers = p.placeErrorLines(lines, timers, termW)
	return p.prependSummary(lines, timers)
}

// teardownOrder is the order of a teardown's unchanged groups.
var teardownOrder = []string{api.StatusRunning, pausedText, notRunningText, notCreatedText}

// teardownGroupedLines returns a teardown's rows under changed: and unchanged:: the
// services compose acted on, and the networks, volumes and jobs it removed, changed;
// the services it left (teardownGroupWord) unchanged, each group one folded line of
// names, or with -v their rows. renderRow appends a service's rows to *lines and
// *timers, which start empty, and renderChanged the changed services' rows, folding
// those that show the same thing.
func (p *consoleEventProcessor) teardownGroupedLines(svcs []string, svcImageMap map[string]string, renderRow func(svc, word string, pending bool), renderChanged func(svcs []string), lines *[]string, timers *[]timerEntry, termW, level int) ([]string, []timerEntry) {
	open := level >= dockerlayout.VerboseDetails
	var changed, unchanged []string
	groups := map[string][]string{}
	groupPending := map[string]bool{}
	anyPending := false
	for _, svc := range svcs {
		word, pending := p.teardownGroupWord(svc)
		if word == "" {
			changed = append(changed, svc)
			continue
		}
		groups[word] = append(groups[word], svc)
		groupPending[word] = groupPending[word] || pending
		anyPending = anyPending || pending
	}
	for _, word := range teardownOrder {
		unchanged = append(unchanged, groups[word]...)
	}
	// take returns the lines appended so far, starting over.
	take := func() ([]string, []timerEntry) {
		l, t := *lines, *timers
		*lines, *timers = nil, nil
		return l, t
	}

	services := kindGroups{singular: "service", plural: "services"}
	renderChanged(changed)
	services.changed = groupSet{count: len(changed), state: p.rollupState(changed, func(id string) string { return svcImageMap[id] }), timer: p.serviceSectionTask(changed)}
	services.changed.lines, services.changed.timers = take()

	services.unchanged = groupSet{count: len(unchanged), state: rollupComplete}
	if anyPending {
		services.unchanged.state = rollupPending
	}
	for _, word := range teardownOrder {
		if len(groups[word]) == 0 {
			continue
		}
		if open {
			for _, svc := range groups[word] {
				_, pending := p.teardownGroupWord(svc)
				renderRow(svc, word, pending)
			}
			continue
		}
		names := slices.Sorted(slices.Values(groups[word]))
		for _, line := range p.groupLines(word, groupPending[word], names, "service", "services", termW) {
			*lines = append(*lines, line)
			*timers = append(*timers, timerEntry{})
		}
	}
	services.unchanged.lines, services.unchanged.timers = take()

	return p.groupedLines([]kindGroups{
		services,
		p.resourceGroups(networkKind, resourceDecl{}, p.visibleNetworkIDs(), termW, level),
		p.resourceGroups(volumeKind, resourceDecl{}, p.volumeIDs, termW, level),
		p.kindGroups(p.modelKind(), termW, level),
		p.jobGroups(termW, level),
		p.kindGroups(p.orphanKind(), termW, level),
	})
}
