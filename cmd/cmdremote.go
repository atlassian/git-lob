package cmd

import (
	"bitbucket.org/sinbad/git-lob/core"
	"bitbucket.org/sinbad/git-lob/util"
	"regexp"
	"strings"
)

// Command line low-level tool to manually mark a remote/commit combo as pushed
func MarkPushed() int {
	// git-lob mark-pushed <remote> <ref>...

	// Validate custom options (none)
	errorList := validateCustomOptions(util.GlobalOptions, nil, []string{})
	if len(errorList) > 0 {
		util.LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	if len(util.GlobalOptions.Args) < 1 {
		util.LogConsoleError("Too few arguments; must supply a remote")
		return 9
	}
	// first parameter must be remote
	remoteName := util.GlobalOptions.Args[0]
	// Check valid remote
	if !core.IsGitRemote(remoteName) {
		util.LogConsoleError(remoteName, "is not a valid remote name")
		return 9
	}

	if len(util.GlobalOptions.Args) > 1 {
		// Remaining args are refs
		refs := util.GlobalOptions.Args[1:]
		var expandedrefs []string
		// expand refs
		shaRegex := regexp.MustCompile("^[A-Fa-f0-9]{40}$")
		for _, ref := range refs {
			if shaRegex.MatchString(ref) {
				// already a full sha
				expandedrefs = append(expandedrefs, ref)
			} else {
				expanded, err := core.GitRefToFullSHA(ref)
				if err != nil {
					util.LogConsoleErrorf("Invalid ref '%v': %v\n", ref, err.Error())
					return 12
				}
				expandedrefs = append(expandedrefs, expanded)
			}
		}

		// If all refs were ok, do it
		util.LogConsole("Marking", remoteName, "as pushed at", refs)

		for i, sha := range expandedrefs {
			err := core.MarkBinariesAsPushed(remoteName, sha, "")
			if err != nil {
				util.LogErrorf("Unable to mark %v as pushed at %v (%v): %v\n", remoteName, sha, refs[i], err.Error())
			} else {
				util.LogConsolef("Marked %v as pushed at %v (%v)\n", remoteName, sha, refs[i])
			}
		}
	} else {
		err := core.MarkAllBinariesPushed(remoteName)
		if err != nil {
			util.LogErrorf("Unable to mark %v as pushed: %v\n", remoteName, err.Error())
		} else {
			util.LogConsolef("Marked %v as pushed\n", remoteName)
		}
	}

	return 0

}

func MarkPushedHelp() {
	util.LogConsole(`Usage: git-lob mark-pushed [options] <remote> [<ref>...]

  Manually marks a commit as pushed for a remote (and all its ancestors)

  This is a low-level command which tells git-lob's remote state cache
  that as at a given commit, it should assume that all binaries have been
  pushed to the named remote. This means next time a 'git lob push' is 
  performed for that remote, history won't be scanned earlier than this
  commit. See HISTORY CHECKING in 'git lob push --help' for more details.

  The most common case of using this command would be directly after adding
  a secondary remote (e.g. a fork), assuming that the fork already has access
  to all the binaries you have locally. Without doing this, the first push
  of binaries to this fork will take much longer as it scans all history.

Parameters:
  <remote>: The name of the remote.

     <ref>: One or more refs or commit SHAs at which to record as pushed. 
            Any ancestors of this ref are also assumed to be pushed. 
            If you don't supply any refs, all refs are marked pushed (which
            is what you might do when adding a fork)

Options:
  --quiet, -q   Print less output
  --verbose, -v Print more output

`)

}

// Command line low-level tool to manually reset the pushed state of a remote
func ResetPushed() int {
	// git-lob reset-pushed <remote>

	// Validate custom options (none)
	errorList := validateCustomOptions(util.GlobalOptions, nil, []string{})
	if len(errorList) > 0 {
		util.LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	if len(util.GlobalOptions.Args) < 1 {
		util.LogConsoleError("Too few arguments; must supply remote name")
		return 9
	}
	// first parameter must be remote
	remoteName := util.GlobalOptions.Args[0]

	// Check valid remote
	if !core.IsGitRemote(remoteName) {
		util.LogConsoleError(remoteName, "is not a valid remote name")
		return 9
	}

	err := core.ResetPushedBinaryState(remoteName)
	if err != nil {
		util.LogError("Unable to reset pushed marker for", remoteName, ": ", err.Error())
		return 12
	} else {
		util.LogConsole("Successfully reset pushed markers for", remoteName)
	}

	return 0

}

func ResetPushedHelp() {
	util.LogConsole(`Usage: git-lob reset-pushed [options] <remote>

  Manually resets the cached pushed state for a remote

  This is a low-level command which tells git-lob's remote state cache
  to be cleared for a given remote. This means that the next 'git lob push'
  for that remote will do a full history scan and so will take longer.

  An alternative to this is to use the --recheck command on the next push.

Parameters:
  <remote>: The name of the remote.

Options:
  --quiet, -q   Print less output
  --verbose, -v Print more output

`)

}

// Command line low-level tool to report the last pushed ancestor of a ref
func LastPushed() int {
	// git-lob last-pushed <remote> <ref>

	// Validate custom options (none)
	errorList := validateCustomOptions(util.GlobalOptions, nil, []string{})
	if len(errorList) > 0 {
		util.LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	if len(util.GlobalOptions.Args) != 2 {
		util.LogConsoleError("Wrong number of arguments; must supply a remote name and a ref")
		return 9
	}
	// first parameter must be remote
	remoteName := util.GlobalOptions.Args[0]
	// Check valid remote
	if !core.IsGitRemote(remoteName) {
		util.LogConsoleError(remoteName, "is not a valid remote name")
		return 9
	}

	ref := util.GlobalOptions.Args[1]
	// Convert the ref into a SHA
	commitSHA, err := core.GitRefToFullSHA(ref)
	if err != nil {
		util.LogConsoleErrorf("Invalid ref: %v: %v\n", ref, err.Error())
		return 9
	}

	last, err := core.FindLatestAncestorWhereBinariesPushed(remoteName, commitSHA)
	if err != nil {
		util.LogErrorf("Unable to locate last pushed commit for %v at %v: %v\n", remoteName, ref, err.Error())
		return 12
	} else {
		if last == "" {
			util.LogConsolef("No ancestor of %v has been pushed to %v\n", ref, remoteName)
		} else {
			util.LogConsolef("Last ancestor of %v that has been pushed to %v: %v\n", ref, remoteName, last)
		}
	}

	return 0

}

func LastPushedHelp() {
	util.LogConsole(`Usage: git-lob last-pushed [options] <remote> <ref>

  Reports the most recent ancestor of <ref> where binaries are considered
  to have been fully pushed to <remote>

  git-lob stores a remote state cache so it doesn't have to check the whole
  history when pushing. This command reports where git-lob will look back to
  when asked to push to a given remote. See HISTORY CHECKING in the 
  'git lob push --help' output for more information.


Parameters:
  <remote>: The name of the remote.

     <ref>: The ref at which you want to start scanning backwards for the
            last push marker. 

Options:
  --quiet, -q   Print less output
  --verbose, -v Print more output

`)
}
