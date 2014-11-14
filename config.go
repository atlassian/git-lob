package main

// Options (command line or config file TODO)
type Options struct {
	// Output verbosely
	Verbose bool
	// Output quietly
	Quiet bool
	// Never prompt for user input, rely on command line options only
	NonInteractive bool
	// The command to run
	Command string
	// Other value options not converted
	StringOpts map[string]string
	// Other arguments to the command
	Args []string
	// Force option (not used for all commands)
	Force bool
	// Whether to write output to a log
	EnableLogFile bool
	// Log file (optional, defaults to ~/git-lob.log if not specified)
	LogFile string
}

func NewOptions() *Options {
	return &Options{
		StringOpts: make(map[string]string),
		Args:       make([]string, 0, 5)}
}
