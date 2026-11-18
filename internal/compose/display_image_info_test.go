package compose

import (
	"strings"
	"testing"

	"github.com/GhostWriters/semstyle"
	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/image"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"DockSTARTer2/internal/console"
)

func inspectWith(labels map[string]string) image.InspectResponse {
	return image.InspectResponse{
		Created:     "2026-11-11T01:16:29.123456789Z",
		RepoDigests: []string{"lscr.io/linuxserver/bazarr@sha256:832820dec70bce9e912c6eee7a95baf6551e03dbc63325a7208daaa455423752"},
		Config:      &dockerspec.DockerOCIImageConfig{ImageConfig: ocispec.ImageConfig{Labels: labels}},
	}
}

func TestMetaFromInspect(t *testing.T) {
	ls := metaFromInspect(inspectWith(map[string]string{
		"build_version":                   "Linuxserver.io version:- v1.6.2-ls372 Build-date:- 2026-11-11T01:16:29+00:00",
		"org.opencontainers.image.source": "https://github.com/linuxserver/docker-bazarr",
	}))
	if ls.version != "v1.6.2-ls372" {
		t.Errorf("LinuxServer version = %q, want v1.6.2-ls372", ls.version)
	}
	if ls.digest != "sha256:832820dec70bce9e912c6eee7a95baf6551e03dbc63325a7208daaa455423752" {
		t.Errorf("digest = %q", ls.digest)
	}
	if ls.source != "https://github.com/linuxserver/docker-bazarr" {
		t.Errorf("source = %q", ls.source)
	}
	if !strings.HasPrefix(ls.built, "2026-11-1") || len(ls.built) != len("2006-01-02 15:04:05") {
		t.Errorf("built = %q, want yyyy-mm-dd hh:mm:ss", ls.built)
	}

	oci := metaFromInspect(inspectWith(map[string]string{"org.opencontainers.image.version": "2.37.2", "build_version": "ignored"}))
	if oci.version != "2.37.2" {
		t.Errorf("OCI version = %q, want 2.37.2 (it takes precedence)", oci.version)
	}
	if none := metaFromInspect(image.InspectResponse{}); none != (imageMeta{}) {
		t.Errorf("empty inspect = %+v, want nothing", none)
	}
}

func TestImageInfoLinesVersionChange(t *testing.T) {
	p := newHealthTestProcessor()
	p.imageMetaBefore[testImg] = imageMeta{version: "v1.6.2-ls368"}
	if lines := p.imageInfoLines(testImg, 160, false); lines != nil {
		t.Errorf("not pulled: %q, want no line without -v", lines)
	}
	p.imageMetaAfter[testImg] = imageMeta{version: "v1.6.2-ls369"}
	lines := p.imageInfoLines(testImg, 160, false)
	if len(lines) != 1 || !strings.Contains(semstyle.ToPlain(lines[0]), "v1.6.2-ls368 → v1.6.2-ls369") {
		t.Errorf("updated: %q, want the version change", plainLines(lines))
	}
	p.imageMetaAfter[testImg] = imageMeta{version: "v1.6.2-ls368"}
	if lines := p.imageInfoLines(testImg, 160, false); lines != nil {
		t.Errorf("unchanged: %q, want no line without -v", lines)
	}
}

func TestVersionLinksToLinuxServerRelease(t *testing.T) {
	const src = "https://github.com/linuxserver/docker-bazarr"
	if got := releaseURL(imageMeta{version: "v1.6.2-ls372", source: src}); got != src+"/releases/tag/v1.6.2-ls372" {
		t.Errorf("LinuxServer release URL = %q", got)
	}
	if got := releaseURL(imageMeta{version: "main", source: "https://github.com/AzuraCast/AzuraCast"}); got != "" {
		t.Errorf("other image's release URL = %q, want none", got)
	}

	p := newHealthTestProcessor()
	p.imageMetaBefore[testImg] = imageMeta{version: "v1.6.2-ls368", source: src}
	p.imageMetaAfter[testImg] = imageMeta{version: "v1.6.2-ls369", source: src}
	lines := p.imageInfoLines(testImg, 160, false)
	if len(lines) != 1 {
		t.Fatalf("lines = %q, want the version change", plainLines(lines))
	}
	for _, v := range []string{"v1.6.2-ls368", "v1.6.2-ls369"} {
		if want := console.FormatLink("URL", v, src+"/releases/tag/"+v); !strings.Contains(lines[0], want) {
			t.Errorf("line = %q, want %s linked to its release", lines[0], v)
		}
	}
	if plain := semstyle.ToPlain(lines[0]); !strings.Contains(plain, "v1.6.2-ls368 → v1.6.2-ls369") {
		t.Errorf("plain = %q, want old → new", plain)
	}
}

func TestImageInfoLinesBuiltOnImageChange(t *testing.T) {
	p := newHealthTestProcessor()
	p.imageMetaBefore[testImg] = imageMeta{version: "2.1", built: "2026-11-01 10:00:00", digest: "sha256:aaa"}

	// Rebuilt under the same version: only the build date, since the version didn't change.
	p.imageMetaAfter[testImg] = imageMeta{version: "2.1", built: "2026-11-13 21:28:03", digest: "sha256:bbb"}
	lines := plainLines(p.imageInfoLines(testImg, 160, false))
	if len(lines) != 1 || !strings.Contains(lines[0], "built: 2026-11-01 10:00:00 → 2026-11-13 21:28:03") {
		t.Errorf("rebuilt: %q, want only the build date change", lines)
	}

	// A new version: both lines.
	p.imageMetaAfter[testImg] = imageMeta{version: "2.2", built: "2026-11-13 21:28:03", digest: "sha256:bbb"}
	lines = plainLines(p.imageInfoLines(testImg, 160, false))
	if len(lines) != 2 || !strings.Contains(lines[0], "version: 2.1 → 2.2") || !strings.Contains(lines[1], "built:   2026-11-01 10:00:00 → 2026-11-13 21:28:03") {
		t.Errorf("updated: %q, want the version and build date changes", lines)
	}

	// Pulled but unchanged: nothing without -v, and with -v a plain build date.
	p.imageMetaAfter[testImg] = p.imageMetaBefore[testImg]
	if lines := p.imageInfoLines(testImg, 160, false); lines != nil {
		t.Errorf("unchanged: %q, want no lines without -v", plainLines(lines))
	}
	if lines := plainLines(p.imageInfoLines(testImg, 160, true)); len(lines) < 2 || !strings.HasSuffix(strings.TrimSpace(lines[1]), "built:   2026-11-01 10:00:00") {
		t.Errorf("unchanged with -v: %q, want the build date", lines)
	}
}

func TestImageInfoLinesOnFirstPull(t *testing.T) {
	p := newHealthTestProcessor()
	// Not on disk before: a first pull shows what arrived, without -v, as "→ new".
	p.imageMetaAfter[testImg] = imageMeta{version: "2.38.0", built: "2026-11-17 23:13:01", digest: "sha256:ccc"}
	lines := plainLines(p.imageInfoLines(testImg, 160, false))
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "version: → 2.38.0") || !strings.HasSuffix(lines[1], "built:   → 2026-11-17 23:13:01") {
		t.Errorf("first pull: %q, want its version and build date after an arrow", lines)
	}
	if got := strings.Join(lines, ""); strings.Contains(got, "digest") {
		t.Errorf("first pull: %q, want no digest without -v", lines)
	}
	// An image with no version label: only its build date.
	p.imageMetaAfter[testImg] = imageMeta{built: "2026-11-17 23:13:01"}
	if lines := plainLines(p.imageInfoLines(testImg, 160, false)); len(lines) != 1 || !strings.HasSuffix(lines[0], "built: → 2026-11-17 23:13:01") {
		t.Errorf("first pull without a version: %q, want only the build date", lines)
	}
}

func TestInfoValuesLineUp(t *testing.T) {
	p := newHealthTestProcessor()
	p.imageMetaBefore[testImg] = imageMeta{version: "2.1", built: "2026-11-01 10:00:00", digest: "sha256:aaa", source: "https://example.com/app"}
	lines := plainLines(p.imageInfoLines(testImg, 160, true))
	col := -1
	for _, l := range lines {
		_, value, _ := strings.Cut(l, ": ")
		c := strings.Index(l, strings.TrimLeft(value, " "))
		if col >= 0 && c != col {
			t.Errorf("values start at different columns: %q", lines)
			break
		}
		col = c
	}
}
