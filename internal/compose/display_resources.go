package compose

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// existingText is the group of networks, volumes and models that already exist, which
// the run left as they were (a model's routine configure aside).
const existingText = "Existing"

// resourceDecl is the networks or volumes the listed services use, by name, and
// which of them exist before the run.
type resourceDecl struct {
	names  []string
	exists map[string]bool
}

// resourceKind is what networks' or volumes' rows show.
type resourceKind struct {
	singular, plural string
	nameTag          string // the style of a name, e.g. {{|IPAddress|}}
	statusTag        func(api.EventStatus, string, string) string
}

var (
	networkKind = resourceKind{"network", "networks", "{{|IPAddress|}}", networkStatusTag}
	volumeKind  = resourceKind{"volume", "volumes", "{{|Folder|}}", volumeStatusTag}
)

// groupsResources reports whether networks and volumes are grouped like services,
// listing every one the services use: on the commands that bring services up, while
// nothing has failed. A teardown groups only the ones compose removes.
func (p *consoleEventProcessor) groupsResources() bool {
	return p.groupsRows() && p.command != "pull" && !p.isTeardownCommand()
}

// resourceGroups returns networks' or volumes' changed and unchanged sets: the ones
// the run created or removed (eventIDs) changed, the rest unchanged, grouped Not
// Created (until compose creates them) and Existing; folded, a group is a line of
// names, with -v their own rows.
func (p *consoleEventProcessor) resourceGroups(kind resourceKind, decl resourceDecl, eventIDs []string, termW, level int) kindGroups {
	open := level >= dockerlayout.VerboseDetails
	out := kindGroups{singular: kind.singular, plural: kind.plural, folded: !open}
	names := slices.Clone(decl.names)
	for _, id := range eventIDs {
		if !slices.Contains(names, id) {
			names = append(names, id)
		}
	}
	slices.Sort(names)
	var changed []string
	groups := map[string][]string{}
	for _, name := range names {
		switch {
		case p.tasks[name] != nil:
			changed = append(changed, name)
		case decl.exists[name]:
			groups[existingText] = append(groups[existingText], name)
		case p.finished:
			changed = append(changed, name) // never created, nor reported: its own row
		default:
			groups[notCreatedText] = append(groups[notCreatedText], name)
		}
	}

	// Each set's state: the changed rows' rollup, and for the rest Complete, or Pending
	// while some aren't created yet.
	unchangedState := rollupComplete
	if len(groups[notCreatedText]) > 0 {
		unchangedState = rollupPending
	}
	var lines []string
	var timers []timerEntry
	add := func(line string, t *consoleTask, style timerStyle) {
		lines = append(lines, line)
		timers = append(timers, timerEntry{task: t, style: style})
	}
	// take returns the lines added so far, starting over.
	take := func() ([]string, []timerEntry) {
		l, t := lines, timers
		lines, timers = nil, nil
		return l, t
	}
	row := func(name string) {
		t := p.tasks[name]
		var icon, statusText, statusTag string
		switch {
		case t != nil:
			icon, statusText, statusTag = p.spinnerIcon(t), abbreviateStatus(t.text), kind.statusTag(t.status, t.text, p.command)
		case decl.exists[name]:
			icon, statusText, statusTag = "{{|DockerMarkerDone|}}"+p.icons().done+"{{[-]}}", existingText, serviceFinalTag+existingText+"{{[-]}}"
		default:
			icon, statusText, statusTag = "{{|DockerStatusPending|}}"+p.icons().pending+"{{[-]}}", notCreatedText, "{{|DockerStatusPending|}}"+notCreatedText+"{{[-]}}"
		}
		add(globalIndent+icon+" "+statusTag+strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(statusText))+
			sectionChildIndent+kind.nameTag+name+"{{[-]}}{{|DockerColon|}}:{{[-]}}", t, timerService)
		if line := p.errorLine(t); line != "" {
			add(line, nil, timerLayer)
		}
	}

	for _, name := range changed {
		row(name)
	}
	out.changed = groupSet{count: len(changed), state: p.rollupState(changed, nil), timer: p.sectionTaskFor(changed)}
	out.changed.lines, out.changed.timers = take()

	unchanged := append(slices.Clone(groups[notCreatedText]), groups[existingText]...)
	if open {
		for _, name := range unchanged {
			row(name)
		}
	} else {
		for _, word := range []string{notCreatedText, existingText} {
			if len(groups[word]) == 0 {
				continue
			}
			for _, line := range p.resourceGroupLines(kind, word, groups[word], termW) {
				add(line, nil, timerLayer)
			}
		}
	}
	out.unchanged = groupSet{count: len(unchanged), state: unchangedState}
	out.unchanged.lines, out.unchanged.timers = take()
	return out
}

// resourceGroupLines returns a folded network or volume group: groupLines' layout,
// the names in the kind's own style rather than a service's.
func (p *consoleEventProcessor) resourceGroupLines(kind resourceKind, word string, names []string, termW int) []string {
	lines := p.groupLines(word, word == notCreatedText, names, kind.singular, kind.plural, termW)
	for i, line := range lines {
		for _, name := range names {
			line = strings.ReplaceAll(line, dockerlayout.StyleServiceName(name), kind.nameTag+name+"{{[-]}}")
		}
		lines[i] = line
	}
	return lines
}
