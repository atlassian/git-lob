package main

import (
	"fmt"
	"strings"
)

func cmdCleanup() int {
	files, err := PurgeUnreferenced(GlobalOptions.DryRun)
	if err != nil {
		LogErrorf("Cleanup failed: %v\n", err)
		return 3
	}
	if GlobalOptions.DryRun {
		fmt.Println("LOBs which would have been deleted:")
		fmt.Println(strings.Join(files, "\n"))
	} else {
		LogDebug("Deleted LOBs:")
		LogDebug(strings.Join(files, "\n"))
	}
	return 0

}

func cmdCleanupShared() int {
	files, err := PurgeSharedStore(GlobalOptions.DryRun)
	if err != nil {
		LogErrorf("Cleanup failed: %v\n", err)
		return 3
	}
	if GlobalOptions.DryRun {
		fmt.Println("LOBs which would have been deleted:")
		fmt.Println(strings.Join(files, "\n"))
	} else {
		LogDebug("Deleted LOBs:")
		LogDebug(strings.Join(files, "\n"))
	}
	return 0
}

func cmdCleanupHelp() {
	fmt.Println(`Usage: git-lob cleanup [options]

  Removes binaries unreferenced by any commit or the index from the local repo
  binary store (and shared if no other usage).

  To do this, git-lob scans all reachable commits and your staged changes, then
  deletes any files in the binary store not referenced by one of these. If your
  repository is quite large, this might take a little time.

  If you are using a shared store, then once the local repo's hard link is
  deleted, if there are no other repos referencing this binary file then it is
  also deleted from the shared store.

Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't actually delete anything, just report
`)
}

func cmdCleanupSharedHelp() {
	fmt.Println(`Usage: git-lob cleanup-shared [options]

  Removes binaries from the shared store which are no longer linked to by any
  repo. 

  Usually 'git-lob cleanup' will delete files from the shared store too once
  the last repo link is removed, but if you manually delete repositories then
  this won't happen. cleanup-shared deletes any binaries in the shared
  store which have no other links left in the file system. This is relatively
  quick compared to the repo cleanup since it doesn't require checking any
  git repos.
  
Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't actually delete anything, just report
`)
}
