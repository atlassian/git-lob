package core

import (
	"bitbucket.org/sinbad/git-lob/providers"
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"io/ioutil"
	"os"
)

// Some temporary storage used to pre-calculate the amount of data we'll need to upload
type PushCommitContentDetails struct {
	CommitSHA  string      // the commit's SHA
	Deltas     []*LOBDelta // delta uploads, items in here won't be in Files
	Files      []string    // list of files we'll need to upload, relative path, if not doing deltas
	BaseDir    string      // the base dir of the above files
	FileBytes  int64       // total bytes for all files in the list
	DeltaBytes int64       // total bytes for all deltas in the list
	Incomplete bool        // File list is not complete because of missing local data, we shouldn't mark this commit as pushed
}

func Push(provider providers.SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, recheck bool,
	callback util.ProgressCallback) error {

	util.LogDebugf("Pushing to %v via %v\n", remoteName, provider.TypeID())
	smartProvider := providers.UpgradeToSmartSyncProvider(provider)

	// for use when --force used
	shasAlreadyQueued := util.NewStringSet()

	for i, refspec := range refspecs {
		// We now perform a complete push per refspec before proceeding to the nex
		// estimates & progress is measured within the refspec
		// This is how we mark pushed anyway, more consistent than trying to do for all refspecs in 1
		var refCommitsToPush []*PushCommitContentDetails
		var anyIncomplete bool

		if util.GlobalOptions.Verbose {
			callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf("Calculating data to push for %v", refspec),
				int64(i), int64(len(refspecs)), 0, 0})
		}

		var refFileSize, refDeltaSize int64
		var deltaSavings int64

		// First we walk the commits to push & build up a picture of size etc
		walkFunc := func(commit *CommitLOBRef) (quit bool, err error) {
			var problemSHAs []string
			var allfilenamesforcommit []string
			var alldeltasforcommit []*LOBDelta
			var commitFileSize int64
			var commitDeltaSize int64
			// Always use local LOB root since files are hardlinked there in shared case
			basedir := GetLocalLOBRoot()
			commitIncomplete := false
			for _, filelob := range commit.FileLOBs {
				var err error
				filesMissing := false

				// We can end up duplicating work (and uploads in --force mode) when a LOB is referred
				// to multiple times in one push, so skip duplicates
				// We still add the commit to the list, it just might not need anything done, but
				// important to mark it as pushed anyway
				if shasAlreadyQueued.Contains(filelob.SHA) {
					continue
				}

				// check size integrity but don't recalculate sha
				filenames, filesize, err := GetLOBFilesForSHA(filelob.SHA, basedir, true, false)
				if err != nil {
					if IsNotFoundError(err) {
						filesMissing = true
						problemSHAs = append(problemSHAs, filelob.SHA)
					} else {
						return true, err
					}
				}
				// Pre-check if we can/should do a delta
				var delta *LOBDelta
				if !filesMissing && smartProvider != nil && filesize > util.GlobalOptions.PushDeltasAboveSize {
					// This will return nil if not possible
					delta = preparePushDelta(filelob.SHA, filelob.Filename, smartProvider, remoteName, force)
				}

				if delta != nil {
					// We'll try this as a delta; if it fails later then we'll fall back on normal
					alldeltasforcommit = append(alldeltasforcommit, delta)
					commitDeltaSize += delta.DeltaSize + ApproximateMetadataSize
					deltaSavings += (delta.DeltaSize + ApproximateMetadataSize) - filesize
				} else {
					allfilenamesforcommit = append(allfilenamesforcommit, filenames...)
					commitFileSize += filesize
				}
				shasAlreadyQueued.Add(filelob.SHA)

			}
			if len(problemSHAs) > 0 {
				// If we got here it means one or more sets of files for SHAs were not available or were bad locally
				// We still want to push the rest though, we want to be tolerant of partial data

				// This MAY be ok to still mark as pushed - the commits may have come from someone else,
				// and may just be outside of our fetch range. If all the missing ones are already present
				// on the remote then we're OK

				// Check the remote for the presence of missing SHA data
				remoteHasOurMissingSHAs := true
				for _, sha := range problemSHAs {
					remoteerr := CheckRemoteLOBFilesForSHA(sha, provider, remoteName)
					if remoteerr != nil {
						// Damn, missing
						util.LogDebug(fmt.Sprintf("Commit %v locally missing %v, not on remote: %v", commit.Commit[:7], sha, remoteerr.Error()))
						remoteHasOurMissingSHAs = false
						break
					}
				}

				if !remoteHasOurMissingSHAs {
					// Genuinely incomplete data in this commit that isn't present on remote
					// We can't mark this (or following) commits as pushed, but we still want to
					// push everything we can
					commitIncomplete = true
					anyIncomplete = true
					util.LogDebug(fmt.Sprintf("Some content for commit %v is missing & not on remote already", commit.Commit[:7]))
					callback(&util.ProgressCallbackData{util.ProgressNotFound, fmt.Sprintf("data for commit %v", commit.Commit[:7]),
						int64(i + 1), int64(len(refspecs)), 0, 0})
				}
				// If we DID manage to find the missing data on the remote though, we treat this as
				// being able to push everything
			}

			refCommitsToPush = append(refCommitsToPush, &PushCommitContentDetails{
				CommitSHA:  commit.Commit,
				Files:      allfilenamesforcommit,
				BaseDir:    basedir,
				FileBytes:  commitFileSize,
				DeltaBytes: commitDeltaSize,
				Incomplete: commitIncomplete,
				Deltas:     alldeltasforcommit,
			})

			refDeltaSize += commitDeltaSize
			refFileSize += commitFileSize

			return false, nil
		}

		err := WalkGitCommitLOBsToPushForRefSpec(remoteName, refspec, recheck, walkFunc)
		// defer delete any delta files we created so we always clean up
		for _, commit := range refCommitsToPush {
			for _, delta := range commit.Deltas {
				if delta.DeltaFilename != "" {
					defer os.Remove(delta.DeltaFilename)
				}
			}
		}
		if err != nil {
			return err
		}

		refCommitsSize := refFileSize + refDeltaSize

		if len(refCommitsToPush) == 0 {
			callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf(" * %v: Nothing to push", refspec),
				int64(i), int64(len(refspecs)), 0, 0})
			// if nothing to push, then mark this ref as pushed to make querying faster next time
			// Only for normal ref where we've checked for all ancestors to be pushed, not a manual range
			if !dryRun && !refspec.IsRange() {
				commitSHA, err := GitRefToFullSHA(refspec.Ref1)
				if err != nil {
					return err
				}
				err = MarkBinariesAsPushed(remoteName, commitSHA, "")
				if err != nil {
					return err
				}

			}

		} else {
			if refCommitsSize > 0 {
				callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf(" * %v: %d commits with %v to push (if not already on remote)",
					refspec, len(refCommitsToPush), util.FormatSize(refCommitsSize)), int64(i + 1), int64(len(refspecs)), 0, 0})
				if deltaSavings > 0 {
					callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf("   Saving %v by using binary deltas",
						util.FormatSize(deltaSavings)), int64(i + 1), int64(len(refspecs)), 0, 0})
				}
			} else {
				callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf(" * %v: Nothing to push, remote is up to date", refspec),
					int64(i + 1), int64(len(refspecs)), 0, 0})
			}
		}
		if util.GlobalOptions.Verbose {
			callback(&util.ProgressCallbackData{util.ProgressCalculate, fmt.Sprintf("Finished calculating data to push for %v", refspec),
				int64(i + 1), int64(len(refspecs)), 0, 0})
		}

		if !dryRun && len(refCommitsToPush) > 0 {
			// Even if size == 0 we still skim through marking them as pushed (must have been that data was on remote)
			if refCommitsSize > 0 {
				callback(&util.ProgressCallbackData{util.ProgressCalculate,
					fmt.Sprintf("Uploading up to %v to %v via %v", util.FormatSize(refCommitsSize), remoteName, provider.TypeID()),
					0, 0, 0, 0})
			}

			var bytesDoneSoFar int64
			previousCommitIncomplete := false
			previousCommitSHA := ""
			basedir := GetLocalLOBRoot()
			for _, commit := range refCommitsToPush {

				// Push this one
				// Firstly, do any deltas (may be some deltas and some not in one commit)
				if smartProvider != nil && len(commit.Deltas) > 0 {
					// add any failed deltas back to the regular file-based upload for the next step
					faileddeltas := pushCommitDeltas(commit, smartProvider, remoteName, force, bytesDoneSoFar, refCommitsSize, callback)
					for _, delta := range faileddeltas {
						// Add the files for failed deltas to the standard route
						filenames, filesize, err := GetLOBFilesForSHA(delta.TargetSHA, basedir, true, false)
						if err != nil {
							// We already checked local files were there earlier so this is fatal
							return fmt.Errorf("Error while trying to fall back from delta to standard push: %v", err)
						}
						commit.Files = append(commit.Files, filenames...)
						// Just add the filesize on, don't subtract the delta size since we'll mark that as done
						commit.FileBytes += filesize
						refFileSize += filesize
						refCommitsSize += filesize
					}

				}
				bytesDoneSoFar += commit.DeltaBytes
				// Then, do any regular file-based uploads (and also any delta fallbacks)
				err := pushCommitStandard(commit, provider, remoteName, force, bytesDoneSoFar, refCommitsSize, callback)
				if err != nil {
					// stop at commit we can't push
					return err
				}
				// in the case of a failed delta & fallback we would have uploaded more bytes but gloss over this
				bytesDoneSoFar += commit.FileBytes

				// Otherwise mark commit as pushed IF complete
				if commit.Incomplete {
					previousCommitIncomplete = true
					// Any subsequent commits will also not be marked as pushed so we always go back to the incomplete commit
					// until this is resolved. Our commits are in ancestor order.
					// note that in the case of multiple refs is also means other following commits aren't marked as complete either
					// this will result in longer than necessary calculations in subsequent pushes, but better to be safe.
					// Sync provider will avoid any duplicate uploads anyway.
				}
				if !commit.Incomplete && !previousCommitIncomplete {
					// replace the previous commit SHA we marked as pushed each time, IF it was the direct parent
					// it's important not to just replace all because where there are merges even --topo-order will
					// walk through multiple threads of development in parallel, the only constraint is that ancestors
					// are always seen before descendants. Replacing a SHA in a parallel stream would give an incorrect
					// result if the merge wasn't finished. Although the worst case is that the other stream would
					// think it's not pushed, worth avoiding.
					// If we end up adding extra SHAs in this case, they'll get tidied up in CleanupPushState at end
					// avoids having to consolidate tons of commits later & means we generally store
					// one pushed SHA per ref, before consolidation
					replaceSHA := ""
					if previousCommitSHA != "" {
						isancestor, err := GitIsAncestor(previousCommitSHA, commit.CommitSHA)
						if err != nil {
							return err
						}
						if isancestor {
							replaceSHA = previousCommitSHA
						}
					}
					// This writes data to disk every time and that's fine, for robustness & interruptability
					err = MarkBinariesAsPushed(remoteName, commit.CommitSHA, replaceSHA)
					if err != nil {
						// Stop at commit we can't mark, order is important
						return err
					}
					previousCommitSHA = commit.CommitSHA
				}
			}
			// now perform cleanup of the push state to ensure we simplify it
			// do this per ref so that subsequent refs have a simpler git log call
			CleanupPushState(remoteName)
		}

		if anyIncomplete {
			util.LogDebugf("Partial push to %v for %v\n", remoteName, refspec)
		} else {
			util.LogDebugf("Successfully pushed to %v for %v\n", remoteName, refspec)
		}
	}
	return nil

}

// Push deltas in a commit & report those which didn't make it
func pushCommitDeltas(commit *PushCommitContentDetails, provider providers.SmartSyncProvider, remoteName string,
	force bool, bytesDoneSoFar, refDeltaBytes int64, callback util.ProgressCallback) []*LOBDelta {

	// First add up the sizes
	var faileddeltas []*LOBDelta

	for _, delta := range commit.Deltas {
		// Push metadata for this individually
		metacallback := func(fileInProgress string, progressType util.ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
			// Don't bother to track partial completion, only small
			if progressType == util.ProgressSkip || progressType == util.ProgressNotFound {
				return callback(&util.ProgressCallbackData{progressType, fileInProgress, totalBytes, totalBytes,
					bytesDoneSoFar + ApproximateMetadataSize, refDeltaBytes})
				// Remote did not have this file
			} else {
				return callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesDoneSoFar + bytesDone, refDeltaBytes})
			}
			return false
		}
		metafile := GetLOBMetaRelativePath(delta.TargetSHA)
		err := provider.Upload(remoteName, []string{metafile}, GetLocalLOBRoot(), force, metacallback)
		if err != nil {
			faileddeltas = append(faileddeltas, delta)
			continue
		}
		bytesDoneSoFar += ApproximateMetadataSize
		// Now upload delta
		completionSeen := false
		deltacallback := func(txt string, progressType util.ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
			if bytesDone == totalBytes {
				completionSeen = true
			}
			return callback(&util.ProgressCallbackData{util.ProgressTransferBytes, getDeltaProgressDesc(delta), bytesDone, totalBytes,
				bytesDoneSoFar + bytesDone, refDeltaBytes})
		}
		in, err := os.OpenFile(delta.DeltaFilename, os.O_RDONLY, 0644)
		if err != nil {
			faileddeltas = append(faileddeltas, delta)
			continue
		}
		defer in.Close()
		err = provider.UploadDelta(remoteName, delta.BaseSHA, delta.TargetSHA, in, delta.DeltaSize, deltacallback)
		bytesDoneSoFar += delta.DeltaSize

		if err != nil {
			faileddeltas = append(faileddeltas, delta)
			callback(&util.ProgressCallbackData{util.ProgressError, getDeltaProgressDesc(delta), delta.DeltaSize, delta.DeltaSize,
				bytesDoneSoFar, refDeltaBytes})
			continue
		}
		if !completionSeen {
			// Do a final callback to make sure 100% is there
			callback(&util.ProgressCallbackData{util.ProgressTransferBytes, getDeltaProgressDesc(delta), delta.DeltaSize, delta.DeltaSize,
				bytesDoneSoFar, refDeltaBytes})
		}
	}
	return faileddeltas
}

// Push a single commit using the standard approach
func pushCommitStandard(commit *PushCommitContentDetails, provider providers.SyncProvider, remoteName string,
	force bool, bytesDoneSoFar, refCommitsSize int64, callback util.ProgressCallback) error {
	// Upload now
	var lastFilename string
	var lastFileBytes int64
	localcallback := func(fileInProgress string, progressType util.ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
		if lastFilename != fileInProgress {
			// New file, always callback
			if lastFilename != "" {
				// we obviously never got a 100% call for previous file
				bytesDoneSoFar += lastFileBytes
				callback(&util.ProgressCallbackData{util.ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
					bytesDoneSoFar, refCommitsSize})
				lastFilename = ""
			}
			if progressType == util.ProgressSkip || progressType == util.ProgressNotFound {
				// 'not found' will have caused an error earlier anyway so just pass through
				bytesDoneSoFar += totalBytes
				callback(&util.ProgressCallbackData{progressType, fileInProgress, totalBytes, totalBytes,
					bytesDoneSoFar, refCommitsSize})
			} else {
				// Start new file
				callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesDoneSoFar + bytesDone, refCommitsSize})
				lastFilename = fileInProgress
				lastFileBytes = totalBytes
			}
		} else {
			if bytesDone == totalBytes {
				// finished
				bytesDoneSoFar += totalBytes
				callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesDoneSoFar, refCommitsSize})
				lastFilename = ""
			} else {
				// Otherwise this is a progress callback
				return callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesDoneSoFar + bytesDone, refCommitsSize})
			}
		}
		return false
	}
	// It IS possible to have a commit here with no files to upload. E.g. missing data locally (see above)
	// which was present on remote. We still include it in the commit list for completeness
	if len(commit.Files) > 0 {
		err := provider.Upload(remoteName, commit.Files, commit.BaseDir, force, localcallback)
		if err != nil {
			return err
		}
	}
	if lastFilename != "" {
		// We obviously never got a 100% progress update from the last file
		bytesDoneSoFar += lastFileBytes
		callback(&util.ProgressCallbackData{util.ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
			bytesDoneSoFar, refCommitsSize})
		lastFilename = ""
	}
	return nil

}

func preparePushDelta(lobsha, filename string, provider providers.SmartSyncProvider, remoteName string, force bool) *LOBDelta {
	// Don't bother to try to generate a delta if lob is already on remote & not force; will be skipped in regular upload
	if !force {
		exists, _ := provider.LOBExists(remoteName, lobsha)
		if exists {
			return nil
		}
	}

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
	// Now ask the server to pick a sha
	chosenbasesha, err := provider.GetFirstCompleteLOBFromList(remoteName, localbaseshas)
	if err != nil {
		util.LogErrorf("Unable to get common base for delta %v(%v): %v\n", lobsha, filename, err.Error())
		return nil
	}
	// No common base to use
	if chosenbasesha == "" {
		return nil
	}
	// Now we calculate the delta locally
	tempf, err := ioutil.TempFile("", "uploaddelta")
	if err != nil {
		util.LogErrorf("Unable to open temp file for delta %v(%v): %v\n", lobsha, filename, err.Error())
		return nil
	}
	defer tempf.Close()
	tempfilename := tempf.Name()
	sz, err := GenerateLOBDelta(chosenbasesha, lobsha, tempf)
	if err != nil {
		util.LogErrorf("Error calculating delta %v(%v): %v\n", lobsha, filename, err.Error())
		tempf.Close() // have to close before remove & defer is in wrong order
		os.Remove(tempfilename)
		return nil
	}
	return &LOBDelta{
		BaseSHA:       chosenbasesha,
		TargetSHA:     lobsha,
		DeltaSize:     sz,
		DeltaFilename: tempfilename,
	}
}

// Push a single LOB to a remote
func PushSingle(sha string, provider providers.SyncProvider, remoteName string, force bool,
	callback util.ProgressCallback) error {
	basedir := GetLocalLOBRoot()
	filenames, totalSize, err := GetLOBFilesForSHA(basedir, sha, true, false)
	if err != nil {
		return err
	}

	var lastFilename string
	var lastFileBytes int64
	var bytesFromFilesDoneSoFar int64
	localcallback := func(fileInProgress string, progressType util.ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
		if lastFilename != fileInProgress {
			// New file, always callback
			if lastFilename != "" {
				// we obviously never got a 100% call for previous file
				bytesFromFilesDoneSoFar += lastFileBytes
				callback(&util.ProgressCallbackData{util.ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
					bytesFromFilesDoneSoFar, totalSize})
				lastFilename = ""
			}
			if progressType == util.ProgressSkip || progressType == util.ProgressNotFound {
				// 'not found' will have caused an error earlier anyway so just pass through
				bytesFromFilesDoneSoFar += totalBytes
				callback(&util.ProgressCallbackData{progressType, fileInProgress, totalBytes, totalBytes,
					bytesFromFilesDoneSoFar, totalSize})
			} else {
				// Start new file
				callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesFromFilesDoneSoFar + bytesDone, totalSize})
				lastFilename = fileInProgress
				lastFileBytes = totalBytes
			}
		} else {
			if bytesDone == totalBytes {
				// finished
				bytesFromFilesDoneSoFar += totalBytes
				callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesFromFilesDoneSoFar, totalSize})
				lastFilename = ""
			} else {
				// Otherwise this is a progress callback
				return callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
					bytesFromFilesDoneSoFar + bytesDone, totalSize})
			}
		}
		return false
	}

	return provider.Upload(remoteName, filenames, basedir, force, localcallback)
}
