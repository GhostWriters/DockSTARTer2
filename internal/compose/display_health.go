package compose

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	"DockSTARTer2/internal/strutil"
)

// Health states, as Docker reports them.
const (
	healthStarting  = string(container.Starting)
	healthHealthy   = string(container.Healthy)
	healthUnhealthy = string(container.Unhealthy)
)

const (
	healthInspectTimeout = 5 * time.Second        // bounds each container inspect for its health
	healthWaitTimeout    = 10 * time.Second       // how long the end of a run waits for first check results
	healthWaitPoll       = 100 * time.Millisecond // how often that wait looks for them
	healthWatchRetry     = time.Second            // pause before reopening an event stream that ended
)

// commandWatchesHealth reports whether command can leave containers running, so
// their health is worth showing.
func commandWatchesHealth(command string) bool {
	switch command {
	case "down", "rm", "stop", "kill", "pause", "pull", "create":
		return false
	}
	return true
}

// startsContainer reports whether a service event means its container is running.
func startsContainer(text string) bool {
	switch text {
	case api.StatusStarted, api.StatusRunning, api.StatusRestarted, api.StatusHealthy, api.StatusWaiting, "Unpaused":
		return true
	}
	return false
}

// isHealthFailure reports whether an error message is compose's report of a
// dependency failing its health check.
func isHealthFailure(msg string) bool {
	return strings.Contains(msg, "is unhealthy") ||
		strings.HasPrefix(msg, "dependency ") && strings.HasSuffix(msg, " failed to start")
}

// containerHealth returns a running container's health, or "" when it isn't running
// or has no healthcheck. A container whose first check hasn't run yet is starting.
func containerHealth(c container.InspectResponse) string {
	if c.State == nil || !c.State.Running {
		return ""
	}
	if h := c.State.Health; h != nil {
		switch s := string(h.Status); s {
		case healthStarting, healthHealthy, healthUnhealthy:
			return s
		}
		return ""
	}
	if hc := c.Config; hc != nil && hc.Healthcheck != nil && len(hc.Healthcheck.Test) > 0 && hc.Healthcheck.Test[0] != "NONE" {
		return healthStarting
	}
	return ""
}

// healthInfo is what an inspect tells about a container's health: its state, its
// healthcheck's command and its last check's output.
type healthInfo struct {
	status, command, output string
}

// containerHealthInfo returns a container's health state, command and last output.
func containerHealthInfo(c container.InspectResponse) healthInfo {
	info := healthInfo{status: containerHealth(c)}
	if hc := c.Config; hc != nil && hc.Healthcheck != nil && len(hc.Healthcheck.Test) > 1 {
		// Test is ["CMD-SHELL", "<command>"] or ["CMD", "<arg>", ...].
		info.command = strings.Join(hc.Healthcheck.Test[1:], " ")
	}
	if c.State != nil && c.State.Health != nil {
		if log := c.State.Health.Log; len(log) > 0 && log[len(log)-1] != nil {
			info.output = log[len(log)-1].Output
		}
	}
	return info
}

// startHealthWatch follows Docker's health_status events for the project's containers
// until stopHealthWatch, recording each tracked container's health as it changes. A
// stream that ends early is reopened; refreshHealth covers anything missed meanwhile.
func (p *consoleEventProcessor) startHealthWatch(ctx context.Context) {
	if p.dockerClient == nil || p.projectName == "" || !commandWatchesHealth(p.command) || p.healthStop != nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	f := dockerclient.Filters{}
	f.Add("type", "container")
	f.Add("event", "health_status")
	f.Add("label", api.ProjectLabel+"="+p.projectName)
	done := make(chan struct{})
	p.healthStop = func() {
		cancel()
		<-done
	}
	go func() {
		defer close(done)
		for {
			p.followHealthEvents(ctx, p.dockerClient.Events(ctx, dockerclient.EventsListOptions{Filters: f}))
			select {
			case <-ctx.Done():
				return
			case <-time.After(healthWatchRetry):
			}
		}
	}()
}

// followHealthEvents records health from res until its stream ends or ctx is done.
func (p *consoleEventProcessor) followHealthEvents(ctx context.Context, res dockerclient.EventsResult) {
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-res.Err:
			_ = err // the caller reopens the stream
			return
		case m, ok := <-res.Messages:
			if !ok {
				return
			}
			_, status, found := strings.Cut(string(m.Action), ":")
			name := m.Actor.Attributes["name"]
			if !found || name == "" {
				continue
			}
			status = strings.TrimSpace(status)
			p.mtx.Lock()
			svc, tracked := p.healthChecked[name] // only containers this run started
			if tracked {
				p.setHealth(svc, name, status)
				if status == healthUnhealthy {
					p.fetchHealthOutput(svc, name)
				}
			}
			p.mtx.Unlock()
		}
	}
}

// waitForHealth waits, up to healthWaitTimeout from when it is called, for every
// container still on its first health check to report a result; those still without
// one then show No Result.
func (p *consoleEventProcessor) waitForHealth() {
	deadline := time.Now().Add(healthWaitTimeout)
	inspected := make(chan struct{})
	go func() {
		p.healthWG.Wait()
		close(inspected)
	}()
	select {
	case <-inspected:
	case <-time.After(time.Until(deadline)):
	}
	checking := func() bool {
		p.mtx.Lock()
		defer p.mtx.Unlock()
		for _, containers := range p.health {
			for _, h := range containers {
				if h == healthStarting {
					return true
				}
			}
		}
		return false
	}
	for checking() {
		if time.Now().After(deadline) {
			p.mtx.Lock()
			p.healthWaitDone = true
			p.mtx.Unlock()
			return
		}
		time.Sleep(healthWaitPoll)
	}
}

// stopHealthWatch waits for pending health inspects, ends the event watch, then
// refreshes every tracked container's health for the final render.
func (p *consoleEventProcessor) stopHealthWatch() {
	p.healthWG.Wait()
	if p.healthStop != nil {
		p.healthStop()
		p.healthStop = nil
	}
	p.refreshHealth()
}

// refreshHealth inspects every tracked container once more, so the final render shows
// its latest health even if events were missed while the stream was down.
func (p *consoleEventProcessor) refreshHealth() {
	if p.dockerClient == nil {
		return
	}
	p.mtx.Lock()
	tracked := make(map[string]string, len(p.healthChecked))
	for name, svc := range p.healthChecked {
		tracked[name] = svc
	}
	p.mtx.Unlock()
	var wg sync.WaitGroup
	for name, svc := range tracked {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if info := p.inspectHealth(name); info.status != "" {
				p.mtx.Lock()
				p.recordHealth(svc, name, info)
				p.mtx.Unlock()
			}
		}()
	}
	wg.Wait()
}

// checkHealth inspects a container once for its current health, which covers a
// container that has no events yet and any event missed before the watch began.
// Callers hold p.mtx.
func (p *consoleEventProcessor) checkHealth(svc, containerName string) {
	if p.dockerClient == nil || !commandWatchesHealth(p.command) {
		return
	}
	if _, ok := p.healthChecked[containerName]; ok {
		return
	}
	p.healthChecked[containerName] = svc
	p.healthWG.Add(1)
	go func() {
		defer p.healthWG.Done()
		info := p.inspectHealth(containerName)
		if info.status == "" {
			return
		}
		p.mtx.Lock()
		defer p.mtx.Unlock()
		if h, ok := p.health[svc][containerName]; ok { // an event since is newer
			info.status = h
		}
		p.recordHealth(svc, containerName, info)
	}()
}

// fetchHealthOutput inspects a container that just turned unhealthy for its last
// check's output. Callers hold p.mtx.
func (p *consoleEventProcessor) fetchHealthOutput(svc, containerName string) {
	if p.dockerClient == nil {
		return
	}
	p.healthWG.Add(1)
	go func() {
		defer p.healthWG.Done()
		info := p.inspectHealth(containerName)
		p.mtx.Lock()
		defer p.mtx.Unlock()
		if info.output != "" {
			p.healthOutput[containerName] = info.output
		}
		if info.command != "" {
			p.healthCmd[svc] = info.command
		}
	}()
}

// inspectHealth returns what an inspect tells about a container's health, with an
// empty status when it has none or the inspect fails.
func (p *consoleEventProcessor) inspectHealth(containerName string) healthInfo {
	ctx, cancel := context.WithTimeout(context.Background(), healthInspectTimeout)
	defer cancel()
	res, err := p.dockerClient.ContainerInspect(ctx, containerName, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return healthInfo{}
	}
	return containerHealthInfo(res.Container)
}

// recordHealth records an inspect's health for one container. Callers hold p.mtx.
func (p *consoleEventProcessor) recordHealth(svc, containerName string, info healthInfo) {
	p.setHealth(svc, containerName, info.status)
	if info.command != "" {
		p.healthCmd[svc] = info.command
	}
	if info.output != "" {
		p.healthOutput[containerName] = info.output
	}
}

// setHealth records one container's health. Callers hold p.mtx.
func (p *consoleEventProcessor) setHealth(svc, containerName, status string) {
	if p.health[svc] == nil {
		p.health[svc] = make(map[string]string)
	}
	p.health[svc][containerName] = status
}

// serviceHealth returns a service's health across its containers: unhealthy if any
// is, else starting if any is still on its first check, else healthy, or "" when none
// has a healthcheck.
func (p *consoleEventProcessor) serviceHealth(svc string) string {
	worst := ""
	for _, h := range p.health[svc] {
		switch {
		case h == healthUnhealthy:
			return healthUnhealthy
		case h == healthStarting:
			worst = healthStarting
		case h == healthHealthy && worst == "":
			worst = healthHealthy
		}
	}
	return worst
}

// actionStatus returns a service row's action text and tag. A row that failed only
// because its health check did keeps its action, styled as before the failure.
func (p *consoleEventProcessor) actionStatus(t *consoleTask) (text, tag string) {
	if t.healthErr {
		if p.serviceHealth(t.id) == healthUnhealthy {
			return abbreviateStatus(t.text), serviceStatusTag(t.prevStatus, t.text, p.command)
		}
		return api.StatusError, serviceStatusTag(api.Error, api.StatusError, p.command)
	}
	return abbreviateStatus(t.text), serviceStatusTag(t.status, t.text, p.command)
}

// healthChecking reports whether svc's container is on its first health check and
// the run is still waiting for the result.
func (p *consoleEventProcessor) healthChecking(svc string) bool {
	return p.serviceHealth(svc) == healthStarting && !p.healthWaitDone
}

// serviceIcon returns a service row's icon for worst, with a warning for a container
// that is unhealthy without having failed the operation.
func (p *consoleEventProcessor) serviceIcon(t *consoleTask, worst api.EventStatus) string {
	if worst != api.Error && p.serviceHealth(t.id) == healthUnhealthy {
		worst = api.Warning
	}
	return p.propagatedIcon(t, worst)
}

// unhealthyServices returns the services whose containers are unhealthy, in display order.
func (p *consoleEventProcessor) unhealthyServices() []string {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	var out []string
	add := func(svc string) {
		if p.serviceHealth(svc) == healthUnhealthy && !contains(out, svc) {
			out = append(out, svc)
		}
	}
	for _, img := range p.imageOrder {
		for _, svc := range p.serviceIDsForImage(img) {
			add(svc)
		}
	}
	for _, svc := range p.serviceIDs {
		add(svc)
	}
	return out
}

// healthLine returns the line under svc's row showing its container's health, or ""
// when it has no healthcheck: the state in the status column, then the check's command,
// or its last output while unhealthy.
func (p *consoleEventProcessor) healthLine(svc string, termW int) string {
	health := p.serviceHealth(svc)
	if health == "" {
		return ""
	}
	ic := p.icons()
	var icon, word, style string
	detail := p.healthCmd[svc]
	switch {
	case health == healthStarting && !p.healthWaitDone:
		icon, word, style = p.activeSpinnerTag(ic.spinner), "Checking", "{{|DockerHealthChecking|}}"
	case health == healthStarting:
		icon, word, style = "{{|DockerMarkerWarn|}}"+ic.warn+"{{[-]}}", "No Result", "{{|DockerHealthNoResult|}}"
	case health == healthHealthy:
		icon, word, style = "{{|DockerMarkerDone|}}"+ic.done+"{{[-]}}", "Healthy", detailFinalTag
	default:
		icon, word, style = "{{|DockerMarkerWarn|}}"+ic.warn+"{{[-]}}", "Unhealthy", "{{|DockerHealthUnhealthy|}}"
		if t := p.tasks[svc]; t != nil && t.status == api.Error {
			icon = "{{|DockerMarkerError|}}" + ic.error + "{{[-]}}" // its check failed the run
		}
		for name, h := range p.health[svc] {
			if out := p.healthOutput[name]; h == healthUnhealthy && out != "" {
				detail = out
				break
			}
		}
	}
	prefix := globalIndent + icon + " " + style + word + "{{[-]}}" +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(word)) +
		strutil.Repeat(" ", 2*sectionChildIndentW) + "{{|DockerDetailLabel|}}healthcheck{{[-]}}{{|DockerColon|}}:{{[-]}}"
	if detail = strings.Join(strings.Fields(stripControl(detail)), " "); detail == "" {
		return prefix
	}
	room := termW - timerReserveW - summaryHeaderIndentW - utf8.RuneCountInString(semstyle.ToPlain(prefix)) - 1
	return prefix + " {{[::D]}}" + cutRunes(detail, max(room, minDetailW)) + "{{[-]}}"
}
