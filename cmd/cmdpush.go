package cmd

import (
	"bitbucket.org/sinbad/git-lob/core"
	"bitbucket.org/sinbad/git-lob/providers"
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Push command line tool
func Push() int {

	// git-lob push [--all] [--recheck] [--force] [<remote> [<ref>...]]

	// Validate custom options
	errorList := validateCustomOptions(util.GlobalOptions, nil, []string{"all", "a", "recheck", "r", "force", "f"})
	if len(errorList) > 0 {
		util.LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	optAll := util.GlobalOptions.BoolOpts.Contains("all") || util.GlobalOptions.BoolOpts.Contains("a")
	optRecheck := util.GlobalOptions.BoolOpts.Contains("recheck") || util.GlobalOptions.BoolOpts.Contains("r")
	optForce := util.GlobalOptions.BoolOpts.Contains("force") || util.GlobalOptions.BoolOpts.Contains("f")
	optDryRun := util.GlobalOptions.DryRun

	// Determine remote
	var remoteName string
	var refspecs []*core.GitRefSpec
	if len(util.GlobalOptions.Args) > 0 {
		// first parameter must be remote if there are arguments
		remoteName = util.GlobalOptions.Args[0]

		// Remaining args are refspecs
		if len(util.GlobalOptions.Args) > 1 {
			// Not valid if --all
			if optAll {
				util.LogConsoleError("git-lob: Too many arguments; cannot include refspec when using --all")
				return 7
			}
			for _, arg := range util.GlobalOptions.Args[1:] {
				r := core.ParseGitRefSpec(arg)
				// Only allow .. range for push, not ...
				if r.RangeOp == "..." {
					util.LogConsoleError("git-lob: '...' range operator is not supported for push, only '..'")
					return 7
				} else if r.IsRange() && r.IsEmptyRange() {
					util.LogConsoleErrorf("Warning: %v is an empty range, did you mean to use %v^..%v ?\n", r, r.Ref1, r.Ref2)
				}

				refspecs = append(refspecs, r)
			}
		}

	} else {
		remoteName = core.GetGitDefaultRemoteForPush()
	}

	// check the remote config to make sure it's valid
	provider, err := providers.GetProviderForRemote(remoteName)
	if err != nil {
		util.LogConsoleErrorf("git-lob: %v\n", err)
		return 6
	}
	if err = provider.ValidateConfig(remoteName); err != nil {
		util.LogConsoleErrorf("git-lob: remote %v has configuration problems:\n%v\n", remoteName, err)
		return 6
	}

	if len(refspecs) == 0 {
		// No refspecs specified, so determine default
		if optAll {
			branches, err := core.GetGitLocalBranches()
			if err != nil {
				util.LogErrorf("git-lob: unable to get local branch list - %v\n", err)
				return 7
			}
			for _, branch := range branches {
				refspecs = append(refspecs, &core.GitRefSpec{branch, "", ""})
			}

		} else {
			// Check remote.<remote>.push for default refspec
			def, ok := util.GlobalOptions.GitConfig[fmt.Sprintf("remote.%v.push", remoteName)]
			if ok {
				pushspecs := strings.Fields(def)
				for _, s := range pushspecs {
					refspecs = append(refspecs, &core.GitRefSpec{s, "", ""})
				}
			} else {
				// determine refspec from current branch & push settings
				branches := core.GetGitPushDefaultBranches(remoteName)
				for _, s := range branches {
					refspecs = append(refspecs, &core.GitRefSpec{s, "", ""})
				}
			}
		}
	}

	if len(refspecs) == 0 {
		util.LogConsole("No default refs to push based on config, current HEAD & tracking branches")
		util.LogConsole("Specify --all or a specific ref/branch to push something")
		return 0
	}

	util.LogConsole("Pushing binaries for", refspecs, "to", remoteName)

	// Warn about long calculation processes
	if optRecheck {
		util.LogConsole("Re-checking all history as requested, this may take a while on large repos")
	} else if !core.HasPushedBinaryState(remoteName) {
		util.LogConsole("No cached state for this remote, first time may take a while on large repos")
	}

	// Do the actual pushing in Goroutine, because we want to update the download rate & time estimates
	// on a regular schedule, regardless of whether any actual callbacks are received
	// If we only updated when callbacks happened (ie when data was transferred), if the data transfer halts
	// then we'd never update the rates / time estimates.

	var pusherr error

	// 100 items in the queue should be good enough, this means that it won't block
	callbackChan := make(chan *util.ProgressCallbackData, 100)
	go func(provider providers.SyncProvider, remoteName string, refspecs []*core.GitRefSpec, dryRun, force, recheck bool,
		progresschan chan<- *util.ProgressCallbackData) {

		// Progress callback just passes the result back to the channel
		progress := func(data *util.ProgressCallbackData) (abort bool) {
			progresschan <- data

			return false
		}

		err := core.Push(provider, remoteName, refspecs, dryRun, force, recheck, progress)

		close(progresschan)

		if err != nil {
			pusherr = err
		}

	}(provider, remoteName, refspecs, optDryRun, optForce, optRecheck, callbackChan)

	// Update the console once every half second regardless of how many callbacks
	// (or zero callbacks, so we can reduce xfer rate)
	pushCounts := util.ReportProgressToConsole(callbackChan, "Push", time.Millisecond*500)

	if pusherr != nil {
		util.LogErrorf("git-lob: push error(s):\n%v\n", pusherr.Error())
		return 12
	}
	if util.GlobalOptions.DryRun {
		util.LogConsole("Done, run again without --dry-run to perform real push")
	} else {
		// Because no newlines in progress reporting
		if pushCounts.ErrorCount > 0 {
			util.LogConsole("WARNING: non-fatal errors were encountered, not all data was pushed.")
		} else if pushCounts.NotFoundCount > 0 {
			util.LogConsole("WARNING: some binaries referred to by commits to push were not found locally")
			util.LogConsole("Push will re-try these next time.")
		} else {
			util.LogConsole("Successfully pushed binaries to", remoteName)
		}
	}
	provider.Release()

	return 0
}

// Low level push command line tool
func PushLob() int {

	// git-lob push-lob [--force] <remote> <sha>...

	// Validate custom options
	errorList := validateCustomOptions(util.GlobalOptions, nil, []string{"force", "f"})
	if len(errorList) > 0 {
		util.LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	if len(util.GlobalOptions.Args) < 2 {
		util.LogConsoleError("Too few arguments; must supply remote and at least one SHA")
		return 9
	}

	optForce := util.GlobalOptions.BoolOpts.Contains("force") || util.GlobalOptions.BoolOpts.Contains("f")

	// first parameter must be remote
	remoteName := util.GlobalOptions.Args[0]

	// check the remote config to make sure it's valid
	provider, err := providers.GetProviderForRemote(remoteName)
	if err != nil {
		util.LogConsoleError(err.Error())
		return 6
	}
	if err = provider.ValidateConfig(remoteName); err != nil {
		util.LogConsoleErrorf("Remote %v has configuration problems:\n%v\n", remoteName, err)
		return 6
	}

	// Remaining args are SHAs
	shas := util.GlobalOptions.Args[1:]
	// Validate that they are SHAs
	shaRegex := regexp.MustCompile("^[A-Fa-f0-9]{40}$")
	for _, sha := range shas {
		if !shaRegex.MatchString(sha) {
			util.LogConsoleErrorf("Invalid SHA: %v\n", sha)
			return 9
		}
	}

	// Do the actual pushing in Goroutine, because we want to update the download rate & time estimates
	// on a regular schedule, regardless of whether any actual callbacks are received
	// If we only updated when callbacks happened (ie when data was transferred), if the data transfer halts
	// then we'd never update the rates / time estimates.

	var pusherr error

	// 100 items in the queue should be good enough, this means that it won't block
	callbackChan := make(chan *util.ProgressCallbackData, 100)
	go func(provider providers.SyncProvider, remoteName string, shas []string, force bool,
		progresschan chan<- *util.ProgressCallbackData) {

		// Progress callback just passes the result back to the channel
		progress := func(data *util.ProgressCallbackData) (abort bool) {
			progresschan <- data

			return false
		}

		var err error
		for _, sha := range shas {
			err = core.PushSingle(sha, provider, remoteName, force, progress)
			if err != nil {
				break
			}
		}

		close(progresschan)

		if err != nil {
			pusherr = err
		}

	}(provider, remoteName, shas, optForce, callbackChan)

	util.LogConsole("Pushing binaries to", remoteName)

	// Update the console once every half second regardless of how many callbacks
	// (or zero callbacks, so we can reduce xfer rate)
	pushCounts := util.ReportProgressToConsole(callbackChan, "Push", time.Millisecond*500)

	if pusherr != nil {
		util.LogErrorf("git-lob: push error(s):\n%v\n", pusherr.Error())
		return 12
	}
	if pushCounts.ErrorCount > 0 {
		util.LogConsole("WARNING: non-fatal errors were encountered, not all data was pushed.")
	} else if pushCounts.NotFoundCount > 0 {
		util.LogConsole("WARNING: some binaries referred to by commits to push were not found locally")
	} else {
		util.LogConsole("Successfully pushed binaries to", remoteName)
	}

	provider.Release()

	return 0
}

func PushHelp() {
	util.LogConsole(`Usage: git-lob push [options] [<remote> [<ref>...]]

  Uploads binaries to a remote, sending only binaries required to ensure 
  that remote has the binary resources referenced at a set of commits.

  Behaves much like 'git push' except there are no destination refs, only
  supporting binary files.

Parameters:
  <remote>: The destination to upload to. This should correspond to the 
            name of a remote (no direct URLs permitted) which is configured
            in .git/config. See REMOTES below for more details, additional
            config parameters are required in the remote.

            If no remote is specified, branch.*.remote configuration for the
            current branch is consulted to determine where to push. If the 
            configuration is missing, it defaults to origin.
     <ref>: Which local reference(s) up to which we should make sure binaries
            are uploaded for. You can specify zero, one, or many local refs.
            There is no destination ref as in git push.

            If no ref is specified, and --all is not used, then 
            remote.<remotename>.push is used if present, otherwise push.default
            is checked (matching, simple, current) to determine branches to 
            push by default.

            COMMIT RANGES

            You can also specify a range of refs in the form <ref1>..<ref2> to
            force git-lob push to check a specific range of commits for
            binaries, instead of using its own records of which commits it
            thinks are already up to date on this remote. 
            See HISTORY CHECKING below.


Options:
  --all, -a     Push all branches; cannot be used with other refs.
  --recheck, -r Re-check entire commit history to each ref instead of only 
                back to last commit we believe is already pushed. 
                See HISTORY CHECKING below for more details.
  --force, -f   Always upload files even if the provider believes the file is 
                already present on the remote. You shouldn't need this.
  --quiet, -q   Print less output
  --verbose, -v Print more output
  --dry-run     Don't actually push anything, just report

HISTORY CHECKING

When pushing binaries for a given ref, git-lob performs a search for commits
which reference git-lob binaries from that ref backwards, before checking
which of those binaries it needs to upload. This is so that we only upload
binaries that are actually referenced by the ref you're choosing to push, 
and don't waste time on binaries in unpublished feature branches etc. 

Because searching the whole of the git history can be slow on large 
repositories, git-lob speeds this search up by keeping a record of which 
commits it believes the remote already has all binaries for. 

These records are updated whenever you git-lob push/pull. We do not use git's
own remote branch refs to track this, because pushing commits can be done
completely separately from binaries so we can't rely on that information.
So pushing and pulling branches in git has no effect on this state, only
git-lob push/pull.

If for some reason these records are wrong, and you need to push binaries
for a bigger range of commits, you can do this 2 ways:

1. Use the --recheck option. This is the 'nuclear option'; it will scan the
   entire history of the repo again to make 100% sure everything is correct.
   Can take a while on large repos.

2. Use a commit range for <ref>, i.e. <ref1>..<ref2>. git-lob will check that
   entire range of commits for binary references which will then be checked
   with the remote. 

There are not many circumstances where you need to manually override the commit
range that is checked for binaries. Even if you edit commits, rebase etc, 
git-lob should not miss any binaries, because the commit SHAs would change and
it would know to check any referenced binaries again. The main reason why
you would need to override the history checking is if the remote changed, for
example if someone manually deleted the remote binary store, or you moved to 
a new URL without copying the data and needed to re-populate it from your local
repo.

REMOTES
  Type 'git lob help remotes' for details

`)
}
func PushLobHelp() {
	util.LogConsole(`Usage: git-lob push-lob [options] <remote> <sha>...

  Uploads one or more specific binaries to a remote, identified by shas.

  This is a low-level alternative to the main push command, allowing
  you to manually upload a specific binary identified by its SHA.
  Files already on the remote are still skipped unless you use --force.

  Does not check or update the remote state cache recording what we
  think has already been pushed to this remote.

Parameters:
  <remote>: The destination to upload to. This should correspond to the 
            name of a remote (no direct URLs permitted) which is configured
            in .git/config. See REMOTES below for more details, additional
            config parameters are required in the remote.

     <sha>: One or more 40-character SHAs identifying a binary. Note this is
            the SHA of the binary, not of a git commit object. If you want
            to push binaries for a commit, use regular 'git lob push'

Options:
  --force, -f   Always upload files even if the provider believes the file is 
                already present on the remote. You shouldn't need this.
  --quiet, -q   Print less output
  --verbose, -v Print more output

REMOTES
  Type 'git lob help remotes' for details

`)
}
