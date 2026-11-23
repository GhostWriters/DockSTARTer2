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
// A claim from a range of ports names the range, which is where the port comes from.
func (p *consoleEventProcessor) portsLines(svc string, termW int) []string {
	c := p.portConflict
	if c == nil || !c.involves(svc) {
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

// involves reports whether svc is the service that couldn't bind the port, the one
// holding it, or one not running that publishes it too.
func (c *portConflict) involves(svc string) bool {
	return svc == c.failed.name || c.holder != nil && c.holder.service && c.holder.name == svc ||
		slices.ContainsFunc(c.wanters, func(w portClaim) bool { return w.service && w.name == svc })
}

// portConflictText is the status of a ports line.
const portConflictText = "Conflict"

// portsLine returns a ports line with Conflict as its status (portsLineAs).
func (p *consoleEventProcessor) portsLine(icon, style, value string, termW int) []string {
	return p.portsLineAs(portConflictText, icon, style, value, termW)
}

// portsLineAs returns a ports line: icon, word in style, then value, wrapped to termW
// under its first word. The value is dim except each host port (or range), which
// clashes, in the icon's style, and each message in parentheses, which says why, in
// style: a warning's or an error's. A service holding the port is a warning marked as
// the cause, so its port shows red and its message yellow.
func (p *consoleEventProcessor) portsLineAs(word, icon, style, value string, termW int) []string {
	prefix := globalIndent + icon + " " + style + word + "{{[-]}}" +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(word)) +
		strutil.Repeat(" ", 2*sectionChildIndentW) + "{{|DockerDetailLabel|}}ports{{[-]}}{{|DockerColon|}}:{{[-]}} "
	prefixW := utf8.RuneCountInString(semstyle.ToPlain(prefix))
	room := max(termW-timerReserveW-summaryHeaderIndentW-prefixW, minDetailW)
	portStyle := style
	if strings.HasPrefix(icon, "{{|") {
		if end := strings.Index(icon, "|}}"); end >= 0 {
			portStyle = icon[:end+3]
		}
	}
	kinds := classifyPortsValue(value)
	var lines []string
	cursor := 0
	for i, part := range wrapWords(value, room, 6) {
		lead := prefix
		if i > 0 {
			lead = strutil.Repeat(" ", prefixW)
		}
		// The last line, when more was cut, ends in an ellipsis that isn't in value.
		body, tail := part, ""
		if strings.HasSuffix(part, "…") {
			body, tail = strings.TrimSuffix(part, "…"), "…"
			if strings.HasSuffix(body, " ") {
				body, tail = strings.TrimSuffix(body, " "), " …"
			}
		}
		start := -1
		if at := strings.Index(value[cursor:], body); at >= 0 {
			start = cursor + at
			cursor = start + len(body)
		}
		styled := styleValuePart(body, start, kinds, portStyle, style)
		if tail != "" {
			last := byte(portDim)
			if start >= 0 && len(body) > 0 {
				last = kinds[start+len(body)-1]
			}
			styled += styleValueRun(tail, last, portStyle, style)
		}
		lines = append(lines, lead+styled)
	}
	return lines
}

// How a byte of a ports value is styled.
const (
	portDim     = iota // text around the ports
	portHost           // a host port or range, which clashes
	portMessage        // a message in parentheses, which says why
)

// classifyPortsValue returns how each byte of a ports value is styled. The value is
// entries separated by ", " outside parentheses, each a host port or range (80, 80-90)
// with what follows it, and a message in parentheses to its end.
func classifyPortsValue(value string) []byte {
	kinds := make([]byte, len(value))
	digit := func(c byte) bool { return c >= '0' && c <= '9' }
	entryStart := 0
	flush := func(end int) {
		entry := value[entryStart:end]
		i := 0
		for i < len(entry) && digit(entry[i]) {
			i++
		}
		if i > 0 && i+1 < len(entry) && entry[i] == '-' && digit(entry[i+1]) {
			i++
			for i < len(entry) && digit(entry[i]) {
				i++
			}
		}
		for k := 0; k < i; k++ {
			kinds[entryStart+k] = portHost
		}
		if at := strings.Index(entry, " ("); at >= 0 {
			for k := at + 1; k < len(entry); k++ {
				kinds[entryStart+k] = portMessage
			}
		}
	}
	depth := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 && i+1 < len(value) && value[i+1] == ' ' {
				flush(i)
				entryStart = i + 2
			}
		}
	}
	flush(len(value))
	return kinds
}

// styleValuePart returns part, which starts at byte start of a ports value (-1 when it
// is no part of it), styled by kinds: dim, a host port in portStyle, a message in style.
func styleValuePart(part string, start int, kinds []byte, portStyle, style string) string {
	if start < 0 {
		return styleValueRun(part, portDim, portStyle, style)
	}
	var b strings.Builder
	for j := 0; j < len(part); {
		k := j
		for k < len(part) && kinds[start+k] == kinds[start+j] {
			k++
		}
		b.WriteString(styleValueRun(part[j:k], kinds[start+j], portStyle, style))
		j = k
	}
	return b.String()
}

// styleValueRun returns text in the style of kind.
func styleValueRun(text string, kind byte, portStyle, style string) string {
	switch kind {
	case portHost:
		return portStyle + text + "{{[-]}}"
	case portMessage:
		return style + text + "{{[-]}}"
	}
	return "{{[::D]}}" + text + "{{[-]}}"
}

// ── Predicted conflicts, before the run ────────────────────────────────────

// predictedConflict is a run of host ports a service of the run publishes that
// something else claims too: a running container outside the run, which holds them,
// or another service of the run, which wants them too. target is the service's
// container port for a single port.
type predictedConflict struct {
	lo, hi uint16
	proto  string
	target uint32
	other  string
	inRun  bool
}

// portSpec is one host port a service or container publishes.
type portSpec struct {
	ip     string
	port   uint16
	proto  string
	target uint32
}

// serviceSpecs returns the host ports ports publish, a range expanded to each port.
func serviceSpecs(ports []types.ServicePortConfig) []portSpec {
	var out []portSpec
	for _, p := range ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
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
			out = append(out, portSpec{ip: p.HostIP, port: uint16(n), proto: proto, target: p.Target + uint32(n-first)})
		}
	}
	return out
}

// hostIPsOverlap reports whether two host addresses a port is published on can
// clash: the same address, or either one unspecified, which binds them all.
func hostIPsOverlap(a, b string) bool {
	wild := func(s string) bool { return s == "" || s == "0.0.0.0" || s == "::" }
	return wild(a) || wild(b) || a == b
}

// predictPortConflicts finds, before the run, the host ports each service of run
// publishes that a running container outside the run holds, or another service of
// run publishes too, for the unconfirmed ports lines. A service's own container, and
// those of the other services in run, aren't holders: compose recreates or keeps them.
func (p *consoleEventProcessor) predictPortConflicts(ctx context.Context, run []string) {
	if p.dockerClient == nil {
		return
	}
	inRun := make(map[string]bool, len(run))
	for _, svc := range run {
		inRun[svc] = true
	}
	type claimant struct {
		name  string
		inRun bool
		specs []portSpec
	}
	var others []claimant
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if res, err := p.dockerClient.ContainerList(ctx, dockerclient.ContainerListOptions{}); err == nil {
		for _, ctr := range res.Items {
			name, isSvc := p.containerClaimName(ctr)
			if isSvc && inRun[name] {
				continue
			}
			var specs []portSpec
			for _, pt := range ctr.Ports {
				if pt.PublicPort == 0 {
					continue
				}
				ip := ""
				if pt.IP.IsValid() {
					ip = pt.IP.String()
				}
				specs = append(specs, portSpec{ip: ip, port: pt.PublicPort, proto: pt.Type, target: uint32(pt.PrivatePort)})
			}
			if len(specs) > 0 {
				others = append(others, claimant{name: name, specs: specs})
			}
		}
	}
	for _, svc := range slices.Sorted(slices.Values(run)) {
		others = append(others, claimant{name: svc, inRun: true, specs: serviceSpecs(p.servicePorts[svc])})
	}

	predicted := map[string][]predictedConflict{}
	for _, svc := range run {
		mine := serviceSpecs(p.servicePorts[svc])
		if len(mine) == 0 {
			continue
		}
		for _, o := range others {
			if o.inRun && o.name == svc {
				continue
			}
			for _, proto := range []string{"tcp", "udp", "sctp"} {
				shared := map[uint16]bool{}
				targets := map[uint16]uint32{}
				for _, m := range mine {
					if m.proto != proto {
						continue
					}
					for _, s := range o.specs {
						if s.proto == proto && s.port == m.port && hostIPsOverlap(m.ip, s.ip) {
							shared[m.port], targets[m.port] = true, m.target
						}
					}
				}
				for _, port := range slices.Sorted(maps.Keys(shared)) {
					if shared[port-1] && port > 0 {
						continue // inside a run already started
					}
					lo, hi := portRun(shared, port)
					predicted[svc] = append(predicted[svc], predictedConflict{lo: lo, hi: hi, proto: proto, target: targets[lo], other: o.name, inRun: o.inRun})
				}
			}
		}
	}
	for _, list := range predicted {
		slices.SortFunc(list, func(a, b predictedConflict) int {
			if a.lo != b.lo {
				return int(a.lo) - int(b.lo)
			}
			return strings.Compare(a.other, b.other)
		})
	}
	p.mtx.Lock()
	p.predicted = predicted
	p.mtx.Unlock()
}

// predictedLines returns svc's unconfirmed ports line: its predicted conflicts, until
// compose reports its container running (a false alarm, gone) or the conflict
// confirmed (portsLines' own line instead). One the run never reached stays.
func (p *consoleEventProcessor) predictedLines(svc string, termW int) []string {
	conflicts := p.predicted[svc]
	if len(conflicts) == 0 || p.containerStarted[svc] || p.portConflict != nil && p.portConflict.involves(svc) {
		return nil
	}
	// One entry per run of ports, naming everything that claims it.
	type entry struct {
		ports          string
		held, wantedBy []string
	}
	var entries []*entry
	byPorts := map[string]*entry{}
	for _, c := range conflicts {
		ports := fmt.Sprintf("%d:%d/%s", c.lo, c.target, c.proto)
		if c.lo != c.hi {
			ports = fmt.Sprintf("%d-%d/%s", c.lo, c.hi, c.proto)
		}
		e := byPorts[ports]
		if e == nil {
			e = &entry{ports: ports}
			byPorts[ports] = e
			entries = append(entries, e)
		}
		if c.inRun {
			e.wantedBy = append(e.wantedBy, c.other)
		} else {
			e.held = append(e.held, c.other)
		}
	}
	var parts []string
	for _, e := range entries {
		var how []string
		if len(e.held) > 0 {
			how = append(how, "held by "+strings.Join(e.held, ", "))
		}
		if len(e.wantedBy) > 0 {
			how = append(how, "also wanted by "+strings.Join(e.wantedBy, ", "))
		}
		parts = append(parts, e.ports+" ("+strings.Join(how, "; ")+")")
	}
	return p.portsLineAs(portPossibleText, "{{|DockerMarkerWarn|}}"+p.icons().unsure+"{{[-]}}", "{{|DockerStatusWarn|}}", strings.Join(parts, ", "), termW)
}

// portPossibleText is the status of a predicted conflict's ports line; its unsure
// marker tells it apart from a confirmed one.
const portPossibleText = portConflictText

// possibleGroup is a set of services in one folded group with identical possible
// port conflicts: their ports, and the unconfirmed line's text.
type possibleGroup struct {
	members []string
	text    string
}

// splitPossible splits a folded group's services names by their possible port
// conflicts: those with none, then one group per identical set of conflicts. Two
// services' conflicts are identical when they claim the same runs of host ports with
// the same containers outside the run holding them; other services of the run wanting
// them too are named only when they aren't in the group. A service whose container
// compose reports running, or that a confirmed conflict involves, has none.
func (p *consoleEventProcessor) splitPossible(names []string) (plain []string, groups []possibleGroup) {
	type portEntry struct {
		ports string
		held  []string
	}
	signature := func(svc string) []portEntry {
		if p.containerStarted[svc] || p.portConflict != nil && p.portConflict.involves(svc) {
			return nil
		}
		var out []portEntry
		index := map[string]int{}
		for _, c := range p.predicted[svc] {
			ports := fmt.Sprintf("%d/%s", c.lo, c.proto)
			if c.lo != c.hi {
				ports = fmt.Sprintf("%d-%d/%s", c.lo, c.hi, c.proto)
			}
			i, ok := index[ports]
			if !ok {
				i = len(out)
				index[ports] = i
				out = append(out, portEntry{ports: ports})
			}
			if !c.inRun && !slices.Contains(out[i].held, c.other) {
				out[i].held = append(out[i].held, c.other)
			}
		}
		for i := range out {
			slices.Sort(out[i].held)
		}
		return out
	}
	key := func(sig []portEntry) string {
		var b strings.Builder
		for _, e := range sig {
			b.WriteString(e.ports + "|" + strings.Join(e.held, ",") + ";")
		}
		return b.String()
	}
	byKey := map[string]int{}
	var sigs [][]portEntry
	for _, svc := range names {
		sig := signature(svc)
		if len(sig) == 0 {
			plain = append(plain, svc)
			continue
		}
		k := key(sig)
		i, ok := byKey[k]
		if !ok {
			i = len(groups)
			byKey[k] = i
			groups = append(groups, possibleGroup{})
			sigs = append(sigs, sig)
		}
		groups[i].members = append(groups[i].members, svc)
	}
	for i := range groups {
		var parts []string
		for _, e := range sigs[i] {
			var how []string
			if len(e.held) > 0 {
				how = append(how, "held by "+strings.Join(e.held, ", "))
			}
			// Other services of the run wanting these ports, outside the group.
			var wanted []string
			for _, svc := range groups[i].members {
				for _, c := range p.predicted[svc] {
					ports := fmt.Sprintf("%d/%s", c.lo, c.proto)
					if c.lo != c.hi {
						ports = fmt.Sprintf("%d-%d/%s", c.lo, c.hi, c.proto)
					}
					if ports == e.ports && c.inRun && !slices.Contains(groups[i].members, c.other) && !slices.Contains(wanted, c.other) {
						wanted = append(wanted, c.other)
					}
				}
			}
			if len(wanted) > 0 {
				slices.Sort(wanted)
				how = append(how, "also wanted by "+strings.Join(wanted, ", "))
			}
			part := e.ports
			if len(how) > 0 {
				part += " (" + strings.Join(how, "; ") + ")"
			}
			parts = append(parts, part)
		}
		groups[i].text = strings.Join(parts, ", ")
	}
	return plain, groups
}

// possibleGroupLine returns a possible group's unconfirmed ports line.
func (p *consoleEventProcessor) possibleGroupLine(g possibleGroup, termW int) []string {
	return p.portsLineAs(portPossibleText, "{{|DockerMarkerWarn|}}"+p.icons().unsure+"{{[-]}}", "{{|DockerStatusWarn|}}", g.text, termW)
}
