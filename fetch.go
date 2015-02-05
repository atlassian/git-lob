package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Fetch command line tool
func cmdFetch() int {

	// git-lob fetch [--prune] [--force] [<remote> [<ref>...]]

	// Validate custom options
	errorList := validateCustomOptions(GlobalOptions, nil, []string{"prune", "force"})
	if len(errorList) > 0 {
		LogConsoleError(strings.Join(errorList, "\n"))
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
					LogConsoleError("git-lob: '...' range operator is not supported for fetch, only '..'")
					return 7
				} else if r.IsRange() && r.IsEmptyRange() {
					LogConsoleErrorf("Warning: %v is an empty range, did you mean to use %v^..%v ?\n", r, r.Ref1, r.Ref2)
				}

				refspecs = append(refspecs, r)
			}
		}

	} else {
		remoteName = GetGitDefaultRemoteForPull()
	}

	// check the remote config to make sure it's valid
	provider, err := GetProviderForRemote(remoteName)
	if err != nil {
		LogConsoleErrorf("git-lob: %v\n", err)
		return 6
	}
	if err = provider.ValidateConfig(remoteName); err != nil {
		LogConsoleErrorf("git-lob: remote %v has configuration problems:\n%v\n", remoteName, err)
		return 6
	}

	if len(refspecs) > 0 {
		LogConsole("Fetching binaries for", refspecs, "from", remoteName)
	} else {
		LogConsole("Fetching recent binaries from", remoteName)
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
	fetchCounts := ReportProgressToConsole(callbackChan, "Fetch", time.Millisecond*500)

	if fetcherr != nil {
		LogError("git-lob: fetch error(s):\n%v", fetcherr.Error())
		return 12
	}
	if GlobalOptions.DryRun {
		LogConsole("Done, run again without --dry-run to perform real fetch")
	} else {
		// Because no newlines in progress reporting
		// Warn if anything wasn't found or non-fatal errors
		if fetchCounts.ErrorCount > 0 {
			LogConsole("WARNING: non-fatal errors were encountered, not all data was retrieved.")
		} else if fetchCounts.NotFoundCount > 0 {
			LogConsole("WARNING: some requested data was not available on remote", remoteName)
		} else {
			LogConsole("Successfully fetched binaries from", remoteName)
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

	LogDebugf("Fetching from %v via %v\n", remoteName, provider.TypeID())

	var lobsNeeded []string

	if len(refspecs) == 0 {
		// No refs specified, use 'Recent' fetch algorithm
		if GlobalOptions.Verbose {
			callback(&ProgressCallbackData{ProgressCalculate, "Calculating recent commits...",
				int64(0), int64(1), 0, 0})
		}
		// Get HEAD LOBs first
		headlobs, err := GetGitAllLOBsToCheckoutAtCommitAndRecent("HEAD", GlobalOptions.RecentCommitsPeriodHEAD,
			GlobalOptions.FetchIncludePaths, GlobalOptions.FetchExcludePaths)
		if err != nil {
			return errors.New(fmt.Sprintf("Error determining recent HEAD commits: %v", err.Error()))
		}
		if GlobalOptions.Verbose {
			callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf(" * HEAD: %d binary references", len(headlobs)),
				0, 0, 0, 0})
		}
		lobsNeeded = headlobs
		if GlobalOptions.RecentRefsPeriodDays > 0 {
			// Find recent other refs (only include remote branches for this remote)
			recentrefs, err := GetGitRecentRefs(GlobalOptions.RecentRefsPeriodDays, true, remoteName)
			if err != nil {
				return errors.New(fmt.Sprintf("Error determining recent refs: %v", err.Error()))
			}
			// Now each other ref, they should be in reverse date order from GetGitRecentRefs so we're doing
			// things by priority, HEAD first then most recent
			headSHA, _ := GitRefToFullSHA("HEAD")
			for i, ref := range recentrefs {
				// Don't duplicate HEAD commit though
				if ref == headSHA {
					continue
				}
				recentreflobs, err := GetGitAllLOBsToCheckoutAtCommitAndRecent(ref, GlobalOptions.RecentCommitsPeriodOther,
					GlobalOptions.FetchIncludePaths, GlobalOptions.FetchExcludePaths)
				if err != nil {
					return errors.New(fmt.Sprintf("Error determining recent commits on %v: %v", ref, err.Error()))
				}
				if GlobalOptions.Verbose {
					callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf(" * %v: %d binary references", ref, len(recentreflobs)),
						int64(i), int64(len(refspecs)), 0, 0})
				}
				lobsNeeded = append(lobsNeeded, recentreflobs...)
			}
		}
	} else {
		// Get LOBs directly from specified refs/ranges
		for i, refspec := range refspecs {
			if GlobalOptions.Verbose {
				callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf("Calculating data to fetch for %v", refspec),
					int64(i), int64(len(refspecs)), 0, 0})
			}
			refshas, err := GetGitAllLOBsToCheckoutInRefSpec(refspec, GlobalOptions.FetchIncludePaths, GlobalOptions.FetchExcludePaths)
			if err != nil {
				return errors.New(fmt.Sprintf("Error determining LOBs to fetch for %v: %v", refspec, err.Error()))
			}
			if GlobalOptions.Verbose {
				callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf(" * %v: %d binary references", refspec, len(refspecs)),
					int64(i), int64(len(refspecs)), 0, 0})
			}
			lobsNeeded = append(lobsNeeded, refshas...)
		}
	}

	if len(lobsNeeded) == 0 {
		callback(&ProgressCallbackData{ProgressCalculate, "No binaries to download.",
			int64(len(refspecs)), int64(len(refspecs)), 0, 0})
	} else {

		// Duplicates are not eliminated by methods we call, for efficiency
		// We need to remove them though because otherwise we can report much higher download requirements
		// than necessary when multiple refs include the same SHA
		StringRemoveDuplicates(&lobsNeeded)

		var lobsToDownload []string
		if force {
			// Just download all
			lobsToDownload = lobsNeeded
		} else {
			lobsToDownload = GetMissingLOBs(lobsNeeded, false)
		}

		if len(lobsToDownload) == 0 {
			callback(&ProgressCallbackData{ProgressCalculate, "No binaries to download.",
				int64(len(refspecs)), int64(len(refspecs)), 0, 0})
			return nil
		} else {
			callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf("%d binaries to download.", len(lobsToDownload)),
				int64(len(refspecs)), int64(len(refspecs)), 0, 0})
		}
		if !dryRun {

			err := fetchLOBs(lobsToDownload, provider, remoteName, force, callback)
			if err != nil {
				return err
			}

		}
	}

	LogDebugf("Successfully fetched from %v via %v\n", remoteName, provider.TypeID())

	return nil

}

// Internal method for fetching
func fetchLOBs(lobshas []string, provider SyncProvider, remoteName string, force bool, callback ProgressCallback) error {
	// Download metafiles first
	// This will allow us to estimate the time required
	callback(&ProgressCallbackData{ProgressCalculate, "Downloading metadata",
		0, 0, 0, 0})
	err := fetchMetadata(lobshas, provider, remoteName, force, callback)
	if err != nil {
		return err
	}

	// So now we have all the metadata available locally, we can know what files to download
	var filesTotalBytes int64
	var files []string
	callback(&ProgressCallbackData{ProgressCalculate, "Calculating content files to download",
		0, 0, 0, 0})
	for _, sha := range lobshas {
		info, err := GetLOBInfo(sha)
		if err != nil {
			// If we could not get the lob data, it means that we could not download the meta file
			// it's OK for this to happen since the remote may not have all the data we need (e.g.
			// local branch with changes that actually came from somewhere else, or remote hasn't been
			// fully updated yet)
			// We notified earlier
			continue
		}
		filesTotalBytes += info.Size
		for i := 0; i < info.NumChunks; i++ {
			// get relative filename for download purposes
			files = append(files, getLOBChunkRelativePath(sha, i))
		}
	}
	callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf("Metadata done, downloading content (%v)", FormatSize(filesTotalBytes)),
		0, 0, 0, 0})

	// Download content now
	return fetchContentFiles(files, filesTotalBytes, provider, remoteName, force, callback)

}

// Internal method for fetching
func fetchMetadata(lobshas []string, provider SyncProvider, remoteName string, force bool, callback ProgressCallback) error {
	// Use average metafile bytes as estimate of download, usually around 100 bytes of JSON
	averageMetaSize := 100
	metaTotalBytes := int64(len(lobshas) * averageMetaSize)
	var metafilesDone int
	metacallback := func(fileInProgress string, progressType ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
		// Don't bother to track partial completion, only 100 bytes each
		if progressType == ProgressSkip || progressType == ProgressNotFound {
			metafilesDone++
			callback(&ProgressCallbackData{progressType, fileInProgress, totalBytes, totalBytes,
				int64(metafilesDone * averageMetaSize), metaTotalBytes})
			// Remote did not have this file
		} else {
			if bytesDone == totalBytes {
				// finished
				metafilesDone++
				callback(&ProgressCallbackData{ProgressTransferBytes, fileInProgress, totalBytes, totalBytes,
					int64(metafilesDone * averageMetaSize), metaTotalBytes})
			}
		}
		return false
	}
	// Download all meta files
	var metafilesToDownload []string
	for _, sha := range lobshas {
		// Note get relative file name
		metafilesToDownload = append(metafilesToDownload, getLOBMetaRelativePath(sha))
	}
	// Download to shared if using shared area (we link later)
	destDir := getFetchDestination()
	err := provider.Download(remoteName, metafilesToDownload, destDir, force, metacallback)

	// If shared store, link any metadata we downloaded into local
	if isUsingSharedStorage() {
		for _, sha := range lobshas {
			// filenames are relative (for download)
			localfile := getLocalLOBMetaPath(sha)
			sharedfile := getSharedLOBMetaPath(sha)
			if (force || !FileExists(localfile)) && FileExists(sharedfile) {
				linkerr := linkSharedLOBFilename(sharedfile)
				if linkerr != nil {
					// we want to continue so don't return this
					LogErrorf("Failed to link shared file %v into local repo: %v\n", sharedfile, linkerr.Error())
				}
			}
		}
	}
	// Deal with errors afterwards so we linked partial successes
	if err != nil {
		return err
	}

	return nil
}

func getFetchDestination() string {
	// Download to shared if using shared area (we link later)
	if isUsingSharedStorage() {
		return GetSharedLOBRoot()
	} else {
		return GetLocalLOBRoot()
	}
}

func fetchContentFiles(files []string, filesTotalBytes int64, provider SyncProvider,
	remoteName string, force bool, callback ProgressCallback) error {
	var lastFilename string
	var lastFileBytes int64
	var bytesFromFilesDoneSoFar int64
	contentcallback := func(fileInProgress string, progressType ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {

		var ret bool
		if progressType == ProgressSkip || progressType == ProgressNotFound {
			bytesFromFilesDoneSoFar += totalBytes
			ret = callback(&ProgressCallbackData{progressType, fileInProgress, totalBytes, totalBytes,
				bytesFromFilesDoneSoFar, filesTotalBytes})
		} else {

			if lastFilename != fileInProgress && lastFilename != "" {
				// we obviously never got a 100% call for previous file
				bytesFromFilesDoneSoFar += lastFileBytes
				ret = callback(&ProgressCallbackData{ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
					bytesFromFilesDoneSoFar, filesTotalBytes})
				lastFilename = ""
			}

			if bytesDone == totalBytes {
				// finished
				bytesFromFilesDoneSoFar += totalBytes
				ret = callback(&ProgressCallbackData{ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesFromFilesDoneSoFar, filesTotalBytes})
				lastFilename = ""
			} else {
				// partly progressed file
				lastFilename = fileInProgress
				lastFileBytes = totalBytes
				ret = callback(&ProgressCallbackData{ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesFromFilesDoneSoFar + bytesDone, filesTotalBytes})

			}

		}

		return ret
	}
	destDir := getFetchDestination()
	err := provider.Download(remoteName, files, destDir, force, contentcallback)
	// Also if shared store, link meta into local
	// Link any we successfully downloaded
	if isUsingSharedStorage() {
		localroot := GetLocalLOBRoot()
		sharedroot := GetSharedLOBRoot()
		for _, relfile := range files {
			// filenames are relative (for download)
			localfile := filepath.Join(localroot, relfile)
			sharedfile := filepath.Join(sharedroot, relfile)
			if (force || !FileExists(localfile)) && FileExists(sharedfile) {
				linkerr := linkSharedLOBFilename(sharedfile)
				if linkerr != nil {
					// we want to continue so don't return this
					LogErrorf("Failed to link shared file %v into local repo: %v\n", sharedfile, linkerr.Error())
				}
			}
		}
	}

	return err
}

// Fetch the files required for a single LOB
func FetchSingle(lobsha string, provider SyncProvider, remoteName string, force bool, callback ProgressCallback) error {

	var lobToDownload []string
	if force {
		lobToDownload = append(lobToDownload, lobsha)
	} else {
		lobToDownload = GetMissingLOBs([]string{lobsha}, false)
	}

	if len(lobToDownload) > 0 {
		return fetchLOBs(lobToDownload, provider, remoteName, force, callback)
	} else {
		return nil
	}
}

// Auto-fetch a single LOB from the default locations
// If the required files are not found this won't cause an error
func AutoFetch(lobsha string, reportProgress bool) error {
	remoteName := GetGitDefaultRemoteForPull()
	LogDebugf("Trying to auto-fetch %v from %v\n", lobsha, remoteName)
	// check the remote config to make sure it's valid
	provider, err := GetProviderForRemote(remoteName)
	if err != nil {
		return err
	}
	if err = provider.ValidateConfig(remoteName); err != nil {
		return err
	}

	var fetcherr error
	if reportProgress {
		// We need to run this in a goroutine to report progress deterministically
		// 100 items in the queue should be good enough, this means that it won't block
		callbackChan := make(chan *ProgressCallbackData, 100)
		go func(lobsha string, provider SyncProvider, remoteName string, progresschan chan<- *ProgressCallbackData) {

			// Progress callback just passes the result back to the channel
			progress := func(data *ProgressCallbackData) (abort bool) {
				progresschan <- data

				return false
			}

			err := FetchSingle(lobsha, provider, remoteName, false, progress)

			close(progresschan)

			if err != nil {
				fetcherr = err
			}

		}(lobsha, provider, remoteName, callbackChan)

		// Report progress on operation every 0.5s
		ReportProgressToConsole(callbackChan, "Fetch", time.Millisecond*500)
		// Because no final newline from report progress
		LogConsole("")
	} else {
		// no progress, just do it
		fetcherr = FetchSingle(lobsha, provider, remoteName, false, func(data *ProgressCallbackData) (abort bool) { return false })
	}

	if fetcherr == nil {
		LogDebugf("Successfully fetched %v from %v\n", lobsha, remoteName)
	} else {
		LogDebugf("Failed to auto fetch %v from %v: %v\n", lobsha, remoteName, fetcherr)
	}

	return fetcherr
}

func cmdFetchHelp() {
	LogConsole(`Usage: git-lob fetch [options] [<remote> [<ref>...]]

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

  git-lob.fetch-include
  git-lob.fetch-exclude

These 2 settings you probably only want to define at the repo level. 
fetch-include limits the binaries downloaded to only matching paths, while
fetch-exclude downloads everything except files matching these paths.
The contents of each are comma-separated paths with wildcard (*) matching.
Note that wildcards do not match path separators, like gitignore.
`)
}
