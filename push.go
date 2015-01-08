package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Push command line tool
func cmdPush() int {

	// git-lob push [--all] [--recheck] [--force] [<remote> [<ref>...]]

	// Validate custom options
	errorList := validateCustomOptions(GlobalOptions, nil, []string{"all", "recheck", "force"})
	if len(errorList) > 0 {
		fmt.Fprintf(os.Stderr, strings.Join(errorList, "\n"))
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
				fmt.Fprintf(os.Stderr, "git-lob: Too many arguments; cannot include refspec when using --all\n")
				return 7
			}
			for _, arg := range GlobalOptions.Args[1:] {
				r := ParseGitRefSpec(arg)
				// Only allow .. range for push, not ...
				if r.RangeOp == "..." {
					fmt.Fprintf(os.Stderr, "git-lob: '...' range operator is not supported for push, only '..'\n")
					return 7
				} else if r.IsRange() && r.IsEmptyRange() {
					fmt.Fprintf(os.Stderr, "Warning: %v is an empty range, did you mean to use %v^..%v ?\n", r, r.Ref1, r.Ref2)
				}

				refspecs = append(refspecs, r)
			}
		}

	} else {
		remoteName = GetGitDefaultRemote()
	}

	// check the remote config to make sure it's valid
	provider, err := GetProviderForRemote(remoteName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "git-lob: %v\n", err)
		return 6
	}
	if err = provider.ValidateConfig(remoteName); err != nil {
		fmt.Fprintf(os.Stderr, "git-lob: remote %v has configuration problems:\n%v\n", remoteName, err)
		return 6
	}

	if len(refspecs) == 0 {
		// No refspecs specified, so determine default
		if optAll {
			branches, err := GetGitLocalBranches()
			if err != nil {
				fmt.Fprintf(os.Stderr, "git-lob: unable to get local branch list - %v\n", err)
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

	if !GlobalOptions.Quiet {
		fmt.Println("Pushing binaries for", refspecs, "to", remoteName)
	}

	// Warn about long calculation processes
	if optRecheck {
		fmt.Println("Re-checking all history as requested, this may take a while on large repos")
	} else if !HasPushedBinaryState(remoteName) {
		fmt.Println("No cached state for this remote, first time may take a while on large repos")
	}

	// Do the actual pushing in Goroutine, because we want to update the download rate & time estimates
	// on a regular schedule, regardless of whether any actual callbacks are received
	// If we only updated when callbacks happened (ie when data was transferred), if the data transfer halts
	// then we'd never update the rates / time estimates.

	var pusherr error

	// 100 items in the queue should be good enough, this means that it won't block
	callbackChan := make(chan *PushCallbackData, 100)
	go func(provider SyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, recheck bool,
		progresschan chan<- *PushCallbackData) {

		// Progress callback just passes the result back to the channel
		progress := func(data *PushCallbackData) (abort bool) {
			progresschan <- data

			return false
		}

		var err error
		switch p := provider.(type) {
		case BasicSyncProvider:
			err = PushBasic(p, remoteName, refspecs, dryRun, force, recheck, progress)
		case SmartSyncProvider:
			err = PushSmart(p, remoteName, refspecs, dryRun, force, recheck, progress)
		}

		close(progresschan)

		if err != nil {
			pusherr = err
		}

	}(provider, remoteName, refspecs, optDryRun, optForce, optRecheck, callbackChan)

	// Update the console once every half second regardless of how many callbacks
	// (or zero callbacks, so we can reduce xfer rate)
	tickChan := time.Tick(time.Millisecond * 500)
	// samples of data transferred over the last 4 ticks (2s average)
	transferRate := NewTransferRateCalculator(4)

	var lastTotalBytesDone int64
	var lastTime = time.Now()
	var lastProgress *PushCallbackData
	complete := false
	lastConsoleLineLen := 0
	for _ = range tickChan {
		// We run this every 0.5s
		var finalUploadProgress *PushCallbackData
		for stop := false; !stop && !complete; {
			select {
			case data := <-callbackChan:
				if data == nil {
					// channel was closed, we've finished
					stop = true
					complete = true
					break
				}

				// Some progress data is available
				// May get many of these and we only want to display the last one
				// unless it's general infoo or we're in verbose mode
				switch data.Type {
				case PushCallbackCalculate:
					finalUploadProgress = nil
					// Always print these if not quiet
					if !GlobalOptions.Quiet {
						fmt.Println(data.Desc)
					}
				case PushCallbackSkip:
					finalUploadProgress = nil
					// Only print if verbose
					if GlobalOptions.Verbose {
						fmt.Println("Skipped:", data.Desc, "(Up to date)")
					}
				case PushCallbackUpload:
					// Print completion in verbose mode
					if data.ItemBytesDone == data.ItemBytes && GlobalOptions.Verbose {
						msg := fmt.Sprintf("Pushed: %v 100%%", data.Desc)
						OverwriteConsoleLine(msg, lastConsoleLineLen, os.Stdout)
						lastConsoleLineLen = len(msg)
						// Clear line on completion in verbose mode
						// Don't do this as \n in string above since we need to clear spaces after
						fmt.Println()
					} else if !GlobalOptions.Quiet {
						// Otherwise we only really want to display the last one
						finalUploadProgress = data
					}
				}
			default:
				// No (more) progress data
				stop = true
			}
		}
		// Write progress data for this 0.5s if relevant
		// If either we have new progress data, or unfinished progress data from previous
		if finalUploadProgress != nil || lastProgress != nil {
			var bytesPerSecond int64
			if finalUploadProgress != nil && finalUploadProgress.ItemBytes != 0 && finalUploadProgress.TotalBytes != 0 {
				lastProgress = finalUploadProgress
				bytesDoneThisTick := finalUploadProgress.TotalBytesDone - lastTotalBytesDone
				lastTotalBytesDone = finalUploadProgress.TotalBytesDone
				seconds := float32(time.Since(lastTime).Seconds())
				if seconds > 0 {
					bytesPerSecond = int64(float32(bytesDoneThisTick) / seconds)
				}
			} else {
				// Actually the default but lets be specific
				bytesPerSecond = 0
			}
			// Calculate transfer rate
			transferRate.AddSample(bytesPerSecond)
			avgRate := transferRate.Average()

			if lastProgress.ItemBytes != 0 && lastProgress.TotalBytes != 0 {
				itemPercent := int((100 * lastProgress.ItemBytesDone) / lastProgress.ItemBytes)
				overallPercent := int((100 * lastProgress.TotalBytesDone) / lastProgress.TotalBytes)
				bytesRemaining := lastProgress.TotalBytes - lastProgress.TotalBytesDone
				secondsRemaining := bytesRemaining / avgRate
				timeRemaining := time.Duration(secondsRemaining) * time.Second
				var msg string
				if GlobalOptions.Verbose {
					msg = fmt.Sprintf("Pushing: %v %d%% Overall: %d%% (%v ETA %v)", lastProgress.Desc, itemPercent,
						overallPercent, FormatTransferRate(avgRate), timeRemaining)
				} else {
					msg = fmt.Sprintf("Pushing: %d%% (%v ETA %v)", overallPercent, FormatTransferRate(avgRate), timeRemaining)
				}
				OverwriteConsoleLine(msg, lastConsoleLineLen, os.Stdout)
				lastConsoleLineLen = len(msg)
			}
		}

		if complete {
			break
		}

	}

	if pusherr != nil {
		fmt.Fprintf(os.Stderr, "git-lob: push error - %v", err.Error())
		return 12
	}
	if !GlobalOptions.Quiet {
		if GlobalOptions.DryRun {
			fmt.Println("Done, run again without --dry-run to perform real push")
		} else {
			// Because no newlines in progress reporting
			fmt.Println()
			fmt.Println("Successfully pushed binaries to", remoteName)
		}
	}

	return 0
}

type PushCallbackType int

const (
	// Push process is figuring out what to push
	PushCallbackCalculate PushCallbackType = iota
	// Push process is transferring data
	PushCallbackUpload PushCallbackType = iota
	// Push process is skipping data because it's already up to date
	PushCallbackSkip PushCallbackType = iota
)

// Collected callback data for a push operation
type PushCallbackData struct {
	// What stage of the push process this is for, preparing, uploading or skipping something
	Type PushCallbackType
	// Either a general message or an item name (e.g. file name in upload stage)
	Desc string
	// If applicable, how many bytes transferred for this item
	ItemBytesDone int64
	// If applicable, how many bytes comprise this item
	ItemBytes int64
	// The number of bytes transferred for all items
	TotalBytesDone int64
	// The number of bytes needed to transfer all of this task
	TotalBytes int64
}

// Callback when progress is made during push
// return true to abort the (entire) process
type PushCallback func(data *PushCallbackData) (abort bool)

// Some temporary storage used to pre-calculate the amount of data we'll need to upload
type PushCommitContentDetails struct {
	CommitSHA  string   // the commit's SHA
	Files      []string // list of files we'll need to upload, relative path
	BaseDir    string   // the base dir of the above files
	TotalBytes int64    // total bytes for all files in the list
}

func PushBasic(provider BasicSyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, recheck bool,
	callback PushCallback) error {

	LogDebugf("Pushing to %v via %v\n", remoteName, provider.TypeID())

	// First, build up details of what it is we need to push so we can estimate %
	var allCommitsSize int64
	var commitsToPush []*PushCommitContentDetails
	for i, refspec := range refspecs {
		if GlobalOptions.Verbose {
			callback(&PushCallbackData{PushCallbackCalculate, fmt.Sprintf("Calculating data to push for %v", refspec),
				int64(i), int64(len(refspecs)), 0, 0})
		}
		refcommits, err := GetCommitLOBsToPushForRefSpec(remoteName, refspec, recheck)
		if err != nil {
			return err
		}

		var refCommitsSize int64

		if len(refcommits) == 0 {
			callback(&PushCallbackData{PushCallbackCalculate, fmt.Sprintf(" * %v: Nothing to push", refspec),
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
			callback(&PushCallbackData{PushCallbackCalculate, fmt.Sprintf(" * %v: %d commits with %v to push",
				refspec, len(refcommits), FormatSize(refCommitsSize)), int64(i + 1), int64(len(refspecs)), 0, 0})
		}

		if GlobalOptions.Verbose {
			callback(&PushCallbackData{PushCallbackCalculate, fmt.Sprintf("Finished calculating data to push for %v", refspec),
				int64(i + 1), int64(len(refspecs)), 0, 0})
		}
	}

	if !dryRun && len(commitsToPush) > 0 {
		filesdone := 0
		callback(&PushCallbackData{PushCallbackCalculate,
			fmt.Sprintf("Uploading %v to %v via %v", FormatSize(allCommitsSize), remoteName, provider.TypeID()),
			0, 0, 0, 0})

		var bytesFromFilesDoneSoFar int64
		for _, commit := range commitsToPush {
			// Upload now
			var lastFilename string
			var lastFileBytes int64
			localcallback := func(fileInProgress string, isSkipped bool, bytesDone, totalBytes int64) (abort bool) {
				if lastFilename != fileInProgress {
					// New file, always callback
					if isSkipped {
						filesdone++
						bytesFromFilesDoneSoFar += totalBytes
						callback(&PushCallbackData{PushCallbackSkip, fileInProgress, totalBytes, totalBytes,
							bytesFromFilesDoneSoFar, allCommitsSize})
					} else {
						if lastFilename != "" {
							// we obviously never got a 100% call for previous file
							filesdone++
							bytesFromFilesDoneSoFar += lastFileBytes
							callback(&PushCallbackData{PushCallbackUpload, lastFilename, lastFileBytes, lastFileBytes,
								bytesFromFilesDoneSoFar, allCommitsSize})
						}
						// Start new file
						callback(&PushCallbackData{PushCallbackUpload, fileInProgress, bytesDone, totalBytes,
							bytesFromFilesDoneSoFar + bytesDone, allCommitsSize})
					}
					lastFilename = fileInProgress
					lastFileBytes = totalBytes
				} else {
					if bytesDone == totalBytes {
						// finished
						filesdone++
						bytesFromFilesDoneSoFar += totalBytes
						callback(&PushCallbackData{PushCallbackUpload, fileInProgress, bytesDone, totalBytes,
							bytesFromFilesDoneSoFar, allCommitsSize})
						lastFilename = ""
					} else {
						// Otherwise this is a progress callback
						return callback(&PushCallbackData{PushCallbackUpload, fileInProgress, bytesDone, totalBytes,
							bytesFromFilesDoneSoFar + bytesDone, allCommitsSize})
					}
				}
				return false
			}
			err := provider.Upload(remoteName, commit.Files, commit.BaseDir, force, localcallback)
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

	return nil

}

func PushSmart(provider SmartSyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, recheck bool,
	callback PushCallback) error {
	// TODO
	return nil
}

func cmdPushHelp() {
	fmt.Println(`Usage: git-lob push [options] [<remote> [<ref>...]]

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
