package core

import (
	"bitbucket.org/sinbad/git-lob/providers"
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
)

// Some temporary storage used to pre-calculate the amount of data we'll need to upload
type PushCommitContentDetails struct {
	CommitSHA  string   // the commit's SHA
	Files      []string // list of files we'll need to upload, relative path
	BaseDir    string   // the base dir of the above files
	TotalBytes int64    // total bytes for all files in the list
	Incomplete bool     // File list is not complete because of missing local data, we shouldn't mark this commit as pushed
}

func Push(provider providers.SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, recheck bool,
	callback util.ProgressCallback) error {

	util.LogDebugf("Pushing to %v via %v\n", remoteName, provider.TypeID())

	// for use when --force used
	filesUploaded := util.NewStringSet()
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

		var refCommitsSize int64

		// First we walk the commits to push & build up a picture of size etc
		walkFunc := func(commit *CommitLOBRef) (quit bool, err error) {
			filenames, basedir, totalSize, err := GetLOBFilenamesWithBaseDir(commit.LobSHAs, true)
			commitIncomplete := false
			if err != nil {
				var problemSHAs []string
				switch errSpec := err.(type) {
				case *NotFoundForSHAsError:
					problemSHAs = errSpec.SHAsNotFound
				case *IntegrityError:
					return true, err
				default:
					return true, err
				}
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
				Files:      filenames,
				BaseDir:    basedir,
				TotalBytes: totalSize,
				Incomplete: commitIncomplete})

			refCommitsSize += totalSize

			return false, nil
		}

		err := WalkGitCommitLOBsToPushForRefSpec(remoteName, refspec, recheck, walkFunc)
		if err != nil {
			return err
		}

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
			filesdone := 0

			// Even if size == 0 we still skim through marking them as pushed (must have been that data was on remote)
			if refCommitsSize > 0 {
				callback(&util.ProgressCallbackData{util.ProgressCalculate,
					fmt.Sprintf("Uploading up to %v to %v via %v", util.FormatSize(refCommitsSize), remoteName, provider.TypeID()),
					0, 0, 0, 0})
			}

			var bytesFromFilesDoneSoFar int64
			previousCommitIncomplete := false
			previousCommitSHA := ""
			for _, commit := range refCommitsToPush {
				// Upload now
				var lastFilename string
				var lastFileBytes int64
				localcallback := func(fileInProgress string, progressType util.ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
					if lastFilename != fileInProgress {
						// New file, always callback
						if lastFilename != "" {
							// we obviously never got a 100% call for previous file
							filesdone++
							bytesFromFilesDoneSoFar += lastFileBytes
							callback(&util.ProgressCallbackData{util.ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
								bytesFromFilesDoneSoFar, refCommitsSize})
							lastFilename = ""
						}
						if progressType == util.ProgressSkip || progressType == util.ProgressNotFound {
							// 'not found' will have caused an error earlier anyway so just pass through
							filesdone++
							bytesFromFilesDoneSoFar += totalBytes
							callback(&util.ProgressCallbackData{progressType, fileInProgress, totalBytes, totalBytes,
								bytesFromFilesDoneSoFar, refCommitsSize})
						} else {
							// Start new file
							callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
								bytesFromFilesDoneSoFar + bytesDone, refCommitsSize})
							lastFilename = fileInProgress
							lastFileBytes = totalBytes
						}
					} else {
						if bytesDone == totalBytes {
							// finished
							filesdone++
							bytesFromFilesDoneSoFar += totalBytes
							callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
								bytesFromFilesDoneSoFar, refCommitsSize})
							lastFilename = ""
						} else {
							// Otherwise this is a progress callback
							return callback(&util.ProgressCallbackData{util.ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
								bytesFromFilesDoneSoFar + bytesDone, refCommitsSize})
						}
					}
					return false
				}
				var err error
				if force {
					// We can end up duplicating uploads when in force mode because the underlying provider will not
					// stop the upload in force mode if it's already there. So instead, make sure we only upload each
					// file at most once
					commitFilesSet := util.NewStringSetFromSlice(commit.Files)
					newFilesSet := commitFilesSet.Difference(filesUploaded)
					if len(newFilesSet) > 0 {
						newFiles := make([]string, 0, len(newFilesSet))
						for f := range newFilesSet {
							newFiles = append(newFiles, f)
						}
						err = provider.Upload(remoteName, newFiles, commit.BaseDir, force, localcallback)
						if err == nil {
							for f := range newFilesSet {
								filesUploaded.Add(f)
							}
						}
					}
				} else {
					// It IS possible to have a commit here with no files to upload. E.g. missing data locally (see above)
					// which was present on remote. We still include it in the commit list for completeness
					if len(commit.Files) > 0 {
						err = provider.Upload(remoteName, commit.Files, commit.BaseDir, force, localcallback)
					}
				}
				if err != nil {
					// Stop at commit we can't upload
					return err
				}
				if lastFilename != "" {
					// We obviously never got a 100% progress update from the last file
					bytesFromFilesDoneSoFar += lastFileBytes
					callback(&util.ProgressCallbackData{util.ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
						bytesFromFilesDoneSoFar, refCommitsSize})
					lastFilename = ""
				}
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

// Push a single LOB to a remote
func PushSingle(sha string, provider providers.SyncProvider, remoteName string, force bool,
	callback util.ProgressCallback) error {

	filenames, basedir, totalSize, err := GetLOBFilenamesWithBaseDir([]string{sha}, true)
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

func cmdPushHelp() {
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
func cmdPushLobHelp() {
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
