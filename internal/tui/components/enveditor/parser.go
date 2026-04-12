package enveditor

import (
	"strings"
)

// ParseEnv takes raw contents of an .env file along with a function that returns the
// default value for a given variable name, and populates the Model's value and line metadata.
func (m *Model) ParseEnv(content string, defaultFunc func(string) string, readOnlyVars []string) {
	rawLines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	
	m.Reset() // ensures clean state
	m.defaultFunc = defaultFunc
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
	m.GotoFirstEditable()
}

// ReclassifyEnv re-runs the section/variable classification pass on the current
// editor content without reloading from disk or resetting diff-tracking state.
// Pending-delete lines are skipped for section-tracking purposes but kept as-is.
// InitialLine, IsNewLine, and PendingDelete are preserved on every line.
func (m *Model) ReclassifyEnv(defaultFunc func(string) string, readOnlyVars []string) {
	if len(m.value) != len(m.lineMeta) {
		return
	}
	m.defaultFunc = defaultFunc
	m.diffCache = make(map[int][]bool)

	inUserDefinedSection := false
	inDisabledSection := false
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
			if strings.HasPrefix(trimmed, "###") {
				if strings.Contains(trimmed, "(User Defined") {
					inUserDefinedSection = true
					inDisabledSection = false
				} else if strings.Contains(trimmed, "(Disabled)") {
					inDisabledSection = true
					inUserDefinedSection = false
				}
			}
		} else if trimmed == "" {
			l.ReadOnly = false
			if inUserDefinedSection || inDisabledSection {
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
					if inUserDefinedSection || inDisabledSection {
						l.IsInvalid = true
					}
				} else {
					if inUserDefinedSection || inDisabledSection {
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
				if !inUserDefinedSection && !inDisabledSection {
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

// GetVarValue returns the current staged value for the given variable key,
// and whether the key was found in the buffer.
func (m *Model) GetVarValue(key string) (string, bool) {
	key = strings.TrimSpace(key)
	for i, meta := range m.lineMeta {
		if !meta.IsVariable || meta.PendingDelete {
			continue
		}
		line := string(m.value[i])
		eqIdx := strings.Index(line, "=")
		if eqIdx < 0 {
			continue
		}
		if strings.TrimSpace(line[:eqIdx]) == key {
			return line[eqIdx+1:], true
		}
	}
	return "", false
}

// ReformatEnv replaces the buffer by calling formatFunc on the staged variable lines,
// then re-parsing the result. Diff-tracking fields (InitialLine, IsNewLine, PendingDelete)
// are preserved by variable key so markers survive the reformat. Lines with no valid
// KEY=value (incomplete user edits) are preserved at the end of the buffer.
func (m *Model) ReformatEnv(defaultFunc func(string) string, readOnlyVars []string, formatFunc func([]string) []string) {
	if formatFunc == nil {
		return
	}

	// Snapshot cursor position by variable key and column offset so we can restore
	// it after the buffer is rebuilt (the line number will change but the key won't).
	cursorKey := ""
	cursorCol := m.col
	if m.row < len(m.value) && m.row < len(m.lineMeta) {
		line := string(m.value[m.row])
		if eqIdx := strings.Index(line, "="); eqIdx > 0 {
			cursorKey = strings.TrimSpace(line[:eqIdx])
		}
	}

	// Snapshot diff metadata by key for all complete variable lines.
	type diffSnap struct {
		InitialLine   string
		IsNewLine     bool
		PendingDelete bool
	}
	snapshot := make(map[string]diffSnap)
	deletedInitial := make(map[string]string) // key → InitialLine of the deleted original

	// Collect complete lines (valid KEY=value, non-PendingDelete) and incomplete lines.
	type incLine struct {
		value []rune
		meta  Line
	}
	var completeLines []string
	var incompleteLines []incLine

	for i, meta := range m.lineMeta {
		if !meta.IsVariable {
			continue
		}
		line := string(m.value[i])
		eqIdx := strings.Index(line, "=")
		if eqIdx > 0 {
			key := strings.TrimSpace(line[:eqIdx])
			if key != "" {
				if meta.PendingDelete {
					if _, seen := deletedInitial[key]; !seen {
						deletedInitial[key] = meta.InitialLine
					}
				} else if _, exists := snapshot[key]; !exists {
					// First occurrence wins — it's the original line whose diff markers
					// should survive after duplicates are collapsed by the formatter.
					snapshot[key] = diffSnap{
						InitialLine:   meta.InitialLine,
						IsNewLine:     meta.IsNewLine,
						PendingDelete: false,
					}
				}
				completeLines = append(completeLines, line)
				continue
			}
		}
		// Incomplete line (no valid key yet) — preserve if it's a user-added line.
		if meta.IsNewLine && !meta.PendingDelete {
			incompleteLines = append(incompleteLines, incLine{value: m.value[i], meta: meta})
		}
	}

	// Call the format function with the staged variable lines.
	formatted := formatFunc(completeLines)
	if len(formatted) == 0 {
		return
	}

	// Re-parse the formatted result.
	m.ParseEnv(strings.Join(formatted, "\n"), defaultFunc, readOnlyVars)

	// Restore diff markers by key. For keys that were pending-delete, re-apply that
	// status even if FormatLinesCore re-introduced them from the template — they will
	// be correctly positioned in the buffer and still show the `-` delete marker.
	for i, meta := range m.lineMeta {
		if !meta.IsVariable {
			continue
		}
		line := string(m.value[i])
		eqIdx := strings.Index(line, "=")
		if eqIdx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		if snap, ok := snapshot[key]; ok {
			// Live replacement exists — not deleted.
			origInitial, wasDeleted := deletedInitial[key]
			if wasDeleted && origInitial != "" {
				// Original was deleted and replaced: diff the replacement against the
				// original's on-disk value so it shows as modified, not brand-new.
				m.lineMeta[i].InitialLine = origInitial
				m.lineMeta[i].IsNewLine = false
			} else {
				m.lineMeta[i].InitialLine = snap.InitialLine
				m.lineMeta[i].IsNewLine = snap.IsNewLine
			}
			m.lineMeta[i].PendingDelete = false
		} else if origInitial, wasDeleted := deletedInitial[key]; wasDeleted {
			// Key was pending-delete with no live replacement.
			// Built-in vars are re-introduced by the template — restore them to their
			// template default and clear the delete marker so the diff shows the change.
			// User-defined vars have no template entry so they won't appear here anyway.
			m.lineMeta[i].InitialLine = origInitial
			m.lineMeta[i].IsNewLine = false
			m.lineMeta[i].PendingDelete = false
			m.lineMeta[i].ReadOnly = false
		}
	}

	// Append incomplete lines (user edits without a valid key yet).
	for _, il := range incompleteLines {
		m.value = append(m.value, il.value)
		m.lineMeta = append(m.lineMeta, il.meta)
	}

	// Restore cursor to the same variable key and column offset if still present.
	// Fall back to the original row number (clamped) if the key is gone.
	savedRow := m.row
	restored := false
	if cursorKey != "" {
		for i, meta := range m.lineMeta {
			if !meta.IsVariable {
				continue
			}
			line := string(m.value[i])
			if eqIdx := strings.Index(line, "="); eqIdx > 0 {
				if strings.TrimSpace(line[:eqIdx]) == cursorKey {
					m.row = i
					m.col = min(cursorCol, len(m.value[i]))
					restored = true
					break
				}
			}
		}
	}
	if !restored {
		m.row = min(savedRow, max(0, len(m.value)-1))
		m.col = min(cursorCol, len(m.value[m.row]))
	}

	m.diffCache = make(map[int][]bool)
	m.InvalidateCache()
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
