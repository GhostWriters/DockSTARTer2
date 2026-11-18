package compose

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	"DockSTARTer2/internal/strutil"
)

// Docker's errors for a published port it can't bind: another container already has
// it ("port is already allocated", no protocol given), or the host refused the bind
// ("address already in use"): a program on the host, or a container using host
// networking.
var (
	portAllocatedPattern = regexp.MustCompile(`Bind for \[?[0-9A-Fa-f:.]*\]?:(\d+) failed: port is already allocated`)
	portInUsePattern     = regexp.MustCompile(`failed to bind host port \[?[0-9A-Fa-f:.]*\]?:(\d+)/([a-z]+): address already in use`)
)

// parseBindError returns the host port and protocol msg says couldn't be bound, and
// whether the host refused it; proto is "" when Docker's message doesn't give it.
func parseBindError(msg string) (port uint16, proto string, hostHeld, ok bool) {
	if m := portInUsePattern.FindStringSubmatch(msg); m != nil {
		n, err := strconv.ParseUint(m[1], 10, 16)
		return uint16(n), m[2], true, err == nil
	}
	if m := portAllocatedPattern.FindStringSubmatch(msg); m != nil {
		n, err := strconv.ParseUint(m[1], 10, 16)
		return uint16(n), "", false, err == nil
	}
	return 0, "", false, false
}

// portClaim is a container or service that publishes a port: its name as the display
// shows it, whether it is a service of this project, its port in the container, and
// the run of consecutive host ports it publishes around it (lo == hi for one port).
type portClaim struct {
	name    string
	service bool
	target  uint32
	lo, hi  uint16
}

// rangeText returns c's run of ports as "lo-hi", or "" for a single port.
func (c portClaim) rangeText() string {
	if c.lo == c.hi {
		return ""
	}
	return fmt.Sprintf("%d-%d", c.lo, c.hi)
}

// claimText returns c's name, with its run of ports when it has one.
func (c portClaim) claimText() string {
	if r := c.rangeText(); r != "" {
		return c.name + ", ports " + r
	}
	return c.name
}

// portRun returns the run of consecutive ports in published around port. Compose
// expands a range like 8000-8100 into one port each, so the run is the range.
func portRun(published map[uint16]bool, port uint16) (lo, hi uint16) {
	lo, hi = port, port
	for lo > 0 && published[lo-1] {
		lo--
	}
	for hi < 65535 && published[hi+1] {
		hi++
	}
	return lo, hi
}

// servicePublished returns the host ports ports publish with protocol proto.
func servicePublished(ports []types.ServicePortConfig, proto string) map[uint16]bool {
	out := map[uint16]bool{}
	for _, p := range ports {
		if pr := p.Protocol; pr != proto && (pr != "" || proto != "tcp") {
			continue
		}
		lo, hi, isRange := strings.Cut(p.Published, "-")
		first, err := strconv.ParseUint(lo, 10, 16)
		if err != nil {
			continue
		}
		last := first
		if isRange {
			if last, err = strconv.ParseUint(hi, 10, 16); err != nil {
				continue
			}
		}
		for n := first; n <= last; n++ {
			out[uint16(n)] = true
		}
	}
	return out
}

// serviceClaim returns svc's claim on port, from its published ports.
func serviceClaim(svc string, ports []types.ServicePortConfig, port uint16, proto string) (portClaim, bool) {
	target, ok := publishedTarget(ports, port, proto)
	if !ok {
		return portClaim{}, false
	}
	lo, hi := portRun(servicePublished(ports, proto), port)
	return portClaim{name: svc, service: true, target: target, lo: lo, hi: hi}, true
}

// portConflict is a published port a service couldn't bind: the service, the port
// and protocol, what holds the port (none found: the host, or an unseen container),
// and the containers or services that aren't running but publish it too.
type portConflict struct {
	failed   portClaim
	port     uint16
	proto    string
	holder   *portClaim
	hostHeld bool
	wanters  []portClaim
}

// publishedTarget returns the container port that ports publish on host port port
// and protocol proto, a range taken by offset.
func publishedTarget(ports []types.ServicePortConfig, port uint16, proto string) (uint32, bool) {
	for _, p := range ports {
		if pr := p.Protocol; pr != proto && (pr != "" || proto != "tcp") {
			continue
		}
		lo, hi, ok := strings.Cut(p.Published, "-")
		first, err := strconv.ParseUint(lo, 10, 16)
		if err != nil {
			continue
		}
		last := first
		if ok {
			if last, err = strconv.ParseUint(hi, 10, 16); err != nil {
				continue
			}
		}
		if uint64(port) >= first && uint64(port) <= last {
			return p.Target + uint32(uint64(port)-first), true
		}
	}
	return 0, false
}

// findPortConflict looks, when err is Docker refusing a service's published port,
// for what holds the port and what else publishes it, for the ports lines. Callers
// hold no lock.
func (p *consoleEventProcessor) findPortConflict(err error) {
	if err == nil || p.dockerClient == nil {
		return
	}
	port, proto, hostHeld, ok := parseBindError(err.Error())
	if !ok {
		return
	}
	p.mtx.Lock()
	failed := ""
	for _, id := range p.serviceIDs {
		if t := p.tasks[id]; t != nil && t.status == api.Error {
			if q, _, _, ok := parseBindError(t.message); ok && q == port {
				failed = id
				break
			}
		}
	}
	servicePorts := p.servicePorts
	p.mtx.Unlock()
	if failed == "" {
		return
	}
	if proto == "" {
		proto = "tcp"
		for _, pr := range []string{"tcp", "udp", "sctp"} {
			if _, ok := publishedTarget(servicePorts[failed], port, pr); ok {
				proto = pr
				break
			}
		}
	}
	failedClaim, _ := serviceClaim(failed, servicePorts[failed], port, proto)
	failedClaim.name, failedClaim.service = failed, true
	c := &portConflict{failed: failedClaim, port: port, proto: proto, hostHeld: hostHeld}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seen := map[string]bool{failed: true}
	if res, err := p.dockerClient.ContainerList(ctx, dockerclient.ContainerListOptions{All: true}); err == nil {
		for _, ctr := range res.Items {
			claim, isSvc := p.containerClaimName(ctr)
			if isSvc && seen[claim] {
				continue
			}
			if ctr.State == container.StateRunning || ctr.State == container.StatePaused {
				published := map[uint16]bool{}
				var target uint32
				for _, pt := range ctr.Ports {
					if pt.Type == proto && pt.PublicPort != 0 {
						published[pt.PublicPort] = true
						if pt.PublicPort == port {
							target = uint32(pt.PrivatePort)
						}
					}
				}
				if published[port] && c.holder == nil {
					lo, hi := portRun(published, port)
					c.holder = &portClaim{name: claim, service: isSvc, target: target, lo: lo, hi: hi}
					seen[claim] = true
				}
				continue
			}
			info, err := p.dockerClient.ContainerInspect(ctx, ctr.ID, dockerclient.ContainerInspectOptions{})
			if err != nil || info.Container.HostConfig == nil || seen[claim] {
				continue
			}
			published := map[uint16]bool{}
			var target uint32
			for k, bindings := range info.Container.HostConfig.PortBindings {
				if string(k.Proto()) != proto {
					continue
				}
				for _, b := range bindings {
					if n, err := strconv.ParseUint(b.HostPort, 10, 16); err == nil {
						published[uint16(n)] = true
						if uint16(n) == port {
							target = uint32(k.Num())
						}
					}
				}
			}
			if published[port] {
				lo, hi := portRun(published, port)
				c.wanters = append(c.wanters, portClaim{name: claim, service: isSvc, target: target, lo: lo, hi: hi})
				seen[claim] = true
			}
		}
	}
	// The project's services with no container yet publish it too.
	for _, svc := range slices.Sorted(maps.Keys(servicePorts)) {
		if seen[svc] {
			continue
		}
		if claim, ok := serviceClaim(svc, servicePorts[svc], port, proto); ok {
			c.wanters = append(c.wanters, claim)
			seen[svc] = true
		}
	}
	slices.SortFunc(c.wanters, func(a, b portClaim) int { return strings.Compare(a.name, b.name) })

	p.mtx.Lock()
	p.portConflict = c
	p.mtx.Unlock()
}

// containerClaimName returns how a container shows in a port conflict: its service's
// name for a container of this project, else its name, with its project's when it
// has one.
func (p *consoleEventProcessor) containerClaimName(ctr container.Summary) (string, bool) {
	if ctr.Labels[api.ProjectLabel] == p.projectName && ctr.Labels[api.ServiceLabel] != "" {
		return ctr.Labels[api.ServiceLabel], true
	}
	name := ""
	if len(ctr.Names) > 0 {
		name = strings.TrimPrefix(ctr.Names[0], "/")
	}
	if proj := ctr.Labels[api.ProjectLabel]; proj != "" {
		name += " (project " + proj + ")"
	}
	return name, false
}

// portsLines returns the ports line under svc's row in a port conflict: the service
// that couldn't bind the port, with what holds it, as a failure; the service holding
// it, marked as the cause, or one not running that publishes it too, as a warning.
// None otherwise. A claim
// from a range of ports names the range, which is where the port comes from.
func (p *consoleEventProcessor) portsLines(svc string, termW int) []string {
	c := p.portConflict
	if c == nil {
		return nil
	}
	ic := p.icons()
	held := "held by another container"
	switch {
	case c.holder != nil:
		held = "held by " + c.holder.claimText()
		// Docker names the first port that clashed; two ranges can share more.
		if h := c.holder; h.lo != h.hi && c.failed.lo != c.failed.hi {
			if lo, hi := max(h.lo, c.failed.lo), min(h.hi, c.failed.hi); hi > lo {
				held += fmt.Sprintf("; the ranges overlap on %d-%d", lo, hi)
			}
		}
	case c.hostHeld:
		held = "held by a program on the host, or a container using host networking"
		if c.port == 53 {
			held += ", often systemd-resolved"
		}
	}
	mapping := func(claim portClaim) string {
		m := fmt.Sprintf("%d:%d/%s", c.port, claim.target, c.proto)
		if r := claim.rangeText(); r != "" {
			m += ", part of " + r
		}
		return m
	}
	var names []string
	for _, w := range c.wanters {
		names = append(names, w.claimText())
	}
	switch {
	case svc == c.failed.name:
		value := mapping(c.failed) + " (" + held
		if len(names) > 0 {
			value += "; also wanted by " + strings.Join(names, "; ") + ", not running"
		}
		return p.portsLine("{{|DockerMarkerError|}}"+ic.error+"{{[-]}}", "{{|DockerStatusFail|}}", value+")", termW)
	case c.holder != nil && c.holder.service && svc == c.holder.name:
		return p.portsLine("{{|DockerMarkerCause|}}"+ic.warn+"{{[-]}}", "{{|DockerStatusWarn|}}",
			mapping(*c.holder)+" (wanted by "+c.failed.name+")", termW)
	}
	for _, w := range c.wanters {
		if w.service && svc == w.name {
			return p.portsLine("{{|DockerMarkerWarn|}}"+ic.warn+"{{[-]}}", "{{|DockerStatusWarn|}}",
				mapping(w)+" ("+held+"; wanted by "+c.failed.name+")", termW)
		}
	}
	return nil
}

// portRole reports whether svc holds the port another service couldn't bind, or
// publishes it too without running.
func (p *consoleEventProcessor) portRole(svc string) (holder, wanter bool) {
	c := p.portConflict
	if c == nil {
		return false, false
	}
	if c.holder != nil && c.holder.service && c.holder.name == svc {
		return true, false
	}
	return false, slices.ContainsFunc(c.wanters, func(w portClaim) bool { return w.service && w.name == svc })
}

// portConflictText is the status of a ports line.
const portConflictText = "Conflict"

// portsLine returns a ports line: icon, Conflict in style, then value dim, wrapped to
// termW under its first word.
func (p *consoleEventProcessor) portsLine(icon, style, value string, termW int) []string {
	prefix := globalIndent + icon + " " + style + portConflictText + "{{[-]}}" +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(portConflictText)) +
		strutil.Repeat(" ", 2*sectionChildIndentW) + "{{|DockerDetailLabel|}}ports{{[-]}}{{|DockerColon|}}:{{[-]}} "
	prefixW := utf8.RuneCountInString(semstyle.ToPlain(prefix))
	room := max(termW-timerReserveW-summaryHeaderIndentW-prefixW, minDetailW)
	var lines []string
	for i, part := range wrapWords(value, room, 6) {
		lead := prefix
		if i > 0 {
			lead = strutil.Repeat(" ", prefixW)
		}
		lines = append(lines, lead+"{{[::D]}}"+part+"{{[-]}}")
	}
	return lines
}
