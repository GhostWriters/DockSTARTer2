package compose

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
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

// buildLines renders the full output. Layer rows are always processed; showLayers
// controls whether they're included — the console passes p.verbose, the final log
// passes true so layers always appear there regardless of the -v flag.
func (p *consoleEventProcessor) buildLines(termW int, showLayers bool) []string {
	if p.isTeardownCommand() {
		return p.buildTeardownLines(termW)
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
	svcIcon, svcStatusTag, svcStatusText, _ := p.sectionRollupWithPropagation(svcRollupIDs, func(id string) string { return svcImageMap[id] })
	svcStatusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(svcStatusText))
	var lines []string
	var timers []timerEntry

	appendLine := func(line string, t *consoleTask, style timerStyle) {
		lines = append(lines, line)
		timers = append(timers, timerEntry{task: t, style: style})
	}
	appendError := func(t *consoleTask) {
		if line := p.errorLine(t, termW); line != "" {
			appendLine(line, nil, timerLayer)
		}
	}

	// ── services: section ──────────────────────────────────────────────────
	appendLine(globalIndent+svcIcon+" "+svcStatusTag+"{{[-]}}"+svcStatusPad+"{{[white::B]}}services{{[-]}}{{|DockerColon|}}:{{[-]}}", p.serviceSectionTask(svcRollupIDs), timerSection)

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

	coveredSvcs := make(map[string]bool)

	for _, imgName := range p.imageOrder {
		svcs := p.serviceIDsForImage(imgName)
		for _, s := range svcs {
			coveredSvcs[s] = true
		}
		img := p.tasks[imgName]

		for _, svc := range svcs {
			t := p.tasks[svc]
			nameTag := dockerlayout.StyleServiceName(svc)
			var statusText, statusTag, icon string
			if t == nil {
				if img != nil && p.allLayersAlreadyExist(imgName) {
					statusText, statusTag, icon = p.cachedStatus(isPullingText(img.text))
				} else if img != nil {
					statusText = abbreviateStatus(img.text)
					statusTag = imageStatusTag(img.status, img.text, p.command)
					icon = p.spinnerIcon(img)
				} else if p.command == "pull" && p.imageFetched(imgName) {
					statusText, statusTag, icon = p.cachedStatus(false)
				} else {
					impliedText, impliedTag := p.impliedStatus()
					statusText = impliedText
					statusTag = impliedTag + statusText + "{{[-]}}"
					if impliedTag == "{{|DockerStatusPending|}}" {
						icon = "{{|DockerStatusPending|}}" + p.icons().pending + "{{[-]}}"
					} else {
						icon = "{{|DockerMarkerDone|}}" + p.icons().done + "{{[-]}}"
					}
				}
			} else {
				statusText, statusTag, icon = p.serviceRowStatus(svc, imgName, t)
			}
			statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
			appendLine(globalIndent+icon+" "+statusTag+"{{[-]}}"+statusPad+sectionChildIndent+nameTag+"{{|DockerColon|}}:{{[-]}}", p.serviceTimerTask(svc), timerService)
			appendError(p.tasks[svc])
		}

		layers := p.layersForImage(imgName)

		appendLine(p.buildImageLine(imgName, img, layers, maxImgNameW, termW), img, timerImage)
		appendError(img)

		if showLayers {
			layerLines, layerTasks := p.buildLayerLines(layers, maxImgNameW, termW)
			for i, line := range layerLines {
				appendLine(line, layerTasks[i], timerLayer)
				appendError(layerTasks[i])
			}
		}
	}

	// Orphan services with no image group.
	for _, svcID := range p.serviceIDs {
		if coveredSvcs[svcID] || looksLikeImageName(svcID) || contains(p.imageIDs, svcID) {
			continue
		}
		t := p.tasks[svcID]
		nameTag := dockerlayout.StyleServiceName(svcID)
		var statusText, statusTag, icon string
		if t == nil {
			impliedText, impliedTag := p.impliedStatus()
			statusText = impliedText
			statusTag = impliedTag + statusText + "{{[-]}}"
			if impliedTag == "{{|DockerStatusPending|}}" {
				icon = "{{|DockerStatusPending|}}·{{[-]}}"
			} else {
				icon = "{{|DockerMarkerDone|}}✓{{[-]}}"
			}
		} else {
			statusText = abbreviateStatus(t.text)
			statusTag = serviceStatusTag(t.status, t.text, p.command)
			icon = p.propagatedIcon(t, t.status)
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		appendLine(globalIndent+icon+" "+statusTag+"{{[-]}}"+statusPad+sectionChildIndent+nameTag+"{{|DockerColon|}}:{{[-]}}", p.serviceTimerTask(svcID), timerService)
		appendError(p.tasks[svcID])
	}

	netLines, netTimers := p.buildNetworkLines(termW)
	for i, line := range netLines {
		appendLine(line, netTimers[i].task, netTimers[i].style)
	}
	volLines, volTimers := p.buildVolumeLines(termW)
	for i, line := range volLines {
		appendLine(line, volTimers[i].task, volTimers[i].style)
	}

	fitErrorLines(lines)
	return p.prependSummary(lines, timers)
}

// errorIndentW is the column of a row's error message: two past the service name column.
const errorIndentW = globalIndentW + iconW + spaceW + sectionStatusW + sectionChildIndentW + 2

// errorLinePrefix starts every line errorLine returns, so fitErrorLines can find them.
var errorLinePrefix = strutil.Repeat(" ", errorIndentW) + "{{|DockerStatusFail|}}"

// errorLine returns the line under a failed row showing its message on one line,
// without Docker's "Error response from daemon: " prefix (the row already says
// Error) and cut to fit termW, or "" when t has no message. fitErrorLines later
// cuts it to the table's width.
func (p *consoleEventProcessor) errorLine(t *consoleTask, termW int) string {
	if t == nil || t.status != api.Error || t.message == "" {
		return ""
	}
	msg := strings.Join(strings.Fields(t.message), " ")
	msg = strings.TrimPrefix(msg, "Error response from daemon: ")
	return errorLinePrefix + cutRunes(msg, termW-errorIndentW-timerReserveW-summaryHeaderIndentW) + "{{[-]}}"
}

// fitErrorLines cuts the error lines in lines to the width of the other lines, so
// a message never pushes the timer column right.
func fitErrorLines(lines []string) {
	width := 0
	for _, line := range lines {
		if !strings.HasPrefix(line, errorLinePrefix) {
			width = max(width, utf8.RuneCountInString(semstyle.ToPlain(line)))
		}
	}
	for i, line := range lines {
		if msg, ok := strings.CutPrefix(line, errorLinePrefix); ok {
			msg = strings.TrimSuffix(msg, "{{[-]}}")
			lines[i] = errorLinePrefix + cutRunes(msg, width-errorIndentW) + "{{[-]}}"
		}
	}
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
// or the call has returned.
func (p *consoleEventProcessor) cachedStatus(checking bool) (text, tag, icon string) {
	if checking && !p.finished {
		return "Cached", "{{|DockerStatusActive|}}Cached{{[-]}}", p.activeSpinnerTag(p.icons().spinner)
	}
	return "Cached", "{{|DockerStatusFinal|}}Cached{{[-]}}", "{{|DockerMarkerDone|}}" + p.icons().done + "{{[-]}}"
}

// serviceRowStatus returns a service row's status, showing Cached in place of
// Pulling or Pulled when its image needed no download.
func (p *consoleEventProcessor) serviceRowStatus(svc, imgName string, t *consoleTask) (text, tag, icon string) {
	if p.allLayersAlreadyExist(imgName) {
		switch t.text {
		case api.StatusPulling, api.StatusBuilding:
			return p.cachedStatus(true)
		case api.StatusPulled:
			return p.cachedStatus(false)
		}
	}
	return abbreviateStatus(t.text), serviceStatusTag(t.status, t.text, p.command), p.propagatedIcon(t, p.worstServiceStatus(svc, imgName))
}

// layersForImage returns the ordered layer tasks for an image: from pre-flight/remapped
// DiffIDs when present, else the parentID fallback for images with no DiffID data.
func (p *consoleEventProcessor) layersForImage(imgName string) []*consoleTask {
	var layers []*consoleTask
	for _, sha := range p.imageLayerDiffIDs[imgName] {
		if t := p.tasks[sha]; t != nil {
			layers = append(layers, t)
		}
	}
	if len(layers) == 0 {
		for _, id := range p.ids {
			t := p.tasks[id]
			if t.parentID == imgName {
				layers = append(layers, t)
			}
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
	imgStr := styleImage(imgName)
	imgNameW := utf8.RuneCountInString(imgName)

	// Layer count is appended directly to the image URL (e.g. "radarr:latest [9/2]").
	// Brackets use the image-tag color (DockerTag); the count interior is dim.
	layerCount := ""
	countW := 0
	if inner := p.layerCountText(layers); inner != "" {
		layerCount = " {{|DockerTag|}}[{{[-]}}{{[::D]}}" + inner + "{{[-]}}{{|DockerTag|}}]{{[-]}}"
		countW = 1 + 1 + len(inner) + 1 // " " + "[" + inner + "]"
	}
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
		statusText, statusTag, icon := p.cachedStatus(t != nil && isPullingText(t.text))
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
	lines := []string{globalIndent + netIcon + " " + netStatusTag + "{{[-]}}" + netStatusPad + "{{[white::B]}}networks{{[-]}}{{|DockerColon|}}:{{[-]}}"}
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
		if line := p.errorLine(t, termW); line != "" {
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
	lines := []string{globalIndent + volIcon + " " + volStatusTag + "{{[-]}}" + volStatusPad + "{{[white::B]}}volumes{{[-]}}{{|DockerColon|}}:{{[-]}}"}
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
		if line := p.errorLine(t, termW); line != "" {
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

	idPad := (imageSizesColBase + maxImgNameW) - (layerSizesColBase + statusW + spaceW + idW + badgeW)
	if idPad < 1 {
		idPad = 1
	}
	barReset := ""
	if bar != "" {
		barReset = "{{[-]}}"
	}

	return "{{[::D]}}" + layerPrefix + icon + " " + statusTag + "{{[-]}}{{[::D]}}" + statusPad + " " + id + badge + strutil.Repeat(" ", idPad) + sizes + barReset + bar + "{{[-]}}"
}

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

		out = append(out, p.buildLayerLine(t, layerStatusW, maxImgNameW, layerPcts, p.diffIDGroupNum[t.id]))
		if t.text == "Already exists" && t.total == 0 {
			outTasks = append(outTasks, nil) // never downloaded, so no time to show
		} else {
			outTasks = append(outTasks, t)
		}
	}
	return out, outTasks
}

func (p *consoleEventProcessor) buildTeardownLines(termW int) []string {
	impliedText, impliedTag := p.impliedStatus()
	ic := p.icons()
	var impliedIcon string
	if impliedTag == "{{|DockerStatusPending|}}" {
		impliedIcon = "{{|DockerStatusPending|}}" + ic.pending + "{{[-]}}"
	} else {
		impliedIcon = "{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}"
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
	svcIcon, svcStatusTag, svcStatusText, _ := p.sectionRollupWithPropagation(svcRollupIDs, func(id string) string { return svcImageMap[id] })
	svcStatusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(svcStatusText))

	var lines []string
	var timers []timerEntry
	appendLine := func(line string, t *consoleTask, style timerStyle) {
		lines = append(lines, line)
		timers = append(timers, timerEntry{task: t, style: style})
	}
	appendError := func(t *consoleTask) {
		if line := p.errorLine(t, termW); line != "" {
			appendLine(line, nil, timerLayer)
		}
	}

	appendLine(globalIndent+svcIcon+" "+svcStatusTag+"{{[-]}}"+svcStatusPad+"{{[white::B]}}services{{[-]}}{{|DockerColon|}}:{{[-]}}", p.serviceSectionTask(svcRollupIDs), timerSection)

	seenSvcs := make(map[string]bool)
	for _, imgName := range p.imageOrder {
		for _, svc := range p.serviceIDsForImage(imgName) {
			if seenSvcs[svc] {
				continue
			}
			seenSvcs[svc] = true
			t := p.tasks[svc]
			nameTag := dockerlayout.StyleServiceName(svc)
			var icon, statusText, statusTag string
			if t == nil {
				icon, statusTag, statusText = impliedIcon, impliedTag+impliedText+"{{[-]}}", impliedText
			} else {
				statusText, statusTag, icon = p.serviceRowStatus(svc, imgName, t)
			}
			statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
			appendLine(globalIndent+icon+" "+statusTag+"{{[-]}}"+statusPad+sectionChildIndent+nameTag+"{{|DockerColon|}}:{{[-]}}", p.serviceTimerTask(svc), timerService)
			appendError(p.tasks[svc])
		}
	}
	for _, svcID := range p.serviceIDs {
		if seenSvcs[svcID] {
			continue
		}
		seenSvcs[svcID] = true
		t := p.tasks[svcID]
		nameTag := dockerlayout.StyleServiceName(svcID)
		var icon, statusText, statusTag string
		if t == nil {
			icon, statusTag, statusText = impliedIcon, impliedTag+impliedText+"{{[-]}}", impliedText
		} else {
			statusText = abbreviateStatus(t.text)
			statusTag = serviceStatusTag(t.status, t.text, p.command)
			icon = p.propagatedIcon(t, t.status)
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		appendLine(globalIndent+icon+" "+statusTag+"{{[-]}}"+statusPad+sectionChildIndent+nameTag+"{{|DockerColon|}}:{{[-]}}", p.serviceTimerTask(svcID), timerService)
		appendError(p.tasks[svcID])
	}

	netLines, netTimers := p.buildNetworkLines(termW)
	for i, line := range netLines {
		appendLine(line, netTimers[i].task, netTimers[i].style)
	}
	volLines, volTimers := p.buildVolumeLines(termW)
	for i, line := range volLines {
		appendLine(line, volTimers[i].task, volTimers[i].style)
	}

	fitErrorLines(lines)
	return p.prependSummary(lines, timers)
}
