package env

import (
	"testing"
)

func TestVarNameToAppName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SONARR_CONTAINER_NAME", ""},
		{"SONARR__CONTAINER_NAME", "SONARR"},
		{"SONARR__4K__CONTAINER_NAME", "SONARR__4K"},
		{"SONARR__4K__CONTAINER_NAME__TEST", "SONARR__4K"},
		{"SONARR__4K__CONTAINER__NAME", "SONARR__4K"},
		{"SONARR_4K__CONTAINER__NAME", ""},
		{"DOCKER_VOLUME_STORAGE", ""},
	}

	for _, test := range tests {
		result := VarNameToAppName(test.input)
		if result != test.expected {
			t.Errorf("VarNameToAppName(%q) = %q; want %q", test.input, result, test.expected)
		}
	}
}
