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
		cmdHelpUsage()
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
	case "fetch-lob":
		if GlobalOptions.HelpRequested {
			cmdFetchLobHelp()
			return 0
		}
		return cmdFetchLob()
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
		// Support help as a command since 'git lob --help' uses git's help system
		// You have to use "git-lob --help" otherwise
		// Also this version accepts sub-topics e.g. 'git lob help config'
		cmdHelp()
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
	case "push-lob":
		if GlobalOptions.HelpRequested {
			cmdPushLobHelp()
			return 0
		}
		return cmdPushLob()
	case "mark-pushed":
		if GlobalOptions.HelpRequested {
			cmdMarkPushedHelp()
			return 0
		}
		return cmdMarkPushed()
	case "reset-pushed":
		if GlobalOptions.HelpRequested {
			cmdResetPushedHelp()
			return 0
		}
		return cmdResetPushed()
	default:
		if GlobalOptions.HelpRequested {
			cmdHelp()
			return 0
		}
		LogConsoleErrorf("git-lob: unknown command '%v'\n", GlobalOptions.Command)
		return 1
	}

	return -1
}
