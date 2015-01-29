package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
)

var (
	GlobalOptions  *Options = NewOptions()
	VersionMajor            = 0
	VersionMinor            = 2
	VersionPatch            = 0
	VersionBuildID string   // populated in build.sh to the git hash
)

func Version() string {
	if VersionBuildID != "" {
		return fmt.Sprintf("%d.%d.%d [%v]", VersionMajor, VersionMinor, VersionPatch, VersionBuildID)
	} else {
		return fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
	}
}

func main() {
	// Need to send the result code to the OS but also need to support 'defer'
	// os.Exit would finish before any defers, so wrap everything in mainImpl()
	os.Exit(mainImpl())
}

func mainImpl() int {

	// Generic panic handler so we get stack trace
	defer func() {
		if e := recover(); e != nil {
			fmt.Fprintf(os.Stderr, "git-lob panic: %v\n", e)
			fmt.Fprint(os.Stderr, string(debug.Stack()))
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
	InitCoreProviders()
	defer ShutDownLogging()

	if len(errors) > 0 {
		LogConsoleError(strings.Join(errors, "\n"))
		printUsage()
		return 1
	}

	// Check we're in a git repo and if not fail early
	// Unless help requested, in which case allow from anywhere
	_, _, err := GetRepoRoot()
	if err != nil && !GlobalOptions.HelpRequested &&
		GlobalOptions.Command != "help" {
		LogConsole(err.Error())
		return 33
	}

	switch GlobalOptions.Command {
	case "checkout":
		if GlobalOptions.HelpRequested {
			cmdCheckoutHelp()
			return 0
		}
		return cmdCheckout()
	case "prune":
		if GlobalOptions.HelpRequested {
			cmdPruneHelp()
			return 0
		}
		return cmdPrune()
	case "prune-shared":
		if GlobalOptions.HelpRequested {
			cmdPruneSharedHelp()
			return 0
		}
		return cmdPruneShared()
	case "fetch":
		if GlobalOptions.HelpRequested {
			cmdFetchHelp()
			return 0
		}
		return cmdFetch()
	case "filter-smudge":
		if GlobalOptions.HelpRequested {
			cmdSmudgeFilterHelp()
			return 0
		}
		return cmdSmudgeFilter()
	case "filter-clean":
		if GlobalOptions.HelpRequested {
			cmdCleanFilterHelp()
			return 0
		}
		return cmdCleanFilter()
	case "help":
		// Also support help as a command since 'git lob --help' uses git's help system
		// You have to use "git-lob --help" otherwise
		printHelp()
		return 0
	case "listproviders":
		return cmdListProviders()
	case "provider":
		return cmdProviderDetails()
	case "pull":
		if GlobalOptions.HelpRequested {
			cmdPullHelp()
			return 0
		}
		return cmdPull()
	case "push":
		if GlobalOptions.HelpRequested {
			cmdPushHelp()
			return 0
		}
		return cmdPush()
	default:
		if GlobalOptions.HelpRequested {
			printHelp()
			return 0
		}
		LogConsoleErrorf("git-lob: unknown command '%v'\n", GlobalOptions.Command)
		return 1
	}

	return -1
}
func printUsage() {
	// For safety, these always go to stderr not stdout
	// That's because this is before the command has been chosen and therefore has not
	// had the chanced to call LogAllConsoleOutputToStdErr. A poorly
	// configured filter shouldn't be allowed to corrupt file output
	LogConsoleError(usageTxt)
}
func printHelp() {
	// See above for why this is stderr not stdout
	LogConsoleError(helpTxt)
}

const usageTxt = `Usage: git-lob [command] [options] [file...]
`

const helpTxt = `Usage: git-lob [command] [options] [file...]

  git-lob improves handling of large objects (including binary files) in git

  Use 'git-lob <command> --help' for more details

Commands:
  push                Upload local binaries to a remote.
  fetch               Download binaries from a remote.
  checkout            Check the working copy and fill in any binary content
                      that's missing
  pull                Perform 'fetch' then 'checkout'

  filter-smudge       Execute the git smudge filter (when checking out)
                      This should be set up in .gitattributes
  filter-clean        Execute the git clean filter (when adding/committing)
                      This should be set up in .gitattributes

  listproviders       List the available remote providers
  provider <name>     Print detail about named provider

  prune               Remove binaries unreferenced by any commit or the index
                      from the local repo binary store (and shared if no other
                      usage)
  prune-shared        Delete any binaries in the shared store which have become
                      unreferenced because repos were manually deleted

Global Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't perform actions, just report
  --noninteractive, -n Never prompt for user input

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
  git-lob.autofetch  Automatically download binaries required on checkout if
                     they're not already present in the binary store
  git-lob.sharedstore
                     A shared folder in which to store binary content rather
                     than storing it inside each repo. This minimises storage
                     when you have multiple clones.
                     Files will be hard linked into your repo so that
                     they actually appear there as usual but storage is only
                     used once if the same SHA appears in multiple repos.
                     When the number of hard links on a file in the shared
                     area reaches 1 during cleanup, the shared file is deleted.
                     NOTE: requires a file system capable of hard links
                     e.g. ext3, HFS, NTFS, and the shared store and the repos
                     using it must be on the same filesystem (drive on Windows)

  Other settings may be used by specific commands, see their help text.
`
