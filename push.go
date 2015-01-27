package main

import (
	"fmt"
	"strings"
	"time"
)

// Push command line tool
func cmdPush() int {

	// git-lob push [--all] [--recheck] [--force] [<remote> [<ref>...]]

	// Validate custom options
	errorList := validateCustomOptions(GlobalOptions, nil, []string{"all", "recheck", "force"})
	if len(errorList) > 0 {
		LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	optAll := GlobalOptions.BoolOpts.Contains("all")
	optRecheck := GlobalOptions.BoolOpts.Contains("recheck")
	optForce := GlobalOptions.BoolOpts.Contains("force")
	optDryRun := GlobalOptions.DryRun

	// Determine remote
	var remoteName string
	var refspecs []*GitRefSpec
	if len(GlobalOptions.Args) > 0 {
		// first parameter must be remote if there are arguments
		remoteName = GlobalOptions.Args[0]

		// Remaining args are refspecs
		if len(GlobalOptions.Args) > 1 {
			// Not valid if --all
			if optAll {
				LogConsoleError("git-lob: Too many arguments; cannot include refspec when using --all")
				return 7
			}
			for _, arg := range GlobalOptions.Args[1:] {
				r := ParseGitRefSpec(arg)
				// Only allow .. range for push, not ...
				if r.RangeOp == "..." {
					LogConsoleError("git-lob: '...' range operator is not supported for push, only '..'")
					return 7
				} else if r.IsRange() && r.IsEmptyRange() {
					LogConsoleErrorf("Warning: %v is an empty range, did you mean to use %v^..%v ?\n", r, r.Ref1, r.Ref2)
				}

				refspecs = append(refspecs, r)
			}
		}

	} else {
		remoteName = GetGitDefaultRemoteForPush()
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

	if len(refspecs) == 0 {
		// No refspecs specified, so determine default
		if optAll {
			branches, err := GetGitLocalBranches()
			if err != nil {
				LogConsoleErrorf("git-lob: unable to get local branch list - %v\n", err)
				return 7
			}
			for _, branch := range branches {
				refspecs = append(refspecs, &GitRefSpec{branch, "", ""})
			}

		} else {
			// Check remote.<remote>.push for default refspec
			def, ok := GlobalOptions.GitConfig[fmt.Sprintf("remote.%v.push", remoteName)]
			if ok {
				pushspecs := strings.Fields(def)
				for _, s := range pushspecs {
					refspecs = append(refspecs, &GitRefSpec{s, "", ""})
				}
			} else {
				// determine refspec from current branch & push settings
				branches := GetGitPushDefaultBranches(remoteName)
				for _, s := range branches {
					refspecs = append(refspecs, &GitRefSpec{s, "", ""})
				}
			}
		}
	}

	LogConsole("Pushing binaries for", refspecs, "to", remoteName)

	// Warn about long calculation processes
	if optRecheck {
		LogConsole("Re-checking all history as requested, this may take a while on large repos")
	} else if !HasPushedBinaryState(remoteName) {
		LogConsole("No cached state for this remote, first time may take a while on large repos")
	}

	// Do the actual pushing in Goroutine, because we want to update the download rate & time estimates
	// on a regular schedule, regardless of whether any actual callbacks are received
	// If we only updated when callbacks happened (ie when data was transferred), if the data transfer halts
	// then we'd never update the rates / time estimates.

	var pusherr error

	// 100 items in the queue should be good enough, this means that it won't block
	callbackChan := make(chan *ProgressCallbackData, 100)
	go func(provider SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, recheck bool,
		progresschan chan<- *ProgressCallbackData) {

		// Progress callback just passes the result back to the channel
		progress := func(data *ProgressCallbackData) (abort bool) {
			progresschan <- data

			return false
		}

		var err error
		switch p := provider.(type) {
		case SmartSyncProvider:
			err = PushSmart(p, remoteName, refspecs, dryRun, force, recheck, progress)
		default:
			err = PushBasic(p, remoteName, refspecs, dryRun, force, recheck, progress)
		}

		close(progresschan)

		if err != nil {
			pusherr = err
		}

	}(provider, remoteName, refspecs, optDryRun, optForce, optRecheck, callbackChan)

	// Update the console once every half second regardless of how many callbacks
	// (or zero callbacks, so we can reduce xfer rate)
	ReportProgressToConsole(callbackChan, "Push", time.Millisecond*500)

	if pusherr != nil {
		LogConsoleErrorf("git-lob: push error - %v", pusherr.Error())
		return 12
	}
	if GlobalOptions.DryRun {
		LogConsole("Done, run again without --dry-run to perform real push")
	} else {
		// Because no newlines in progress reporting
		LogConsole()
		LogConsole("Successfully pushed binaries to", remoteName)
	}

	return 0
}

// Some temporary storage used to pre-calculate the amount of data we'll need to upload
type PushCommitContentDetails struct {
	CommitSHA  string   // the commit's SHA
	Files      []string // list of files we'll need to upload, relative path
	BaseDir    string   // the base dir of the above files
	TotalBytes int64    // total bytes for all files in the list
}

func PushBasic(provider SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, recheck bool,
	callback ProgressCallback) error {

	LogDebugf("Pushing to %v via %v\n", remoteName, provider.TypeID())

	// First, build up details of what it is we need to push so we can estimate %
	var allCommitsSize int64
	var commitsToPush []*PushCommitContentDetails
	// for use when --force used
	filesUploaded := NewStringSet()
	for i, refspec := range refspecs {
		if GlobalOptions.Verbose {
			callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf("Calculating data to push for %v", refspec),
				int64(i), int64(len(refspecs)), 0, 0})
		}
		refcommits, err := GetCommitLOBsToPushForRefSpec(remoteName, refspec, recheck)
		if err != nil {
			return err
		}

		var refCommitsSize int64

		if len(refcommits) == 0 {
			callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf(" * %v: Nothing to push", refspec),
				int64(i), int64(len(refspecs)), 0, 0})
			// if nothing to push, then mark this ref as pushed to make querying faster next time
			if !dryRun {
				var commitSHA string
				var err error
				if refspec.IsRange() {
					commitSHA, err = GitRefToFullSHA(refspec.Ref2)
				} else {
					commitSHA, err = GitRefToFullSHA(refspec.Ref1)
				}
				if err != nil {
					return err
				}
				SuccessfullyPushedBinariesForCommit(remoteName, commitSHA)
			}

		} else {
			for _, commit := range refcommits {
				filenames, basedir, totalSize, err := GetLOBFilenamesWithBaseDir(commit.lobSHAs, true)
				if err != nil {
					return err
				}
				commitsToPush = append(commitsToPush, &PushCommitContentDetails{
					CommitSHA:  commit.commit,
					Files:      filenames,
					BaseDir:    basedir,
					TotalBytes: totalSize})

				refCommitsSize += totalSize
				allCommitsSize += totalSize
			}
			callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf(" * %v: %d commits with %v to push",
				refspec, len(refcommits), FormatSize(refCommitsSize)), int64(i + 1), int64(len(refspecs)), 0, 0})
		}

		if GlobalOptions.Verbose {
			callback(&ProgressCallbackData{ProgressCalculate, fmt.Sprintf("Finished calculating data to push for %v", refspec),
				int64(i + 1), int64(len(refspecs)), 0, 0})
		}
	}

	if !dryRun && len(commitsToPush) > 0 {
		filesdone := 0
		callback(&ProgressCallbackData{ProgressCalculate,
			fmt.Sprintf("Uploading %v to %v via %v", FormatSize(allCommitsSize), remoteName, provider.TypeID()),
			0, 0, 0, 0})

		var bytesFromFilesDoneSoFar int64
		for _, commit := range commitsToPush {
			// Upload now
			var lastFilename string
			var lastFileBytes int64
			localcallback := func(fileInProgress string, progressType ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
				if lastFilename != fileInProgress {
					// New file, always callback
					if progressType == ProgressSkip || progressType == ProgressNotFound {
						// 'not found' will cause an error anyway so just pass through
						filesdone++
						bytesFromFilesDoneSoFar += totalBytes
						callback(&ProgressCallbackData{progressType, fileInProgress, totalBytes, totalBytes,
							bytesFromFilesDoneSoFar, allCommitsSize})
					} else {
						if lastFilename != "" {
							// we obviously never got a 100% call for previous file
							filesdone++
							bytesFromFilesDoneSoFar += lastFileBytes
							callback(&ProgressCallbackData{ProgressTransferBytes, lastFilename, lastFileBytes, lastFileBytes,
								bytesFromFilesDoneSoFar, allCommitsSize})
						}
						// Start new file
						callback(&ProgressCallbackData{ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
							bytesFromFilesDoneSoFar + bytesDone, allCommitsSize})
					}
					lastFilename = fileInProgress
					lastFileBytes = totalBytes
				} else {
					if bytesDone == totalBytes {
						// finished
						filesdone++
						bytesFromFilesDoneSoFar += totalBytes
						callback(&ProgressCallbackData{ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
							bytesFromFilesDoneSoFar, allCommitsSize})
						lastFilename = ""
					} else {
						// Otherwise this is a progress callback
						return callback(&ProgressCallbackData{ProgressTransferBytes, fileInProgress, bytesDone, totalBytes,
							bytesFromFilesDoneSoFar + bytesDone, allCommitsSize})
					}
				}
				return false
			}
			var err error
			if force {
				// We can end up duplicating uploads when in force mode because the underlying provider will not
				// stop the upload in force mode if it's already there. So instead, make sure we only upload each
				// file at most once
				commitFilesSet := NewStringSetFromSlice(commit.Files)
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
				err = provider.Upload(remoteName, commit.Files, commit.BaseDir, force, localcallback)
			}
			if err != nil {
				// Stop at commit we can't upload
				return err
			}
			// Otherwise mark commit as pushed
			err = SuccessfullyPushedBinariesForCommit(remoteName, commit.CommitSHA)
			if err != nil {
				// Stop at commit we can't mark
				return err
			}
		}
	}

	LogDebugf("Successfully pushed to %v via %v\n", remoteName, provider.TypeID())
	return nil

}

func PushSmart(provider SmartSyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, recheck bool,
	callback ProgressCallback) error {
	// TODO
	return nil
}

func cmdPushHelp() {
	LogConsole(`Usage: git-lob push [options] [<remote> [<ref>...]]

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
  --all         Push all branches; cannot be used with other refs.
  --recheck     Re-check entire commit history to each ref instead of only 
                back to last commit we believe is already pushed. 
                See HISTORY CHECKING below for more details.
  --force       Always upload files even if the provider believes the file is 
                already present on the remote. You shouldn't need this.
  --quiet, -q   Print less output
  --verbose, -v Print more output
  --dry-run     Don't actually push anything, just report

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

`)
}
