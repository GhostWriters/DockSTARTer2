package compose

import (
	"strings"
	"testing"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/image"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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
	if !strings.HasPrefix(ls.created, "2026-11-1") || len(ls.created) != len("2006-01-02 15:04:05") {
		t.Errorf("created = %q, want yyyy-mm-dd hh:mm:ss", ls.created)
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
	if len(lines) != 1 || !strings.Contains(lines[0], "v1.6.2-ls368 → v1.6.2-ls369") {
		t.Errorf("updated: %q, want the version change", plainLines(lines))
	}
	p.imageMetaAfter[testImg] = imageMeta{version: "v1.6.2-ls368"}
	if lines := p.imageInfoLines(testImg, 160, false); lines != nil {
		t.Errorf("unchanged: %q, want no line without -v", lines)
	}
}
