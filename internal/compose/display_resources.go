package compose

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/dockerlayout"
	"DockSTARTer2/internal/strutil"
)

// existingText is the group of networks and volumes that already exist, which the
// run left as they were.
const existingText = "Existing"

// resourceDecl is the networks or volumes the listed services use, by name, and
// which of them exist before the run.
type resourceDecl struct {
	names  []string
	exists map[string]bool
}

// resourceKind is what a networks: or volumes: section shows its rows with.
type resourceKind struct {
	section, singular, plural string
	nameTag                   string // the style of a name, e.g. {{|IPAddress|}}
	statusTag                 func(api.EventStatus, string, string) string
}

var (
	networkKind = resourceKind{"networks", "network", "networks", "{{|IPAddress|}}", networkStatusTag}
	volumeKind  = resourceKind{"volumes", "volume", "volumes", "{{|Folder|}}", volumeStatusTag}
)

// groupsResources reports whether the networks: and volumes: sections group their
// rows like the services: section, listing every one the services use: on the
// commands that bring services up, while nothing has failed.
func (p *consoleEventProcessor) groupsResources() bool {
	return p.groupsRows() && p.command != "pull"
}

// buildResourceLines returns a networks: or volumes: section grouped like services:
// the ones the run created or removed (eventIDs) under a changed heading, the rest
// under an unchanged one, grouped Not Created (until compose creates them) and
// Existing; folded, a group is a line of names, with -v their own rows.
func (p *consoleEventProcessor) buildResourceLines(kind resourceKind, decl resourceDecl, eventIDs []string, termW, level int) ([]string, []timerEntry) {
	names := slices.Clone(decl.names)
	for _, id := range eventIDs {
		if !slices.Contains(names, id) {
			names = append(names, id)
		}
	}
	if len(names) == 0 {
		return nil, nil
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

	// The section's and each heading's state: the changed rows' rollup, and for the rest
	// Complete, or Pending while some aren't created yet.
	unchangedState := rollupComplete
	if len(groups[notCreatedText]) > 0 {
		unchangedState = rollupPending
	}
	changedState := p.rollupState(changed, nil)
	sectionState := unchangedState
	switch {
	case len(changed) == 0:
	case changedState != rollupComplete || unchangedState == rollupComplete:
		sectionState = changedState
	default:
		sectionState = rollupProcessing // some created, others still to come
	}
	stateLine := func(state sectionRollupState) (icon, tag, text string) {
		text, stTag, iconTag := sectionStatusText(state)
		icon = iconTag + p.sectionRollupIcon(state) + "{{[-]}}"
		if state == rollupProcessing {
			icon = p.activeSpinnerTag(p.icons().spinner)
		}
		return icon, stTag + text + "{{[-]}}", text
	}

	icon, tag, text := stateLine(sectionState)
	lines := []string{globalIndent + icon + " " + tag + strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(text)) +
		"{{|DockerSection|}}" + kind.section + "{{[-]}}{{|DockerColon|}}:{{[-]}}"}
	timers := []timerEntry{{task: p.sectionTaskFor(changed), style: timerSection}}
	add := func(line string, t *consoleTask, style timerStyle) {
		lines = append(lines, line)
		timers = append(timers, timerEntry{task: t, style: style})
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

	open := level >= dockerlayout.VerboseDetails
	unchanged := append(slices.Clone(groups[notCreatedText]), groups[existingText]...)
	for _, set := range []struct {
		adjective string
		names     []string
		state     sectionRollupState
	}{{"changed", changed, changedState}, {"unchanged", unchanged, unchangedState}} {
		if len(set.names) == 0 {
			continue
		}
		icon, tag, text := stateLine(set.state)
		add(groupHeading(icon, tag, text, len(set.names), set.adjective, kind.singular, kind.plural), nil, timerSection)
		start := len(lines)
		if set.adjective == "changed" || open {
			for _, name := range set.names {
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
		for i := start; i < len(lines); i++ {
			lines[i] = nestLine(lines[i])
		}
	}
	return lines, timers
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
