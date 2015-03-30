package cmd

import (
	"bitbucket.org/sinbad/git-lob/core"
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
)

// Actual implementation of main()
func MainImpl() int {

	// Generic panic handler so we get stack trace
	defer func() {
		if e := recover(); e != nil {
			fmt.Fprintf(os.Stderr, "git-lob panic: %v\n", e)
			fmt.Fprint(os.Stderr, string(debug.Stack()))
			os.Exit(99)
		}

	}()

	// Load up configuration from gitconfig
	util.LoadConfig(util.GlobalOptions)

	// Command line processing
	// Don't use flag package because it doesn't support options after commands, and
	// uses the form -option instead of --option which is non-standard for git
	var errors []string
	errors = ParseCommandLine(util.GlobalOptions, os.Args)

	// Init logging after command line opts
	util.InitLogging()
	core.InitCoreProviders()
	defer util.ShutDownLogging()

	if len(errors) > 0 {
		util.LogConsoleError(strings.Join(errors, "\n"))
		HelpUsage()
		return 1
	}

	// Check we're in a git repo and if not fail early
	// Unless help requested, in which case allow from anywhere
	_, _, err := util.GetRepoRoot()
	if err != nil && !util.GlobalOptions.HelpRequested &&
		util.GlobalOptions.Command != "help" {
		util.LogConsole(err.Error())
		return 33
	}

	switch util.GlobalOptions.Command {
	case "checkout":
		if util.GlobalOptions.HelpRequested {
			CheckoutHelp()
			return 0
		}
		return Checkout()
	case "prune":
		if util.GlobalOptions.HelpRequested {
			PruneHelp()
			return 0
		}
		return Prune()
	case "prune-shared":
		if util.GlobalOptions.HelpRequested {
			PruneSharedHelp()
			return 0
		}
		return PruneShared()
	case "fetch":
		if util.GlobalOptions.HelpRequested {
			FetchHelp()
			return 0
		}
		return Fetch()
	case "fetch-lob":
		if util.GlobalOptions.HelpRequested {
			FetchLobHelp()
			return 0
		}
		return FetchLob()
	case "filter-smudge":
		if util.GlobalOptions.HelpRequested {
			SmudgeFilterHelp()
			return 0
		}
		return SmudgeFilter()
	case "filter-clean":
		if util.GlobalOptions.HelpRequested {
			CleanFilterHelp()
			return 0
		}
		return CleanFilter()
	case "fsck":
		if util.GlobalOptions.HelpRequested {
			FsckHelp()
			return 0
		}
		return Fsck()
	case "help":
		// Support help as a command since 'git lob --help' uses git's help system
		// You have to use "git-lob --help" otherwise
		// Also this version accepts sub-topics e.g. 'git lob help config'
		Help()
		return 0
	case "listproviders":
		return ListProviders()
	case "missing":
		if util.GlobalOptions.HelpRequested {
			MissingHelp()
			return 0
		}
		return Missing()
	case "provider":
		return ProviderDetails()
	case "pull":
		if util.GlobalOptions.HelpRequested {
			PullHelp()
			return 0
		}
		return Pull()
	case "push":
		if util.GlobalOptions.HelpRequested {
			PushHelp()
			return 0
		}
		return Push()
	case "push-lob":
		if util.GlobalOptions.HelpRequested {
			PushLobHelp()
			return 0
		}
		return PushLob()
	case "mark-pushed":
		if util.GlobalOptions.HelpRequested {
			MarkPushedHelp()
			return 0
		}
		return MarkPushed()
	case "reset-pushed":
		if util.GlobalOptions.HelpRequested {
			ResetPushedHelp()
			return 0
		}
		return ResetPushed()
	case "last-pushed":
		if util.GlobalOptions.HelpRequested {
			LastPushedHelp()
			return 0
		}
		return LastPushed()
	default:
		if util.GlobalOptions.HelpRequested {
			Help()
			return 0
		}
		util.LogConsoleErrorf("git-lob: unknown command '%v'\n", util.GlobalOptions.Command)
		return 1
	}

	return -1
}
