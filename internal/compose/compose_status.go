package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"
	dockerclient "github.com/moby/moby/client"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// statusCommand is -c status: the display's opening frame with no compose operation,
// every service under unchanged in the group of its state, with the end-of-run health
// and crash checks.
const statusCommand = "status"

// The groups status gives a service with a problem, ahead of the states in groupOrder:
// its container restarting, dead, exited non-zero, failed to start, or with its start
// cancelled (notStartedText) when another service failed (downLog.problem), or
// unhealthy.
const (
	restartingText  = "Restarting"
	deadText        = "Dead"
	exitedText      = "Exited"
	failedStartText = "Start Failed"
	unhealthyText   = "Unhealthy"
)

// statusProblem reports whether word is one of the groups status gives a service with
// a problem.
func statusProblem(word string) bool {
	switch word {
	case restartingText, deadText, exitedText, failedStartText, notStartedText, unhealthyText:
		return true
	}
	return false
}

// statusGroupWord returns the group status puts svc in, imgName being its image ("" for
// none): a problem first (its container down, or unhealthy), else its container's
// state, Not Pulled for one with no container whose image isn't on disk. Callers hold
// p.mtx.
func (p *consoleEventProcessor) statusGroupWord(svc, imgName string) string {
	if d := p.downLogs[svc]; d != nil && statusProblem(d.problem) {
		return d.problem
	}
	state, seen := p.preRun[svc]
	switch {
	case !seen:
		if _, onDisk := p.imageMetaBefore[imgName]; imgName != "" && !onDisk {
			return notPulledText
		}
		return notCreatedText
	case state == preRunPaused:
		return pausedText
	case state == "stopped":
		return notRunningText
	case p.serviceHealth(svc) == healthUnhealthy:
		return unhealthyText
	}
	return api.StatusRunning
}

// statusIdle reports whether status found svc left as it was, not running and with no
// problem: no container, stopped, paused or its image not pulled. Callers hold p.mtx.
func (p *consoleEventProcessor) statusIdle(svc string) bool {
	imgName := ""
	for img, svcs := range p.imageServices {
		if contains(svcs, svc) {
			imgName = img
			break
		}
	}
	switch p.statusGroupWord(svc, imgName) {
	case notCreatedText, notRunningText, pausedText, notPulledText:
		return true
	}
	return false
}

// statusRowStatus returns the status of svc's row under status: its group's word, a
// problem's as a warning (status reports what it finds; nothing it did failed), and a
// state other than Running in the pending style, as its group's folded line shows it
// (groupLines). Callers hold p.mtx.
func (p *consoleEventProcessor) statusRowStatus(svc, imgName string) (text, tag, icon string) {
	text = p.statusGroupWord(svc, imgName)
	ic := p.icons()
	switch {
	case statusProblem(text):
		return text, "{{|DockerStatusWarn|}}" + text + "{{[-]}}", "{{|DockerMarkerWarn|}}" + ic.warn + "{{[-]}}"
	case text != api.StatusRunning:
		return text, "{{|DockerStatusPending|}}" + text + "{{[-]}}", "{{|DockerStatusPending|}}" + ic.pending + "{{[-]}}"
	}
	return text, serviceFinalTag + text + "{{[-]}}", "{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}"
}

// statusRollup returns the state of status's services: processing while the checks
// run (statusChecked), then a warning when one has a problem, else complete. Callers
// hold p.mtx.
func (p *consoleEventProcessor) statusRollup(svcs []string, imageOf func(string) string) sectionRollupState {
	if !p.statusChecked {
		return rollupProcessing
	}
	for _, svc := range svcs {
		if statusProblem(p.statusGroupWord(svc, imageOf(svc))) {
			return rollupWarning
		}
	}
	return rollupComplete
}

// beginStatus readies status's checks as its display opens: health is tracked for every
// running container (statusRunning), not only those a run starts; the settle before the
// final crash check counts from now, so the crash watch gets its window; the dependency
// lines are final at once, nothing being started; and a first look finds the containers
// already down, and what holds a port one couldn't bind.
func (p *consoleEventProcessor) beginStatus() {
	p.mtx.Lock()
	p.lastStart = time.Now()
	p.depsSettled = true
	for svc, names := range p.statusRunning {
		for _, name := range names {
			p.checkHealth(svc, name)
		}
	}
	p.mtx.Unlock()
	p.scanServicesDown(false)
	p.findStatusPortConflict()
}

// finishStatusChecks marks status's checks done, once Done's waits and final crash
// check have run: Done marks the run finished before them, status having no compose
// operation to wait on, so its headings use this instead.
func (p *consoleEventProcessor) finishStatusChecks() {
	p.mtx.Lock()
	p.statusChecked = true
	p.mtx.Unlock()
}

// findStatusPortConflict looks, under status, for what holds the port a service's last
// start couldn't bind (its Start Failed error), for the ports lines, as a run does for
// the port it is refused (findPortConflict). The first such service by name.
func (p *consoleEventProcessor) findStatusPortConflict() {
	if p.command != statusCommand {
		return // a run finds its own from the error it gets (findPortConflict)
	}
	p.mtx.Lock()
	failed, msg := "", ""
	for _, svc := range slices.Sorted(maps.Keys(p.downLogs)) {
		if d := p.downLogs[svc]; d.problem == failedStartText {
			if _, _, _, ok := parseBindError(d.err); ok {
				failed, msg = svc, d.err
				break
			}
		}
	}
	p.mtx.Unlock()
	if failed == "" {
		return
	}
	port, proto, hostHeld, _ := parseBindError(msg)
	p.resolvePortConflict(failed, port, proto, hostHeld)
}

// unknownText is the group status gives a model when the runner couldn't be asked.
const unknownText = "Unknown"

// statusModelWord returns the group status puts model name in: Existing when the runner
// has pulled it, Not Pulled when not (the next up pulls it), Unknown when it couldn't
// be asked. Callers hold p.mtx.
func (p *consoleEventProcessor) statusModelWord(name string) string {
	switch {
	case p.modelsUnknown:
		return unknownText
	case p.modelsOnDisk[p.models[name]]:
		return existingText
	}
	return notPulledText
}

// statusModelStatus returns model name's row status under status: Unknown as a
// warning, Not Pulled in the pending style, as a service's image not pulled is, and
// Existing as done. Callers hold p.mtx.
func (p *consoleEventProcessor) statusModelStatus(name string) (text, tag, icon string) {
	text = p.statusModelWord(name)
	ic := p.icons()
	switch text {
	case unknownText:
		return text, "{{|DockerStatusWarn|}}" + text + "{{[-]}}", "{{|DockerMarkerWarn|}}" + ic.warn + "{{[-]}}"
	case notPulledText:
		return text, "{{|DockerStatusPending|}}" + text + "{{[-]}}", "{{|DockerStatusPending|}}" + ic.pending + "{{[-]}}"
	}
	return text, serviceFinalTag + text + "{{[-]}}", "{{|DockerMarkerDone|}}" + ic.done + "{{[-]}}"
}

// statusKindState returns the state of a kind's rows under status, which nothing
// reports on: processing while the checks run (statusChecked), then a warning when one
// is a problem or unknown, else complete. Callers hold p.mtx.
func (p *consoleEventProcessor) statusKindState(k rowKind, names []string) sectionRollupState {
	if !p.statusChecked {
		return rollupProcessing
	}
	for _, name := range names {
		if word, _ := k.group(name); word == unknownText || statusProblem(word) {
			return rollupWarning
		}
	}
	return rollupComplete
}

// readModelsOnDisk asks the model runner, through its plugin at pluginPath, which
// models it has pulled, as compose does before deciding to pull one: each pulled
// model's tags.
func readModelsOnDisk(ctx context.Context, pluginPath string) (map[string]bool, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, pluginPath, "ls", "--json")
	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "DOCKER_CLI_PLUGIN") })
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var models []struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(out, &models); err != nil {
		return nil, err
	}
	onDisk := map[string]bool{}
	for _, m := range models {
		for _, tag := range m.Tags {
			onDisk[tag] = true
		}
	}
	return onDisk, nil
}

// updateCheckTimeout caps each image's registry query; updateWaitMax caps how long the
// final render waits for them, the digest lookup naming a new version included.
const (
	updateCheckTimeout = 5 * time.Second
	updateWaitMax      = updateCheckTimeout + digestLookupTimeout
)

// checkImageUpdates asks the registry of each image on disk, without pulling, for the
// digest its tag points at now, as docker buildx imagetools inspect does: one that
// differs from the digest pulled is an update (imageUpdates). Its version and date
// (updateMeta) come from its config on ghcr.io, lscr.io's images included (the build
// date its labels give), or from the Docker Hub tags for that digest (the version a tag
// names, and when it was pushed). Runs in the background (waitUpdateChecks); writes
// under p.mtx, so it starts after the unlocked first render.
func (p *consoleEventProcessor) checkImageUpdates() {
	if p.dockerClient == nil {
		return
	}
	p.mtx.Lock()
	p.imageUpdates, p.updateErrs, p.updateMeta = map[string]string{}, map[string]string{}, map[string]imageMeta{}
	var imgs []string
	for _, img := range p.imageOrder {
		if m, ok := p.imageMetaBefore[img]; ok && m.digest != "" { // pulled from a registry
			imgs = append(imgs, img)
		}
	}
	p.mtx.Unlock()
	slots := make(chan struct{}, updateChecksAtOnce)
	for _, img := range imgs {
		p.updateWG.Add(1)
		go func() {
			defer p.updateWG.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			auth := ""
			if p.registryAuth != nil {
				auth = p.registryAuth(img)
			}
			ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
			res, err := p.distributionInspect(ctx, img, auth)
			cancel()
			p.mtx.Lock()
			local := p.imageMetaBefore[img].digest
			switch remote := res.Descriptor.Digest.String(); {
			case err != nil:
				p.updateErrs[img] = cleanLogLine(err.Error())
			case remote != "" && remote != local:
				p.imageUpdates[img] = remote
			}
			remote, platform := p.imageUpdates[img], p.imageMetaBefore[img]
			p.mtx.Unlock()
			if remote == "" {
				return
			}
			meta := updateMeta(img, remote, platform)
			p.mtx.Lock()
			p.updateMeta[img] = meta
			if meta.version != "" {
				p.digestVersions[remote] = meta.version
			}
			p.mtx.Unlock()
		}()
	}
}

// updateChecksAtOnce caps the registry queries in flight: a registry's burst limit
// (lscr.io's answers toomanyrequests to many at the same instant) refuses the rest.
const updateChecksAtOnce = 4

// burstRetries is how many times a query refused by a registry's burst limit is tried
// again, after burstRetryWait plus up to as much again at random, so retries spread out.
const (
	burstRetries   = 3
	burstRetryWait = 250 * time.Millisecond
)

// distributionInspect asks the daemon for the registry's descriptor of img, trying again
// when the registry's burst limit refuses it (toomanyrequests), until ctx ends.
func (p *consoleEventProcessor) distributionInspect(ctx context.Context, img, auth string) (dockerclient.DistributionInspectResult, error) {
	for try := 0; ; try++ {
		res, err := p.dockerClient.DistributionInspect(ctx, img, dockerclient.DistributionInspectOptions{EncodedRegistryAuth: auth})
		if err == nil || try == burstRetries || !strings.Contains(err.Error(), "toomanyrequests") {
			return res, err
		}
		select {
		case <-ctx.Done():
			return res, err
		case <-time.After(burstRetryWait + time.Duration(rand.Int64N(int64(burstRetryWait)))):
		}
	}
}

// updateMeta returns what can be told without pulling of the image digest names for
// img's tag: from ghcr.io (lscr.io serves the same images), its version and build date
// from its config, through the manifest for platform's OS and architecture; from Docker
// Hub, the version a tag for it names and when that tag was pushed. Empty when neither
// says.
func updateMeta(img, digest string, platform imageMeta) imageMeta {
	ctx, cancel := context.WithTimeout(context.Background(), digestLookupTimeout)
	defer cancel()
	name := imageName(img)
	for _, host := range []string{"ghcr.io/", "lscr.io/"} {
		if repo, ok := strings.CutPrefix(name, host); ok {
			meta, _ := ghcrImageMeta(ctx, repo, digest, platform)
			return meta
		}
	}
	repo := dockerHubRepo(name)
	if repo == "" {
		return imageMeta{}
	}
	tags, pushed := hubTagsAndPushed(ctx, repo, digest)
	return imageMeta{version: versionFromTags(tags), built: localTime(pushed)}
}

// waitUpdateChecks waits for status's update checks, at most updateWaitMax, so the final
// render can show what they found.
func (p *consoleEventProcessor) waitUpdateChecks() {
	if p.command != statusCommand {
		return
	}
	done := make(chan struct{})
	go func() {
		p.updateWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(updateWaitMax):
	}
}

// statusUpdateValue returns the update info line's value for imgName under status: the
// new image's version, linked as a version is, or its digest cut short, then its date
// and "not pulled" in brackets, as a version line shows its build date; nil when it has
// no update. Callers hold p.mtx.
func (p *consoleEventProcessor) statusUpdateValue(imgName string) []infoPart {
	remote := p.imageUpdates[imgName]
	if p.command != statusCommand || remote == "" {
		return nil
	}
	return []infoPart{p.updateName(imgName), {" (" + p.updateDate(imgName) + "not pulled)", ""}}
}

// updateName returns how imgName's update is named: its version, linked when there's a
// page for it, else its digest cut short. Callers hold p.mtx.
func (p *consoleEventProcessor) updateName(imgName string) infoPart {
	remote := p.imageUpdates[imgName]
	if v := p.digestVersions[remote]; v != "" {
		source := p.updateMeta[imgName].source
		if source == "" {
			source = p.imageMetaBefore[imgName].source
		}
		return infoPart{v, p.versionURL(imgName, imageMeta{version: v, source: source})}
	}
	if _, hex, ok := strings.Cut(remote, ":"); ok && len(hex) > 12 {
		return infoPart{"sha256:" + hex[:12], ""}
	}
	return infoPart{remote, ""}
}

// updateDate returns imgName's update's date followed by ", " (its build date from
// ghcr.io, or when its Docker Hub tag was pushed), or "" when it isn't known. Callers
// hold p.mtx.
func (p *consoleEventProcessor) updateDate(imgName string) string {
	if built := p.updateMeta[imgName].built; built != "" {
		return built + ", "
	}
	return ""
}

// hasUpdate reports whether status found an update for image imgName. Callers hold p.mtx.
func (p *consoleEventProcessor) hasUpdate(imgName string) bool {
	return p.command == statusCommand && p.imageUpdates[imgName] != ""
}

// updateSummaryLines returns the recaps after status's display, laid out as the down
// list is: the services with an update, each line of services sharing an image followed
// by its image and its update's version or digest and date, under a heading that says
// they aren't pulled, as information; and the services whose image's registry couldn't
// be asked, with their image and why, as a warning. Nil for any other command.
func (p *consoleEventProcessor) updateSummaryLines() (updates, failed []string) {
	if p.command != statusCommand {
		return nil, nil
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	const labelW = len("update")
	label := func(name string) string {
		return "    {{|DockerDetailLabel|}}" + name + "{{[-]}}{{|DockerColon|}}:{{[-]}} " + strutil.Repeat(" ", labelW-len(name))
	}
	// entries returns each image's services' line and the services they count, in order
	// of their first service.
	entries := func(imgs []string) (lines map[string]string, order []string, n int) {
		lines = map[string]string{}
		for _, img := range imgs {
			svcs := slices.Sorted(slices.Values(p.imageServices[img]))
			if len(svcs) == 0 {
				svcs = []string{img} // an image no service uses
			}
			n += len(svcs)
			names := make([]string, len(svcs))
			for i, svc := range svcs {
				names[i] = dockerlayout.StyleServiceName(svc)
			}
			lines[img] = "  " + strings.Join(names, "{{|DockerColon|}},{{[-]}} ") + "{{|DockerColon|}}:{{[-]}}"
			order = append(order, img)
		}
		slices.SortFunc(order, func(a, b string) int { return strings.Compare(semstyle.ToPlain(lines[a]), semstyle.ToPlain(lines[b])) })
		return lines, order, n
	}
	if len(p.imageUpdates) > 0 {
		lines, order, n := entries(slices.Collect(maps.Keys(p.imageUpdates)))
		updates = append(updates, fmt.Sprintf("Found %d %s with updates (not pulled):", n, plural(n, "service", "services")))
		for _, img := range order {
			text := p.updateName(img).text
			if date := strings.TrimSuffix(p.updateDate(img), ", "); date != "" {
				text += " (" + date + ")"
			}
			updates = append(updates, lines[img], label("image")+styleImage(img), label("update")+text)
		}
	}
	if len(p.updateErrs) > 0 {
		lines, order, n := entries(slices.Collect(maps.Keys(p.updateErrs)))
		failed = append(failed, fmt.Sprintf("Couldn't check %d %s for updates:", n, plural(n, "service", "services")))
		for _, img := range order {
			failed = append(failed, lines[img], label("image")+styleImage(img), label("error")+p.updateErrs[img])
		}
	}
	return updates, failed
}

// recapLead starts the lists after the display: what the run left, or what status found.
func (p *consoleEventProcessor) recapLead() string {
	if p.command == statusCommand {
		return "Found"
	}
	return "Compose finished with"
}
