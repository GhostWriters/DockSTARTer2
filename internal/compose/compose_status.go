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

	"github.com/docker/compose/v5/pkg/api"
	dockerclient "github.com/moby/moby/client"
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
// run, then a warning when one has a problem, else complete. Callers hold p.mtx.
func (p *consoleEventProcessor) statusRollup(svcs []string, imageOf func(string) string) sectionRollupState {
	if !p.finished {
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
// reports on: processing while the checks run, then a warning when one is a problem
// or unknown, else complete. Callers hold p.mtx.
func (p *consoleEventProcessor) statusKindState(k rowKind, names []string) sectionRollupState {
	if !p.finished {
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
// differs from the digest pulled is an update (imageUpdates). The new version, when
// Docker Hub has a tag for that digest, names it (digestVersions); a LinuxServer image
// from lscr.io is looked up under linuxserver/ there. Runs in the background
// (waitUpdateChecks); writes under p.mtx, so it starts after the unlocked first render.
func (p *consoleEventProcessor) checkImageUpdates() {
	if p.dockerClient == nil {
		return
	}
	p.mtx.Lock()
	p.imageUpdates, p.updateErrs = map[string]string{}, map[string]string{}
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
			remote := p.imageUpdates[img]
			p.mtx.Unlock()
			if remote == "" {
				return
			}
			name := imageName(img)
			repo := dockerHubRepo(strings.TrimPrefix(name, "lscr.io/"))
			if repo == "" {
				return
			}
			ctx, cancel = context.WithTimeout(context.Background(), digestLookupTimeout)
			version := versionFromTags(hubTagsForDigest(ctx, repo, remote))
			cancel()
			if version != "" {
				p.mtx.Lock()
				p.digestVersions[remote] = version
				p.mtx.Unlock()
			}
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
// new version a Docker Hub tag gives the registry's digest, linked as a version is, or
// that digest cut short, then "(not pulled)"; nil when it has no update. Callers hold
// p.mtx.
func (p *consoleEventProcessor) statusUpdateValue(imgName string) []infoPart {
	remote := p.imageUpdates[imgName]
	if p.command != statusCommand || remote == "" {
		return nil
	}
	if v := p.digestVersions[remote]; v != "" {
		return []infoPart{{v, p.versionURL(imgName, imageMeta{version: v, source: p.imageMetaBefore[imgName].source})}, {" (not pulled)", ""}}
	}
	short := remote
	if _, hex, ok := strings.Cut(remote, ":"); ok && len(hex) > 12 {
		short = "sha256:" + hex[:12]
	}
	return []infoPart{{short, ""}, {" (not pulled)", ""}}
}

// hasUpdate reports whether status found an update for image imgName. Callers hold p.mtx.
func (p *consoleEventProcessor) hasUpdate(imgName string) bool {
	return p.command == statusCommand && p.imageUpdates[imgName] != ""
}

// updateCount returns how many of status's images have an update. Callers hold p.mtx.
func (p *consoleEventProcessor) updateCount() int {
	return len(p.imageUpdates)
}

// updateSummaryLines returns the recaps after status's display: the images with an
// update, each with its new version or digest as its update line shows it, under a
// heading that says they aren't pulled, as information; and the images whose registry
// couldn't be asked, with why, as a warning. Nil for any other command.
func (p *consoleEventProcessor) updateSummaryLines() (updates, failed []string) {
	if p.command != statusCommand {
		return nil, nil
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if n := len(p.imageUpdates); n > 0 {
		updates = append(updates, fmt.Sprintf("Found %d image %s (not pulled):", n, plural(n, "update", "updates")))
		for _, img := range slices.Sorted(maps.Keys(p.imageUpdates)) {
			value := p.statusUpdateValue(img)
			updates = append(updates, "  "+styleImage(img)+"{{|DockerColon|}}:{{[-]}} "+value[0].text) // the heading says it isn't pulled
		}
	}
	if n := len(p.updateErrs); n > 0 {
		failed = append(failed, fmt.Sprintf("Couldn't check %d %s for updates:", n, plural(n, "image", "images")))
		for _, img := range slices.Sorted(maps.Keys(p.updateErrs)) {
			failed = append(failed, "  "+styleImage(img)+"{{|DockerColon|}}:{{[-]}} "+p.updateErrs[img])
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
