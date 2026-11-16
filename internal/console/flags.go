package console

// Flag state variables set by cmd/executor.go
var (
	GlobalYes     bool
	GlobalForce   bool
	GlobalGUI     bool
	GlobalVerbose int // verbosity level, as the -v flags set it (commands.ApplyVerbose)
	GlobalDebug   bool
)

// CurrentFlags holds the modifier flags (like -f, --force) encountered on the command line,
// mirroring Bash's CURRENT_FLAGS_ARRAY.
var CurrentFlags []string

// RestArgs holds the remaining command line arguments and commands that haven't been processed yet,
// mirroring Bash's REST_OF_ARGS_ARRAY.
var RestArgs []string

// Force returns true if the --force flag is set.
func Force() bool {
	return GlobalForce
}

// AssumeYes returns true if the --yes flag is set.
func AssumeYes() bool {
	return GlobalYes
}

// Verbose returns true if the --verbose flag is set at any level.
func Verbose() bool {
	return GlobalVerbose > 0
}

// VerboseLevel returns the verbosity level: 0 without --verbose, one more for each -v,
// or N for -v=N.
func VerboseLevel() int {
	return GlobalVerbose
}

// Debug returns true if the --debug flag is set.
func Debug() bool {
	return GlobalDebug
}

// GUI returns true if the --gui flag is set.
func GUI() bool {
	return GlobalGUI
}

// SetForce sets the --force flag.
func SetForce(v bool) {
	GlobalForce = v
}

// SetAssumeYes sets the --yes flag.
func SetAssumeYes(v bool) {
	GlobalYes = v
}

// SetVerboseLevel sets the verbosity level (0 = off).
func SetVerboseLevel(level int) {
	GlobalVerbose = max(level, 0)
}

// SetDebug sets the --debug flag.
func SetDebug(v bool) {
	GlobalDebug = v
}
