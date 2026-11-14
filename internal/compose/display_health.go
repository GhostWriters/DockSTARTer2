package compose

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"
)

// Health states, as Docker reports them.
const (
	healthStarting  = string(container.Starting)
	healthHealthy   = string(container.Healthy)
	healthUnhealthy = string(container.Unhealthy)
)

// healthSuffixW is the room a service row's health suffix takes: " (Unhealthy)" and
// " (No Result)", its longest. Reserving the longest keeps the columns still as a
// container's health changes.
const healthSuffixW = len(" (Unhealthy)")

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
			p.mtx.Lock()
			if svc, tracked := p.healthChecked[name]; tracked { // only containers this run started
				p.setHealth(svc, name, strings.TrimSpace(status))
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
			if status := p.inspectHealth(name); status != "" {
				p.mtx.Lock()
				p.setHealth(svc, name, status)
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
		status := p.inspectHealth(containerName)
		if status == "" {
			return
		}
		p.mtx.Lock()
		defer p.mtx.Unlock()
		if _, ok := p.health[svc][containerName]; !ok { // an event since is newer
			p.setHealth(svc, containerName, status)
		}
	}()
}

// inspectHealth returns a container's health, or "" when it has none or the inspect fails.
func (p *consoleEventProcessor) inspectHealth(containerName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), healthInspectTimeout)
	defer cancel()
	res, err := p.dockerClient.ContainerInspect(ctx, containerName, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return ""
	}
	return containerHealth(res.Container)
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

// healthSuffix returns a service's health as a suffix for its status, with its
// leading space, or "" when its container has no healthcheck.
func (p *consoleEventProcessor) healthSuffix(svc string) (text, tag string) {
	var word, style string
	switch p.serviceHealth(svc) {
	case healthStarting:
		word, style = "Checking", "{{|DockerHealthChecking|}}"
		if p.healthWaitDone {
			word, style = "No Result", "{{|DockerHealthNoResult|}}"
		}
	case healthHealthy:
		word, style = "Healthy", "{{|DockerHealthHealthy|}}"
	case healthUnhealthy:
		word, style = "Unhealthy", "{{|DockerHealthUnhealthy|}}"
	default:
		return "", ""
	}
	return " (" + word + ")", " " + style + "(" + word + "){{[-]}}"
}

// serviceStatus returns a service row's status: its action, then its health.
func (p *consoleEventProcessor) serviceStatus(t *consoleTask) (text, tag string) {
	text, tag = p.actionStatus(t)
	suffixText, suffixTag := p.healthSuffix(t.id)
	return text + suffixText, tag + suffixTag
}

// healthChecking reports whether svc's container is on its first health check and
// the run is still waiting for the result.
func (p *consoleEventProcessor) healthChecking(svc string) bool {
	return p.serviceHealth(svc) == healthStarting && !p.healthWaitDone
}

// serviceIcon returns a service row's icon for worst: a spinner while its container
// is on its first health check, and a warning for one that is unhealthy without
// having failed the operation.
func (p *consoleEventProcessor) serviceIcon(t *consoleTask, worst api.EventStatus) string {
	if worst != api.Error && p.healthChecking(t.id) {
		return p.activeSpinnerTag(p.icons().spinner)
	}
	if worst != api.Error && p.serviceHealth(t.id) == healthUnhealthy {
		worst = api.Warning
	}
	return p.propagatedIcon(t, worst)
}

// updateStatusW sets the status column's width for this render: the default, or room
// for the longest action that has a health suffix plus the longest suffix. An action
// counts at most the default width, so a status longer than any compose sends can
// only misalign its own row.
func (p *consoleEventProcessor) updateStatusW() {
	w := sectionStatusTextW
	for svc := range p.health {
		t := p.tasks[svc]
		if t == nil || p.serviceHealth(svc) == "" {
			continue
		}
		text, _ := p.actionStatus(t)
		w = max(w, min(utf8.RuneCountInString(text), sectionStatusTextW)+healthSuffixW)
	}
	p.statusColW = w + sectionStatusGutterW
}

// statusW returns the status column's width, including its gutter.
func (p *consoleEventProcessor) statusW() int {
	if p.statusColW == 0 {
		return sectionStatusW
	}
	return p.statusColW
}

// imageSizesColBase returns the column where an image row's name starts.
func (p *consoleEventProcessor) imageSizesColBase() int {
	return globalIndentW + iconW + spaceW + p.statusW() + imageLabelW
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
