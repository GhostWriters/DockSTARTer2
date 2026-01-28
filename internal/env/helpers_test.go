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

func TestIsGlobalVar(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Global variables (no __ or only one segment)
		{"PUID", true},
		{"PGID", true},
		{"TZ", true},
		{"DOCKER_VOLUME_STORAGE", true},
		{"HOME", true},

		// App variables (have __ with segments)
		{"RADARR__ENABLED", false},
		{"RADARR__PORT_7878", false},
		{"RADARR__4K__ENABLED", false},
		{"SONARR__CONTAINER_NAME", false},
	}

	for _, test := range tests {
		result := IsGlobalVar(test.input)
		if result != test.expected {
			t.Errorf("IsGlobalVar(%q) = %v; want %v", test.input, result, test.expected)
		}
	}
}

func TestAppVarsLines(t *testing.T) {
	lines := []string{
		"PUID='1000'",
		"PGID='1000'",
		"TZ='UTC'",
		"RADARR__ENABLED='true'",
		"RADARR__PORT_7878='7878'",
		"RADARR__CONTAINER_NAME='radarr'",
		"RADARR__4K__ENABLED='true'",
		"RADARR__4K__PORT_7878='7879'",
		"RADARR__4K__CONTAINER_NAME='radarr__4k'",
		"SONARR__ENABLED='true'",
		"# Comment line",
		"",
	}

	// Test globals (empty appName)
	globals := AppVarsLines("", lines)
	expectedGlobals := []string{
		"PUID='1000'",
		"PGID='1000'",
		"TZ='UTC'",
	}
	if len(globals) != len(expectedGlobals) {
		t.Errorf("AppVarsLines(\"\") returned %d variables, want %d", len(globals), len(expectedGlobals))
	}
	for i, expected := range expectedGlobals {
		if i >= len(globals) || globals[i] != expected {
			t.Errorf("AppVarsLines(\"\")[%d] = %q; want %q", i, globals[i], expected)
		}
	}

	// Test RADARR (should not include RADARR__4K vars)
	radarrVars := AppVarsLines("RADARR", lines)
	expectedRadarr := []string{
		"RADARR__ENABLED='true'",
		"RADARR__PORT_7878='7878'",
		"RADARR__CONTAINER_NAME='radarr'",
	}
	if len(radarrVars) != len(expectedRadarr) {
		t.Errorf("AppVarsLines(\"RADARR\") returned %d variables, want %d", len(radarrVars), len(expectedRadarr))
		t.Logf("Got: %v", radarrVars)
	}
	for i, expected := range expectedRadarr {
		if i >= len(radarrVars) || radarrVars[i] != expected {
			t.Errorf("AppVarsLines(\"RADARR\")[%d] = %q; want %q", i, radarrVars[i], expected)
		}
	}

	// Test RADARR__4K (should only include RADARR__4K vars)
	radarr4kVars := AppVarsLines("RADARR__4K", lines)
	expectedRadarr4k := []string{
		"RADARR__4K__ENABLED='true'",
		"RADARR__4K__PORT_7878='7879'",
		"RADARR__4K__CONTAINER_NAME='radarr__4k'",
	}
	if len(radarr4kVars) != len(expectedRadarr4k) {
		t.Errorf("AppVarsLines(\"RADARR__4K\") returned %d variables, want %d", len(radarr4kVars), len(expectedRadarr4k))
		t.Logf("Got: %v", radarr4kVars)
	}
	for i, expected := range expectedRadarr4k {
		if i >= len(radarr4kVars) || radarr4kVars[i] != expected {
			t.Errorf("AppVarsLines(\"RADARR__4K\")[%d] = %q; want %q", i, radarr4kVars[i], expected)
		}
	}

	// Test SONARR
	sonarrVars := AppVarsLines("SONARR", lines)
	if len(sonarrVars) != 1 {
		t.Errorf("AppVarsLines(\"SONARR\") returned %d variables, want 1", len(sonarrVars))
	}
}
