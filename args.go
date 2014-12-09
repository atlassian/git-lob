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
				opts.BoolOpts.Add(stropt)
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
				opts.BoolOpts.Add(stropt)
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

// Having already called parseCommandLine, perform context-specific validation
// only to accept certain options. Errors will be returned for any options present that are
// not in validValueOpts / validBoolOpts
func validateCustomOptions(opts *Options, validValueOpts, validBoolOpts []string) (errors []string) {
	validValueSet := NewStringSetFromSlice(validValueOpts)
	validBoolSet := NewStringSetFromSlice(validBoolOpts)

	for k, v := range opts.StringOpts {
		if !validValueSet.Contains(k) {
			if validBoolSet.Contains(k) {
				errors = append(errors, fmt.Sprintf("git-lob: option --%v should not include a value (boolean option)", k))
			} else {
				errors = append(errors, fmt.Sprintf("git-lob: invalid option --%v=%v", k, v))
			}
		}
	}
	for k := range opts.BoolOpts.Iter() {
		if !validBoolSet.Contains(k) {
			if validValueSet.Contains(k) {
				errors = append(errors, fmt.Sprintf("git-lob: option --%v requires a value", k))
			} else {
				if len(k) > 1 {
					errors = append(errors, fmt.Sprintf("git-lob: invalid option --%v", k))
				} else {
					errors = append(errors, fmt.Sprintf("git-lob: invalid option -%v", k))
				}
			}
		}
	}
	return
}
