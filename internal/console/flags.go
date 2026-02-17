package console

import (
	"github.com/spf13/pflag"
)

// CurrentFlags holds the modifier flags (like -f, --force) encountered on the command line,
// mirroring Bash's CURRENT_FLAGS_ARRAY.
var CurrentFlags []string

// RestArgs holds the remaining command line arguments and commands that haven't been processed yet,
// mirroring Bash's REST_OF_ARGS_ARRAY.
var RestArgs []string

// Force returns true if the --force flag is set.
func Force() bool {
	v, _ := pflag.CommandLine.GetBool("force")
	return v
}

// AssumeYes returns true if the --yes flag is set.
func AssumeYes() bool {
	v, _ := pflag.CommandLine.GetBool("yes")
	return v
}

// Verbose returns true if the --verbose flag is set.
func Verbose() bool {
	v, _ := pflag.CommandLine.GetBool("verbose")
	return v
}

// Debug returns true if the --debug flag is set.
func Debug() bool {
	v, _ := pflag.CommandLine.GetBool("debug")
	return v
}

// GUI returns true if the --gui flag is set.
func GUI() bool {
	v, _ := pflag.CommandLine.GetBool("gui")
	return v
}
