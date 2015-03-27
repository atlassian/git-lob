package main

import (
	"bitbucket.org/sinbad/git-lob/cmd"
	. "bitbucket.org/sinbad/git-lob/core"
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
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
	InitCoreProviders()
	defer util.ShutDownLogging()

	if len(errors) > 0 {
		util.LogConsoleError(strings.Join(errors, "\n"))
		cmdHelpUsage()
		return 1
	}

	// Check we're in a git repo and if not fail early
	// Unless help requested, in which case allow from anywhere
	_, _, err := GetRepoRoot()
	if err != nil && !util.GlobalOptions.HelpRequested &&
		util.GlobalOptions.Command != "help" {
		util.LogConsole(err.Error())
		return 33
	}

	switch util.GlobalOptions.Command {
	case "checkout":
		if util.GlobalOptions.HelpRequested {
			cmd.CheckoutHelp()
			return 0
		}
		return cmd.Checkout()
	case "prune":
		if util.GlobalOptions.HelpRequested {
			cmdPruneHelp()
			return 0
		}
		return cmdPrune()
	case "prune-shared":
		if util.GlobalOptions.HelpRequested {
			cmdPruneSharedHelp()
			return 0
		}
		return cmdPruneShared()
	case "fetch":
		if util.GlobalOptions.HelpRequested {
			cmdFetchHelp()
			return 0
		}
		return cmdFetch()
	case "fetch-lob":
		if util.GlobalOptions.HelpRequested {
			cmdFetchLobHelp()
			return 0
		}
		return cmdFetchLob()
	case "filter-smudge":
		if util.GlobalOptions.HelpRequested {
			cmdSmudgeFilterHelp()
			return 0
		}
		return cmdSmudgeFilter()
	case "filter-clean":
		if util.GlobalOptions.HelpRequested {
			cmdCleanFilterHelp()
			return 0
		}
		return cmdCleanFilter()
	case "fsck":
		if util.GlobalOptions.HelpRequested {
			cmdFsckHelp()
			return 0
		}
		return cmdFsck()
	case "help":
		// Support help as a command since 'git lob --help' uses git's help system
		// You have to use "git-lob --help" otherwise
		// Also this version accepts sub-topics e.g. 'git lob help config'
		cmdHelp()
		return 0
	case "listproviders":
		return cmdListProviders()
	case "missing":
		if util.GlobalOptions.HelpRequested {
			cmdMissingHelp()
			return 0
		}
		return cmdMissing()
	case "provider":
		return cmdProviderDetails()
	case "pull":
		if util.GlobalOptions.HelpRequested {
			cmdPullHelp()
			return 0
		}
		return cmdPull()
	case "push":
		if util.GlobalOptions.HelpRequested {
			cmdPushHelp()
			return 0
		}
		return cmdPush()
	case "push-lob":
		if util.GlobalOptions.HelpRequested {
			cmdPushLobHelp()
			return 0
		}
		return cmdPushLob()
	case "mark-pushed":
		if util.GlobalOptions.HelpRequested {
			cmdMarkPushedHelp()
			return 0
		}
		return cmdMarkPushed()
	case "reset-pushed":
		if util.GlobalOptions.HelpRequested {
			cmdResetPushedHelp()
			return 0
		}
		return cmdResetPushed()
	case "last-pushed":
		if util.GlobalOptions.HelpRequested {
			cmdLastPushedHelp()
			return 0
		}
		return cmdLastPushed()
	default:
		if util.GlobalOptions.HelpRequested {
			cmdHelp()
			return 0
		}
		util.LogConsoleErrorf("git-lob: unknown command '%v'\n", util.GlobalOptions.Command)
		return 1
	}

	return -1
}
