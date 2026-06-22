package compose

import (
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/dockerlayout"
	"github.com/GhostWriters/semstyle"

	"github.com/docker/compose/v5/pkg/api"
)

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
		return "Error", "{{|DockerStatusFail|}}", "{{|DockerMarkerError|}}"
	case rollupWarning:
		return "Warning", "{{|DockerStatusWarn|}}", "{{|DockerMarkerWarn|}}"
	case rollupPending:
		return "Pending", "{{|DockerStatusPending|}}", "{{|DockerStatusPending|}}"
	case rollupComplete:
		return "Complete", "{{|DockerStatusFinal|}}", "{{|DockerMarkerDone|}}"
	default: // rollupProcessing
		return "Processing", "{{|DockerStatusActive|}}", "{{|DockerSpinner|}}"
	}
}

// visibleNetworkIDs returns the network IDs that should be shown/counted. On teardown
// commands, networks that ended in a Warning (e.g. "Resource is still in use" when downing
// a subset of services) are expected and hidden, so they don't show or skew the rollup.
func (p *consoleEventProcessor) visibleNetworkIDs() []string {
	if !p.isTeardownCommand() {
		return p.networkIDs
	}
	out := make([]string, 0, len(p.networkIDs))
	for _, id := range p.networkIDs {
		if t := p.tasks[id]; t != nil && t.status == api.Warning {
			continue
		}
		out = append(out, id)
	}
	return out
}

// overallRollupIcon returns just the icon for the whole operation: a spinner while any
// work is in progress, else a success/warning/error marker. Used on the summary header.
func (p *consoleEventProcessor) overallRollupIcon() string {
	netIDs := p.visibleNetworkIDs()
	ids := make([]string, 0, len(p.serviceIDs)+len(p.imageOrder)+len(netIDs)+len(p.volumeIDs))
	ids = append(ids, p.serviceIDs...)
	ids = append(ids, p.imageOrder...)
	ids = append(ids, netIDs...)
	ids = append(ids, p.volumeIDs...)
	state := p.rollupState(ids, nil)
	if state == rollupProcessing || state == rollupPending {
		return p.activeSpinnerANSI(p.icons().spinner)
	}
	_, _, iconTag := sectionStatusText(state)
	return semstyle.ToANSI(iconTag + p.sectionRollupIcon(state) + "{{[-]}}")
}

// sectionRollupWithPropagation is like sectionRollup but also checks child tasks
// (layers for image IDs, images for service IDs) for propagated errors/warnings.
func (p *consoleEventProcessor) sectionRollupWithPropagation(ids []string, imageForID func(string) string) (icon, statusANSI, statusText, labelTag string) {
	state := p.rollupState(ids, imageForID)
	text, stTag, iconTag := sectionStatusText(state)
	ic := p.icons()
	if state != rollupProcessing {
		icon = semstyle.ToANSI(iconTag + p.sectionRollupIcon(state) + "{{[-]}}")
	} else {
		icon = p.activeSpinnerANSI(ic.spinner)
	}
	statusANSI = semstyle.ToANSI(stTag + text + "{{[-]}}")
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
			anyStarted = true
		} else {
			allDone = false
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

// worstChildStatus returns the worst EventStatus among all layer children of imgName.
// With sha256-keyed tasks, layers are looked up via imageLayerDiffIDs rather than parentID.
func (p *consoleEventProcessor) worstChildStatus(imgName string) api.EventStatus {
	worst := api.Done
	diffIDs := p.imageLayerDiffIDs[imgName]
	if len(diffIDs) > 0 {
		for _, sha := range diffIDs {
			t := p.tasks[sha]
			if t == nil {
				continue
			}
			switch t.status {
			case api.Error:
				return api.Error
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
	// Fallback for images with no pre-flight data: use parentID matching.
	for _, id := range p.ids {
		t := p.tasks[id]
		if t.parentID != imgName {
			continue
		}
		switch t.status {
		case api.Error:
			return api.Error
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
	// If all layers report "Already exists" the image is fully cached; treat as Done
	// so the rollup propagates correctly even before the SDK sends a Done event.
	if p.allLayersAlreadyExist(imgName) {
		return api.Done
	}
	return img.status
}

// allLayersAlreadyExist reports whether every layer of imgName has text "Already exists".
func (p *consoleEventProcessor) allLayersAlreadyExist(imgName string) bool {
	diffIDs := p.imageLayerDiffIDs[imgName]
	if len(diffIDs) == 0 {
		return false
	}
	for _, sha := range diffIDs {
		l := p.tasks[sha]
		if l == nil || l.text != "Already exists" {
			return false
		}
	}
	return true
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
func (p *consoleEventProcessor) propagatedIcon(t *consoleTask, worstStatus api.EventStatus) string {
	ic := p.icons()
	if worstStatus == api.Error {
		return semstyle.ToANSI("{{|DockerMarkerError|}}" + ic.error + "{{[-]}}")
	}
	if worstStatus == api.Warning {
		return semstyle.ToANSI("{{|DockerMarkerWarn|}}" + ic.warn + "{{[-]}}")
	}
	return p.spinnerIcon(t)
}

type iconSet struct {
	done, error, warn, pending, spinner string
}

func (p *consoleEventProcessor) icons() iconSet {
	if p.asciiMode {
		spinnerChar := "-"
		if console.SpinnerEnabled {
			spinnerChar = asciiSpinnerFrames[p.spinnerFrame%len(asciiSpinnerFrames)]
		}
		return iconSet{done: "+", error: "x", warn: "!", pending: "-", spinner: spinnerChar}
	}
	spinnerChar := "·"
	if console.SpinnerEnabled {
		spinnerChar = spinnerFrames[p.spinnerFrame]
	}
	return iconSet{done: "✓", error: "×", warn: "⚠", pending: "·", spinner: spinnerChar}
}

func (p *consoleEventProcessor) activeSpinnerANSI(char string) string {
	if console.SpinnerEnabled {
		return semstyle.ToANSI("{{|DockerSpinner|}}" + char + "{{[-]}}")
	}
	return semstyle.ToANSI("{{[::D]}}" + char + "{{[-]}}")
}

func (p *consoleEventProcessor) spinnerIcon(t *consoleTask) string {
	ic := p.icons()
	var s string
	if t == nil {
		s = p.activeSpinnerANSI(ic.spinner)
	} else {
		switch t.status {
		case api.Done:
			s = semstyle.ToANSI("{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}")
		case api.Error:
			s = semstyle.ToANSI("{{|DockerMarkerError|}}" + ic.error + "{{[-]}}")
		case api.Warning:
			s = semstyle.ToANSI("{{|DockerMarkerWarn|}}" + ic.warn + "{{[-]}}")
		default:
			if t.completed() {
				s = semstyle.ToANSI("{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}")
			} else {
				s = p.activeSpinnerANSI(ic.spinner)
			}
		}
	}
	if s == "" {
		return " "
	}
	return s
}

// impliedStatus returns the status text and ANSI tag for a service that received no events.
func (p *consoleEventProcessor) impliedStatus() (text, ansiTag string) {
	switch p.command {
	case "down":
		return "Removed", "{{|DockerStatusFinal|}}"
	case "stop", "kill":
		return "Stopped", "{{|DockerStatusFinal|}}"
	case "pause":
		return "Paused", "{{|DockerStatusFinal|}}"
	case "unpause", "start":
		return "Running", "{{|DockerStatusFinal|}}"
	default:
		return "Pending", "{{|DockerStatusPending|}}"
	}
}

// abbreviateStatus shortens verbose Docker layer/image status strings.
func abbreviateStatus(text string) string {
	// api.StatusDownloadComplete handled here since dockerlayout can't import api.
	if text == api.StatusDownloadComplete {
		return "Downloaded"
	}
	// SDK emits bare verb forms (Working) — normalize to *ing for consistency.
	switch text {
	case "Recreate":
		return "Recreating"
	case "Restart":
		return "Restarting"
	}
	return dockerlayout.AbbreviateStatus(text)
}

// applyStatusTag wraps short in the appropriate semantic tag based on event status.
func applyStatusTag(s api.EventStatus, text string, finalTexts, activeTexts, successTexts []string) string {
	short := abbreviateStatus(text)
	switch s {
	case api.Warning:
		return "{{|DockerStatusWarn|}}" + short + "{{[-]}}"
	case api.Error:
		return "{{|DockerStatusFail|}}" + short + "{{[-]}}"
	case api.Done:
		if contains(finalTexts, text) {
			return "{{|DockerStatusFinal|}}" + short + "{{[-]}}"
		}
		return "{{|DockerStatusSuccess|}}" + short + "{{[-]}}"
	default: // Working
		if contains(finalTexts, text) {
			return "{{|DockerStatusFinal|}}" + short + "{{[-]}}"
		}
		if contains(activeTexts, text) {
			return "{{|DockerStatusActive|}}" + short + "{{[-]}}"
		}
		if contains(successTexts, text) {
			return "{{|DockerStatusSuccess|}}" + short + "{{[-]}}"
		}
		return "{{|DockerStatusPending|}}" + short + "{{[-]}}"
	}
}

// serviceStatusTag styles a service (container lifecycle) status.
func serviceStatusTag(s api.EventStatus, text string, command string) string {
	final := serviceFinalStatuses(command)
	success := []string{api.StatusCreated, api.StatusStarted, api.StatusStopped,
		api.StatusRestarted, api.StatusKilled, api.StatusRemoved, api.StatusPulled, "Recreated"}
	filtered := success[:0:len(success)]
	for _, v := range success {
		if !contains(final, v) {
			filtered = append(filtered, v)
		}
	}
	return applyStatusTag(s, text,
		final,
		[]string{api.StatusCreating, api.StatusStarting, api.StatusStopping,
			api.StatusRestarting, api.StatusKilling, api.StatusRemoving,
			"Recreate", "Restart", api.StatusWaiting},
		filtered,
	)
}

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
	default:
		return []string{api.StatusRunning, api.StatusHealthy, api.StatusCreated, "Recreated"}
	}
}

// imageStatusTag styles an image-level (pull/build) status.
func imageStatusTag(s api.EventStatus, text string, command string) string {
	_ = command
	return applyStatusTag(s, text,
		[]string{api.StatusPulled, api.StatusBuilt},
		[]string{api.StatusPulling, api.StatusBuilding},
		nil,
	)
}

// layerStatusTag styles a layer-level (download/extract) status.
// Note: the SDK drops "Pull complete" events (jm.Progress == nil guard in toPullProgressEvent),
// so "Extracting" is the terminal state for extracted layers and must be treated as final.
func layerStatusTag(s api.EventStatus, text string) string {
	return applyStatusTag(s, text,
		[]string{api.StatusDownloadComplete, "Pull complete", "Already exists", "Extracted"},
		[]string{api.StatusDownloading, "Extracting", "Verifying Checksum", "Pulling fs layer", api.StatusWaiting},
		nil,
	)
}

// networkStatusTag styles a network lifecycle status.
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

func networkFinalStatuses(command string) []string {
	switch command {
	case "down", "rm":
		return []string{api.StatusRemoved}
	default:
		return []string{api.StatusCreated}
	}
}

// volumeStatusTag styles a volume lifecycle status.
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

func volumeFinalStatuses(command string) []string {
	switch command {
	case "down", "rm":
		return []string{api.StatusRemoved}
	default:
		return []string{api.StatusCreated}
	}
}
