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
