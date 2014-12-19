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

	progress := func(t PushCallbackType, desc string, itempercent, overallpercent int, rate string) (abort bool) {
		switch t {
		case PushCallbackCalculate:
			if !GlobalOptions.Quiet {
				fmt.Println(desc)
			}
		case PushCallbackSkip:
			if GlobalOptions.Verbose {
				fmt.Println("Skipped:", desc, "(Up to date)")
			}
		case PushCallbackUpload:
			// Re-use the same line for each of these updates, except in verbose mode when we newline on completion
			if GlobalOptions.Verbose {
				// report file name too
				if itempercent == 100 {
					fmt.Printf("\rPushed: %v 100%%\n", desc)
				} else {
					fmt.Printf("\rPushing: %v %d%%\tOverall: %d%%\t(%v)", desc, itempercent, overallpercent, rate)
				}
			} else if !GlobalOptions.Quiet {
				fmt.Printf("\rPushing: %d%%\t(%v)", overallpercent, rate)
			}

		}

		return false

	}

	if !GlobalOptions.Quiet {
		fmt.Println("Pushing binaries for", refspecs, "to", remoteName)
	}

	var pusherr error
	switch p := provider.(type) {
	case BasicSyncProvider:
		pusherr = PushBasic(p, remoteName, refspecs, optDryRun, optForce, optRecheck, progress)
	case SmartSyncProvider:
		pusherr = PushSmart(p, remoteName, refspecs, optDryRun, optForce, optRecheck, progress)
	}

	if pusherr != nil {
		fmt.Fprintf(os.Stderr, "git-lob: push error - %v", err.Error())
		return 12
	}
	// Because no newlines in progress reporting
	if !GlobalOptions.Quiet {
		fmt.Println()
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

// Callback when progress is made during push
// t: what stage of the push process this is for, preparing, uploading or skipping something
// desc: either a general message or an item name (e.g. file name in upload stage)
// itempercent: if applicable, what percent of this item is done
// overallpercent: what overall percent of the process is done
// rate: upload rate for info if applicable
// return true to abort the (entire) process
type PushCallback func(t PushCallbackType, desc string, itempercent, overallpercent int, rate string) (abort bool)

func PushBasic(provider BasicSyncProvider, remoteName string, refspecs []*GitRefSpec, dryRun, force, recheck bool,
	callback PushCallback) error {

	LogDebugf("Pushing to %v via %v\n", remoteName, provider.TypeID())

	// First, build up details of what it is we need to push so we can estimate %
	numfiles := 0
	var commitsToPush []CommitLOBRef
	const calculatePercent = 10
	const uploadPercent = 100 - calculatePercent
	for i, refspec := range refspecs {
		callback(PushCallbackCalculate, fmt.Sprintf("Calculating data to push for %v", refspec),
			0, calculatePercent*i/len(refspecs), "")
		// TODO - goroutine each ref in parallel?
		refcommits, err := GetCommitLOBsToPushForRefSpec(remoteName, refspec, recheck)
		if err != nil {
			return err
		}

		if len(refcommits) == 0 {
			LogDebugf("Refspec %v: Nothing to push\n", refspec)
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
			oldnumfiles := numfiles
			for _, commit := range refcommits {
				numfiles += len(commit.lobSHAs)
			}
			LogDebugf("Refspec %v: %d commits with %d binaries to push\n",
				refspec, len(refcommits), numfiles-oldnumfiles)
			commitsToPush = append(commitsToPush, refcommits...)
		}
		callback(PushCallbackCalculate, fmt.Sprintf("Finished calculating data to push for %v", refspec),
			100, calculatePercent*(i+1)/len(refspecs), "")
	}

	if !dryRun {
		filesdone := 0
		percentStart := float32(calculatePercent)
		percentPerFile := float32(uploadPercent) / float32(numfiles)
		LogDebugf("Uploading %d files to %v via %v\n", numfiles, remoteName, provider.TypeID())

		for _, commit := range commitsToPush {
			filenames, basedir, err := GetLOBFilenamesWithBaseDir(commit.lobSHAs, true)
			if err != nil {
				return err
			} else {
				// Upload now
				// Use a local callback which calls the higher level callback at most every 1s
				lastCallbackTime := time.Now()
				var lastFilename string
				var percentFileStart float32
				localcallback := func(fileInProgress string, isSkipped bool, percent int) (abort bool) {
					if lastFilename != fileInProgress {
						percentFileStart = percentStart + percentPerFile*float32(filesdone)
						// New file, always callback
						if isSkipped {
							filesdone++
							callback(PushCallbackSkip, fileInProgress, 100, int(percentFileStart+percentPerFile), "")
						} else {
							if lastFilename != "" {
								// we obviously never got a 100% call for previous file
								filesdone++
								callback(PushCallbackUpload, lastFilename, 100, int(percentFileStart), "")
							}
							// Start new file
							callback(PushCallbackUpload, fileInProgress, 0, int(percentFileStart), "")
						}
						lastFilename = fileInProgress
					} else {
						if percent == 100 {
							// finished
							filesdone++
							callback(PushCallbackUpload, fileInProgress, 100, int(percentFileStart+percentPerFile), "")
							lastFilename = ""
						} else {
							// Otherwise this is a progress callback, don't post more than 1 per second for this type
							elapsed := time.Since(lastCallbackTime)
							if elapsed.Seconds() > 1.0 {
								lastCallbackTime = time.Now()
								return callback(PushCallbackUpload, fileInProgress, percent,
									int(percentFileStart+percentPerFile*float32(percent/100)), "")
							}
						}
					}
					return false
				}
				err = provider.Upload(remoteName, filenames, basedir, force, localcallback)
				if err != nil {
					// Stop at commit we can't upload
					return err
				}
				// Otherwise mark commit as pushed
				err = SuccessfullyPushedBinariesForCommit(remoteName, commit.commit)
				if err != nil {
					// Stop at commit we can't mark
					return err
				}
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
