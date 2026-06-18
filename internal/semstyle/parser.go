package semstyle

import "strings"

// RegisterColor is a legacy alias for RegisterConsoleTag that also strips the legacy
// underscore-wrapper format (_Name_).
func (st *Styler) RegisterColor(name, value string) {
	name = strings.TrimPrefix(name, "_")
	name = strings.TrimSuffix(name, "_")
	st.RegisterConsoleTag(name, value)
}

// --- package-level delegator to Default ---

func RegisterColor(name, value string) { Default.RegisterColor(name, value) }
