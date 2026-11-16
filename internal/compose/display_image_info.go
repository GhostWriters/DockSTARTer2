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
	built   string // when its publisher built it, in yyyy-mm-dd hh:mm:ss local time
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
		m.built = t.Local().Format("2006-01-02 15:04:05")
	}
	if len(r.RepoDigests) > 0 {
		if _, digest, ok := strings.Cut(r.RepoDigests[0], "@"); ok {
			m.digest = digest
		}
	}
	m.source = labels["org.opencontainers.image.source"]
	return m
}

// releaseURL returns the release page for a LinuxServer image's version, or "" for any
// other image. LinuxServer's GitHub repos tag every release with the exact version their
// labels give, so the link needs no check; other images' labels don't reliably match a
// release (2.1 vs v2.1, or a branch name), so findVersionLink checks theirs.
func releaseURL(m imageMeta) string {
	if m.version == "" || !strings.HasPrefix(m.source, "https://github.com/linuxserver/") {
		return ""
	}
	return strings.TrimSuffix(m.source, "/") + "/releases/tag/" + m.version
}

// infoPart is one piece of an info line's value, linked to url when it isn't "".
type infoPart struct{ text, url string }

// imageInfoLines returns the info lines under an image's row: with verbose its version,
// build date, digest and source; without it, only once a pull changed the image (its
// digest or version), its build date and, when that changed too, its version, each as
// "old → new". A version links to its release or tag page when versionURL has one.
func (p *consoleEventProcessor) imageInfoLines(imgName string, termW int, verbose bool) []string {
	before, hadBefore := p.imageMetaBefore[imgName]
	after, pulled := p.imageMetaAfter[imgName]
	current := before
	if pulled {
		current = after
	}
	differs := func(old, cur string) bool { return hadBefore && pulled && old != "" && cur != "" && old != cur }
	versionChanged := differs(before.version, after.version)
	imageChanged := versionChanged || differs(before.digest, after.digest)

	version := []infoPart{{current.version, p.versionURL(imgName, current)}}
	if versionChanged {
		version = []infoPart{{before.version, p.versionURL(imgName, before)}, {" → ", ""}, {after.version, p.versionURL(imgName, after)}}
	}
	built := []infoPart{{current.built, ""}}
	if imageChanged && differs(before.built, after.built) {
		built = []infoPart{{before.built, ""}, {" → ", ""}, {after.built, ""}}
	}
	type field struct {
		label string
		value []infoPart
	}
	fields := []field{{"version", version}, {"built", built}}
	switch {
	case verbose:
		fields = append(fields, field{"digest", []infoPart{{current.digest, ""}}}, field{"source", []infoPart{{current.source, current.source}}})
	case !imageChanged:
		return nil
	case !versionChanged:
		fields = fields[1:] // the version, if any, is the same as before
	}
	labelW := 0 // the widest label shown, so the values line up
	for _, f := range fields {
		if f.value[0].text != "" {
			labelW = max(labelW, len(f.label))
		}
	}
	var lines []string
	for _, f := range fields {
		if f.value[0].text != "" {
			lines = append(lines, p.infoLine(f.label, labelW, f.value, termW))
		}
	}
	return lines
}

// infoLine returns one info line: an "i" marker and Info in the status column, then
// label: value one step in under image:, all but the value in DockerInfo, the value
// padded to start labelW past the label's start. Each part of the value is linked to its
// url when it has one, else dim. A value too long for termW is cut: still linked when it
// is a single linked part (a source URL), else dim and unlinked.
func (p *consoleEventProcessor) infoLine(label string, labelW int, value []infoPart, termW int) string {
	const icon, word = "i", "Info" // a plain i: the info sign isn't in every terminal font
	prefix := globalIndent + "{{|DockerInfo|}}" + icon + "{{[-]}} {{|DockerInfo|}}" + word + "{{[-]}}" +
		strutil.Repeat(" ", sectionStatusW-utf8.RuneCountInString(word)) +
		strutil.Repeat(" ", 3*sectionChildIndentW) + "{{|DockerInfo|}}" + label + "{{[-]}}{{|DockerColon|}}:{{[-]}} " +
		strutil.Repeat(" ", max(labelW-len(label), 0))
	room := max(termW-timerReserveW-summaryHeaderIndentW-utf8.RuneCountInString(semstyle.ToPlain(prefix)), minDetailW)
	var plain strings.Builder
	for i := range value {
		value[i].text = stripControl(value[i].text)
		plain.WriteString(value[i].text)
	}
	if utf8.RuneCountInString(plain.String()) > room {
		if len(value) == 1 && value[0].url != "" {
			return prefix + console.FormatLink("URL", cutRunes(plain.String(), room), value[0].url)
		}
		return prefix + "{{[::D]}}" + cutRunes(plain.String(), room) + "{{[-]}}"
	}
	out := prefix
	for _, part := range value {
		if part.url != "" {
			out += console.FormatLink("URL", part.text, part.url)
		} else {
			out += "{{[::D]}}" + part.text + "{{[-]}}"
		}
	}
	return out
}

// layersLine returns the layers: line under an image at -vv: the layers'
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
