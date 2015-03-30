package core

import (
	"bitbucket.org/sinbad/git-lob/util"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// Implementation of fetch
func Fetch(provider SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force bool,
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
			refSHAsDone := util.NewStringSet()
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
