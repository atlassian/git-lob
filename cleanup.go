package main

import (
	"strings"
)

func Cleanup() int {

	files, err := PurgeUnreferenced(GlobalOptions.DryRun)
	if err != nil {
		LogErrorf("Cleanup failed: %v\n", err)
		return 3
	}
	if GlobalOptions.DryRun {
		LogDebug("LOBs which would have been deleted:")
		LogDebug(strings.Join(files, "\n"))
	} else {
		LogDebug("Deleted LOBs:")
		LogDebug(strings.Join(files, "\n"))
	}
	return 0

}
