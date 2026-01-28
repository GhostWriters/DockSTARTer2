package apps

import (
	"testing"
)

func TestIsAppNameValid(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"SONARR", true},
		{"Sonarr", true},
		{"SONARR_4K", false},
		{"SONARR__4K", true},
		{"SONARR 4K", false},
		{"SONARR:", true},
		{":SONARR", true},
		{"SONARR__TAG", false},
		{"SONARR__PORT", false},
		{"SONARR__ENABLED", false},
		{"INVALID-NAME", false},
		{"1STAPP", false}, // Must start with A-Z
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAppNameValid(tt.name); got != tt.expected {
				t.Errorf("IsAppNameValid(%q) = %v; want %v", tt.name, got, tt.expected)
			}
		})
	}
}
