package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
)

var (
	GlobalOptions *Options = NewOptions()
	VersionMajor           = 0
	VersionMinor           = 1
	VersionPatch           = 0
	VersionString          = fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
)

func main() {
	// Need to send the result code to the OS but also need to support 'defer'
	// os.Exit would finish before any defers, so wrap everything in mainImpl()
	os.Exit(mainImpl())
}

func mainImpl() int {

	// Generic panic handler so we get stack trace
	defer func() {
		if e := recover(); e != nil {
			LogErrorf("git-lob panic: \n", e)
			LogError(string(debug.Stack()))
			os.Exit(99)
		}

	}()

	// Load up configuration from gitconfig
	LoadConfig(GlobalOptions)

	// Command line processing
	// Don't use flag package because it doesn't support options after commands, and
	// uses the form -option instead of --option which is non-standard for git
	var errors []string
	errors = parseCommandLine(GlobalOptions, os.Args)

	// Init logging after command line opts
	InitLogging()
	defer ShutDownLogging()

	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "%v\n", strings.Join(errors, "\n"))
		printUsage()
		return 1
	}

	switch GlobalOptions.Command {
	case "cleanup":
		return Cleanup()
	case "filter-smudge":
		return SmudgeFilter()
	case "filter-clean":
		return CleanFilter()
	default:
		fmt.Fprintf(os.Stderr, "git-lob: unknown command '%v'\n", GlobalOptions.Command)
		return 1
	}

	return -1
}
func printUsage() {
	fmt.Fprintf(os.Stderr, usageTxt)
}
func printHelp() {
	fmt.Fprintf(os.Stderr, helpTxt)
}

const usageTxt = `Usage: git-lob [command] [options] [file...]
`

const helpTxt = `Usage: git-lob [command] [options] [file...]

  git-lob improves handling of large objects (including binary files) in git

Commands:
  cleanup             Remove binaries unreferenced by any commit or the index

  filter-smudge       Execute the git smudge filter
  filter-clean        Execute the git clean filter

 

Global Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't perform actions, just report
  --noninteractive, -n Never prompt for user input
  --force, -f          Force action, break rules

  --help               Print this message

Config files:
  ~/.gitconfig and $REPO/.git/config can be used to modify default behaviour.
  All settings are in the [git-lob] section

  git-lob.verbose    Same as --verbose on command line
  git-lob.quiet      Same as --quiet on the command line
  git-lob.logenabled Enable logging of messages to a file
  git-lob.logfile    Log file to write if logenabled (default: ~/git-lob.log)
  git-lob.logverbose Verbose logging in log file
                     (separate to console)
  git-lob.chunksize  The size chunks to split binary files into in binary store
  git-lob.retention  Number of days before latest commit that other revisions
                     of files will be kept for

`
