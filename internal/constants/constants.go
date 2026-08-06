package constants

const LegacyApplicationName = "dockstarter"

// Folder Names
const (
	TemplatesDirName  = ".apps"
	InstancesDirName  = "instances"
	TimestampsDirName = "timestamps"
	ThemesDirName     = "themes"
	TempDirName       = "temp"
	EnvFilesDirName   = "env_files"
)

// File Names
const (
	ComposeFileName         = "docker-compose.yml"
	ComposeOverrideFileName = "docker-compose.override.yml"
	EnvFileName             = ".env"
	EnvExampleFileName      = ".env.example"
	AppEnvFileNamePrefix    = ".env.app."
	AppConfigFileName       = "dockstarter2.toml"
)

// Marker Prefixes
const (
	YmlMergeMarkerPrefix      = "yml_merge_"
	AppVarsCreateMarkerPrefix = "appvars_create_"
	EnvUpdateMarkerPrefix     = "env_update_"
)

// InternalFixPermissionsArg is the hidden first argument that turns a DS2
// invocation into the elevated permission-fixing helper (see
// internal/system's RunInternalFixPermissions). Defined here, in a leaf
// package, because internal/boot's init-time sudo demotion must recognize
// it too (that helper deliberately runs as root and must NOT be demoted),
// and boot cannot import internal/system without a cycle.
const InternalFixPermissionsArg = "--internal-fix-permissions"
