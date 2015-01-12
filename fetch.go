package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Fetch command line tool
func cmdFetch() int {

	// git-lob fetch [--all] [--prune] [--force] [<remote> [<ref>...]]

	// Validate custom options
	errorList := validateCustomOptions(GlobalOptions, nil, []string{"prune", "force"})
	if len(errorList) > 0 {
		fmt.Fprintf(os.Stderr, strings.Join(errorList, "\n"))
		return 9
	}

	optPrune := GlobalOptions.BoolOpts.Contains("prune")
	optForce := GlobalOptions.BoolOpts.Contains("force")
	optDryRun := GlobalOptions.DryRun

	// Determine remote
	var remoteName string
	// Ordered list of the commmits we're going to ls-tree to find binaries
	var refspecs []*GitRefSpec

	if len(GlobalOptions.Args) > 0 {
		// first parameter must be remote if there are arguments
		remoteName = GlobalOptions.Args[0]

		// Remaining args are refspecs
		if len(GlobalOptions.Args) > 1 {
			for _, arg := range GlobalOptions.Args[1:] {
				r := ParseGitRefSpec(arg)

				// Only allow .. range for fetch, not ...
				if r.RangeOp == "..." {
					fmt.Fprintf(os.Stderr, "git-lob: '...' range operator is not supported for fetch, only '..'\n")
					return 7
				} else if r.IsRange() && r.IsEmptyRange() {
					fmt.Fprintf(os.Stderr, "Warning: %v is an empty range, did you mean to use %v^..%v ?\n", r, r.Ref1, r.Ref2)
				}

				refspecs = append(refspecs, r)
			}
		}

	} else {
		remoteName = GetGitDefaultRemoteForPull()
	}

	if !GlobalOptions.Quiet {
		if len(refspecs) > 0 {
			fmt.Println("Fetching binaries for", refspecs, "from", remoteName)
		} else {
			fmt.Println("Fetching recent binaries from", remoteName)
		}
	}

	// Do the actual fetching in a Goroutine, because we want to update the download rate & time estimates
	// on a regular schedule, regardless of whether any actual callbacks are received
	// If we only updated when callbacks happened (ie when data was transferred), if the data transfer halts
	// then we'd never update the rates / time estimates.

	var fetcherr error

	// 100 items in the queue should be good enough, this means that it won't block
	callbackChan := make(chan *ProgressCallbackData, 100)
	go func(provider SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, recheck bool,
		progresschan chan<- *ProgressCallbackData) {

		// Progress callback just passes the result back to the channel
		progress := func(data *ProgressCallbackData) (abort bool) {
			progresschan <- data

			return false
		}

		err := Fetch(provider, remoteName, refspecs, dryRun, force, recheck, progress)

		close(progresschan)

		if err != nil {
			fetcherr = err
		}

	}(provider, remoteName, refspecs, optDryRun, optForce, optPrune, callbackChan)

	// Report progress on operation every 0.5s
	ReportProgressToConsole(callbackChan, "Fetch", time.Millisecond*500)

	if fetcherr != nil {
		fmt.Fprintf(os.Stderr, "git-lob: fetch error - %v", err.Error())
		return 12
	}
	if !GlobalOptions.Quiet {
		if GlobalOptions.DryRun {
			fmt.Println("Done, run again without --dry-run to perform real fetch")
		} else {
			// Because no newlines in progress reporting
			fmt.Println()
			fmt.Println("Successfully fetched binaries to", remoteName)
		}
	}

	return 0
}

// Implementation of fetch
func Fetch(provider SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, prune bool,
	callback ProgressCallback) error {
	// We need to build a list of commits ranges at which we want to ensure binaries are present locally
	// We can't build the list of binaries solely from the log, because  not all binaries needed may have been
	// modified in the range. Therefore we need 'git ls-tree' at the base ancestor in each range, followed by
	// a 'git log -G' query for subsequent LOB changes. This is faster than doing ls-tree for every individual commit
	// and eliminating duplicates.

	if len(refspecs) == 0 {
		// Append HEAD commits first
		headrefspec, err := GetGitRecentCommitRange("HEAD", GlobalOptions.RecentCommitsPeriodHEAD)
		if err != nil {
			return errors.New(fmt.Sprintf("Error determining recent HEAD commits: %v", err.Error()))
		}
		// Find recent other refs
		recentrefs, err := GetGitRecentRefs(GlobalOptions.RecentRefsPeriodDays)
		if err != nil {
			return errors.New(fmt.Sprintf("Error determining recent refs: %v", err.Error()))
		}
		refspecs = append(refspecs, headrefspec)
		// Now each other ref, they should be in reverse date order from GetGitRecentRefs so we're doing
		// things by priority, HEAD first then most recent
		headSHA, _ := GitRefToFullSHA("HEAD")
		for _, ref := range recentrefs {
			// Don't duplicate HEAD commit though
			if ref == headSHA {
				continue
			}
			recentrefspec, err := GetGitRecentCommitRange(ref, GlobalOptions.RecentCommitsPeriodOther)
			if err != nil {
				return errors.New(fmt.Sprintf("Error determining recent commits on %v: %v", ref, err.Error()))
			}
			refspecs = append(refspecs, recentrefspec)
		}
	}

	if len(refspecs) > 0 {
		// OK, now we have a list of commit refspecs, which may be ranges, where we want to have binaries for
		// all of the commits within them.
		// For a single commit it's just an ls-tree, for a range it's an ls-tree on the start commit and
		// a git log for changes in the range.

		// TODO

		// WAIT WAIT WAIT!!
		// Could I instead use ls-tree on the END of the range, then use git log with a date range and -G at the same time?
		// thus cutting out an extra git log (would need one to identify start commit and another for the -G)
		// Diffs would be backwards, we'd have to look for minus entries instead?

	}

	return nil

}

func cmdFetchHelp() {
	fmt.Println(`Usage: git-lob fetch [options] [<remote> [<ref>...]]

  Download binaries from a remote, retrieving only the binaries referenced
  by recent commits visible in the git repo. The definition of 'recent' depends
  on whether you specify a list of references to fetch or not, and some
  configuration parameters. See RECENT COMMITS below.

  These files are stored in your local binary store (shared store if 
  configured) ready to be checked out into your working copy either with
  'git checkout', or 'git-lob pull'.

Parameters:
  <remote>: The remote to download from. This should correspond to the 
            name of a remote (no direct URLs permitted) which is configured
            in .git/config. See REMOTES below for more details, additional
            config parameters are required in the remote.

            If no remote is specified, the remote that your current branch 
            is tracking will be used, or origin if tracking is not configured.
     <ref>: Which reference(s) we should make sure binaries downloaded for. 
            You can specify zero, one, or many refs, but these refs must be
            present locally (ie already fetched with plain git). git-lob
            does not perform the 'git fetch' for you and can only look at
            commits already present in your repo (but they don't have to be
            checked out or even in local branches).

            If refs are specified only binaries needed to fulfil references in
            those commits will be downloaded. For example, use HEAD to 
            download only the binaries required to complete the current
            checkout. You can use ranges (<ref1>..<ref2>) to download all 
            in a range of commits.

            If no ref is specified, git-lob will download binaries for 'recent' 
            commits, starting with the current HEAD but also recent branches.
            See RECENT COMMITS below. 

Options:
  --force       Always download files even if the provider believes the file is 
                already present locally. 
  --prune       As well as downloading files referenced by 'recent' commits, 
                delete any local files you already have which now fall outside
                this definition of 'recent'. See RECENT COMMITS below.
  --quiet, -q   Print less output
  --verbose, -v Print more output
  --dry-run     Don't actually delete anything, just report

REMOTES

Binaries in git-lob are not stored in the regular git repo, but a corresponding
binary store must always exist to provide the actual binary content. A remote
in git usually only gives you the real git repo, so git-lob needs to expand
the configuration parameters to git remotes to specify the location of the 
corresponding remote binary store. 

The <remote> parameter refers to a named remote in .git/config (plain URLs 
cannot be supported). The remote entry itself is the same as any normal git
remote, except that it requires additional git-lob specific parameters. The
parameters depend on the type of binary storage ('provider') being used; see
'git-lob providers' for a list of available providers and 
'git-lob provider <provider>' for specific details of one provider.

As an example, let's take the 'filesystem' provider, which simply uses the OS's
file system as a remote transport (obviously very simplistic):

[remote "origin"]
    # these 2 lines are standard git
    url = ssh://git@bitbucket.org/something/somthing.git
    fetch = +refs/heads/*:refs/remotes/origin/*
    # these next 2 lines are required to configure the remote binary store
    git-lob-provider = filesystem
    git-lob-path = /Volumes/shared/something/something/binary/store
    
Other providers may require other parameters. It's important to note that you
can share a binary store among multiple remote repos if you wish, much like
the local git-lob.sharedstore option, since binaries are stored by SHA. 
Identical file content in multiple repos can be stored only once this way.
Of course, access control may be an issue to consider here though.

RECENT COMMITS

If no refs are specified on the command line, git-lob will fetch binaries 
referenced by 'recent commits'. There are user parameters which can control
how this behaves, see CONFIG below.

Recent commits means:
  * The current HEAD, plus
  * Any ancestors of HEAD within git-lob.recent_commits_head days of its last
    commit date
  * Any branches (local and remote) or tags which have a commit within
    git-lob.recent_refs days of the current date
  * Any ancestors of those branches/tags within git-lob.recent_commits_other
    days of its last commit date

CONFIG

There are a few user config settings specific to the fetch command which can
be in ~/.gitconfig or $REPO/.git/config.

  git-lob.recent-refs          default: 90 days
  git-lob.recent-commits-head  default: 30 days
  git-lob.recent-commits-other default: 0 days

These 3 settings are used to control the meaning of 'recent commits', see
RECENT COMMITS above.

  git-lob.includepaths
  git-lob.excludepaths

These 2 settings you probably only want to define at the repo level. 
includepaths limits the binaries downloaded to only matching paths, while
excludepaths downloads everything except files matching these paths.
The contents of each are comma-separated paths with wildcard matching.

`)
}
