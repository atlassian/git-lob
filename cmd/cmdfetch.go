package cmd

import (
	"bitbucket.org/sinbad/git-lob/core"
	"bitbucket.org/sinbad/git-lob/providers"
	"bitbucket.org/sinbad/git-lob/util"
	"regexp"
	"strings"
	"time"
)

// Fetch command line tool
func Fetch() int {

	// git-lob fetch [--prune] [--force] [<remote> [<ref>...]]

	// Validate custom options
	errorList := validateCustomOptions(util.GlobalOptions, nil, []string{"prune", "force"})
	if len(errorList) > 0 {
		util.LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	optPrune := util.GlobalOptions.BoolOpts.Contains("prune")
	optForce := util.GlobalOptions.BoolOpts.Contains("force")
	optDryRun := util.GlobalOptions.DryRun

	// Determine remote
	var remoteName string
	// Ordered list of the commmits we're going to ls-tree to find binaries
	var refspecs []*core.GitRefSpec

	if len(util.GlobalOptions.Args) > 0 {
		// first parameter must be remote if there are arguments
		remoteName = util.GlobalOptions.Args[0]

		// Remaining args are refspecs
		if len(util.GlobalOptions.Args) > 1 {
			for _, arg := range util.GlobalOptions.Args[1:] {
				r := core.ParseGitRefSpec(arg)

				// Only allow .. range for fetch, not ...
				if r.RangeOp == "..." {
					util.LogConsoleError("git-lob: '...' range operator is not supported for fetch, only '..'")
					return 7
				} else if r.IsRange() && r.IsEmptyRange() {
					util.LogConsoleErrorf("Warning: %v is an empty range, did you mean to use %v^..%v ?\n", r, r.Ref1, r.Ref2)
				}

				refspecs = append(refspecs, r)
			}
		}

	} else {
		remoteName = core.GetGitDefaultRemoteForPull()
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

	if len(refspecs) > 0 {
		util.LogConsole("Fetching binaries for", refspecs, "from", remoteName)
	} else {
		util.LogConsole("Fetching recent binaries from", remoteName)
	}

	// Do the actual fetching in a Goroutine, because we want to update the download rate & time estimates
	// on a regular schedule, regardless of whether any actual callbacks are received
	// If we only updated when callbacks happened (ie when data was transferred), if the data transfer halts
	// then we'd never update the rates / time estimates.

	var fetcherr error

	// 100 items in the queue should be good enough, this means that it won't block
	callbackChan := make(chan *util.ProgressCallbackData, 100)
	go func(provider providers.SyncProvider, remoteName string, refspecs []*core.GitRefSpec, dryRun, force bool,
		progresschan chan<- *util.ProgressCallbackData) {

		// Progress callback just passes the result back to the channel
		progress := func(data *util.ProgressCallbackData) (abort bool) {
			progresschan <- data

			return false
		}

		err := core.Fetch(provider, remoteName, refspecs, dryRun, force, progress)

		close(progresschan)

		if err != nil {
			fetcherr = err
		}

	}(provider, remoteName, refspecs, optDryRun, optForce, callbackChan)

	// Report progress on operation every 0.5s
	fetchCounts := util.ReportProgressToConsole(callbackChan, "Fetch", time.Millisecond*500)

	if fetcherr != nil {
		util.LogError("git-lob: fetch error(s):\n%v", fetcherr.Error())
		return 12
	}

	if optPrune && !optDryRun {
		util.LogConsole("Performing post-fetch prune...")
		PostFetchPullPrune()
	}

	if util.GlobalOptions.DryRun {
		util.LogConsole("Done, run again without --dry-run to perform real fetch")
	} else {
		// Because no newlines in progress reporting
		// Warn if anything wasn't found or non-fatal errors
		if fetchCounts.ErrorCount > 0 {
			util.LogConsole("WARNING: non-fatal errors were encountered, not all data was retrieved.")
		} else if fetchCounts.NotFoundCount > 0 {
			util.LogConsole("WARNING: some requested data was not available on remote", remoteName)
		} else {
			util.LogConsole("Successfully fetched binaries from", remoteName)
		}

	}

	return 0
}

// Low-level LOB fetch command
func FetchLob() int {

	// git-lob fetch-lob [--force] <remote> <sha>...

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

	// Determine remote
	var remoteName string

	// first parameter must be remote
	remoteName = util.GlobalOptions.Args[0]

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

	util.LogConsole("Fetching binaries from", remoteName)

	// Do the actual fetching in a Goroutine, because we want to update the download rate & time estimates
	// on a regular schedule, regardless of whether any actual callbacks are received
	// If we only updated when callbacks happened (ie when data was transferred), if the data transfer halts
	// then we'd never update the rates / time estimates.

	var fetcherr error

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
			err = core.FetchSingle(sha, provider, remoteName, force, progress)
			if err != nil {
				break
			}
		}

		close(progresschan)

		if err != nil {
			fetcherr = err
		}

	}(provider, remoteName, shas, optForce, callbackChan)

	// Report progress on operation every 0.5s
	fetchCounts := util.ReportProgressToConsole(callbackChan, "Fetch", time.Millisecond*500)

	if fetcherr != nil {
		util.LogError("git-lob: fetch error(s):\n%v", fetcherr.Error())
		return 12
	}

	// Warn if anything wasn't found or non-fatal errors
	if fetchCounts.ErrorCount > 0 {
		util.LogConsole("WARNING: non-fatal errors were encountered, not all data was retrieved.")
	} else if fetchCounts.NotFoundCount > 0 {
		util.LogConsole("WARNING: some requested data was not available on remote", remoteName)
	} else {
		util.LogConsole("Successfully fetched binaries from", remoteName)
	}

	return 0
}

func FetchHelp() {
	util.LogConsole(`Usage: git-lob fetch [options] [<remote> [<ref>...]]

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
  --dry-run     Don't actually download anything, just report

RECENT COMMITS

If no refs are specified on the command line, git-lob will fetch binaries 
referenced by 'recent commits'. There are user parameters which can control
how this behaves, see CONFIG below.

Recent commits means:
  * The current HEAD, plus
  * Any ancestors of HEAD within git-lob.fetch-commits-head days of its last
    commit date
  * Any branches (local and remote) or tags which have a commit within
    git-lob.fetch-refs days of the current date
  * Any ancestors of those branches/tags within git-lob.fetch-commits-other
    days of its last commit date

REMOTES
  Type 'git lob help remotes' for details

CONFIG
  Type 'git lob help config' for details, see the 'fetch' section
`)
}
func FetchLobHelp() {
	util.LogConsole(`Usage: git-lob fetch-lob [options] <remote> <sha>...

  Download a one or more binaries from a named remote.

  This is a low-level alternative to the main fetch command, allowing
  you to manually download a specific binary identified by its SHA. Files
  already on the remote are still skipped unless you use --force.

  These files are stored in your local binary store (shared store if 
  configured) ready to be checked out into your working copy either with
  'git checkout', or 'git-lob pull'.

Parameters:
  <remote>: The remote to download from. This should correspond to the 
            name of a remote (no direct URLs permitted) which is configured
            in .git/config. See REMOTES below for more details, additional
            config parameters are required in the remote.

     <sha>: One or more 40-character SHAs identifying a binary. Note this is
            the SHA of the binary, not of a git commit object. If you want
            to fetch binaries for a commit, use regular 'git lob fetch'

Options:
  --force, -f   Always download files even if the provider believes the file is 
                already present locally. 
  --quiet, -q   Print less output
  --verbose, -v Print more output

REMOTES
  Type 'git lob help remotes' for details
`)
}
