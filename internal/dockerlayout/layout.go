package dockerlayout

import (
	"strings"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/strutil"
)

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

// StyleImageRef styles an image reference with DockerImage/DockerTag tags.
// Handles three forms:
//   - "registry/name:tag"  → name styled as DockerImage, ":tag" as DockerTag
//   - "sha256:digest"      → "sha256:" as DockerTag (dim), digest as DockerImage
//   - "name" (no colon)    → entire string as DockerImage
func StyleImageRef(ref string) string {
	if strings.HasPrefix(ref, "sha256:") {
		return console.ToConsoleANSI("{{|DockerTag|}}sha256:{{[-]}}{{|DockerImage|}}" + ref[7:] + "{{[-]}}")
	}
	if idx := strings.LastIndex(ref, ":"); idx >= 0 {
		return console.ToConsoleANSI("{{|DockerImage|}}" + ref[:idx] + "{{[-]}}{{|DockerTag|}}:" + ref[idx+1:] + "{{[-]}}")
	}
	return console.ToConsoleANSI("{{|DockerImage|}}" + ref + "{{[-]}}")
}
