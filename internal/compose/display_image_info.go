package compose

import (
	"context"
	"fmt"
	"regexp"
	"slices"
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
	id      string // its local image ID, to tell which image a container ran
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
	m := imageMeta{id: r.ID}
	var labels map[string]string
	if r.Config != nil {
		labels = r.Config.Labels
	}
	switch {
	case labels["org.opencontainers.image.version"] != "":
		m.version = labels["org.opencontainers.image.version"]
	case buildVersionRE.MatchString(labels["build_version"]):
		m.version = buildVersionRE.FindStringSubmatch(labels["build_version"])[1]
	case labels["org.label-schema.version"] != "":
		m.version = labels["org.label-schema.version"]
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

// inspectPreviousImages records, for each container running a different image than
// the one now on disk for its service (one pulled since it started, by an earlier
// pull or another tool), that image's details (prevMeta), or that it's gone
// (prevGone), so a run that recreates it onto the newer one can show old → new.
// Called from preflight, after imageMetaBefore is known.
func (p *consoleEventProcessor) inspectPreviousImages(ctx context.Context) {
	p.prevMeta = map[string]imageMeta{}
	p.prevGone = map[string]bool{}
	for svc, oldID := range p.preRunImage {
		img := p.imageForService(svc)
		onDisk, ok := p.imageMetaBefore[img]
		if img == "" || !ok || oldID == "" || oldID == onDisk.id {
			continue
		}
		info, err := p.dockerClient.ImageInspect(ctx, oldID)
		if err != nil {
			p.prevGone[svc] = true
			continue
		}
		meta := metaFromInspect(info.InspectResponse)
		p.prevMeta[svc] = meta
		p.checkVersionLink(img, meta)
	}
}

// broughtUp reports whether this run brought svc's container up: created, started
// or recreated.
func (p *consoleEventProcessor) broughtUp(svc string) bool {
	t := p.tasks[svc]
	if t == nil || t.status != api.Done {
		return false
	}
	switch t.text {
	case api.StatusCreated, api.StatusStarted, "Recreated":
		return true
	}
	return false
}

// containerChange returns, for an image with a service this run brought up, the
// details of the image that service's container ran before when it was a different
// one (prev, hadPrev), or that that image is gone (gone). brought reports whether any
// service of the image was brought up at all.
func (p *consoleEventProcessor) containerChange(imgName string) (prev imageMeta, hadPrev, gone, brought bool) {
	for _, svc := range p.serviceIDsForImage(imgName) {
		if !p.broughtUp(svc) {
			continue
		}
		brought = true
		if m, ok := p.prevMeta[svc]; ok && !hadPrev {
			prev, hadPrev = m, true
		}
		gone = gone || p.prevGone[svc]
	}
	return prev, hadPrev, gone && !hadPrev, brought
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
// "old → new", or once a first pull brought it, its version and build date as "→ new".
// A container this run brought up shows the version it now runs, as "old → new" (and
// its build date) when it ran a different image before, e.g. one pulled by an earlier
// pull, with verbose too. A version links to its release or tag page when versionURL
// has one; a version a Docker Hub tag gave its digest (tagVersions) replaces its labels'.
func (p *consoleEventProcessor) imageInfoLines(imgName string, termW int, verbose bool) []string {
	before, hadBefore := p.imageMetaBefore[imgName]
	after, pulled := p.imageMetaAfter[imgName]
	labelled := before // the current image's details as its labels give them
	if pulled {
		labelled = after
	}
	before, after = p.tagVersions(before, after)
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

	// A container brought up onto a different image than it ran before: old → new,
	// when this run's own pull doesn't already show a change.
	prev, hadPrev, prevGone, brought := p.containerChange(imgName)
	if hadPrev {
		prev, _ = p.tagVersions(prev, labelled)
	}
	moved := hadPrev && !imageChanged
	movedVersion := moved && prev.version != "" && current.version != "" && prev.version != current.version
	movedBuilt := moved && prev.built != "" && current.built != "" && prev.built != current.built
	if movedVersion {
		version = []infoPart{{prev.version, p.versionURL(imgName, prev)}, {" → ", ""}, {current.version, p.versionURL(imgName, current)}}
	}
	if movedBuilt {
		built = []infoPart{{prev.built, ""}, {" → ", ""}, {current.built, ""}}
	}

	type field struct {
		label string
		value []infoPart
	}
	fields := []field{{"version", version}, {"built", built}}
	// arrived marks what came with nothing to compare to, a first pull's or a
	// container's whose old image is gone: an arrow before it to show the change.
	arrived := func(v infoPart) []infoPart {
		if v.text == "" {
			return []infoPart{v} // nothing to show
		}
		return []infoPart{{"→ ", ""}, v}
	}
	// withBuilt is one line for a version that didn't change: the version, its build
	// date (or the date's change) in brackets after it; just the build date for an
	// image with no version label. A changed version keeps two lines: with both dates
	// as well, one line would be too wide.
	withBuilt := func(v, b []infoPart) []field {
		switch {
		case current.version == "":
			return []field{{"built", b}}
		case current.built == "":
			return []field{{"version", v}}
		}
		parts := append(slices.Clone(v), infoPart{" (", ""})
		parts = append(append(parts, b...), infoPart{")", ""})
		return []field{{"version", parts}}
	}
	switch {
	case verbose:
		fields = append(fields, field{"digest", []infoPart{{current.digest, ""}}}, field{"source", []infoPart{{current.source, current.source}}})
	case (pulled && !hadBefore || prevGone && !imageChanged) && current.version == "":
		fields = []field{{"built", arrived(built[0])}}
	case pulled && !hadBefore, prevGone && !imageChanged:
		fields = withBuilt(arrived(version[0]), built)
	case imageChanged && !versionChanged, moved && !movedVersion:
		fields = withBuilt(version, built) // rebuilt under the same version
	case imageChanged, moved:
		// A new version: version and build date each old → new, on two lines.
	case brought:
		fields = withBuilt(version, built) // the version it now runs
	default:
		return nil
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
