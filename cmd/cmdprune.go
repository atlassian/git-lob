package cmd

import (
	"strings"

	"github.com/atlassian/git-lob/core"
	"github.com/atlassian/git-lob/util"
)

// Common prune callback
var pruneCallbackImpl = func(t core.PruneCallbackType, lobsha string) {
	// Include this stuff in the log because it's important
	util.LogConsoleDebugf("\r") // to reset any progress spinner but don't want \r in log
	switch t {
	case core.PruneRetainByDate:
		util.LogDebugf("Prune: retaining %v (date)\n", lobsha)
	case core.PruneRetainNotPushed:
		util.LogDebugf("Prune: retaining %v (not pushed)\n", lobsha)
	case core.PruneRetainReferenced:
		util.LogDebugf("Prune: retaining %v (referenced)\n", lobsha)
	case core.PruneDeleted:
		if util.GlobalOptions.DryRun {
			util.LogDebugf("Prune: would delete %v (dry run)\n", lobsha)
		} else {
			util.LogDebugf("Prune: deleted %v\n", lobsha)
		}
	case core.PruneWorking:
		// nothing, just spinner below
	}
	// Always continue spinner
	util.LogConsoleSpinner("Processing: ")
}

func Prune() int {
	errorList := validateCustomOptions(util.GlobalOptions, nil, []string{"unreferenced", "u", "safe", "k"})
	if len(errorList) > 0 {
		util.LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	optOnlyUnreferenced := util.GlobalOptions.BoolOpts.Contains("unreferenced") || util.GlobalOptions.BoolOpts.Contains("u")
	optSafeMode := util.GlobalOptions.BoolOpts.Contains("safe") || util.GlobalOptions.BoolOpts.Contains("k")

	if optOnlyUnreferenced && optSafeMode {
		util.LogConsole("The --safe option does nothing in --unreferenced mode because unreferenced\nbinaries are never pushed")
	}

	// Upgrade to safe mode if configured
	optSafeMode = optSafeMode || util.GlobalOptions.PruneSafeMode

	var shas []string
	var err error
	if optOnlyUnreferenced {
		// Only purge unreferenced
		util.LogConsole("Pruning unreferenced binaries...")
		shas, err = core.PruneUnreferenced(util.GlobalOptions.DryRun, pruneCallbackImpl)
		util.LogConsoleSpinnerFinish("Processing: ")
		if err != nil {
			util.LogErrorf("Prune failed: %v\n", err)
			return 3
		}
	} else {
		// Purge old & unreferenced
		util.LogConsole("Pruning old binaries...")
		shas, err = core.PruneOld(util.GlobalOptions.DryRun, optSafeMode, pruneCallbackImpl)
		util.LogConsoleSpinnerFinish("Processing: ")
		if err != nil {
			util.LogErrorf("Prune failed: %v\n", err)
			return 3
		}

	}
	if util.GlobalOptions.DryRun {
		util.LogConsolef("%d binaries would have been deleted.\n", len(shas))
		util.LogConsole("Run command again without --dry-run to actually perform the deletion.")
	} else {
		util.LogConsolef("%d binaries were deleted.\n", len(shas))
	}

	return 0

}

func PruneShared() int {

	// Quick pre-flight check
	shared := core.GetSharedLOBRoot()
	if shared == "" {
		util.LogConsoleError("No shared store has been configured for this repo, cannot prune it.")
		return 9
	} else if !util.DirExists(shared) {
		util.LogConsoleErrorf("Configured shared store '%v' doesn't exist, cannot prune.\n", shared)
		return 9
	}
	util.LogConsole("Pruning shared store...")
	shas, err := core.PruneSharedStore(util.GlobalOptions.DryRun, pruneCallbackImpl)
	util.LogConsoleSpinnerFinish("Processing: ")
	if err != nil {
		util.LogErrorf("Prune failed: %v\n", err)
		return 3
	}
	if util.GlobalOptions.DryRun {
		if util.GlobalOptions.Verbose {
			util.LogConsolef("%d LOBs would have been deleted:\n", len(shas))
			util.LogConsole(strings.Join(shas, "\n"))
		} else {
			util.LogConsolef("%d LOBs would have been deleted.\n", len(shas))
		}
		util.LogConsole("Run command again without --dry-run to actually perform the deletion.")
	} else {
		if util.GlobalOptions.Verbose {
			util.LogConsolef("%d LOBs were deleted:\n", len(shas))
			util.LogConsoleDebug(strings.Join(shas, "\n"))
		} else {
			util.LogConsolef("%d LOBs were deleted.\n", len(shas))
		}
	}
	return 0
}

// Perform the default prune after fetching or pulling
// Only call this if pruning was requested & not dry running
func PostFetchPullPrune() ([]string, error) {
	shas, err := core.PruneOld(false, util.GlobalOptions.PruneSafeMode, pruneCallbackImpl)
	util.LogConsoleSpinnerFinish("Processing: ")
	return shas, err
}

func PruneHelp() {
	util.LogConsole(`Usage: git-lob prune [options]

  Removes old and unreferenced binaries from local storage.

  A binary will NOT BE PRUNED if:
    1. It is referenced by a reachable commit which is inside the 'retention 
       period' as defined below OR
    2. It is referenced by a commit for which the binaries haven't been pushed

  To put that another way, a binary WILL BE PRUNED if:
    1. It is not referenced by any reachable commit, or only by a reachable 
       commit which is outside the 'retention period' AND
    2. If referenced by an older commit, it has been pushed (i.e. the local
       copy is not the only one)

Options:
  --safe, -k           Before deleting old binaries that we think we've pushed,
                       doubly verify with the remote that it has a copy
                       Also see git-lob.prune-safe config setting
  --unreferenced, -u   Only prune totally unreferenced binaries, not old ones
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't actually delete anything, just report

REACHABLE COMMITS & THE RETENTION PERIOD

  Binaries are retained if:
    * They're used by current HEAD, OR
    * They're referenced by an ancestor of HEAD within the number of days given
      by git-lob.retention-period-head of HEAD's last commit date OR
    * They're used the head of antother branch (local and remote) or tag which
      has a commit within git-lob.retention-period-refs days of the current date
    * They're used by other commits on those branches within 
      git-lob.retention-period-other days of the branch's last commit date

  See 'git lob help config' for a summary of these settings & their defaults, 
  in the 'prune' section.


DEFINITION OF "PUSHED"
  A binary is considered 'pushed' if it has been pushed to 'origin'. You can
  change the remote which is checked via the setting
  git-lob.prune-check-remote, which can be set to another remote name, or '*'
  to allow any remote to count.

  By default, uses only the local records of whether something has been pushed.
  If you use the --safe option or git-lob.prune-safe in your gitconfig, then
  the remote is contacted for each binary to be deleted to confirm it exists
  there, before it is deleted locally. This is slower of course.

SHARED STORE
  If you are using a shared store, when a file is pruned locally, if there 
  are no other repos referencing this binary file then it is also deleted 
  from the shared store.

  If you manually deleted a repository and want to only clean up the shared
  store, use 'git lob prune-shared'

CONFIG
  Type 'git lob help config' for details, see the 'prune' section

`)
}

func PruneSharedHelp() {
	util.LogConsole(`Usage: git-lob prune-shared [options]

  Removes binaries from the shared store which are no longer linked to by any
  repo. 

  Usually 'git-lob prune' will delete files from the shared store too once
  the last repo link is removed, but if you manually delete repositories then
  this won't happen. prune-shared deletes any binaries in the shared
  store which have no other links left in the file system. This is relatively
  quick compared to the repo prune since it doesn't require checking any
  git repos.
  
Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't actually delete anything, just report
`)
}
