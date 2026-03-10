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
	
	for i, raw := range rawLines {
		m.value[i] = []rune(raw)
		trimmed := strings.TrimSpace(raw)
		
		l := Line{Text: raw}
		
		// 1. Comments & empty lines are read-only
		if strings.HasPrefix(trimmed, "#") || trimmed == "" || strings.HasPrefix(trimmed, "***") {
			l.ReadOnly = true
		} else {
			// 2. Identify variables (KEY=VALUE)
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
					// Default StartCol is 0, meaning the whole thing could be edited.
					// But if we want to lock the key for known defaults:
					if defVal, ok := defaults[key]; ok {
						// Find the index of the '=' character
						eqIdx := strings.Index(raw, "=")
						if eqIdx != -1 {
							l.EditableStartCol = eqIdx + 1
							l.DefaultValue = defVal
						}
					} else {
						// For custom variables, we also want to lock the KEY= part so they don't corrupt the env file.
						eqIdx := strings.Index(raw, "=")
						if eqIdx != -1 {
							l.EditableStartCol = eqIdx + 1
						}
					}
				}
			} else {
                // If it's not a variable or a comment, treat as read-only.
                l.ReadOnly = true
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
