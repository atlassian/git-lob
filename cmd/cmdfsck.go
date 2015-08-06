package cmd

import (
	"fmt"
	"strings"

	"github.com/atlassian/git-lob/core"
	"github.com/atlassian/git-lob/util"
)

// Fsck command line tool
func Fsck() int {

	// git-lob fsck [--deep] [--shared]

	// Validate custom options
	errorList := validateCustomOptions(util.GlobalOptions, nil, []string{"deep", "d", "shared", "s", "delete", "x"})
	if len(errorList) > 0 {
		util.LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	optDeep := util.GlobalOptions.BoolOpts.Contains("deep") || util.GlobalOptions.BoolOpts.Contains("d")
	optShared := util.GlobalOptions.BoolOpts.Contains("shared") || util.GlobalOptions.BoolOpts.Contains("s")
	optDelete := util.GlobalOptions.BoolOpts.Contains("delete") || util.GlobalOptions.BoolOpts.Contains("x")

	if optShared {
		// Check we have a shared store
		if util.GlobalOptions.SharedStore == "" {
			util.LogConsoleError("No shared store is configured for this repository, cannot use --shared")
			return 8
		}
		util.LogConsole("Checking shared store at", util.GlobalOptions.SharedStore)
	} else {
		util.LogConsole("Checking local binary store")
	}

	var shas []string
	if len(util.GlobalOptions.Args) > 0 {
		shas = util.GlobalOptions.Args
	}

	callback := func(data *core.FsckCallbackData) (quit bool) {
		// Ensure we clear previous progress
		util.LogConsolef("\r")
		switch data.Type {
		case core.FsckMissing:
			util.LogErrorf(" * %v: file is missing, try fetch/prune (%v)\n", data.SHA[:7], data.Desc)
		case core.FsckCorruptData:
			util.LogErrorf(" * %v: content is corrupt (deleted: %v)\n", data.SHA[:7], optDelete)
		case core.FsckWrongSize:
			util.LogErrorf(" * %v: file is wrong size (%v deleted: %v)\n", data.SHA[:7], data.Desc, optDelete)
		case core.FsckWorking:
			// Do nothing, just progress below
		}
		// Display progress always (fixed line width always large enough)
		util.LogConsoleOverwrite(fmt.Sprintf("Progress: %d%%", data.PercentComplete), 14)
		// Always continue
		return false
	}
	// Add newlines to messages since progress doesn't
	err := core.Fsck(optDeep, optShared, optDelete, shas, callback)
	if err != nil {
		util.LogConsoleError("\nError(s) in fsck, see above.")
		return 12
	}
	util.LogConsole("\nCompleted successfully, no problems found")
	return 0
}

func FsckHelp() {
	util.LogConsole(`Usage: git-lob fsck [options] [SHA...]

  Validates that the local binary store is internally consistent. 

  This utility command checks the contents of the local binary store to make
  sure that each binary stored there is complete & correct. The basic mode
  just ensures that all the required file components are there and are of the
  correct size, wheras --deep mode also checks every byte of the content
  to ensure it is correct (by checking that the SHA matches the content).

  If you're using a shared store across repos (see git-lob.sharedstore in 
  'git lob help config') and a missing local file is available in that shared 
  store, it will automatically be re-linked into your local repo to resolve the
  problem.

  The --delete option can be used to clean any files which are invalid. 
  Partially downloaded binaries where some chunks are missing are not deleted
  so you can resume downloading, but invalid files such as corrupt metadata, 
  incorrectly sized chunks, and content where the SHA doesn't agree (only
  checked with the --deep option) are deleted.

  This command doesn't check your working copy, use 'git lob missing' to check
  why a binary file is still a placeholder.

Parameters:
  SHA...        If you supply one or more 40-character SHA arguments, only
                those binaries are checked rather than the entire store. The
                SHA is the identifier of the binary content itself, not a Git
                object.

Options:
  --deep, -d    Recalculates the SHA of each binary file to ensure the contents
                are correct. Without this option, just checks that all files
                are present and the correct size.
  --shared, -s  Checks the shared store instead of the local repo
  --delete, -x  Delete files which are invalid. Doesn't delete partial binaries
                where 1 or more chunks are missing, but deletes files which are
                internally inconsistent; e.g. invalid meta files, partial 
                chunks, and all files where --deep is used and SHA doesn't 
                agree with content.
  --quiet, -q   Print less output
  --verbose, -v Print more output

`)
}
