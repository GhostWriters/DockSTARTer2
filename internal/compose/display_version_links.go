package compose

import (
	"context"
	"encoding/json"
	"io"
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

// versionCheckTimeout caps each version link check.
const versionCheckTimeout = 3 * time.Second

// digestLookupTimeout caps the search of an image's Docker Hub tags for its digest,
// after its version's link check found nothing; digestLookupPages caps the pages of
// tags searched, newest first.
const (
	digestLookupTimeout = 4 * time.Second
	digestLookupPages   = 5
)

var versionCheckClient = &http.Client{Timeout: versionCheckTimeout}

// hubLookupClient pages through a repo's tags: its context ends the search, so no
// shorter client timeout does, and it follows redirects only within the Docker Hub host.
var hubLookupClient = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if !sameHost(req.URL.String(), dockerHubBaseURL) {
		return http.ErrUseLastResponse
	}
	return nil
}}

// sameHost reports whether raw is an URL on the host of base, with base's scheme.
func sameHost(raw, base string) bool {
	u, err := url.Parse(raw)
	b, berr := url.Parse(base)
	return err == nil && berr == nil && u.Host != "" && u.Scheme == b.Scheme && u.Host == b.Host
}

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

// tagVersions returns an old and a new image's details with the version a Docker Hub
// tag gave each one's digest in place of its labels', when a lookup found one. When
// only one was found and their labels gave the same version (a base image's, say),
// the other's version is unknown: it's left empty rather than shown as old → new.
// Callers hold p.mtx.
func (p *consoleEventProcessor) tagVersions(old, cur imageMeta) (imageMeta, imageMeta) {
	oldTag, curTag := p.digestVersions[old.digest], p.digestVersions[cur.digest]
	if old.digest == "" {
		oldTag = ""
	}
	if cur.digest == "" {
		curTag = ""
	}
	sameLabel := old.version == cur.version
	switch {
	case oldTag != "":
		old.version = oldTag
	case curTag != "" && sameLabel:
		old.version = ""
	}
	switch {
	case curTag != "":
		cur.version = curTag
	case oldTag != "" && sameLabel:
		cur.version = ""
	}
	return old, cur
}

// checkVersionLink starts a background check for a link for an image's version, unless
// it needs none (a LinuxServer release) or one already ran. When its version has no
// release or tag, or it has no version, the image's digest is looked up among its
// Docker Hub tags: a tag sharing it gives the version (tagVersions) and its link. Run
// with p.mtx held or before rendering starts.
func (p *consoleEventProcessor) checkVersionLink(imgRef string, m imageMeta) {
	key := versionKey(imgRef, m.version) + "\x00" + m.digest
	if m.version == "" && m.digest == "" || releaseURL(m) != "" || p.versionChecked[key] {
		return
	}
	p.versionChecked[key] = true
	p.linkWG.Add(1)
	go func() {
		defer p.linkWG.Done()
		if m.version != "" {
			ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
			link := findVersionLink(ctx, imgRef, m)
			cancel()
			if link != "" {
				p.mtx.Lock()
				p.versionLinks[versionKey(imgRef, m.version)] = link
				p.mtx.Unlock()
				return
			}
		}
		name := imageName(imgRef)
		repo := dockerHubRepo(name)
		if m.digest == "" || repo == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), digestLookupTimeout)
		defer cancel()
		version := versionFromTags(hubTagsForDigest(ctx, repo, m.digest))
		if version == "" {
			return
		}
		p.mtx.Lock()
		p.digestVersions[m.digest] = version
		p.versionLinks[versionKey(imgRef, version)] = dockerlayout.ImageTagURL(name, version)
		p.mtx.Unlock()
	}()
}

// hubTagsForDigest returns the tags of a Docker Hub repo (namespace/repo) that point
// at digest (hubTagsAndPushed).
func hubTagsForDigest(ctx context.Context, repo, digest string) []string {
	tags, _ := hubTagsAndPushed(ctx, repo, digest)
	return tags
}

// hubTagsAndPushed returns the tags of a Docker Hub repo (namespace/repo) that point at
// digest, and when the latest of them was pushed (RFC 3339, "" when Docker Hub doesn't
// say), searching its tags newest first, at most digestLookupPages pages, and stopping
// at the end of the first page that has any: tags pushed together sit together. It
// returns nil when none match or the search fails.
func hubTagsAndPushed(ctx context.Context, repo, digest string) (tags []string, pushed string) {
	next := dockerHubBaseURL + "/v2/repositories/" + repo + "/tags?page_size=100&ordering=last_updated"
	for range digestLookupPages {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, ""
		}
		req.Header.Set("User-Agent", "DockSTARTer2")
		resp, err := hubLookupClient.Do(req)
		if err != nil {
			return nil, ""
		}
		var page struct {
			Next    string `json:"next"`
			Results []struct {
				Name   string `json:"name"`
				Digest string `json:"digest"`
				Pushed string `json:"tag_last_pushed"`
			} `json:"results"`
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || err != nil {
			return nil, ""
		}
		for _, r := range page.Results {
			if r.Digest == digest {
				tags = append(tags, r.Name)
				pushed = max(pushed, r.Pushed) // RFC 3339 in UTC sorts as text
			}
		}
		if len(tags) > 0 || page.Next == "" {
			return tags, pushed
		}
		if !sameHost(page.Next, dockerHubBaseURL) {
			return nil, "" // a response must not send the search to another host
		}
		next = page.Next
	}
	return nil, ""
}

// versionFromTags returns the most specific version among an image's tags: of those
// starting with a digit, or v and a digit, the one with the most dot-separated parts,
// e.g. 4.1.12.1122 over 4.1 and 4, the shortest on a tie, e.g. v1.14.1 over its
// variant v1.14.1-alpine. Tags such as latest or dev name no version; "" when only
// those are left.
func versionFromTags(tags []string) string {
	best := ""
	for _, tag := range tags {
		digits := strings.TrimPrefix(tag, "v")
		if digits == "" || digits[0] < '0' || digits[0] > '9' {
			continue
		}
		parts, bestParts := strings.Count(tag, "."), strings.Count(best, ".")
		if best == "" || parts > bestParts || parts == bestParts && len(tag) < len(best) {
			best = tag
		}
	}
	return best
}

// checkShownVersionLinks starts link checks for the versions an image's info lines
// show at the run's verbosity: with -v its current version, without it the old and
// new versions once a pull changed the image (its digest lookups may tell the
// versions apart when the labels don't), or the version a first pull brought.
func (p *consoleEventProcessor) checkShownVersionLinks(imgRef string) {
	before, hadBefore := p.imageMetaBefore[imgRef]
	after, pulled := p.imageMetaAfter[imgRef]
	switch {
	case (p.verbosity >= dockerlayout.VerboseDetails || !hadBefore) && pulled:
		p.checkVersionLink(imgRef, after)
	case p.verbosity >= dockerlayout.VerboseDetails && hadBefore:
		p.checkVersionLink(imgRef, before)
	}
	if hadBefore && pulled && (before.version != after.version || before.digest != after.digest) {
		p.checkVersionLink(imgRef, before)
		p.checkVersionLink(imgRef, after)
	}
}

// waitVersionLinks waits for unfinished version link checks, at most as long as one
// check and its digest lookup take, so the final render can include their links.
func (p *consoleEventProcessor) waitVersionLinks() {
	done := make(chan struct{})
	go func() {
		p.linkWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(versionCheckTimeout + digestLookupTimeout):
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
	name := imageName(imgRef)
	if repo, ok := strings.CutPrefix(name, "ghcr.io/"); ok && ghcrTagExists(ctx, repo, m.version) {
		return dockerlayout.ImageTagURL(name, m.version)
	}
	if repo := dockerHubRepo(name); repo != "" &&
		urlExists(ctx, dockerHubBaseURL+"/v2/repositories/"+repo+"/tags/"+url.PathEscape(m.version), nil) {
		return dockerlayout.ImageTagURL(name, m.version)
	}
	return ""
}

// imageName returns an image reference without its docker.io/ prefix, tag or digest.
func imageName(imgRef string) string {
	name, _, _ := strings.Cut(strings.TrimPrefix(imgRef, "docker.io/"), "@")
	if i := strings.LastIndex(name, ":"); i > strings.LastIndex(name, "/") {
		name = name[:i]
	}
	return name
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

// manifestTypes are the manifest media types a registry query accepts: an image index
// or manifest list (several platforms), or one platform's manifest.
var manifestTypes = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

// ghcrToken returns the anonymous token ghcr.io gives for pulling repo (owner/image), a
// public image's, or "" when it gives none.
func ghcrToken(ctx context.Context, repo string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ghcrBaseURL+"/token?scope=repository:"+repo+":pull", nil)
	if err != nil {
		return ""
	}
	resp, err := versionCheckClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var tok struct {
		Token string `json:"token"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&tok) != nil {
		return ""
	}
	return tok.Token
}

// ghcrTagExists reports whether ghcr.io has tag for repo (owner/image), asking the
// registry with the anonymous token public images allow.
func ghcrTagExists(ctx context.Context, repo, tag string) bool {
	token := ghcrToken(ctx, repo)
	if token == "" {
		return false
	}
	return urlExists(ctx, ghcrBaseURL+"/v2/"+repo+"/manifests/"+url.PathEscape(tag), http.Header{
		"Authorization": {"Bearer " + token},
		"Accept":        {manifestTypes},
	})
}

// ghcrImageMeta returns the version, build date and source of the image digest names in
// ghcr.io's repo (owner/image), from its config there, with the anonymous token public
// images allow: from a multi-platform image, the manifest for platform's OS,
// architecture and variant (each matched only when given). No layer is fetched. False
// when it can't be read.
func ghcrImageMeta(ctx context.Context, repo, digest string, platform imageMeta) (imageMeta, bool) {
	token := ghcrToken(ctx, repo)
	if token == "" {
		return imageMeta{}, false
	}
	get := func(path string, into any) bool {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ghcrBaseURL+"/v2/"+repo+path, nil)
		if err != nil {
			return false
		}
		req.Header.Set("Authorization", "Bearer "+token) // not sent on to a blob's storage host
		req.Header.Set("Accept", manifestTypes)
		req.Header.Set("User-Agent", "DockSTARTer2")
		resp, err := versionCheckClient.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(into) == nil
	}
	type descriptor struct {
		Digest   string `json:"digest"`
		Platform struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
			Variant      string `json:"variant"`
		} `json:"platform"`
	}
	var manifest struct {
		Manifests []descriptor `json:"manifests"`
		Config    descriptor   `json:"config"`
	}
	if !get("/manifests/"+url.PathEscape(digest), &manifest) {
		return imageMeta{}, false
	}
	if len(manifest.Manifests) > 0 { // an index: the platform's own manifest
		chosen := ""
		for _, m := range manifest.Manifests {
			pl := m.Platform
			if (platform.os == "" || pl.OS == platform.os) && (platform.arch == "" || pl.Architecture == platform.arch) &&
				(platform.variant == "" || pl.Variant == platform.variant) {
				chosen = m.Digest
				break
			}
		}
		if chosen == "" || !get("/manifests/"+url.PathEscape(chosen), &manifest) {
			return imageMeta{}, false
		}
	}
	if manifest.Config.Digest == "" {
		return imageMeta{}, false
	}
	var config struct {
		Created string `json:"created"`
		Config  struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if !get("/blobs/"+url.PathEscape(manifest.Config.Digest), &config) {
		return imageMeta{}, false
	}
	return metaFromLabels(config.Config.Labels, config.Created), true
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
