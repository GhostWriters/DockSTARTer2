package compose

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"DockSTARTer2/internal/dockerlayout"
	"github.com/GhostWriters/semstyle"
	"DockSTARTer2/internal/strutil"

	"github.com/docker/compose/v5/pkg/api"
)

func styleImage(imgName string) string { return dockerlayout.StyleImageRef(imgName) }

func plural(n int, singular, pluralForm string) string {
	return dockerlayout.Plural(n, singular, pluralForm)
}

func fixedSize(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	var s string
	switch {
	case b >= gb:
		s = fmt.Sprintf("%.0fGB", float64(b)/gb)
	case b >= mb:
		s = fmt.Sprintf("%.0fMB", float64(b)/mb)
	case b >= kb:
		s = fmt.Sprintf("%.1fkB", float64(b)/kb)
	default:
		s = fmt.Sprintf("%dB", b)
	}
	for len(s) < 8 {
		s = " " + s
	}
	return s
}

// renderProgressBarLayers renders one char per layer, each at its own fill level.
func renderProgressBarLayers(layerPcts []int, chars []string, colorTag string) string {
	levels := len(chars) - 1
	var sb strings.Builder
	for _, pct := range layerPcts {
		if pct > 100 {
			pct = 100
		}
		sb.WriteString(chars[levels*pct/100])
	}
	return "[" + semstyle.ToANSI(colorTag+sb.String()+"{{[-]}}") + "]"
}

// padOrTrunc ensures a line is exactly termW visible chars wide.
func padOrTrunc(line string, termW int) string {
	plain := semstyle.ToPlain(line)
	visible := utf8.RuneCountInString(plain)
	if visible < termW {
		return line + strutil.Repeat(" ", termW-visible)
	}
	runes := []rune(plain)
	if len(runes) > termW {
		return string(runes[:termW-1]) + "…"
	}
	return line
}

// imageBaseName returns the image name portion of a URL for sort purposes.
func imageBaseName(img string) string {
	if i := strings.LastIndex(img, ":"); i >= 0 {
		img = img[:i]
	}
	if i := strings.LastIndex(img, "/"); i >= 0 {
		return img[i+1:]
	}
	return img
}

// looksLikeImageName returns true if id looks like a Docker image reference (contains / or :).
func looksLikeImageName(id string) bool {
	return strings.ContainsAny(id, "/:")
}

// isServiceStatus returns true if text is a container lifecycle status.
func isServiceStatus(text string) bool {
	switch text {
	case api.StatusCreating, api.StatusCreated,
		api.StatusStarting, api.StatusStarted,
		api.StatusStopping, api.StatusStopped,
		api.StatusRestarting, api.StatusRestarted,
		api.StatusRemoving, api.StatusRemoved,
		api.StatusRunning, api.StatusWaiting,
		api.StatusHealthy, api.StatusExited,
		api.StatusKilling, api.StatusKilled,
		api.StatusError:
		return true
	}
	return false
}

// imageMatchesService returns true if imageID's base name matches the service name.
func imageMatchesService(imageID, serviceID string) bool {
	img := imageID
	if idx := strings.LastIndex(img, "/"); idx >= 0 {
		img = img[idx+1:]
	}
	if idx := strings.Index(img, ":"); idx >= 0 {
		img = img[:idx]
	}
	img = strings.ToLower(img)
	svc := strings.ToLower(serviceID)
	base := svc
	if idx := strings.Index(svc, "__"); idx >= 0 {
		base = svc[:idx]
	}
	return img == base || img == svc
}

// serviceIDsForImage returns the service list for an image, merging pre-populated
// config data with any service task IDs received from the SDK at runtime.
func (p *consoleEventProcessor) serviceIDsForImage(imgID string) []string {
	if svcs, ok := p.imageServices[imgID]; ok {
		return svcs
	}
	var result []string
	for _, svcID := range p.serviceIDs {
		if imageMatchesService(imgID, svcID) {
			result = append(result, svcID)
		}
	}
	sort.Strings(result)
	return result
}

// isKnownServiceName returns true if name appears in any imageServices service list or serviceIDs.
func (p *consoleEventProcessor) isKnownServiceName(name string) bool {
	for _, svcs := range p.imageServices {
		if contains(svcs, name) {
			return true
		}
	}
	return contains(p.serviceIDs, name)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
