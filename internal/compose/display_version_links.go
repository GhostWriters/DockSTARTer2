package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"DockSTARTer2/internal/dockerlayout"
)

// Where version links are checked; tests point these at a local server.
var (
	githubBaseURL    = "https://github.com"
	ghcrBaseURL      = "https://ghcr.io"
	dockerHubBaseURL = "https://hub.docker.com"
)

// versionCheckTimeout caps each version link check, and the wait for unfinished ones
// at the end of a run.
const versionCheckTimeout = 3 * time.Second

var versionCheckClient = &http.Client{Timeout: versionCheckTimeout}

// versionKey identifies a version of an image in versionLinks.
func versionKey(imgRef, version string) string {
	return imgRef + "\x00" + version
}

// versionURL returns the link for an image's version on its info line, or "" for none:
// a LinuxServer release (always there), or the link a check found.
func (p *consoleEventProcessor) versionURL(imgRef string, m imageMeta) string {
	if u := releaseURL(m); u != "" {
		return u
	}
	return p.versionLinks[versionKey(imgRef, m.version)]
}

// checkVersionLink starts a background check for a link for an image's version, unless
// it needs none (no version, or a LinuxServer release) or one already ran. Run with p.mtx
// held or before rendering starts.
func (p *consoleEventProcessor) checkVersionLink(imgRef string, m imageMeta) {
	key := versionKey(imgRef, m.version)
	if m.version == "" || releaseURL(m) != "" || p.versionChecked[key] {
		return
	}
	p.versionChecked[key] = true
	p.linkWG.Add(1)
	go func() {
		defer p.linkWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
		defer cancel()
		link := findVersionLink(ctx, imgRef, m)
		if link == "" {
			return
		}
		p.mtx.Lock()
		p.versionLinks[key] = link
		p.mtx.Unlock()
	}()
}

// checkShownVersionLinks starts link checks for the versions an image's info lines
// show at the run's verbosity: with -v its current version, without it the old and
// new versions once a pull changed them.
func (p *consoleEventProcessor) checkShownVersionLinks(imgRef string) {
	before, hadBefore := p.imageMetaBefore[imgRef]
	after, pulled := p.imageMetaAfter[imgRef]
	switch {
	case p.verbosity >= dockerlayout.VerboseDetails && pulled:
		p.checkVersionLink(imgRef, after)
	case p.verbosity >= dockerlayout.VerboseDetails && hadBefore:
		p.checkVersionLink(imgRef, before)
	}
	if hadBefore && pulled && before.version != "" && after.version != "" && before.version != after.version {
		p.checkVersionLink(imgRef, before)
		p.checkVersionLink(imgRef, after)
	}
}

// waitVersionLinks waits for unfinished version link checks, at most
// versionCheckTimeout, so the final render can include their links.
func (p *consoleEventProcessor) waitVersionLinks() {
	done := make(chan struct{})
	go func() {
		p.linkWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(versionCheckTimeout):
	}
}

// findVersionLink returns the first link found for an image's version: its release
// on the GitHub repo its source label names, trying the version with and without a
// leading v; else, for a ghcr.io or Docker Hub image, the version's tag page when the
// registry has that tag. It returns "" when none exists or a check fails.
func findVersionLink(ctx context.Context, imgRef string, m imageMeta) string {
	if repo := githubRepo(m.source); repo != "" {
		for _, tag := range versionTags(m.version) {
			u := githubBaseURL + "/" + repo + "/releases/tag/" + url.PathEscape(tag)
			if urlExists(ctx, u, nil) {
				return u
			}
		}
	}
	name, _, _ := strings.Cut(strings.TrimPrefix(imgRef, "docker.io/"), "@")
	if i := strings.LastIndex(name, ":"); i > strings.LastIndex(name, "/") {
		name = name[:i]
	}
	if repo, ok := strings.CutPrefix(name, "ghcr.io/"); ok && ghcrTagExists(ctx, repo, m.version) {
		return dockerlayout.ImageTagURL(name, m.version)
	}
	if repo := dockerHubRepo(name); repo != "" &&
		urlExists(ctx, dockerHubBaseURL+"/v2/repositories/"+repo+"/tags/"+url.PathEscape(m.version), nil) {
		return dockerlayout.ImageTagURL(name, m.version)
	}
	return ""
}

// githubRepo returns the owner/repo a source label's github.com URL names, or "".
func githubRepo(source string) string {
	rest, ok := strings.CutPrefix(source, "https://github.com/")
	if !ok {
		return ""
	}
	parts := strings.Split(strings.TrimSuffix(rest, ".git"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + strings.TrimSuffix(parts[1], ".git")
}

// dockerHubRepo returns the Docker Hub namespace/repo for an image name without its tag,
// with library/ for an official image, or "" for an image from another registry (its
// first part names a host: it has a dot or colon, or is localhost).
func dockerHubRepo(name string) string {
	first, rest, hasSlash := strings.Cut(name, "/")
	switch {
	case !hasSlash:
		return "library/" + name
	case strings.ContainsAny(first, ".:") || first == "localhost":
		return ""
	}
	return first + "/" + rest
}

// versionTags returns the release tags to try for version: as given, then with its
// leading v added or removed.
func versionTags(version string) []string {
	if rest, ok := strings.CutPrefix(version, "v"); ok && rest != "" {
		return []string{version, rest}
	}
	return []string{version, "v" + version}
}

// ghcrTagExists reports whether ghcr.io has tag for repo (owner/image), asking the
// registry with the anonymous token public images allow.
func ghcrTagExists(ctx context.Context, repo, tag string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ghcrBaseURL+"/token?scope=repository:"+repo+":pull", nil)
	if err != nil {
		return false
	}
	resp, err := versionCheckClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var tok struct {
		Token string `json:"token"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&tok) != nil || tok.Token == "" {
		return false
	}
	return urlExists(ctx, ghcrBaseURL+"/v2/"+repo+"/manifests/"+url.PathEscape(tag), http.Header{
		"Authorization": {"Bearer " + tok.Token},
		"Accept": {strings.Join([]string{
			"application/vnd.oci.image.index.v1+json",
			"application/vnd.oci.image.manifest.v1+json",
			"application/vnd.docker.distribution.manifest.list.v2+json",
			"application/vnd.docker.distribution.manifest.v2+json",
		}, ", ")},
	})
}

// urlExists reports whether a HEAD request for u answers 200.
func urlExists(ctx context.Context, u string, header http.Header) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return false
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", "DockSTARTer2")
	resp, err := versionCheckClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
