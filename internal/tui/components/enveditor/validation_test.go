package enveditor

import (
	"testing"

	"DockSTARTer2/internal/appenv"
)

// TestHasValidationErrors_MultiServiceInstanceTab replicates how
// tabbed_vars_editor wires up a multi-service instance tab (e.g.
// ".env.app.immich-database", fileApp "immich-database") -- see
// tabbed_vars_editor_data.go's validationType/validationApp computation
// and tabbed_vars_editor_update.go's EnvLoadDoneMsg handler -- to check
// that ordinary built-in KEY=VALUE lines for such a tab don't get flagged
// as invalid.
func TestHasValidationErrors_MultiServiceInstanceTab(t *testing.T) {
	content := `### Immich
### Self-hosted photo and video management solution
###
IMMICH_POSTGRES_CONTAINER_NAME="${IMMICH_CONTAINER_NAME?}-postgres"
IMMICH_POSTGRES_HOSTNAME="${IMMICH_HOSTNAME?}-postgres"
IMMICH_POSTGRES_RESTART="${IMMICH_RESTART?}"
IMMICH_POSTGRES_TAG='14-vectorchord0.4.3-pgvectors0.2.0'
IMMICH_REDIS_CONTAINER_NAME="${IMMICH_CONTAINER_NAME?}-redis"
IMMICH_REDIS_HOSTNAME="${IMMICH_HOSTNAME?}-redis"
IMMICH_REDIS_RESTART="${IMMICH_RESTART?}"
IMMICH_REDIS_TAG='9-alpine'
`

	cases := []struct {
		name               string
		fileApp            string
		validationIsGlobal bool
	}{
		{"plain app tab", "immich", false},
		{"per-service tab (hyphen)", "immich-database", false},
		{"virtual/shared tab (triple underscore)", "immich___postgres", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := New()
			m.ValidationType = c.fileApp + ":"
			m.ValidationAppName = c.fileApp
			m.ValidationIsGlobal = c.validationIsGlobal
			m.ValidateFunc = appenv.VarNameIsValid
			m.ParseEnv(content, func(string) string { return "" }, nil)

			if m.HasValidationErrors() {
				t.Errorf("HasValidationErrors() = true for fileApp %q, want false (all keys are well-formed builtins)", c.fileApp)
			}
		})
	}
}

// TestHasValidationErrors_MultiServiceDBTab covers the "immich-database"
// and "immich___postgres" per-service files' own DB_*/POSTGRES_* vars
// (unprefixed with the app name), pulled from the live failure reported in
// dstest.lan's Configure Immich screen.
func TestHasValidationErrors_MultiServiceDBTab(t *testing.T) {
	content := `### Immich
### Self-hosted photo and video management solution
###
DB_HOSTNAME="${IMMICH_POSTGRES_CONTAINER_NAME?}"
DB_PASSWORD=''
REDIS_HOSTNAME="${IMMICH_REDIS_HOSTNAME?}"
`

	m := New()
	m.ValidationType = "immich-database:"
	m.ValidationAppName = "immich-database"
	m.ValidationIsGlobal = false
	m.ValidateFunc = appenv.VarNameIsValid
	m.ParseEnv(content, func(string) string { return "" }, nil)

	if m.HasValidationErrors() {
		t.Errorf("HasValidationErrors() = true for immich-database tab, want false")
	}
}

// TestHasValidationErrors_GlobalTabMultiServiceVars replicates the global
// ".env" tab filtered to a multi-service app (App != "", IsGlobal true) --
// tabbed_vars_editor.go's first ".env" tab spec. Its own vars use the
// "APPNAME___SERVICE__VAR" naming convention (e.g.
// "IMMICH___ML__CONTAINER_NAME"), which VarNameToAppName already resolves
// to the service-qualified "IMMICH___ML" -- confirmed live on dstest.lan
// that this was being compared unstripped against the plain "IMMICH"
// ValidationType, always failing and blocking Save.
func TestHasValidationErrors_GlobalTabMultiServiceVars(t *testing.T) {
	content := `### Immich
### Self-hosted photo and video management solution
###
IMMICH___ML__CONTAINER_NAME="${IMMICH_CONTAINER_NAME?}-ml"
IMMICH___ML__HOSTNAME="${IMMICH_HOSTNAME?}-ml"
IMMICH___ML__RESTART="${IMMICH_RESTART?}"
IMMICH___ML__TAG='latest'
IMMICH___POSTGRES__CONTAINER_NAME="${IMMICH_CONTAINER_NAME?}-postgres"
IMMICH___REDIS__TAG='9-alpine'
`

	m := New()
	m.ValidationType = "IMMICH" // global tab: no trailing colon (see tabbed_vars_editor_data.go)
	m.ValidationAppName = "IMMICH"
	m.ValidationIsGlobal = true
	m.ValidateFunc = appenv.VarNameIsValid
	m.ParseEnv(content, func(string) string { return "" }, nil)

	if m.HasValidationErrors() {
		t.Errorf("HasValidationErrors() = true for global Immich tab's own multi-service vars, want false")
	}
}
