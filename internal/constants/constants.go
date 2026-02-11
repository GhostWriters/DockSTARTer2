package constants

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
