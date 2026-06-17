package semstyle

import (
	"sync"
)

// Styler is an independent semantic-styling configuration: its own tag/color maps,
// delimiters, color profile, and render policy. Multiple Stylers can coexist in one
// process without sharing state. The package-level functions operate on Default, so the
// simple global API and the per-instance API are both available.
type Styler struct {
	mu sync.RWMutex

	// consoleMap: built-in (base) semantic tag -> raw style code.
	consoleMap map[string]string
	// themeMap: theme-loaded semantic tag -> raw style code (takes precedence over console).
	themeMap map[string]string
	// ansiMap: color/modifier names -> ANSI code.
	ansiMap map[string]string
	// attributeMap: non-color attribute names -> ANSI code.
	attributeMap map[string]string

	// renderPolicy, when non-nil and returning false, makes ToConsoleANSI strip instead
	// of render (host apps encode TTY/redirect policy here).
	renderPolicy func() bool
}

// Note: the color profile (terminal capability) and the tag delimiters/regexes remain
// package-level — they are process-wide concerns, not per-style-configuration state, and
// external packages read the exported delimiter vars directly.

// New returns a Styler initialised with the built-in base tags.
func New() *Styler {
	s := &Styler{
		consoleMap:   make(map[string]string),
		themeMap:     make(map[string]string),
		ansiMap:      make(map[string]string),
		attributeMap: make(map[string]string),
	}
	s.BuildColorMap()
	s.RegisterBaseTags()
	return s
}

// Default is the process-wide Styler that the package-level functions delegate to.
var Default = New()

// SetRenderPolicy sets the policy consulted by ToConsoleANSI.
func (s *Styler) SetRenderPolicy(fn func() bool) { s.renderPolicy = fn }

// SetThemeMap replaces the theme map wholesale.
func (s *Styler) SetThemeMap(m map[string]string) {
	s.mu.Lock()
	s.themeMap = m
	s.mu.Unlock()
}
