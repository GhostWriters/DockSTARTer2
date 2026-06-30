package dockerlayout

import (
	"strings"
	"sync"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/strutil"
)

var (
	serviceURLCache   = sync.Map{} // service name → url string or "" if not built-in
)

// StyleServiceName styles a compose service name with the App tag. If the
// service maps to a known built-in app, the name becomes a clickable hyperlink
// to its dockstarter.com page. Results are cached after the first lookup.
// StyleServiceName returns a semstyle tag string for a compose service name.
// If the service maps to a known built-in app, the name is wrapped in a hyperlink tag.
// Callers must convert to ANSI with semstyle.ToANSI when ready to output.
func StyleServiceName(svc string) string {
	url := serviceURL(svc)
	if url == "" {
		return "{{|App|}}" + svc + "{{[-]}}"
	}
	return "{{|App::::"+url+"|}}"+svc+"{{[-]}}"
}

func serviceURL(svc string) string {
	if v, ok := serviceURLCache.Load(svc); ok {
		return v.(string)
	}
	base := strings.ToLower(appenv.AppNameToBaseAppName(svc))
	url := ""
	if appenv.IsAppBuiltIn(svc) {
		url = "https://dockstarter.com/apps/" + base + "/"
	}
	serviceURLCache.Store(svc, url)
	return url
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
	// Known third-party registries: use https:// directly.
	for _, registry := range []string{"ghcr.io/", "lscr.io/", "mcr.microsoft.com/", "quay.io/", "registry.k8s.io/"} {
		if strings.HasPrefix(name, registry) {
			return "https://" + name
		}
	}
	// Docker Hub: strip optional "docker.io/" prefix.
	name = strings.TrimPrefix(name, "docker.io/")
	if strings.Contains(name, "/") {
		return "https://hub.docker.com/r/" + name
	}
	return "https://hub.docker.com/_/" + name
}

// StyleImageRef styles an image reference with DockerImage/DockerTag tags.
// When the terminal supports hyperlinks, the image name becomes a clickable
// link to its registry page. Handles three forms:
//   - "registry/name:tag"  → name styled as DockerImage (linked), ":tag" as DockerTag
//   - "sha256:digest"      → "sha256:" as DockerTag (dim), digest as DockerImage (no link)
//   - "name" (no colon)    → entire string as DockerImage (linked)
// StyleImageRef returns a semstyle tag string for an image reference.
// Callers must convert to ANSI with semstyle.ToANSI when ready to output.
func StyleImageRef(ref string) string {
	if strings.HasPrefix(ref, "sha256:") {
		return "{{|DockerTag|}}sha256:{{[-]}}{{|DockerImage|}}" + ref[7:] + "{{[-]}}"
	}
	if idx := strings.LastIndex(ref, ":"); idx >= 0 {
		name, tag := ref[:idx], ref[idx+1:]
		url := imageRefURL(name)
		return "{{|DockerImage::::"+url+"|}}"+name+"{{[-]}}{{|DockerColon|}}:{{[-]}}{{|DockerTag|}}"+tag+"{{[-]}}"
	}
	url := imageRefURL(ref)
	return "{{|DockerImage::::"+url+"|}}"+ref+"{{[-]}}"
}
