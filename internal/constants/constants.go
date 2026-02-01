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
	AppINIFileName          = "dockstarter2.ini"
)

// Config INI Keys
const (
	ConfigFolderKey   = "ConfigFolder"
	ComposeFolderKey  = "ComposeFolder"
	BordersKey        = "Borders"
	LineCharactersKey = "LineCharacters"
	ShadowKey         = "Shadow"
	ScrollbarKey      = "Scrollbar"
	ThemeKey          = "Theme"
)

// Marker Prefixes
const (
	YmlMergeMarkerPrefix      = "yml_merge_"
	AppVarsCreateMarkerPrefix = "appvars_create_"
	EnvUpdateMarkerPrefix     = "env_update_"
)
