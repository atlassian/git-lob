package cmd

import (
	"os"

	"github.com/atlassian/git-lob/core"
	"github.com/atlassian/git-lob/util"
)

func SmudgeFilter() int {
	// Make sure we never write log output to stdout, filter uses it for content
	util.LogAllConsoleOutputToStdErr()
	// Optional filename context that can be passed
	var filename string
	if len(util.GlobalOptions.Args) > 0 {
		filename = util.GlobalOptions.Args[0]
	} else {
		filename = "[Unknown filename]"
	}
	return core.SmudgeFilterWithReaderWriter(os.Stdin, os.Stdout, filename)
}
func CleanFilter() int {
	// Make sure we never write log output to stdout, filter uses it for content
	util.LogAllConsoleOutputToStdErr()
	// Optional filename context that can be passed
	var filename string
	if len(util.GlobalOptions.Args) > 0 {
		filename = util.GlobalOptions.Args[0]
	} else {
		filename = "[Unknown filename]"
	}
	return core.CleanFilterWithReaderWriter(os.Stdin, os.Stdout, filename)
}

func SmudgeFilterHelp() {
	util.LogConsole(`Usage: git-lob filter-smudge [options] <filename>

  The smudge filter converts a file stored in git to a file in the working
  directory. In this case we look for files containing the git-lob marker
  and replace the content with real binary data from the binary store.

  Not intended to be called directly, see README.md for how to configure
  the filter for your repository.

Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't actually delete anything, just report
`)
}
func CleanFilterHelp() {
	util.LogConsole(`Usage: git-lob filter-clean [options] <filename>

  The clean filter converts a file in the working directory to a form which
  will be stored in git. In this case we calculate the SHA-1 of the binary
  content and write this to git, while storing the real data in the separate
  binary store.

  Not intended to be called directly, see README.md for how to configure
  the filter for your repository.

Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't actually delete anything, just report
`)
}
