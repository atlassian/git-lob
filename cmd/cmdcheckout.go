package cmd

import (
	"bitbucket.org/sinbad/git-lob/core"
	"bitbucket.org/sinbad/git-lob/util"
	"path/filepath"
)

func Checkout() int {

	// git-lob checkout [options] [<pathspec>...]

	// no custom options
	optDryRun := util.GlobalOptions.DryRun

	// All extra arguments must be <pathspec>
	var pathspecs []string
	for _, arg := range util.GlobalOptions.Args {
		p := filepath.Clean(arg)
		pathspecs = append(pathspecs, p)
	}

	var filesCheckedOut int
	var filesFailed int
	var filesUpToDate int
	callback := func(t core.ProgressCallbackType, filelob *core.FileLOB, err error) {
		switch t {
		case core.ProgressSkip:
			filesUpToDate++
		case core.ProgressNotFound:
			util.LogConsole(err.Error())
			filesFailed++
		case core.ProgressError:
			util.LogConsoleError("ERROR:", err.Error())
			filesFailed++
		case core.ProgressTransferBytes:
			if optDryRun {
				util.LogConsoleDebug(filelob.Filename, "needs check out.")
			} else {
				util.LogConsoleDebug(filelob.Filename, "checked out.")
			}
			filesCheckedOut++
		}

	}

	err := core.Checkout(pathspecs, optDryRun, callback)

	if err != nil {
		util.LogConsoleErrorf("git-lob: checkout error - %v\n", err.Error())
		return 7
	}

	// Report final state
	if optDryRun {
		util.LogConsole(filesCheckedOut, "files need updating")
		if filesCheckedOut > 0 {
			util.LogConsole("Run this command again without --dry-run to update these files.")
		}
	} else {
		util.LogConsole(filesCheckedOut, "files were updated")
		if filesFailed > 0 {
			util.LogConsole("WARNING:", filesFailed, "failed to be updated, check errors above")
		}
	}

	if filesFailed > 0 {
		// non-zero error code when failures happened
		return 10
	}
	return 0
}

func CheckoutHelp() {
	util.LogConsole(`Usage: git-lob checkout [options] [<pathspec>...]

  Populate files in the working copy with binary content where they
  currently just have placeholder content, because the real content wasn't
  available.

  NOTE: You probably won't need to run this command yourself.

        Running 'git lob pull' will both fetch (download) AND checkout, so
        most of the time you should use 'git lob pull' instead. 

        Also 'git checkout' will populate the binary content correctly if
        you have it locally so you don't have to run this command after
        switching branches, unless you need to download extra content, in
        which case 'git lob pull' is once again a better bet.

  Because git-lob stores binary content separately from your git repository, 
  it's possible that when you perform a 'git checkout' or 'git clone', you did
  not have the binary content available locally to populate binary files in 
  your working copy. In this situation, git-lob creates placeholders in the
  working copy, whose content looks something like this:

  git-lob: <sha>

  Where <sha> is the identifier of the content of the binary file. Once you
  have downloaded the content (e.g. via 'git lob fetch'), you can then use
  'git lob checkout' to fill in these blanks.

  Specify <pathspec> to limit the checking to particular files or directories.

  Options:
    --quiet, -q   Print less output
    --verbose, -v Print more output
    --dry-run     Don't actually change any files, just report

`)
}
