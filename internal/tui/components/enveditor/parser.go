package enveditor

import (
	"strings"
)

// ParseEnv takes raw contents of an .env file along with a map of built-in/default values
// and populates the Model's value and line metadata.
func (m *Model) ParseEnv(content string, defaults map[string]string, readOnlyVars []string) {
	rawLines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	
	m.Reset() // ensures clean state
	m.value = make([][]rune, len(rawLines))
	m.lineMeta = make([]Line, len(rawLines))
	
	inUserDefinedSection := false
	for i, raw := range rawLines {
		m.value[i] = []rune(raw)
		trimmed := strings.TrimSpace(raw)
		
		l := Line{Text: raw}
		
		// 1. Comments & special markers are read-only.
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "***") {
			l.ReadOnly = true
			if strings.HasPrefix(trimmed, "###") && strings.Contains(trimmed, "(User Defined") {
				inUserDefinedSection = true
			}
		} else if trimmed == "" {
			// 2. Blank lines are editable and can be user-defined
			l.ReadOnly = false
			if inUserDefinedSection {
				l.IsUserDefined = true
			}
		} else {
			// 3. Identify variables (KEY=VALUE)
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				l.IsVariable = true
				key := strings.TrimSpace(parts[0])

				isReadOnly := false
				for _, ro := range readOnlyVars {
					if key == ro {
						isReadOnly = true
						break
					}
				}

				if isReadOnly {
					l.ReadOnly = true
				} else {
						// Identify if it's in the User Defined section
						if inUserDefinedSection {
							l.IsUserDefined = true
						}

						// Lock the key for ALL variables to prevent corruption
						eqIdx := strings.Index(raw, "=")
						_, isBuiltin := defaults[key]
						if eqIdx != -1 {
							l.EditableStartCol = eqIdx + 1
							if isBuiltin {
								l.DefaultValue = defaults[key]
							}
						}
				}
			} else {
				// If it's not a variable, comment, or blank:
				// Only treat as read-only if it's NOT in the user-defined section.
				// This allows users to start typing new variables without an '=' yet.
				if !inUserDefinedSection {
					l.ReadOnly = true
				} else {
					l.IsUserDefined = true
				}
			}
		}
		
		m.lineMeta[i] = l
	}
    m.row = 0
	if len(m.lineMeta) > 0 {
		m.col = m.lineMeta[0].EditableStartCol
	} else {
        m.col = 0
    }
}
