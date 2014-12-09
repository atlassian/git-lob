package main

import (
	"fmt"
	"os"
	"regexp"
)

// Parse incoming arguments and convert to useful structure, with validation
// opts should be the options structure to update
// args should be exactly as provided by os.Args, ie first entry is the executable name
func parseCommandLine(opts *Options, args []string) (errors []string) {

	errors = make([]string, 0, 1)
	valueRegex := regexp.MustCompile(`^--([\w-]+)=(\w+)$`)
	boolRegex := regexp.MustCompile(`^--([\w-]+)$`)
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
			case "version":
				// Just write version and exit
				fmt.Fprintf(os.Stdout, "git-lob version %v\n", VersionString)
				os.Exit(0)
			case "help":
				opts.HelpRequested = true
			case "verbose":
				opts.Verbose = true
			case "quiet":
				opts.Quiet = true
			case "dry-run":
				opts.DryRun = true
			case "noninteractive":
				opts.NonInteractive = true
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

	if opts.Command == "" && !GlobalOptions.HelpRequested {
		errors = append(errors, "git-lob: command required")
	}

	return

}
