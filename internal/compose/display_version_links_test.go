package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withVersionCheckServer points the version link checks at a local server that answers
// 200 for the paths in ok (HEAD) and hands out a token for ghcr.
func withVersionCheckServer(t *testing.T, ok ...string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"token":"anon"}`))
			return
		}
		for _, p := range ok {
			if r.URL.Path == p && (!strings.Contains(p, "/manifests/") || r.Header.Get("Authorization") == "Bearer anon") {
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	oldGitHub, oldGHCR, oldHub := githubBaseURL, ghcrBaseURL, dockerHubBaseURL
	githubBaseURL, ghcrBaseURL, dockerHubBaseURL = srv.URL, srv.URL, srv.URL
	t.Cleanup(func() { githubBaseURL, ghcrBaseURL, dockerHubBaseURL = oldGitHub, oldGHCR, oldHub })
}

func TestFindVersionLink(t *testing.T) {
	withVersionCheckServer(t,
		"/owner/app/releases/tag/v2.1.0", // released with a v the label lacks
		"/v2/acme/tool/manifests/1.4",    // tagged on ghcr, no release
		"/owner/both/releases/tag/3.0",   // a release wins over the ghcr tag
		"/v2/owner/both/manifests/3.0",
		"/v2/repositories/acme/hubapp/tags/4.1",    // tagged on Docker Hub
		"/v2/repositories/library/nginx/tags/1.27", // an official image
	)
	ctx := context.Background()
	for _, tc := range []struct {
		name, imgRef string
		meta         imageMeta
		want         string
	}{
		{"release, v added", "owner/app:latest", imageMeta{version: "2.1.0", source: "https://github.com/owner/app"}, githubBaseURL + "/owner/app/releases/tag/v2.1.0"},
		{"ghcr tag", "ghcr.io/acme/tool:latest", imageMeta{version: "1.4", source: "https://github.com/acme/tool-src"}, "https://ghcr.io/acme/tool?tag=1.4"},
		{"release first", "ghcr.io/owner/both:latest", imageMeta{version: "3.0", source: "https://github.com/owner/both"}, githubBaseURL + "/owner/both/releases/tag/3.0"},
		{"branch name", "ghcr.io/azuracast/azuracast:latest", imageMeta{version: "main", source: "https://github.com/AzuraCast/AzuraCast"}, ""},
		{"Docker Hub tag", "acme/hubapp:latest", imageMeta{version: "4.1"}, "https://hub.docker.com/layers/acme/hubapp/4.1"},
		{"official image", "docker.io/nginx:latest", imageMeta{version: "1.27"}, "https://hub.docker.com/layers/library/nginx/1.27"},
		{"base image's version", "sportarr/sportarr:dev", imageMeta{version: "24.04", source: "https://github.com/Sportarr/Sportarr"}, ""},
		{"other registry", "quay.io/acme/app:latest", imageMeta{version: "1.0"}, ""},
	} {
		if got := findVersionLink(ctx, tc.imgRef, tc.meta); got != tc.want {
			t.Errorf("%s: link = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// withHubTagsServer points the checks at a local server listing tags for
// sportarr/sportarr in two pages, newest first, the release's tags on the second, and
// answering 404 to every other request.
func withHubTagsServer(t *testing.T) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/repositories/sportarr/sportarr/tags" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"next":"","results":[
				{"name":"4","digest":"sha256:rel"},{"name":"4.1","digest":"sha256:rel"},
				{"name":"4.1.12.1122","digest":"sha256:rel"},{"name":"latest","digest":"sha256:rel"},
				{"name":"4.1.11.885-dev","digest":"sha256:old"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"next":"` + srv.URL + `/v2/repositories/sportarr/sportarr/tags?page=2","results":[
			{"name":"4.1.12.900-dev","digest":"sha256:dev"},{"name":"dev","digest":"sha256:dev"}]}`))
	}))
	t.Cleanup(srv.Close)
	oldGitHub, oldGHCR, oldHub := githubBaseURL, ghcrBaseURL, dockerHubBaseURL
	githubBaseURL, ghcrBaseURL, dockerHubBaseURL = srv.URL, srv.URL, srv.URL
	t.Cleanup(func() { githubBaseURL, ghcrBaseURL, dockerHubBaseURL = oldGitHub, oldGHCR, oldHub })
}

func TestHubTagsForDigest(t *testing.T) {
	withHubTagsServer(t)
	ctx := context.Background()
	if got := strings.Join(hubTagsForDigest(ctx, "sportarr/sportarr", "sha256:rel"), ","); got != "4,4.1,4.1.12.1122,latest" {
		t.Errorf("release digest's tags = %q, want the second page's four", got)
	}
	if got := hubTagsForDigest(ctx, "sportarr/sportarr", "sha256:gone"); got != nil {
		t.Errorf("unknown digest's tags = %v, want none", got)
	}
	if got := hubTagsForDigest(ctx, "acme/missing", "sha256:rel"); got != nil {
		t.Errorf("missing repo's tags = %v, want none", got)
	}
}

func TestVersionFromTags(t *testing.T) {
	for _, tc := range []struct {
		tags []string
		want string
	}{
		{[]string{"4", "4.1", "4.1.12.1122", "latest"}, "4.1.12.1122"},
		{[]string{"latest", "v2", "v2.3"}, "v2.3"},
		{[]string{"v1.14.1-alpine", "v1.14.1", "v1.14", "v1", "latest-alpine", "latest"}, "v1.14.1"},
		{[]string{"latest", "dev", "main"}, ""},
		{nil, ""},
	} {
		if got := versionFromTags(tc.tags); got != tc.want {
			t.Errorf("versionFromTags(%v) = %q, want %q", tc.tags, got, tc.want)
		}
	}
}

func TestDigestLookupWhenVersionHasNoTag(t *testing.T) {
	withHubTagsServer(t)
	const img = "sportarr/sportarr:latest"
	p := newHealthTestProcessor()
	before := imageMeta{version: "24.04", digest: "sha256:old", source: "https://github.com/Sportarr/Sportarr"}
	after := imageMeta{version: "24.04", digest: "sha256:rel", source: "https://github.com/Sportarr/Sportarr"}
	p.checkVersionLink(img, after)
	p.checkVersionLink(img, imageMeta{digest: "sha256:none"}) // no label, no tag
	p.waitVersionLinks()
	if got := p.digestVersions["sha256:rel"]; got != "4.1.12.1122" {
		t.Fatalf("looked-up version = %q, want 4.1.12.1122", got)
	}
	if _, ok := p.digestVersions["sha256:none"]; ok {
		t.Errorf("a digest no tag shares got a version")
	}
	oldMeta, newMeta := p.tagVersions(before, after)
	if newMeta.version != "4.1.12.1122" || oldMeta.version != "" {
		t.Errorf("versions = %q → %q, want unknown → 4.1.12.1122 (same base label)", oldMeta.version, newMeta.version)
	}
	if got := p.versionURL(img, newMeta); got != "https://hub.docker.com/layers/sportarr/sportarr/4.1.12.1122" {
		t.Errorf("looked-up version's link = %q", got)
	}
	if oldMeta, _ = p.tagVersions(imageMeta{version: "4.0", digest: "sha256:x"}, after); oldMeta.version != "4.0" {
		t.Errorf("old image's own label = %q, want 4.0 kept", oldMeta.version)
	}
}

func TestVersionLinkCheckedOnlyWhenShown(t *testing.T) {
	withVersionCheckServer(t, "/owner/app/releases/tag/1.1")
	p := newHealthTestProcessor()
	p.imageMetaBefore[testImg] = imageMeta{version: "1.0", source: "https://github.com/owner/app"}
	p.checkShownVersionLinks(testImg)
	if len(p.versionChecked) != 0 {
		t.Errorf("checked %v without -v and no update, want nothing", p.versionChecked)
	}
	p.imageMetaAfter[testImg] = imageMeta{version: "1.1", source: "https://github.com/owner/app"}
	p.checkShownVersionLinks(testImg)
	p.waitVersionLinks()
	if len(p.versionChecked) != 2 {
		t.Errorf("checked %v after an update, want the old and new versions", p.versionChecked)
	}
	if got := p.versionURL(testImg, p.imageMetaAfter[testImg]); got != githubBaseURL+"/owner/app/releases/tag/1.1" {
		t.Errorf("new version's link = %q", got)
	}
	if got := p.versionURL(testImg, p.imageMetaBefore[testImg]); got != "" {
		t.Errorf("old version's link = %q, want none (no such release)", got)
	}
}

func TestHubTagsForDigestStaysOnTheHubHost(t *testing.T) {
	var elsewhereHits int
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhereHits++
		_, _ = w.Write([]byte(`{"next":"","results":[{"name":"9.9.9","digest":"sha256:rel"}]}`))
	}))
	t.Cleanup(elsewhere.Close)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The first page names another host as the next: the search must not follow it.
		_, _ = w.Write([]byte(`{"next":"` + elsewhere.URL + `/tags?page=2","results":[{"name":"dev","digest":"sha256:dev"}]}`))
	}))
	t.Cleanup(hub.Close)
	old := dockerHubBaseURL
	dockerHubBaseURL = hub.URL
	t.Cleanup(func() { dockerHubBaseURL = old })
	if got := hubTagsForDigest(context.Background(), "acme/app", "sha256:rel"); got != nil || elsewhereHits != 0 {
		t.Errorf("tags = %v, other host hit %d times; want none and 0", got, elsewhereHits)
	}
	if !sameHost("https://hub.docker.com/v2/x", "https://hub.docker.com") || sameHost("https://evil.example/v2", "https://hub.docker.com") || sameHost("http://hub.docker.com/v2", "https://hub.docker.com") {
		t.Error("sameHost wrong: want the same scheme and host only")
	}
}

// A ghcr image's version and build date come from its config, through the manifest for
// the local image's platform when the digest names a multi-platform index, with the
// anonymous token on every request.
func TestGHCRImageMeta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"token":"anon"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer anon" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v2/acme/app/manifests/sha256:index":
			_, _ = w.Write([]byte(`{"manifests":[
				{"digest":"sha256:arm","platform":{"os":"linux","architecture":"arm64"}},
				{"digest":"sha256:amd","platform":{"os":"linux","architecture":"amd64"}}]}`))
		case "/v2/acme/app/manifests/sha256:amd":
			_, _ = w.Write([]byte(`{"config":{"digest":"sha256:cfg"}}`))
		case "/v2/acme/app/blobs/sha256:cfg":
			_, _ = w.Write([]byte(`{"created":"2026-11-26T18:02:11Z","config":{"Labels":{"org.opencontainers.image.version":"2.4.0"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := ghcrBaseURL
	ghcrBaseURL = srv.URL
	t.Cleanup(func() { ghcrBaseURL = old })

	meta, ok := ghcrImageMeta(context.Background(), "acme/app", "sha256:index", imageMeta{os: "linux", arch: "amd64"})
	if !ok || meta.version != "2.4.0" || meta.built != localTime("2026-11-26T18:02:11Z") {
		t.Errorf("ghcrImageMeta = %+v, %v; want amd64's version 2.4.0 and its build date", meta, ok)
	}
	if _, ok := ghcrImageMeta(context.Background(), "acme/app", "sha256:index", imageMeta{os: "linux", arch: "riscv64"}); ok {
		t.Error("ghcrImageMeta for a platform the index lacks = ok, want false")
	}
}
