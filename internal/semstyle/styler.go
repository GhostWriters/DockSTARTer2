package semstyle

import (
	"regexp"
	"strings"
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

	// Tag delimiters and their compiled regexes (per-instance). Defaults come from the
	// package-level Default* vars; override per Styler with SetDelimiters.
	semPre, semSuf string
	dirPre, dirSuf string
	semanticRegex  *regexp.Regexp
	directRegex    *regexp.Regexp

	// hyperlinkTags: semantic tag names (lowercased) whose content, up to the next reset,
	// is rendered as a terminal hyperlink. Empty by default (no special hyperlink handling).
	hyperlinkTags map[string]bool
}

// Note: the color profile (terminal capability) remains package-level — it describes the
// output terminal, not an individual style configuration.

// New returns a Styler initialised with the built-in base tags and the standard delimiters
// (the package-level SemanticPrefix/Suffix and DirectPrefix/Suffix values).
func New() *Styler {
	s := &Styler{
		consoleMap:   make(map[string]string),
		themeMap:     make(map[string]string),
		ansiMap:      make(map[string]string),
		attributeMap: make(map[string]string),
		semPre:       SemanticPrefix,
		semSuf:       SemanticSuffix,
		dirPre:       DirectPrefix,
		dirSuf:       DirectSuffix,
	}
	s.rebuildRegexes()
	s.BuildColorMap()
	return s
}

// SetDelimiters overrides this Styler's tag delimiters and rebuilds its regexes.
// semPre/semSuf wrap semantic tag names; dirPre/dirSuf wrap direct style codes.
func (st *Styler) SetDelimiters(semPre, semSuf, dirPre, dirSuf string) {
	st.semPre, st.semSuf, st.dirPre, st.dirSuf = semPre, semSuf, dirPre, dirSuf
	st.rebuildRegexes()
}

// Delimiters returns this Styler's current semantic and direct delimiters.
func (st *Styler) Delimiters() (semPre, semSuf, dirPre, dirSuf string) {
	return st.semPre, st.semSuf, st.dirPre, st.dirSuf
}

// Default is the process-wide Styler that the package-level functions delegate to.
var Default = New()

// SetRenderPolicy sets the policy consulted by ToConsoleANSI.
func (st *Styler) SetRenderPolicy(fn func() bool) { st.renderPolicy = fn }

// RegisterHyperlinkTag marks a semantic tag as a hyperlink trigger on the Default styler.
func RegisterHyperlinkTag(name string) { Default.RegisterHyperlinkTag(name) }

// SetThemeMap replaces the theme map wholesale.
func (st *Styler) SetThemeMap(m map[string]string) {
	st.mu.Lock()
	st.themeMap = m
	st.mu.Unlock()
}

// RegisterHyperlinkTag marks a semantic tag whose content (everything from the tag up to
// the next reset, e.g. {{[-]}}) should be rendered as a terminal hyperlink, with the tag
// content's plain text used as the link destination. For example, registering "URL" makes
// {{|URL|}}https://example.com{{[-]}} a clickable link. Off by default; call once per tag.
func (st *Styler) RegisterHyperlinkTag(name string) {
	st.mu.Lock()
	if st.hyperlinkTags == nil {
		st.hyperlinkTags = make(map[string]bool)
	}
	st.hyperlinkTags[strings.ToLower(name)] = true
	st.mu.Unlock()
}
