package compose

import (
	"fmt"
	"unicode/utf8"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
)

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
	allSvcIDs := append(append([]string{}, p.imageOrder...), svcRollupIDs...)
	appendLine(globalIndent+svcIcon+" "+svcStatusANSI+console.CodeReset+svcStatusPad+console.ToConsoleANSI("{{[white::B]}}services{{[-]}}{{|DockerColon|}}:{{[-]}}"), p.sectionTaskFor(allSvcIDs), timerSection)

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
					statusANSI = console.ToConsoleANSI(imageStatusTag(img.status, img.text, p.command))
					icon = p.spinnerIcon(img)
				} else {
					impliedText, impliedTag := p.impliedStatus()
					statusText = impliedText
					statusANSI = console.ToConsoleANSI(impliedTag + statusText + "{{[-]}}")
					if impliedTag == "{{|DockerStatusPending|}}" {
						icon = console.ToConsoleANSI("{{|DockerStatusPending|}}" + p.icons().pending + "{{[-]}}")
					} else {
						icon = console.ToConsoleANSI("{{|DockerMarkerDone|}}" + p.icons().done + "{{[-]}}")
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

		var layers []*consoleTask
		for _, id := range p.ids {
			t := p.tasks[id]
			if t.parentID == imgName {
				layers = append(layers, t)
			}
		}

		appendLine(p.buildImageLine(imgName, img, layers, maxImgNameW, termW), img, timerImage)

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
			if impliedTag == "{{|DockerStatusPending|}}" {
				icon = console.ToConsoleANSI("{{|DockerStatusPending|}}·{{[-]}}")
			} else {
				icon = console.ToConsoleANSI("{{|DockerMarkerDone|}}✓{{[-]}}")
			}
		} else {
			statusText = abbreviateStatus(t.text)
			statusANSI = console.ToConsoleANSI(serviceStatusTag(t.status, t.text, p.command))
			icon = p.propagatedIcon(t, t.status)
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		appendLine(globalIndent+icon+" "+statusANSI+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"), p.serviceTimerTask(svcID), timerService)
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

func (p *consoleEventProcessor) buildImageLine(imgName string, t *consoleTask, layers []*consoleTask, maxImgNameW int, termW int) string {
	imageLabel := strutil.Repeat(" ", 2*sectionChildIndentW) + console.ToConsoleANSI("{{|DockerMarkerDone|}}image{{[-]}}{{|DockerColon|}}:{{[-]}} ")
	imgStr := styleImage(imgName)
	imgNameW := utf8.RuneCountInString(console.Strip(imgStr))
	imgPad := strutil.Repeat(" ", maxImgNameW-imgNameW)
	layerCount := ""
	if len(layers) > 0 {
		layerCount = console.ToConsoleANSI(fmt.Sprintf(" {{[::D]}}(%d %s){{[-]}}", len(layers), plural(len(layers), "layer", "layers")))
	}
	sizes, bar := p.buildImageSizesAndBar(layers, maxImgNameW, termW)
	if t == nil {
		cachedIcon := console.ToConsoleANSI("{{|DockerMarkerDone|}}" + p.icons().done + "{{[-]}}")
		cachedStatus := console.ToConsoleANSI("{{|DockerStatusSuccess|}}Cached{{[-]}}")
		statusPad := strutil.Repeat(" ", sectionStatusW-len("Cached"))
		return globalIndent + cachedIcon + " " + cachedStatus + console.CodeReset + statusPad + imageLabel + imgStr + imgPad + layerCount + sizes + bar
	}
	worst := p.worstImageStatus(imgName)
	icon := p.propagatedIcon(t, worst)
	statusText := abbreviateStatus(t.text)
	statusANSI := console.ToConsoleANSI(imageStatusTag(t.status, t.text, p.command))
	statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
	return globalIndent + icon + " " + statusANSI + console.CodeReset + statusPad + imageLabel + imgStr + imgPad + layerCount + sizes + bar
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
		sizes = " " + console.ToConsoleANSI("{{|DockerMarkerDone|}}"+fixedSize(current)+"{{[-]}}"+
			"{{|DockerColon|}}/{{[-]}}"+
			"{{|DockerMarkerDone|}}"+fixedSize(total)+"{{[-]}}")
	} else {
		sizes = strutil.Repeat(" ", 1+8+1+8)
	}

	usedW := imageSizesColBase + maxImgNameW + spaceW + sizeColW + sizeSepW + sizeColW
	barW := len(layers)
	maxBarW := termW - usedW - 3
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
			icon = console.ToConsoleANSI("{{|DockerStatusPending|}}·{{[-]}}")
			statusText = "Pending"
			statusANSI = console.ToConsoleANSI("{{|DockerStatusPending|}}Pending{{[-]}}")
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		lines = append(lines, globalIndent+icon+" "+statusANSI+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"))
		timers = append(timers, timerEntry{task: t, style: timerService})
	}
	return lines, timers
}

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
			icon = console.ToConsoleANSI("{{|DockerStatusPending|}}·{{[-]}}")
			statusText = "Pending"
			statusANSI = console.ToConsoleANSI("{{|DockerStatusPending|}}Pending{{[-]}}")
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		lines = append(lines, globalIndent+icon+" "+statusANSI+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"))
		timers = append(timers, timerEntry{task: t, style: timerService})
	}
	return lines, timers
}

func (p *consoleEventProcessor) buildLayerLine(t *consoleTask, statusW int, maxImgNameW int, layerPcts []int) string {
	id := t.id
	if len(id) > 19 {
		id = id[:18] + "…"
	}
	idW := utf8.RuneCountInString(id)

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
	short := abbreviateStatus(t.text)
	statusPad := ""
	if pad := statusW - utf8.RuneCountInString(short); pad > 0 {
		statusPad = strutil.Repeat(" ", pad)
	}
	statusANSI := console.ToConsoleANSI(layerStatusTag(t.status, t.text))
	idPad := (imageSizesColBase + maxImgNameW) - (layerSizesColBase + statusW + spaceW + idW)
	if idPad < 1 {
		idPad = 1
	}
	barReset := ""
	if bar != "" {
		barReset = console.CodeReset
	}
	return layerPrefix + console.CodeDim + icon + " " + statusANSI + console.CodeReset + console.CodeDim + statusPad + " " + id + strutil.Repeat(" ", idPad) + sizes + barReset + bar + console.CodeDimOff
}

func (p *consoleEventProcessor) buildLayerLines(layers []*consoleTask, maxImgNameW int, termW int) ([]string, []*consoleTask) {
	if len(layers) == 0 {
		return nil, nil
	}

	maxBarW := termW - 70
	if maxBarW < 1 {
		maxBarW = 1
	}

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
	if len(layerPcts) > maxBarW {
		layerPcts = layerPcts[:maxBarW]
	}

	var out []string
	var outTasks []*consoleTask
	for _, t := range layers {
		out = append(out, p.buildLayerLine(t, layerStatusW, maxImgNameW, layerPcts))
		outTasks = append(outTasks, t)
	}
	return out, outTasks
}

func (p *consoleEventProcessor) buildTeardownLines() []string {
	impliedText, impliedTag := p.impliedStatus()
	impliedANSI := console.ToConsoleANSI(impliedTag + impliedText + "{{[-]}}")
	ic := p.icons()
	var impliedIcon string
	if impliedTag == "{{|DockerStatusPending|}}" {
		impliedIcon = console.ToConsoleANSI("{{|DockerStatusPending|}}" + ic.pending + "{{[-]}}")
	} else {
		impliedIcon = console.ToConsoleANSI("{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}")
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

	allSvcIDs := append(append([]string{}, p.imageOrder...), svcRollupIDs...)
	appendLine(globalIndent+svcIcon+" "+svcStatusANSI+console.CodeReset+svcStatusPad+console.ToConsoleANSI("{{[white::B]}}services{{[-]}}{{|DockerColon|}}:{{[-]}}"), p.sectionTaskFor(allSvcIDs), timerSection)

	seenSvcs := make(map[string]bool)
	for _, imgName := range p.imageOrder {
		for _, svc := range p.serviceIDsForImage(imgName) {
			if seenSvcs[svc] {
				continue
			}
			seenSvcs[svc] = true
			t := p.tasks[svc]
			nameANSI := console.ToConsoleANSI("{{|App|}}" + svc + "{{[-]}}")
			var icon, statusText, statusANSI string
			if t == nil {
				icon, statusANSI, statusText = impliedIcon, impliedANSI, impliedText
			} else {
				statusText = abbreviateStatus(t.text)
				statusANSI = console.ToConsoleANSI(serviceStatusTag(t.status, t.text, p.command))
				icon = p.propagatedIcon(t, p.worstServiceStatus(svc, imgName))
			}
			statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
			appendLine(globalIndent+icon+" "+statusANSI+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"), p.serviceTimerTask(svc), timerService)
		}
	}
	for _, svcID := range p.serviceIDs {
		if seenSvcs[svcID] {
			continue
		}
		seenSvcs[svcID] = true
		t := p.tasks[svcID]
		nameANSI := console.ToConsoleANSI("{{|App|}}" + svcID + "{{[-]}}")
		var icon, statusText, statusANSI string
		if t == nil {
			icon, statusANSI, statusText = impliedIcon, impliedANSI, impliedText
		} else {
			statusText = abbreviateStatus(t.text)
			statusANSI = console.ToConsoleANSI(serviceStatusTag(t.status, t.text, p.command))
			icon = p.propagatedIcon(t, t.status)
		}
		statusPad := strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))
		appendLine(globalIndent+icon+" "+statusANSI+console.CodeReset+statusPad+sectionChildIndent+nameANSI+console.ToConsoleANSI("{{|DockerColon|}}:{{[-]}}"), p.serviceTimerTask(svcID), timerService)
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
