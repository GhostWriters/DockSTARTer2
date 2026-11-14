package compose

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GhostWriters/semstyle"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/image"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
)

// imageMeta is what the info lines under an image show about it.
type imageMeta struct {
	version string // the app version its labels give
	created string // when it was built, in yyyy-mm-dd hh:mm:ss local time
	digest  string // its repo digest (sha256:…)
	source  string // its source repository URL
}

// buildVersionRE finds the version in a LinuxServer build_version label, e.g.
// "Linuxserver.io version:- v1.6.2-ls372 Build-date:- 2026-11-11T01:16:29+00:00".
var buildVersionRE = regexp.MustCompile(`version:-\s*(\S+)`)

// metaFromInspect returns an image's details from its inspect, each left empty when
// the image doesn't say.
func metaFromInspect(r image.InspectResponse) imageMeta {
	var m imageMeta
	var labels map[string]string
	if r.Config != nil {
		labels = r.Config.Labels
	}
	switch {
	case labels["org.opencontainers.image.version"] != "":
		m.version = labels["org.opencontainers.image.version"]
	case buildVersionRE.MatchString(labels["build_version"]):
		m.version = buildVersionRE.FindStringSubmatch(labels["build_version"])[1]
	default:
		m.version = labels["version"]
	}
	if t, err := time.Parse(time.RFC3339Nano, r.Created); err == nil {
		m.created = t.Local().Format("2006-01-02 15:04:05")
	}
	if len(r.RepoDigests) > 0 {
		if _, digest, ok := strings.Cut(r.RepoDigests[0], "@"); ok {
			m.digest = digest
		}
	}
	m.source = labels["org.opencontainers.image.source"]
	return m
}

// imageInfoLines returns the info lines under an image's row: with verbose its version,
// build date, digest and source; without it only its version, and only when a pull
// changed it ("old → new").
func (p *consoleEventProcessor) imageInfoLines(imgName string, termW int, verbose bool) []string {
	before, hadBefore := p.imageMetaBefore[imgName]
	after, pulled := p.imageMetaAfter[imgName]
	current := before
	if pulled {
		current = after
	}
	version := current.version
	changed := hadBefore && pulled && before.version != "" && after.version != "" && before.version != after.version
	if changed {
		version = before.version + " → " + after.version
	}
	if !verbose {
		if !changed {
			return nil
		}
		return []string{p.infoLine("version", version, "", termW)}
	}
	var lines []string
	for _, f := range []struct{ label, value, url string }{
		{"version", version, ""},
		{"created", current.created, ""},
		{"digest", current.digest, ""},
		{"source", current.source, current.source},
	} {
		if f.value != "" {
			lines = append(lines, p.infoLine(f.label, f.value, f.url, termW))
		}
	}
	return lines
}

// infoLine returns one info line: an "i" marker and Info in the status column, then
// label: value one step in under image:, all but the value in DockerInfo; the value is
// cut to fit termW and linked to url when given.
func (p *consoleEventProcessor) infoLine(label, value, url string, termW int) string {
	const icon, word = "i", "Info" // a plain i: the info sign isn't in every terminal font
	prefix := globalIndent + "{{|DockerInfo|}}" + icon + "{{[-]}} {{|DockerInfo|}}" + word + "{{[-]}}" +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(word)) +
		strutil.Repeat(" ", 3*sectionChildIndentW) + "{{|DockerInfo|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}} "
	room := max(termW-timerReserveW-summaryHeaderIndentW-utf8.RuneCountInString(semstyle.ToPlain(prefix)), minDetailW)
	value = cutRunes(stripControl(value), room)
	if url != "" {
		return prefix + console.FormatLink("URL", value, url)
	}
	return prefix + "{{[::D]}}" + value + "{{[-]}}"
}

// layersLine returns the layers: line under an image in verbose mode: the layers'
// overall state in the status column, then a count of them by state.
func (p *consoleEventProcessor) layersLine(layers []*consoleTask) string {
	var cached, downloaded, active, failed int
	for _, t := range layers {
		switch {
		case t.status == api.Error:
			failed++
		case !t.completed():
			active++
		case t.text == "Already exists":
			cached++
		default:
			downloaded++
		}
	}
	ic := p.icons()
	var icon, word, style string
	switch {
	case failed > 0:
		icon, word, style = "{{|DockerMarkerError|}}"+ic.error+"{{[-]}}", "Failed", "{{|DockerStatusFail|}}"
	case active > 0:
		icon, word, style = p.activeSpinnerTag(ic.spinner), "Pulling", "{{|DockerStatusActive|}}"
	case downloaded > 0:
		icon, word, style = "{{|DockerMarkerDone|}}"+ic.done+"{{[-]}}", "Downloaded", detailFinalTag
	default:
		icon, word, style = "{{|DockerMarkerDone|}}"+ic.done+"{{[-]}}", "Cached", detailFinalTag
	}
	var counts []string
	for _, c := range []struct {
		n    int
		what string
	}{{failed, "failed"}, {active, "pulling"}, {downloaded, "downloaded"}, {cached, "cached"}} {
		if c.n > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", c.n, c.what))
		}
	}
	return globalIndent + icon + " " + style + word + "{{[-]}}" +
		strutil.Repeat(" ", max(sectionStatusW-utf8.RuneCountInString(word), 1)) +
		strutil.Repeat(" ", 3*sectionChildIndentW) + "{{|DockerDetailLabel|}}layers{{[-]}}{{|DockerColon|}}:{{[-]}} " +
		"{{[::D]}}" + strings.Join(counts, ", ") + "{{[-]}}"
}
