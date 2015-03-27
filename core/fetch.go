package core

import (
	"bitbucket.org/sinbad/git-lob/util"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Fetch command line tool
func cmdFetch() int {

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
	var refspecs []*GitRefSpec

	if len(util.GlobalOptions.Args) > 0 {
		// first parameter must be remote if there are arguments
		remoteName = util.GlobalOptions.Args[0]

		// Remaining args are refspecs
		if len(util.GlobalOptions.Args) > 1 {
			for _, arg := range util.GlobalOptions.Args[1:] {
				r := ParseGitRefSpec(arg)

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
		remoteName = GetGitDefaultRemoteForPull()
	}

	// check the remote config to make sure it's valid
	provider, err := GetProviderForRemote(remoteName)
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
	callbackChan := make(chan *ProgressCallbackData, 100)
	go func(provider SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, prune bool,
		progresschan chan<- *ProgressCallbackData) {

		// Progress callback just passes the result back to the channel
		progress := func(data *ProgressCallbackData) (abort bool) {
			progresschan <- data

			return false
		}

		err := Fetch(provider, remoteName, refspecs, dryRun, force, prune, progress)

		close(progresschan)

		if err != nil {
			fetcherr = err
		}

	}(provider, remoteName, refspecs, optDryRun, optForce, optPrune, callbackChan)

	// Report progress on operation every 0.5s
	fetchCounts := ReportProgressToConsole(callbackChan, "Fetch", time.Millisecond*500)

	if fetcherr != nil {
		util.LogError("git-lob: fetch error(s):\n%v", fetcherr.Error())
		return 12
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
func cmdFetchLob() int {

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
	provider, err := GetProviderForRemote(remoteName)
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
	callbackChan := make(chan *ProgressCallbackData, 100)
	go func(provider SyncProvider, remoteName string, shas []string, force bool,
		progresschan chan<- *ProgressCallbackData) {

		// Progress callback just passes the result back to the channel
		progress := func(data *ProgressCallbackData) (abort bool) {
			progresschan <- data

			return false
		}

		var err error
		for _, sha := range shas {
			err = FetchSingle(sha, provider, remoteName, force, progress)
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
	fetchCounts := ReportProgressToConsole(callbackChan, "Fetch", time.Millisecond*500)

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

// Implementation of fetch
func Fetch(provider SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, prune bool,
	callback ProgressCallback) error {
	// We need to build a list of commits ranges at which we want to ensure binaries are present locally
	// We can't build the list of binaries solely from the log, because  not all binaries needed may have been
	// modified in the range. Therefore we need 'git ls-tree' at the base ancestor in each range, followed by
	// a 'git log -G' query for subsequent LOB changes. This is faster than doing ls-tree for every individual commit
	// and eliminating duplicates.

	util.LogDebugf("Fetching from %v via %v\n", remoteName, provider.TypeID())

	var lobsNeeded []string
	var fetchranges []*GitRefSpec
	if len(refspecs) == 0 {
		// No refs specified, use 'Recent' fetch algorithm
		if util.GlobalOptions.Verbose {
			callback(&ProgressCallbackData{ProgressCalculate, "Calculating recent commits...",
				int64(0), int64(1), 0, 0})
		}
		// Get HEAD LOBs first
		headlobs, earliestCommit, err := GetGitAllLOBsToCheckoutAtCommitAndRecent("HEAD", util.GlobalOptions.FetchCommitsPeriodHEAD,
			util.GlobalOptions.FetchIncludePaths, util.GlobalOptions.FetchExcludePaths)
		if err != nil {
			return errors.New(fmt.Sprintf("Error determining recent HEAD commits: %v", err.Error()))
		}
		if util.GlobalOptions.Verbose {
			callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf(" * HEAD: %d binary references", len(headlobs)),
				0, 0, 0, 0})
		}
		lobsNeeded = headlobs
		headSHA, err := GitRefToFullSHA("HEAD")
		if err != nil {
			return errors.New(fmt.Sprintf("Error determining HEAD sha: %v", err.Error()))
		}
		fetchranges = append(fetchranges, &GitRefSpec{fmt.Sprintf("^%v", earliestCommit), "..", headSHA})
		if util.GlobalOptions.FetchRefsPeriodDays > 0 {
			// Find recent other refs (only include remote branches for this remote)
			recentrefs, err := GetGitRecentRefs(util.GlobalOptions.FetchRefsPeriodDays, true, remoteName)
			if err != nil {
				return errors.New(fmt.Sprintf("Error determining recent refs: %v", err.Error()))
			}
			// Now each other ref, they should be in reverse date order from GetGitRecentRefs so we're doing
			// things by priority, HEAD first then most recent
			refSHAsDone := NewStringSet()
			refSHAsDone.Add(headSHA)
			for i, ref := range recentrefs {
				// Don't duplicate work when >1 ref has the same SHA
				// Most common with HEAD if not detached but also tags
				if refSHAsDone.Contains(ref.CommitSHA) {
					continue
				}
				refSHAsDone.Add(ref.CommitSHA)

				recentreflobs, earliestCommit, err := GetGitAllLOBsToCheckoutAtCommitAndRecent(ref.Name, util.GlobalOptions.FetchCommitsPeriodOther,
					util.GlobalOptions.FetchIncludePaths, util.GlobalOptions.FetchExcludePaths)
				if err != nil {
					return errors.New(fmt.Sprintf("Error determining recent commits on %v: %v", ref, err.Error()))
				}
				if util.GlobalOptions.Verbose {
					callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf(" * %v: %d binary references", ref, len(recentreflobs)),
						int64(i), int64(len(refspecs)), 0, 0})
				}
				lobsNeeded = append(lobsNeeded, recentreflobs...)

				fetchranges = append(fetchranges, &GitRefSpec{fmt.Sprintf("^%v", earliestCommit), "..", ref.CommitSHA})
			}

		}
	} else {
		// Get LOBs directly from specified refs/ranges
		for i, refspec := range refspecs {
			if util.GlobalOptions.Verbose {
				callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf("Calculating data to fetch for %v", refspec),
					int64(i), int64(len(refspecs)), 0, 0})
			}
			refshas, err := GetGitAllLOBsToCheckoutInRefSpec(refspec, util.GlobalOptions.FetchIncludePaths, util.GlobalOptions.FetchExcludePaths)
			if err != nil {
				return errors.New(fmt.Sprintf("Error determining LOBs to fetch for %v: %v", refspec, err.Error()))
			}
			if util.GlobalOptions.Verbose {
				callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf(" * %v: %d binary references", refspec, len(refspecs)),
					int64(i), int64(len(refspecs)), 0, 0})
			}
			lobsNeeded = append(lobsNeeded, refshas...)

			if refspec.IsRange() {
				fetchranges = append(fetchranges, refspec)
			} else {
				fetchranges = append(fetchranges, &GitRefSpec{fmt.Sprintf("^%v", refspec.Ref1), "..", refspec.Ref2})
			}
		}
	}

	var commitsToMarkPushedAfterFetching []string
	// Before we actually fetch anything, check if we can bulk mark things as pushed
	// Common case - first fetch after clone, user hasn't done any local work
	if !InitSuccessfullyPushedCacheIfAppropriate() {
		// Otherwise, let's see whether we can move the pushed state forward as we fetch
		// Remember, we can have 'gaps' in the history for LOBs if enough time passed since last fetch
		// that the earliest fetch doesn't cover the whole period since the last fetch
		// So the cases where we can move the push markers forward are:
		// 1. There's nothing to push before the first commit we're fetching, OR
		// 2. The intervening 'unpushed' commits already exist on the remote
		for _, fetchrange := range fetchranges {
			anyCommitsUnpushed := false
			allUnpushedCommitsAreOnRemote := true
			unpushedCallback := func(commit *CommitLOBRef) (quit bool, err error) {
				anyCommitsUnpushed = true
				for _, sha := range commit.lobSHAs {
					// check remote
					remoteerr := CheckRemoteLOBFilesForSHA(sha, provider, remoteName)
					if remoteerr != nil {
						// LOB doesn't exist on remote so this is genuinely unpushed
						allUnpushedCommitsAreOnRemote = false
						return true, remoteerr
					}
				}
				return false, nil
			}
			// These are all ranges, Ref1 being exclusive so that's where we measure from
			WalkGitCommitLOBsToPush(remoteName, fetchrange.Ref1, false, unpushedCallback)
			if !anyCommitsUnpushed || allUnpushedCommitsAreOnRemote {
				commitsToMarkPushedAfterFetching = append(commitsToMarkPushedAfterFetching, fetchrange.Ref2)
				util.LogDebugf("Will mark %v as pushed after fetch since there are no unpushed LOBs in ancestors that aren't on %v\n", fetchrange.Ref2, remoteName)
			} else {
				util.LogDebugf("%v will not be marked as pushed after fetch since there are unpushed LOBs in ancestors that aren't on %v\n", fetchrange.Ref2, remoteName)
			}
		}

	}

	fetchAnyNotFound := false

	if len(lobsNeeded) == 0 {
		callback(&ProgressCallbackData{ProgressCalculate, "No binaries to download.",
			int64(len(refspecs)), int64(len(refspecs)), 0, 0})
	} else {

		// Duplicates are not eliminated by methods we call, for efficiency
		// We need to remove them though because otherwise we can report much higher download requirements
		// than necessary when multiple refs include the same SHA
		util.StringRemoveDuplicates(&lobsNeeded)

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
			fetchCallback := func(data *ProgressCallbackData) (abort bool) {
				if data.Type == ProgressNotFound {
					fetchAnyNotFound = true
				}
				// passthrough to external callback
				return callback(data)
			}

			err := fetchLOBs(lobsToDownload, provider, remoteName, force, fetchCallback)
			if err != nil {
				return err
			}

		}
	}

	util.LogDebugf("Successfully fetched from %v via %v\n", remoteName, provider.TypeID())

	// Now mark as pushed if appropriate
	// If any files were not found on the remote, don't do this (we may get them locally later & need to push them)
	if !fetchAnyNotFound && len(commitsToMarkPushedAfterFetching) > 0 {
		for _, c := range commitsToMarkPushedAfterFetching {
			err := MarkBinariesAsPushed(remoteName, c, "")
			if err != nil {
				util.LogErrorf("Error marking %v as pushed after fetch for %v: %v", c, remoteName, err.Error())
			}
		}
		// resolve any redundant state that creates
		CleanupPushState(remoteName)
	}

	if prune && !dryRun {
		callback(&ProgressCallbackData{ProgressCalculate, "Performing post-fetch prune...",
			int64(len(refspecs)), int64(len(refspecs)), 0, 0})
		PostFetchPullPrune()
	}

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
	callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf("Metadata done, downloading content (%v)", util.FormatSize(filesTotalBytes)),
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
			if (force || !util.FileExists(localfile)) && util.FileExists(sharedfile) {
				linkerr := linkSharedLOBFilename(sharedfile)
				if linkerr != nil {
					// we want to continue so don't return this
					util.LogErrorf("Failed to link shared file %v into local repo: %v\n", sharedfile, linkerr.Error())
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
		if lastFilename != fileInProgress && lastFilename != "" {
			// we obviously never got a 100% call for previous file
			bytesFromFilesDoneSoFar += lastFileBytes
			ret = callback(&ProgressCallbackData{ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
				bytesFromFilesDoneSoFar, filesTotalBytes})
			lastFilename = ""
		}
		if progressType == ProgressSkip || progressType == ProgressNotFound {
			bytesFromFilesDoneSoFar += totalBytes
			ret = callback(&ProgressCallbackData{progressType, fileInProgress, totalBytes, totalBytes,
				bytesFromFilesDoneSoFar, filesTotalBytes})
		} else {

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
	if err == nil && lastFilename != "" {
		// we obviously never got a 100% progress call for final file
		callback(&ProgressCallbackData{ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
			filesTotalBytes, filesTotalBytes})
		lastFilename = ""
	}
	// Also if shared store, link meta into local
	// Link any we successfully downloaded
	if isUsingSharedStorage() {
		localroot := GetLocalLOBRoot()
		sharedroot := GetSharedLOBRoot()
		for _, relfile := range files {
			// filenames are relative (for download)
			localfile := filepath.Join(localroot, relfile)
			sharedfile := filepath.Join(sharedroot, relfile)
			if (force || !util.FileExists(localfile)) && util.FileExists(sharedfile) {
				linkerr := linkSharedLOBFilename(sharedfile)
				if linkerr != nil {
					// we want to continue so don't return this
					util.LogErrorf("Failed to link shared file %v into local repo: %v\n", sharedfile, linkerr.Error())
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
	util.LogDebugf("Trying to auto-fetch %v from %v\n", lobsha, remoteName)
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
		util.LogConsole("")
	} else {
		// no progress, just do it
		fetcherr = FetchSingle(lobsha, provider, remoteName, false, func(data *ProgressCallbackData) (abort bool) { return false })
	}

	if fetcherr == nil {
		util.LogDebugf("Successfully fetched %v from %v\n", lobsha, remoteName)
	} else {
		util.LogDebugf("Failed to auto fetch %v from %v: %v\n", lobsha, remoteName, fetcherr)
	}

	return fetcherr
}

func cmdFetchHelp() {
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
func cmdFetchLobHelp() {
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
