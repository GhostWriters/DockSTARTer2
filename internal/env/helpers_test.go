package env

import (
	"testing"
)

func TestVarNameToAppName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// No double underscore
		{"SONARR_CONTAINER_NAME", ""},
		{"DOCKER_VOLUME_STORAGE", ""},

		// Valid app variables
		{"SONARR__CONTAINER_NAME", "SONARR"},
		{"SONARR__4K__CONTAINER_NAME", "SONARR__4K"},
		{"SONARR__4K__CONTAINER_NAME__TEST", "SONARR__4K"},
		{"SONARR__4K__CONTAINER__NAME", "SONARR__4K"},

		// Invalid patterns
		{"SONARR_4K__CONTAINER__NAME", ""}, // Single underscore
		{"4RADARR__ANIME__VAR", ""},        // Starts with number

		// These extract successfully, but AppNameIsValid would reject them
		{"RADARR__ENABLED__FOO", "RADARR__ENABLED"},
		{"RADARR__TAG__VAR", "RADARR__TAG"},
	}

	for _, test := range tests {
		result := VarNameToAppName(test.input)
		if result != test.expected {
			t.Errorf("VarNameToAppName(%q) = %q; want %q", test.input, result, test.expected)
		}
	}
}

func TestAppNameToInstanceName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"RADARR", ""},
		{"RADARR__4K", "4K"},
		{"RADARR__4K__EXTRA", "4K__EXTRA"}, // Everything after first __
		{"SONARR__ANIME", "ANIME"},
	}

	for _, test := range tests {
		result := AppNameToInstanceName(test.input)
		if result != test.expected {
			t.Errorf("AppNameToInstanceName(%q) = %q; want %q", test.input, result, test.expected)
		}
	}
}

func TestAppNameIsValid(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Valid app names
		{"SONARR", true},
		{"RADARR__4K", true},
		{"RADARR__ANIME", true},

		// Invalid patterns
		{"Sonarr", false},    // Lowercase
		{"SONARR_4K", false}, // Single underscore
		{"SONARR 4K", false}, // Space
		{"4SONARR", false},   // Starts with number

		// Invalid instance names (reserved)
		{"RADARR__ENABLED", false},
		{"RADARR__TAG", false},
		{"RADARR__CONTAINER", false},
		{"RADARR__HOSTNAME", false},

		// Colon handling (Bash compatibility)
		{"SONARR:", true},
		{":SONARR", true},
	}

	for _, test := range tests {
		result := AppNameIsValid(test.input)
		if result != test.expected {
			t.Errorf("AppNameIsValid(%q) = %v; want %v", test.input, result, test.expected)
		}
	}
}

func TestInstanceNameIsValid(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Valid instances
		{"4K", true},
		{"ANIME", true},
		{"TEST", true},

		// Invalid instances (reserved)
		{"ENABLED", false},
		{"TAG", false},
		{"CONTAINER", false},
		{"DEVICE", false},
		{"DEVICES", false},
		{"ENVIRONMENT", false},
		{"HOSTNAME", false},
		{"PORT", false},
		{"NETWORK", false},
		{"RESTART", false},
		{"STORAGE", false},
		{"STORAGE2", false},
		{"STORAGE3", false},
		{"STORAGE4", false},

		// Case insensitive
		{"enabled", false},
		{"tag", false},
	}

	for _, test := range tests {
		result := InstanceNameIsValid(test.input)
		if result != test.expected {
			t.Errorf("InstanceNameIsValid(%q) = %v; want %v", test.input, result, test.expected)
		}
	}
}
