package enveditor

import (
	"strings"
)

// ParseEnv takes raw contents of an .env file along with a function that returns the
// default value for a given variable name, and populates the Model's value and line metadata.
func (m *Model) ParseEnv(content string, defaultFunc func(string) string, readOnlyVars []string) {
	rawLines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	
	m.Reset() // ensures clean state
	m.diffCache = make(map[int][]bool)
	m.value = make([][]rune, len(rawLines))
	m.lineMeta = make([]Line, len(rawLines))
	
	inUserDefinedSection := false
	for i, raw := range rawLines {
		m.value[i] = []rune(raw)
		trimmed := strings.TrimSpace(raw)
		
		l := Line{Text: raw, InitialLine: raw}
		
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
					if inUserDefinedSection {
						l.IsInvalid = true
					}
				} else {
						// Identify if it's in the User Defined section
						if inUserDefinedSection {
							l.IsUserDefined = true
						}

						// Lock the key for ALL variables to prevent corruption
						eqIdx := strings.Index(raw, "=")

						if eqIdx != -1 {
							l.EditableStartCol = eqIdx + 1
							if defaultFunc != nil {
								l.DefaultValue = strings.TrimSpace(defaultFunc(key))
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

// ReclassifyEnv re-runs the section/variable classification pass on the current
// editor content without reloading from disk or resetting diff-tracking state.
// Pending-delete lines are skipped for section-tracking purposes but kept as-is.
// InitialLine, IsNewLine, and PendingDelete are preserved on every line.
func (m *Model) ReclassifyEnv(defaultFunc func(string) string, readOnlyVars []string) {
	if len(m.value) != len(m.lineMeta) {
		return
	}
	m.diffCache = make(map[int][]bool)

	inUserDefinedSection := false
	for i, line := range m.value {
		existing := m.lineMeta[i]

		// Pending-delete lines are invisible on save; skip for section tracking
		// but preserve their marked state.
		if existing.PendingDelete {
			continue
		}

		raw := string(line)
		trimmed := strings.TrimSpace(raw)

		l := Line{
			// Preserve diff-tracking fields
			InitialLine: existing.InitialLine,
			IsNewLine:   existing.IsNewLine,
		}

		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "***") {
			l.ReadOnly = true
			if strings.HasPrefix(trimmed, "###") && strings.Contains(trimmed, "(User Defined") {
				inUserDefinedSection = true
			}
		} else if trimmed == "" {
			l.ReadOnly = false
			if inUserDefinedSection {
				l.IsUserDefined = true
			}
		} else {
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
					if inUserDefinedSection {
						l.IsInvalid = true
					}
				} else {
					if inUserDefinedSection {
						l.IsUserDefined = true
					} else if existing.IsNewLine || existing.IsUserDefined {
						// Variable was typed/inserted by the user; preserve user-defined status.
						l.IsUserDefined = true
					}
					eqIdx := strings.Index(raw, "=")
					if eqIdx != -1 {
						l.EditableStartCol = eqIdx + 1
						if defaultFunc != nil {
							l.DefaultValue = strings.TrimSpace(defaultFunc(key))
						}
					}
				}
			} else {
				if !inUserDefinedSection {
					l.ReadOnly = true
				} else {
					l.IsUserDefined = true
				}
			}
		}

		m.lineMeta[i] = l
	}
	m.InvalidateCache()
}

// MergeEnv resolves duplicate variable keys in the editor.
// For each key that appears more than once among editable (non-ReadOnly, non-PendingDelete)
// variable lines, the last occurrence's value is written to the first occurrence and all
// subsequent occurrences are marked PendingDelete. Already-deleted lines are excluded from
// the merge calculation (so they don't contribute their value and aren't double-processed).
// Returns true if any changes were made.
// Call after ReclassifyEnv on F5 so deletions are already excluded before value merging.
func (m *Model) MergeEnv() bool {
	m.diffCache = make(map[int][]bool)
	type occ struct {
		row   int
		raw   string
		value string // everything after '='
	}
	byKey := map[string][]occ{}
	var keyOrder []string // first-seen order preserves file structure

	for i, lineRunes := range m.value {
		if i >= len(m.lineMeta) {
			break
		}
		meta := m.lineMeta[i]
		if !meta.IsVariable || meta.ReadOnly || meta.PendingDelete {
			continue
		}
		raw := string(lineRunes)
		eqIdx := strings.Index(raw, "=")
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(raw[:eqIdx])
		if _, seen := byKey[key]; !seen {
			keyOrder = append(keyOrder, key)
		}
		byKey[key] = append(byKey[key], occ{row: i, raw: raw, value: raw[eqIdx+1:]})
	}

	// Check whether there's anything to do before touching undo state.
	hasDuplicates := false
	for _, occs := range byKey {
		if len(occs) >= 2 {
			hasDuplicates = true
			break
		}
	}
	if !hasDuplicates {
		return false
	}

	m.pushUndoSnapshot()

	for _, key := range keyOrder {
		occs := byKey[key]
		if len(occs) < 2 {
			continue
		}
		lastVal := occs[len(occs)-1].value
		first := occs[0]

		// Update first occurrence value if it differs from the last.
		eqIdx := strings.Index(first.raw, "=")
		if eqIdx >= 0 {
			newRaw := first.raw[:eqIdx+1] + lastVal
			if newRaw != first.raw {
				m.value[first.row] = []rune(newRaw)
			}
		}

		// Mark all subsequent occurrences for deletion.
		for _, o := range occs[1:] {
			m.lineMeta[o.row].PendingDelete = true
			m.lineMeta[o.row].ReadOnly = true
		}
	}

	m.InvalidateCache()
	return true
}

// AfterSave updates the editor's baseline to match the current saved state:
// - InitialLine is set to the current line content (clears ~ modified markers)
// - IsNewLine is cleared (clears + added markers)
// - Pending-delete lines are removed from value and lineMeta (clears - markers)
// Call this immediately after a successful save so gutter markers reflect the
// saved state without waiting for the async reload from disk.
func (m *Model) AfterSave() {
	newValue := m.value[:0:len(m.value)]
	newMeta := m.lineMeta[:0:len(m.lineMeta)]
	for i, meta := range m.lineMeta {
		if meta.PendingDelete {
			continue
		}
		raw := string(m.value[i])
		meta.InitialLine = raw
		meta.IsNewLine = false
		newValue = append(newValue, m.value[i])
		newMeta = append(newMeta, meta)
	}
	m.value = newValue
	m.lineMeta = newMeta
	// Clamp cursor in case rows were removed
	if m.row >= len(m.value) {
		m.row = max(0, len(m.value)-1)
	}
	m.InvalidateCache()
}
