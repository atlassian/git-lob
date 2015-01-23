package main

import (
	"fmt"
)

func cmdPull() int {
	fetchret := cmdFetch()
	if fetchret != 0 {
		// Fetch failed, abort
		return fetchret
	}
	// Now run checkout but with no args
	oldArgs := GlobalOptions.Args
	GlobalOptions.Args = []string{}
	ret := cmdCheckout()
	GlobalOptions.Args = oldArgs

	return ret

}

func cmdPullHelp() {
	fmt.Println(`Usage: git-lob pull [options] [<remote> [<ref>...]]

  Run the 'git lob fetch' command with the same parameters to download binaries
  from a remote store, followed by 'git lob checkout' to populate the working 
  copy with any binary content which your working copy references, but which 
  wasn't available previously.

  You probably want to run this command after cloning a git-lob enabled git
  repository. You may also need to run it after the standard 'git pull' to
  obtain new binaries you don't already have, or if you check out an old commit
  which was previously outside the 'recent' range that 'git lob fetch'
  would usually download when you're on the default branch.

  See 'git lob fetch --help' for full details of the options & parameters you
  can pass to this command, they are the same. Also see
  'git lob checkout --help' for information on how the second stage works.

`)
}
