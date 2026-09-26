package classic

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// InvalidateCache clears the rendered view cache.
// Call this whenever the MenuModel's state is mutated (e.g. selection change, size change, options toggled).
func (m *MenuModel) InvalidateCache() {
	m.cacheValid = false
	m.lastListView = ""
	m.renderVersion++
}

// CheckCache returns the cached rendered screen if it's still valid: nothing
// changed through the menu's own methods, and -- for a menu whose drawing
// can be described (see viewStamp) -- nothing it reads changed either.
// Returns the string and true if valid, or empty string and false if the cache needs rebuilding.
func (m *MenuModel) CheckCache() (string, bool) {
	if !m.cacheValid || m.lastView == "" || m.lastStateVersion != m.renderVersion {
		return "", false
	}
	if stamp, ok := m.viewStamp(); ok && stamp != m.lastViewStamp {
		return "", false
	}
	return m.lastView, true
}

// SaveCache saves the newly generated screen string to the cache and marks it as valid.
// Returns the same string for convenience in return statements.
func (m *MenuModel) SaveCache(view string) string {
	m.lastView = view
	m.cacheValid = true
	m.lastStateVersion = m.renderVersion
	m.lastViewStamp = m.pendingViewStamp
	return view
}

// saveCacheable caches view for a cacheable menu (see cacheableView) and
// returns it; any other menu's drawing here is never cached.
func (m *MenuModel) saveCacheable(view string) string {
	if m.cacheableView() {
		return m.SaveCache(view)
	}
	return view
}

// cacheableView reports whether a menu's drawing depends only on what
// viewStamp describes: a list, flow, or plain-text menu, not one drawing
// sections or a ContentRenderer's output, whose changes it can't see.
func (m *MenuModel) cacheableView() bool {
	return len(m.contentSections) == 0 && m.ContentRenderer == nil
}

// viewStamp describes everything a cacheable menu's drawing reads beyond
// the changes its own methods make (renderVersion): size and layout,
// styles and appearance, focus, list position, title-bar state and the
// title's draw-time closures, and buttons. ok is false for a menu that
// isn't cacheable (see cacheableView).
func (m *MenuModel) viewStamp() (stamp string, ok bool) {
	if !m.cacheableView() {
		return "", false
	}
	var b strings.Builder
	fmt.Fprint(&b, m.width, m.height, m.Layout, StyleGeneration(), StylesScopeKey(), ActiveAppearance(),
		m.focused, m.focusedItem, m.activeColumn, m.listFocusOverride, m.focusedSub, m.frameFocused, m.disabled,
		m.cursor, m.list.Index(), m.ViewStartY, m.MaxFlowRows, m.processingItemIdx, m.focusedBtnIndex,
		m.Scroll.Info.Needed, m.Scroll.Info.Height, m.Scroll.Info.ThumbStart, m.Scroll.Info.ThumbEnd,
		m.Scroll.Info.TotalItems, m.Scroll.Info.VisibleItems, m.Scroll.Drag, m.Scroll.Pending,
		m.tbFocused, m.tbWidget, m.tbPressed, len(m.tbWidgets),
		m.title, m.subtitle, m.plainText, m.bottomBorderLabel, m.borderStyle, m.maximized, m.showButtons,
		m.externalLock, m.commandLock, m.loadingText, m.titleSpinner.active, m.titleSpinner.frame,
		m.GetButtonSpecsForState(), m.lastHeaderView)
	for k, v := range m.sectionLineBackgrounds {
		fmt.Fprint(&b, "|bg", k, v.GetBackground())
	}
	if m.ownBackground != nil {
		fmt.Fprint(&b, "|own", m.ownBackground.GetBackground())
	}
	if m.titleChanged != nil {
		fmt.Fprint(&b, "|changed", m.titleChanged())
	}
	if m.titleSpinnerIndicator != nil {
		l, r := m.titleSpinnerIndicator()
		fmt.Fprint(&b, "|spin", l, r)
	}
	for _, c := range m.titleControls {
		fmt.Fprint(&b, "|ctl", c.Label)
		if c.Checked != nil {
			fmt.Fprint(&b, c.Checked())
		}
		if c.Value != nil {
			fmt.Fprint(&b, c.Value())
		}
		if c.Changed != nil {
			fmt.Fprint(&b, c.Changed())
		}
	}
	return b.String(), true
}

// verifyViewCachePath, from DS2_VERIFY_VIEW_CACHE, turns on view cache
// verification: every cache hit is drawn again uncached, and any
// difference is logged to this file (see verifyCachedView).
var verifyViewCachePath = os.Getenv("DS2_VERIFY_VIEW_CACHE")

var verifyViewCacheMu sync.Mutex

// verifyCachedView draws m again, ignoring cached, and logs where the two
// differ; it returns the fresh drawing.
func (m *MenuModel) verifyCachedView(cached string) string {
	m.cacheValid = false
	fresh := m.renderView()
	if fresh == cached {
		return fresh
	}
	cl, fl := strings.Split(cached, "\n"), strings.Split(fresh, "\n")
	line := 0
	for line < len(cl) && line < len(fl) && cl[line] == fl[line] {
		line++
	}
	verifyViewCacheMu.Lock()
	defer verifyViewCacheMu.Unlock()
	if f, err := os.OpenFile(verifyViewCachePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(f, "%s stale view: menu %q (%q), first difference at line %d of %d\n",
			time.Now().Format("2006-01-02 15:04:05"), m.id, GetPlainText(m.title), line, len(fl))
		f.Close()
	}
	return fresh
}
