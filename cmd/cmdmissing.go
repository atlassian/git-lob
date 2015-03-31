package cmd

import (
	"bitbucket.org/sinbad/git-lob/core"
	"bitbucket.org/sinbad/git-lob/util"
	"strings"
)

// Missing command line tool
func Missing() int {

	// git-lob missing [--ignore-available] [--checkout] [path...]

	// Validate custom options
	errorList := validateCustomOptions(util.GlobalOptions, nil, []string{"ignore-available", "i", "checkout", "c"})
	if len(errorList) > 0 {
		util.LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	optIgnoreAvailable := util.GlobalOptions.BoolOpts.Contains("ignore-available") || util.GlobalOptions.BoolOpts.Contains("i")
	optCheckout := util.GlobalOptions.BoolOpts.Contains("checkout") || util.GlobalOptions.BoolOpts.Contains("c")

	var paths []string
	if len(util.GlobalOptions.Args) > 0 {
		paths = util.GlobalOptions.Args
	}

	anyErrors := false
	anyMissing := false
	callback := func(data *core.MissingCallbackData) (quit bool) {
		// Ensure we clear previous progress
		util.LogConsolef("\r")
		switch data.Type {
		case core.MissingAvailable:
			if !optIgnoreAvailable {
				util.LogConsolef("%v content is available, use checkout\n", data.Path)
				anyMissing = true
			}

		case core.MissingFixed:
			util.LogConsolef("%v checked out\n", data.Path)
			anyMissing = true
		case core.MissingModified:
			util.LogErrorf("%v is locally modified with no content, delete or reset/checkout to resolve\n", data.Path)
			anyMissing = true
		case core.MissingBlamed:
			util.LogConsolef("%v no content available\n", data.Path)
			util.LogConsolef("  Blame: %v(%v) [%v] %v\n", data.CommitSummary.CommitterName, data.CommitSummary.CommitterEmail,
				data.CommitSummary.ShortSHA, data.CommitSummary.Subject)
			anyMissing = true
		case core.MissingError:
			util.LogConsoleErrorf("Error: %v\n", data.Error.Error())
			anyErrors = true // still continue
			anyMissing = true
		case core.MissingWorking:
			util.LogConsoleDebugf("Checking %v\n", data.Path)
		}
		// Display progress always (fixed line width always large enough)
		util.LogConsoleSpinner("Searching: ")
		// Always continue
		return false
	}
	// Add newlines to messages since progress doesn't
	core.Missing(optCheckout, paths, callback)
	util.LogConsoleSpinnerFinish("Searching: ")
	if anyErrors {
		return 12
	}
	if anyMissing {
		util.LogConsole("Missing content exists, see messages above")
	} else {
		util.LogConsole("All file content OK")
	}
	return 0
}
func MissingHelp() {
	util.LogConsole(`Usage: git-lob missing [options] [path...]

  Checks the state of the working copy for binary placeholders which haven't
  been expanded to the full content. If the content isn't available locally,
  because the person who added this binary hasn't pushed the content, reports
  who committed this version & when so you can chase them up.

  By default checks the current folder and all subfolders, unless you supply
  one or more paths.

Parameters:
  path...       Optional list of paths to check instead of checking the whole
                working copy. paths are treated relative to the working
                directory, and wildcard matching is supported.

Options:
  --ignore-available, -i  Ignore placeholders where content is available 
                          locally, it just isn't checked out right now.
  --checkout, -c          If we find content available, expand the placeholder
                          to the full content as per 'git lob checkout'.
  --quiet, -q             Print less output
  --verbose, -v           Print more output

`)
}
