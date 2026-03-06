package tui

// InvalidateCache clears the rendered view cache.
// Call this whenever the MenuModel's state is mutated (e.g. selection change, size change, options toggled).
func (m *MenuModel) InvalidateCache() {
	m.cacheValid = false
	m.lastListView = ""
}

// CheckCache returns the cached rendered screen if it's still valid.
// Returns the string and true if valid, or empty string and false if the cache needs rebuilding.
func (m *MenuModel) CheckCache() (string, bool) {
	if m.cacheValid && m.lastView != "" {
		return m.lastView, true
	}
	return "", false
}

// SaveCache saves the newly generated screen string to the cache and marks it as valid.
// Returns the same string for convenience in return statements.
func (m *MenuModel) SaveCache(view string) string {
	m.lastView = view
	m.cacheValid = true
	return view
}
