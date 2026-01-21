package update

import (
	"DockSTARTer2/internal/testutils"
	"fmt"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		// Equal
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},
		{"2024.01.01", "2024.01.01", 0},

		// Standard Numeric
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.10.0", "1.9.0", 1},

		// Suffixes (Stable > Pre-release)
		{"1.0.0", "1.0.0-beta", 1},
		{"1.0.0-beta", "1.0.0", -1},
		{"1.0.0-rc", "1.0.0-beta", 1}, // String comparison for suffixes

		// Date-based
		{"2024.01.20.1", "2024.01.20", 1},
		{"2024.01.20.2", "2024.01.20.1", 1},

		// Branches/Pre-releases
		{"2024.01.20.1-feat", "2024.01.20.1", -1},
	}

	var cases []testutils.TestCase

	for _, tt := range tests {
		actual := compareVersions(tt.v1, tt.v2)

		pass := actual == tt.expected
		cases = append(cases, testutils.TestCase{
			Input:    fmt.Sprintf("%s vs %s", tt.v1, tt.v2),
			Expected: fmt.Sprintf("%d", tt.expected),
			Actual:   fmt.Sprintf("%d", actual),
			Pass:     pass,
		})
	}

	testutils.PrintTestTable(t, cases)
}
