package dockerlayout

import (
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
)

// StyleServiceName returns a semstyle tag string for a compose service name.
// If the service maps to a known built-in app, the name is wrapped in a
// hyperlink tag pointing to its dockstarter.com docs page (see appenv.AppURL).
// Callers must convert to ANSI with semstyle.ToANSI when ready to output.
func StyleServiceName(svc string) string {
	url := appenv.AppURL(svc)
	if url == "" {
		return "{{|UserApp|}}" + svc + "{{[-]}}"
	}
	return console.FormatLink("App", svc, url)
}

// Layout primitive widths — shared by compose and prune display.
// Change these to adjust the column grid for all Docker output.
const (
	GlobalIndentW        = 1  // left margin for all lines
	IconW                = 1  // width of a spinner/status icon character
	SpaceW               = 1  // single separator space between icon and status
	SectionStatusTextW   = 11 // max status text width ("Downloading", "Untagged")
	SectionStatusGutterW = 1  // spaces after status text before next column
	SectionStatusW       = SectionStatusTextW + SectionStatusGutterW
	SectionChildIndentW  = 2  // extra indent per nesting level (matches YAML convention)
	ImageLabelTextW      = 7  // visible width of "image: "
	TimerGutterW         = 1  // spaces between rightmost content column and timer
	LayerStatusW         = 11 // max layer status width ("Downloading"); shared so prune and compose layer columns align

	// Derived column positions.
	SectionHeaderIndent = GlobalIndentW + IconW + SpaceW + SectionStatusW
	ImageLabelW         = 2*SectionChildIndentW + ImageLabelTextW
	LayerPrefixW        = SectionHeaderIndent + 3*SectionChildIndentW
)

// Indent strings derived from layout constants.
var (
	GlobalIndent       = strutil.Repeat(" ", GlobalIndentW)
	SectionChildIndent = strutil.Repeat(" ", SectionChildIndentW)
	LayerPrefix        = strutil.Repeat(" ", LayerPrefixW)
)

// AbbreviateStatus shortens verbose Docker status strings to compact display labels.
// Both compose and prune use this so renaming a status is a single change.
func AbbreviateStatus(text string) string {
	switch text {
	case "Pulling fs layer":
		return "Pulling fs"
	case "Download complete", "Pull complete":
		return "Downloaded"
	case "Already exists":
		return "Cached"
	case "Verifying Checksum":
		return "Verifying"
	case "Extracting":
		return "Extracting"
	// Prune statuses — pass-through for now, centralised for easy renaming.
	case "Removed", "Untagged", "Deleted", "Error", "Failed":
		return text
	}
	return text
}

// Plural returns singular or pluralForm based on n.
func Plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// imageRefURL builds a browser URL for a Docker image reference (without tag).
// Supports ghcr.io, lscr.io, and Docker Hub (official and namespaced images).
func imageRefURL(name string) string {
	// LinuxServer images: map to their docs page.
	if rest, ok := strings.CutPrefix(name, "lscr.io/linuxserver/"); ok {
		return "https://docs.linuxserver.io/images/docker-" + rest + "/"
	}
	// Hotio images: map to their containers doc page, regardless of registry.
	bare := strings.TrimPrefix(name, "docker.io/")
	for _, registry := range []string{"ghcr.io/", "lscr.io/", "quay.io/", ""} {
		if rest, ok := strings.CutPrefix(bare, registry+"hotio/"); ok {
			return "https://hotio.dev/containers/" + rest
		}
	}
	// Known third-party registries: use https:// directly.
	for _, registry := range []string{"ghcr.io/", "lscr.io/", "mcr.microsoft.com/", "quay.io/", "registry.k8s.io/"} {
		if strings.HasPrefix(name, registry) {
			return "https://" + name
		}
	}
	// Docker Hub: strip optional "docker.io/" prefix.
	if strings.Contains(bare, "/") {
		return "https://hub.docker.com/r/" + bare
	}
	return "https://hub.docker.com/_/" + bare
}

// imageTagURL builds a browser URL for a specific tag of a Docker image
// reference, when the registry's web UI supports deep-linking to one.
// Returns "" only when it genuinely can't -- imageRefURL redirects
// LinuxServer/hotio images to their doc pages (general image docs, not
// tag browsers) for the name link, but both still publish real,
// tag-linkable images elsewhere: LinuxServer to Docker Hub under the same
// name (linuxserver/<rest>), hotio to ghcr.io/lscr.io/quay.io (it moved
// off Docker Hub entirely -- verified a hotio image 404s there, so that
// specific case still returns "").
//
// GHCR-family registries (ghcr.io, lscr.io as a generic mirror, quay.io,
// mcr.microsoft.com, registry.k8s.io) all resolve through GitHub's
// container package page, which reads a "tag" query param client-side to
// jump to that tag -- unofficial (GitHub doesn't document it) but
// confirmed working. Docker Hub's own "layers" page (also undocumented)
// is even more direct: it's normally shown with a "/images/sha256-<digest>"
// suffix for the tag's current manifest, but the plain "/layers/<owner>/
// <image>/<tag>" URL still resolves the same page client-side, so no
// registry lookup is needed to build it -- just owner/image/tag, same as
// everything else here.
func imageTagURL(name, tag string) string {
	if rest, ok := strings.CutPrefix(name, "lscr.io/linuxserver/"); ok {
		return "https://hub.docker.com/layers/linuxserver/" + rest + "/" + tag
	}
	// Hotio moved off Docker Hub entirely (verified: a hotio image 404s
	// there) but does publish to ghcr.io/lscr.io/quay.io, which -- unlike
	// the name link's redirect to hotio.dev's general docs -- can still
	// deep-link a tag on whichever of those the image actually came from.
	bare := strings.TrimPrefix(name, "docker.io/")
	for _, registry := range []string{"ghcr.io/", "lscr.io/", "quay.io/"} {
		if strings.HasPrefix(bare, registry+"hotio/") {
			return "https://" + bare + "?tag=" + tag
		}
	}
	if strings.HasPrefix(bare, "hotio/") {
		return ""
	}
	for _, registry := range []string{"ghcr.io/", "lscr.io/", "mcr.microsoft.com/", "quay.io/", "registry.k8s.io/"} {
		if strings.HasPrefix(name, registry) {
			return "https://" + name + "?tag=" + tag
		}
	}
	if strings.Contains(bare, "/") {
		return "https://hub.docker.com/layers/" + bare + "/" + tag
	}
	return "https://hub.docker.com/layers/library/" + bare + "/" + tag
}

// StyleImageRef styles an image reference with DockerImage/DockerTag tags,
// returning a semstyle tag string (callers convert to ANSI with
// semstyle.ToANSI when ready to output). When the terminal supports
// hyperlinks, the image name becomes a clickable link to its registry
// page, and the tag (when the registry supports deep-linking to one --
// see imageTagURL) becomes a clickable link to that specific tag. Handles
// three forms:
//   - "registry/name:tag" → name styled as DockerImage (linked), ":tag" as DockerTag (linked if possible)
//   - "sha256:digest"     → "sha256:" as DockerTag (dim), digest as DockerImage (no link)
//   - "name" (no colon)   → entire string as DockerImage (linked)
func StyleImageRef(ref string) string {
	if strings.HasPrefix(ref, "sha256:") {
		return "{{|DockerTag|}}sha256:{{[-]}}{{|DockerImage|}}" + ref[7:] + "{{[-]}}"
	}
	if idx := strings.LastIndex(ref, ":"); idx >= 0 {
		name, tag := ref[:idx], ref[idx+1:]
		nameURL := imageRefURL(name)
		nameLink := console.FormatLink("DockerImage", name, nameURL)
		tagText := "{{|DockerTag|}}" + tag + "{{[-]}}"
		if tagURL := imageTagURL(name, tag); tagURL != "" {
			tagText = console.FormatLink("DockerTag", tag, tagURL)
		}
		return nameLink + "{{|DockerColon|}}:{{[-]}}" + tagText
	}
	url := imageRefURL(ref)
	return console.FormatLink("DockerImage", ref, url)
}
