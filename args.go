package main

import (
	"fmt"
	"regexp"
)

// Options to the command
type CommandLineOptions struct {
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
}

// Parse incoming arguments and convert to useful structure, with validation
// Args should be exactly as provided by os.Args, ie first entry is the executable name
func parseCommandLine(args []string) (opts *CommandLineOptions, errors []string) {

	errors = make([]string, 0, 1)
	opts = &CommandLineOptions{
		StringOpts: make(map[string]string),
		Args:       make([]string, 0, 5)}
	valueRegex := regexp.MustCompile(`^--(\w+)=(\w+)$`)
	boolRegex := regexp.MustCompile(`^--(\w+)$`)
	shortBoolRegex := regexp.MustCompile(`^-(\w)$`)
	foundCommand := false
	for _, arg := range args[1:] {

		if match := valueRegex.FindStringSubmatch(arg); match != nil {

			// Must be 3 items if matched
			stropt := match[1]
			strval := match[2]
			opts.StringOpts[stropt] = strval

		} else if match := boolRegex.FindStringSubmatch(arg); match != nil {

			stropt := match[1]
			switch stropt {
			case "verbose":
				opts.Verbose = true
			case "quiet":
				opts.Quiet = true
			case "noninteractive":
				opts.NonInteractive = true
			case "force":
				opts.Force = true
			default:
				errors = append(errors, fmt.Sprintf("git-lob: invalid option: %v", arg))
			}

		} else if match := shortBoolRegex.FindStringSubmatch(arg); match != nil {
			stropt := match[1]
			switch stropt {
			case "v":
				opts.Verbose = true
			case "q":
				opts.Quiet = true
			case "n":
				opts.NonInteractive = true
			case "f":
				opts.Force = true
			default:
				errors = append(errors, fmt.Sprintf("git-lob: invalid option: %v", arg))
			}
		} else {
			if !foundCommand {
				opts.Command = arg
				foundCommand = true
			} else {
				opts.Args = append(opts.Args, arg)
			}

		}
	}

	if opts.Command == "" {
		errors = append(errors, "git-lob: command required")
	}

	return

}
