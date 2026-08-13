package update

import "strings"

const (
	defaultOwner         = "GhostWriters"
	templatesRepoName    = "DockSTARTer-Templates"
	appRepoName          = "DockSTARTer2"
	defaultTemplatesRepo = defaultOwner + "/" + templatesRepoName
	defaultAppRepo       = defaultOwner + "/" + appRepoName
)

// repoSlugFromURL extracts "owner/repo" from a GitHub HTTPS remote URL.
func repoSlugFromURL(url string) string {
	slug := strings.TrimPrefix(url, "https://github.com/")
	slug = strings.TrimSuffix(slug, ".git")
	return slug
}

// repoDisplayPrefix returns a short prefix identifying a non-default
// repoSlug for version/branch display strings, so output from a fork is
// visibly different from the official repo instead of looking identical:
// "owner@" when the repo is same-named as defaultRepoName (the common
// same-named-fork case), or "owner/repo@" when it isn't. Returns "" for an
// empty repoSlug (official repo, no prefix needed).
func repoDisplayPrefix(repoSlug, defaultRepoName string) string {
	if repoSlug == "" {
		return ""
	}
	owner, name, ok := strings.Cut(repoSlug, "/")
	if !ok {
		return ""
	}
	if name == defaultRepoName {
		return owner + "@"
	}
	return owner + "/" + name + "@"
}

// ParseRepoAndRef splits a user-supplied update target of the form
// "[<owner>[/<repo>]@]<ref>" into a repo-slug override and the ref itself.
// defaultRepoName (e.g. "DockSTARTer-Templates") fills in the repo half
// when only an owner is given, so "someuser@" resolves to
// "someuser/DockSTARTer-Templates" -- the common case of a same-named fork.
//
// A bare ref with no "@" returns ("", spec) unchanged, preserving the
// single-token branch/tag/channel syntax that predates repo overrides. An
// empty ref after "@" (e.g. "someuser@") returns ("", ref) for the ref half,
// so the caller falls through to its normal default-ref resolution while
// still switching repos.
//
// A leading "@" with nothing before it (e.g. "@" alone, or "@main") also
// returns ("", ref) -- explicitly the official repo, same as omitting the
// owner entirely would with a bare ref. This is a deliberate escape hatch
// distinct from a genuinely empty spec: some callers (see
// CheckTemplatesUpdate's bareCall) treat "nothing typed at all" as "keep
// whatever repo was last explicitly used" (which could be a fork), so "@"
// gives a way to say "no really, the official repo" without having to
// remember/type the actual branch name.
func ParseRepoAndRef(spec, defaultRepoName string) (repoSlug string, ref string) {
	at := strings.LastIndex(spec, "@")
	if at < 0 || strings.HasPrefix(spec[at:], "@{") {
		// "@{" is git's reserved reflog syntax (e.g. "HEAD@{2}") -- never a
		// valid repo-override prefix, so leave a ref using it untouched.
		return "", spec
	}
	repoPart := spec[:at]
	ref = spec[at+1:]
	if repoPart == "" {
		return "", ref
	}
	if strings.Contains(repoPart, "/") {
		return repoPart, ref
	}
	return repoPart + "/" + defaultRepoName, ref
}
