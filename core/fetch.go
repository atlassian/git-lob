package core

import (
	"bitbucket.org/sinbad/git-lob/providers"
	"bitbucket.org/sinbad/git-lob/util"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

// Implementation of fetch
func Fetch(provider providers.SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force bool,
	callback util.ProgressCallback) error {
	// We need to build a list of commits ranges at which we want to ensure binaries are present locally
	// We can't build the list of binaries solely from the log, because  not all binaries needed may have been
	// modified in the range. Therefore we need 'git ls-tree' at the base ancestor in each range, followed by
	// a 'git log -G' query for subsequent LOB changes. This is faster than doing ls-tree for every individual commit
	// and eliminating duplicates.

	util.LogDebugf("Fetching from %v via %v\n", remoteName, provider.TypeID())

	var fileLobsNeeded []*FileLOB
	var fetchranges []*GitRefSpec
	if len(refspecs) == 0 {
		// No refs specified, use 'Recent' fetch algorithm
		if util.GlobalOptions.Verbose {
			callback(&util.ProgressCallbackData{util.ProgressCalculate, "Calculating recent commits...",
				int64(0), int64(1), 0, 0})
		}
		// Get HEAD LOBs first
		headfilelobs, earliestCommit, err := GetGitAllFileLOBsToCheckoutAtCommitAndRecent("HEAD", util.GlobalOptions.FetchCommitsPeriodHEAD,
			util.GlobalOptions.FetchIncludePaths, util.GlobalOptions.FetchExcludePaths)
		if err != nil {
			return errors.New(fmt.Sprintf("Error determining recent HEAD commits: %v", err.Error()))
		}
		if util.GlobalOptions.Verbose {
			callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf(" * HEAD: %d binary references", len(headfilelobs)),
				0, 0, 0, 0})
		}
		fileLobsNeeded = headfilelobs
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

				recentreflobs, earliestCommit, err := GetGitAllFileLOBsToCheckoutAtCommitAndRecent(ref.Name, util.GlobalOptions.FetchCommitsPeriodOther,
					util.GlobalOptions.FetchIncludePaths, util.GlobalOptions.FetchExcludePaths)
				if err != nil {
					return errors.New(fmt.Sprintf("Error determining recent commits on %v: %v", ref, err.Error()))
				}
				if util.GlobalOptions.Verbose {
					callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf(" * %v: %d binary references", ref, len(recentreflobs)),
						int64(i), int64(len(refspecs)), 0, 0})
				}
				fileLobsNeeded = append(fileLobsNeeded, recentreflobs...)

				fetchranges = append(fetchranges, &GitRefSpec{fmt.Sprintf("^%v", earliestCommit), "..", ref.CommitSHA})
			}

		}
	} else {
		// Get LOBs directly from specified refs/ranges
		for i, refspec := range refspecs {
			if util.GlobalOptions.Verbose {
				callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf("Calculating data to fetch for %v", refspec),
					int64(i), int64(len(refspecs)), 0, 0})
			}
			reffileshas, err := GetGitAllFilesAndLOBsToCheckoutInRefSpec(refspec, util.GlobalOptions.FetchIncludePaths, util.GlobalOptions.FetchExcludePaths)
			if err != nil {
				return errors.New(fmt.Sprintf("Error determining LOBs to fetch for %v: %v", refspec, err.Error()))
			}
			if util.GlobalOptions.Verbose {
				callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf(" * %v: %d binary references", refspec, len(refspecs)),
					int64(i), int64(len(refspecs)), 0, 0})
			}
			fileLobsNeeded = append(fileLobsNeeded, reffileshas...)

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
				for _, sha := range commit.LobSHAs {
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

	if len(fileLobsNeeded) == 0 {
		callback(&util.ProgressCallbackData{util.ProgressCalculate, "No binaries to download.",
			int64(len(refspecs)), int64(len(refspecs)), 0, 0})
	} else {

		// Duplicates are not eliminated by methods we call, for efficiency
		// We need to remove them though because otherwise we can report much higher download requirements
		// than necessary when multiple refs include the same SHA
		// We use this opportunity to build a straight list of unduplicated LOB shas which is also map to a filename
		// which we may use in smart servers to determine other versions to generate deltas on

		lobsToDownload := ConvertFileLOBSliceToMap(fileLobsNeeded)
		if !force {
			// Eliminate any that are OK locally
			// It's safe to delete as you iterate in Go! refreshing :)
			for sha, _ := range lobsToDownload {
				if !IsLOBMissing(sha, false) {
					delete(lobsToDownload, sha)
				}
			}
		}

		if len(lobsToDownload) == 0 {
			callback(&util.ProgressCallbackData{util.ProgressCalculate, "No binaries to download.",
				int64(len(refspecs)), int64(len(refspecs)), 0, 0})
			return nil
		} else {
			callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf("%d binaries to download.", len(lobsToDownload)),
				int64(len(refspecs)), int64(len(refspecs)), 0, 0})
		}
		if !dryRun {
			fetchCallback := func(data *util.ProgressCallbackData) (abort bool) {
				if data.Type == util.ProgressNotFound {
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
func fetchLOBs(lobshas map[string]string, provider providers.SyncProvider, remoteName string, force bool, callback util.ProgressCallback) error {
	// Download metafiles first
	// This will allow us to estimate the time required
	callback(&util.ProgressCallbackData{util.ProgressCalculate, "Downloading metadata",
		0, 0, 0, 0})
	err := fetchMetadata(lobshas, provider, remoteName, force, callback)
	if err != nil {
		return err
	}

	// So now we have all the metadata available locally, we can know what files to download

	var filesTotalBytes int64
	var files []string
	var deltas []*LOBDelta
	var deltaTotalBytes int64
	var deltaSavings int64
	smartProvider := providers.UpgradeToSmartSyncProvider(provider)

	callback(&util.ProgressCallbackData{util.ProgressCalculate, "Calculating content files to download",
		0, 0, 0, 0})
	for sha, filename := range lobshas {
		info, err := GetLOBInfo(sha)
		if err != nil {
			// If we could not get the lob data, it means that we could not download the meta file
			// it's OK for this to happen since the remote may not have all the data we need (e.g.
			// local branch with changes that actually came from somewhere else, or remote hasn't been
			// fully updated yet)
			// We notified earlier
			continue
		}
		// If this is a smart provider, try to download deltas where appropriate
		if info.Size > util.GlobalOptions.FetchDeltasAboveSize && smartProvider != nil {
			// This doesn't download, just prepares and gets size
			delta := prepareFetchDelta(sha, filename, smartProvider, remoteName)
			if delta != nil {
				deltas = append(deltas, delta)
				deltaTotalBytes += delta.DeltaSize
				deltaSavings += info.Size - (delta.DeltaSize + 100) // 100 for metadata which we don't save
				// We'll do a delta for this so don't continue to determine files
				continue
			}
		}
		// fallback to basic file download
		filesTotalBytes += info.Size
		for i := 0; i < info.NumChunks; i++ {
			// get relative filename for download purposes
			files = append(files, GetLOBChunkRelativePath(sha, i))
		}
	}
	totalBytes := filesTotalBytes + deltaTotalBytes
	callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf("Metadata done, downloading content (%v)", util.FormatSize(totalBytes)),
		0, 0, 0, 0})
	if deltaSavings > 0 {
		callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf("Saving %v by fetching deltas", util.FormatSize(deltaSavings)),
			0, 0, 0, 0})
	}

	// Download content now
	if smartProvider != nil && len(deltas) > 0 {
		// First try deltas, if any fail fall back on regular download
		failedDeltas := fetchDeltas(deltas, deltaTotalBytes, smartProvider, remoteName, force, callback)
		if len(failedDeltas) > 0 {
			for _, delta := range failedDeltas {
				// Slightly costly but this should be rare for prepare to work and download not to
				info, err := GetLOBInfo(delta.TargetSHA)
				if err != nil {
					return fmt.Errorf("LOB info for %v went missing, this should be impossible: %v", delta.TargetSHA, err.Error())
				}
				filesTotalBytes += info.Size
				for i := 0; i < info.NumChunks; i++ {
					// get relative filename for download purposes
					files = append(files, GetLOBChunkRelativePath(info.SHA, i))
				}
			}
		}
	}
	return fetchContentFiles(files, filesTotalBytes, provider, remoteName, force, callback)

}

func prepareFetchDelta(lobsha, filename string, provider providers.SmartSyncProvider, remoteName string) *LOBDelta {
	othershas, err := GetGitAllLOBHistoryForFile(filename, lobsha)
	if err != nil {
		util.LogErrorf("Unable to prepare delta for %v(%v): %v\n", lobsha, filename, err.Error())
		return nil
	}
	// This is all the possible base shas, but we can only use ones we have locally too
	// Right now we're not trying to cope with ordered downloads where we might have newer ones part way through fetch (too fiddly)
	var localbaseshas []string
	for _, sha := range othershas {
		if !IsLOBMissing(sha, false) {
			localbaseshas = append(localbaseshas, sha)
		}
	}
	if len(localbaseshas) == 0 {
		// no base shas, cannot do this
		return nil
	}
	// Now ask the server to pick a sha, generate a delta, cache it and tell us how big it is
	sz, chosenbasesha, err := provider.PrepareDeltaForDownload(remoteName, lobsha, localbaseshas)
	if err != nil {
		util.LogErrorf("Unable to prepare delta %v(%v): %v\n", lobsha, filename, err.Error())
		return nil
	}
	// No common base to use
	if chosenbasesha == "" {
		return nil
	}
	return &LOBDelta{
		BaseSHA:   chosenbasesha,
		TargetSHA: lobsha,
		DeltaSize: sz,
	}
}

// Internal method for fetching
func fetchMetadata(lobshas map[string]string, provider providers.SyncProvider, remoteName string, force bool, callback util.ProgressCallback) error {
	// Use average metafile bytes as estimate of download, usually around 100 bytes of JSON
	averageMetaSize := 100
	metaTotalBytes := int64(len(lobshas) * averageMetaSize)
	var metafilesDone int
	metacallback := func(fileInProgress string, progressType util.ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
		// Don't bother to track partial completion, only 100 bytes each
		if progressType == util.ProgressSkip || progressType == util.ProgressNotFound {
			metafilesDone++
			callback(&util.ProgressCallbackData{progressType, fileInProgress, totalBytes, totalBytes,
				int64(metafilesDone * averageMetaSize), metaTotalBytes})
			// Remote did not have this file
		} else {
			if bytesDone == totalBytes {
				// finished
				metafilesDone++
				callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, totalBytes, totalBytes,
					int64(metafilesDone * averageMetaSize), metaTotalBytes})
			}
		}
		return false
	}
	// Download all meta files
	var metafilesToDownload []string
	for sha, _ := range lobshas {
		// Note get relative file name
		metafilesToDownload = append(metafilesToDownload, GetLOBMetaRelativePath(sha))
	}
	// Download to shared if using shared area (we link later)
	destDir := getFetchDestination()
	err := provider.Download(remoteName, metafilesToDownload, destDir, force, metacallback)

	// If shared store, link any metadata we downloaded into local
	if IsUsingSharedStorage() {
		for _, sha := range lobshas {
			// filenames are relative (for download)
			localfile := GetLocalLOBMetaPath(sha)
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
	if IsUsingSharedStorage() {
		return GetSharedLOBRoot()
	} else {
		return GetLocalLOBRoot()
	}
}

func fetchContentFiles(files []string, filesTotalBytes int64, provider providers.SyncProvider,
	remoteName string, force bool, callback util.ProgressCallback) error {
	var lastFilename string
	var lastFileBytes int64
	var bytesFromFilesDoneSoFar int64
	contentcallback := func(fileInProgress string, progressType util.ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {

		var ret bool
		if lastFilename != fileInProgress && lastFilename != "" {
			// we obviously never got a 100% call for previous file
			bytesFromFilesDoneSoFar += lastFileBytes
			ret = callback(&util.ProgressCallbackData{util.ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
				bytesFromFilesDoneSoFar, filesTotalBytes})
			lastFilename = ""
		}
		if progressType == util.ProgressSkip || progressType == util.ProgressNotFound {
			bytesFromFilesDoneSoFar += totalBytes
			ret = callback(&util.ProgressCallbackData{progressType, fileInProgress, totalBytes, totalBytes,
				bytesFromFilesDoneSoFar, filesTotalBytes})
		} else {

			if bytesDone == totalBytes {
				// finished
				bytesFromFilesDoneSoFar += totalBytes
				ret = callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesFromFilesDoneSoFar, filesTotalBytes})
				lastFilename = ""
			} else {
				// partly progressed file
				lastFilename = fileInProgress
				lastFileBytes = totalBytes
				ret = callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesFromFilesDoneSoFar + bytesDone, filesTotalBytes})

			}

		}

		return ret
	}
	destDir := getFetchDestination()
	err := provider.Download(remoteName, files, destDir, force, contentcallback)
	if err == nil && lastFilename != "" {
		// we obviously never got a 100% progress call for final file
		callback(&util.ProgressCallbackData{util.ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
			filesTotalBytes, filesTotalBytes})
		lastFilename = ""
	}
	// Also if shared store, link meta into local
	// Link any we successfully downloaded
	if IsUsingSharedStorage() {
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

// Fetch via deltas which have already been picked & prepared on the server. Any that fail for any reason are added
// to the faileddeltas return list and will be re-tried using the standard download
func fetchDeltas(deltas []*LOBDelta, deltaTotalBytes int64, provider providers.SmartSyncProvider, remoteName string,
	force bool, callback util.ProgressCallback) (faileddeltas []*LOBDelta) {

	var failed []*LOBDelta
	var bytesDoneSoFar int64
	for _, delta := range deltas {

		err := fetchSingleDelta(delta, bytesDoneSoFar, deltaTotalBytes, provider, remoteName, force, callback)
		bytesDoneSoFar += delta.DeltaSize
		if err != nil {
			failed = append(failed, delta)
			msg := fmt.Sprintf("Error applying %v: %v. Falling back to non-delta download", getDeltaProgressDesc(delta), err.Error())
			callback(&util.ProgressCallbackData{util.ProgressError, msg, delta.DeltaSize, delta.DeltaSize,
				bytesDoneSoFar, deltaTotalBytes})
		}

	}
	return failed

}

func getDeltaProgressDesc(delta *LOBDelta) string {
	return fmt.Sprintf("Delta %v..%v", delta.BaseSHA[:7], delta.TargetSHA[:7])
}

func fetchSingleDelta(delta *LOBDelta, bytesSoFar int64, deltaTotalBytes int64, provider providers.SmartSyncProvider, remoteName string,
	force bool, callback util.ProgressCallback) error {

	// Description for progress
	desc := getDeltaProgressDesc(delta)

	// Initial 0% call
	callback(&util.ProgressCallbackData{util.ProgressTransferBytes, desc, 0, delta.DeltaSize,
		bytesSoFar, deltaTotalBytes})

	// We could pipe download output directly into ApplyDelta via a goroutine
	// But for simplicity of fail states, use a temp file
	tempf, err := ioutil.TempFile("", "deltadownload")
	if err != nil {
		return err
	}
	tempfilename := tempf.Name()
	defer os.Remove(tempfilename) // ensure always removed
	defer tempf.Close()           // only used in panic cases, we close manually
	localcallback := func(txt string, progressType util.ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {

		var ret bool
		if bytesDone != totalBytes {
			// only do part progress in here, do final outside to ensure it always happens regardless
			ret = callback(&util.ProgressCallbackData{util.ProgressTransferBytes, desc, bytesDone, totalBytes,
				bytesSoFar + bytesDone, deltaTotalBytes})

		}

		return ret
	}

	err = provider.DownloadDelta(remoteName, delta.BaseSHA, delta.TargetSHA, tempf, localcallback)
	tempf.Close() // Close so available to read back
	if err != nil {
		return err
	}
	// finished download OK, now apply
	deltain, err := os.OpenFile(tempfilename, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	defer deltain.Close()
	// Apply to shared or local
	err = ApplyLOBDeltaInBaseDir(getFetchDestination(), delta.BaseSHA, delta.TargetSHA, deltain)
	if err != nil {
		return err
	}

	// Also if downloading to shared store, link into local
	if IsUsingSharedStorage() {
		ok := recoverLocalLOBFilesFromSharedStore(delta.TargetSHA)
		if !ok {
			return fmt.Errorf("%v was applied to shared store but linking to local failed", desc)
		}
	}

	// yay, call final 100%
	callback(&util.ProgressCallbackData{util.ProgressTransferBytes, desc, delta.DeltaSize, delta.DeltaSize,
		bytesSoFar + delta.DeltaSize, deltaTotalBytes})

	return nil

}

// Fetch the files required for a single LOB
func FetchSingle(lobsha string, provider providers.SyncProvider, remoteName string, force bool, callback util.ProgressCallback) error {

	var lobToDownload map[string]string
	if force || IsLOBMissing(lobsha, false) {
		// We don't know the filename, this is forced
		lobToDownload[lobsha] = ""
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
	provider, err := providers.GetProviderForRemote(remoteName)
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
		callbackChan := make(chan *util.ProgressCallbackData, 100)
		go func(lobsha string, provider providers.SyncProvider, remoteName string, progresschan chan<- *util.ProgressCallbackData) {

			// Progress callback just passes the result back to the channel
			progress := func(data *util.ProgressCallbackData) (abort bool) {
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
		util.ReportProgressToConsole(callbackChan, "Fetch", time.Millisecond*500)
		// Because no final newline from report progress
		util.LogConsole("")
	} else {
		// no progress, just do it
		fetcherr = FetchSingle(lobsha, provider, remoteName, false, func(data *util.ProgressCallbackData) (abort bool) { return false })
	}

	if fetcherr == nil {
		util.LogDebugf("Successfully fetched %v from %v\n", lobsha, remoteName)
	} else {
		util.LogDebugf("Failed to auto fetch %v from %v: %v\n", lobsha, remoteName, fetcherr)
	}

	return fetcherr
}
