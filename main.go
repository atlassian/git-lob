package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
)

var (
	GlobalOptions *CommandLineOptions
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

	// Command line processing
	// Don't use flag package because it doesn't support options after commands, and
	// uses the form -option instead of --option which is non-standard for git
	var errors []string
	GlobalOptions, errors = parseCommandLine(os.Args)

	// Init logging after command line opts
	InitLogging()
	defer ShutDownLogging()

	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "%v\n", strings.Join(errors, "\n"))
		printUsage()
		return 1
	}

	switch GlobalOptions.Command {
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

const usageTxt = `Usage: git-lob [command] [options] [file...]

  git-lob improves handling of large objects (including binary files) in git

Commands:
  filter-smudge       Execute the git smudge filter
  filter-clean        Execute the git clean filter

  .. More TODO

Options:

  -help               Print this message

`
