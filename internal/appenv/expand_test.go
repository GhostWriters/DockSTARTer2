package appenv

import (
	"testing"
)

func TestReplaceWithVars_Deterministic(t *testing.T) {
	vars := map[string]string{
		"HOME":                 "/home/user",
		"DOCKER_CONFIG_FOLDER": "/home/user/.config",
	}

	input := "/home/user/.config/appdata"
	// We expect DOCKER_CONFIG_FOLDER to be used because it's longer/more specific
	expected := "${DOCKER_CONFIG_FOLDER?}/appdata"

	// If random map order picked HOME first, we'd get "${HOME?}/.config/appdata", which is wrong.

	result := ReplaceWithVars(input, vars)

	if result != expected {
		t.Errorf("ReplaceWithVars failed. Expected '%s', got '%s'", expected, result)
	}
}
