package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func cmdPrune() int {
	errorList := validateCustomOptions(GlobalOptions, nil, []string{"unreferenced"})
	if len(errorList) > 0 {
		LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	optOnlyUnreferenced := GlobalOptions.BoolOpts.Contains("unreferenced")

	var shas []string
	var err error
	if optOnlyUnreferenced {
		// Only purge unreferenced
		shas, err = PruneUnreferenced(GlobalOptions.DryRun)
		if err != nil {
			LogErrorf("Prune failed: %v\n", err)
			return 3
		}
	} else {
		// Purge old & unreferenced

	}
	if GlobalOptions.DryRun {
		if GlobalOptions.Verbose {
			LogConsolef("%d LOBs would have been deleted:\n", len(shas))
			LogConsole(strings.Join(shas, "\n"))
		} else {
			LogConsolef("%d LOBs would have been deleted.\n", len(shas))
		}
		LogConsole("Run command again without --dry-run to actually perform the deletion.")
	} else {
		if GlobalOptions.Verbose {
			LogConsolef("%d LOBs were deleted:\n", len(shas))
			LogConsoleDebug(strings.Join(shas, "\n"))
		} else {
			LogConsolef("%d LOBs were deleted.\n", len(shas))
		}
	}

	return 0

}

func cmdPruneShared() int {
	shas, err := PruneSharedStore(GlobalOptions.DryRun)
	if err != nil {
		LogErrorf("Prune failed: %v\n", err)
		return 3
	}
	if GlobalOptions.DryRun {
		if GlobalOptions.Verbose {
			LogConsolef("%d LOBs would have been deleted:\n", len(shas))
			LogConsole(strings.Join(shas, "\n"))
		} else {
			LogConsolef("%d LOBs would have been deleted.\n", len(shas))
		}
		LogConsole("Run command again without --dry-run to actually perform the deletion.")
	} else {
		if GlobalOptions.Verbose {
			LogConsolef("%d LOBs were deleted:\n", len(shas))
			LogConsoleDebug(strings.Join(shas, "\n"))
		} else {
			LogConsolef("%d LOBs were deleted.\n", len(shas))
		}
	}
	return 0
}

func cmdPruneHelp() {
	LogConsole(`Usage: git-lob prune [options]

  Removes old and unreferenced binaries from local storage.

  A binary will NOT BE PRUNED if:
    1. It is referenced by a reachable commit which is 'recent' as defined by 
       what 'git lob fetch' would download, OR
    2. It is referenced by a commit for which the binaries haven't been pushed

  To put that another way, a binary WILL BE PRUNED if:
    1. It is not referenced by any reachable commit, or only by a reachable 
       commit which is not 'recent' as defined by 'git lob fetch'
    2. If referenced by an older commit, it has been pushed (i.e. the local
    	copy is not the only one)

  The meaning of 'reachable commit' is as defined in Git, that the commit would
  be found by working backwards from at least one reference (e.g. branch/tag).

  See 'git lob fetch --help' to see the definition of 'recent commit' and the
  settings which control it.

Options:
  --unreferenced       Only prune totally unreferenced binaries, not old ones
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't actually delete anything, just report

SHARED STORE
  If you are using a shared store, when a file is pruned locally, if there 
  are no other repos referencing this binary file then it is also deleted 
  from the shared store.

  If you manually deleted a repository and want to only clean up the shared
  store, use 'git lob prune-shared'
`)
}

func cmdPruneSharedHelp() {
	LogConsole(`Usage: git-lob prune-shared [options]

  Removes binaries from the shared store which are no longer linked to by any
  repo. 

  Usually 'git-lob prune' will delete files from the shared store too once
  the last repo link is removed, but if you manually delete repositories then
  this won't happen. prune-shared deletes any binaries in the shared
  store which have no other links left in the file system. This is relatively
  quick compared to the repo prune since it doesn't require checking any
  git repos.
  
Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't actually delete anything, just report
`)
}

var (
	diffLOBReferenceRegex *regexp.Regexp
	lobFilenameRegex      *regexp.Regexp
)

// Retrieve the full set of SHAs that currently have files locally (complete or not)
// returned as map[string]bool for fast lookup
func getAllLocalLOBSHAs() (StringSet, error) {
	return getAllLOBSHAsInDir(GetLocalLOBRoot())
}

// Retrieve the full set of SHAs that currently have files in the shared store (complete or not)
// returned as map[string]bool for fast lookup
func getAllSharedLOBSHAs() (StringSet, error) {
	return getAllLOBSHAsInDir(GetSharedLOBRoot())
}

func getAllLOBSHAsInDir(lobroot string) (StringSet, error) {

	// os.File.Readdirnames is the most efficient
	// os.File.Readdir retrieves extra info we don't usually need but in case other unexpected files
	// end up in there (e.g. .DS_Store), we use it to identify directories
	// ioutil.ReadDir and filepath.Walk do sorting which is unnecessary & inefficient

	if lobFilenameRegex == nil {
		lobFilenameRegex = regexp.MustCompile(`^([A-Za-z0-9]{40})_(meta|\d+)$`)
	}
	set := NewStringSet()

	// We only need to support a 2-folder structure here & know that all files are at the bottom level
	// We always work on the local LOB folder (either only copy or hard link)
	rootf, err := os.Open(lobroot)
	if err != nil {
		return set, errors.New(fmt.Sprintf("Unable to open LOB root: %v\n", err))
	}
	dir1, err := rootf.Readdir(0)
	if err != nil {
		return set, errors.New(fmt.Sprintf("Unable to read first level LOB dir: %v\n", err))
	}
	for _, dir1fi := range dir1 {
		if dir1fi.IsDir() {
			dir1path := filepath.Join(lobroot, dir1fi.Name())
			dir1f, err := os.Open(dir1path)
			if err != nil {
				return set, errors.New(fmt.Sprintf("Unable to open LOB dir: %v\n", err))
			}
			dir2, err := dir1f.Readdir(0)
			if err != nil {
				return set, errors.New(fmt.Sprintf("Unable to read second level LOB dir: %v\n", err))
			}
			for _, dir2fi := range dir2 {
				if dir2fi.IsDir() {
					dir2path := filepath.Join(dir1path, dir2fi.Name())
					dir2f, err := os.Open(dir2path)
					if err != nil {
						return set, errors.New(fmt.Sprintf("Unable to open LOB dir: %v\n", err))
					}
					lobnames, err := dir2f.Readdirnames(0)
					if err != nil {
						return set, errors.New(fmt.Sprintf("Unable to read innermost LOB dir: %v\n", err))
					}
					for _, lobname := range lobnames {
						// Make sure it's really a LOB file
						if match := lobFilenameRegex.FindStringSubmatch(lobname); match != nil {
							// Regex pulls out the SHA
							sha := match[1]
							set.Add(sha)
						}
					}

				}
			}
		}

	}

	return set, nil

}

// Determine if a line from git diff output is referencing a LOB (returns "" if not)
func lobReferenceFromDiffLine(line string) string {
	// Because this is a diff, it will start with +/-
	// We only care about +, since - is stopping referencing a SHA
	// important when it comes to purging old files
	if diffLOBReferenceRegex == nil {
		diffLOBReferenceRegex = regexp.MustCompile(`^\+git-lob: ([A-Za-z0-9]{40})$`)
	}

	if match := diffLOBReferenceRegex.FindStringSubmatch(line); match != nil {
		return match[1]
	}
	return ""
}

// Delete unreferenced binary files from local store
// For a file to be deleted it needs to not be referenced by any (reachable) commit
// Returns a list of SHAs that were deleted (unless dryRun = true)
func PruneUnreferenced(dryRun bool) ([]string, error) {
	// Purging requires full git on the command line, no way around this really
	cmd := exec.Command("git", "log", "--all", "--no-color", "--oneline", "-p", "-G", SHALineRegex)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return make([]string, 0), errors.New("Unable to query git log for binary references: " + err.Error())
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return make([]string, 0), errors.New("Unable to open pipe: " + err.Error())
	}
	multi := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(multi)
	cmd.Start()
	referencedSHAs := NewStringSet()
	for scanner.Scan() {
		line := scanner.Text()
		if sha := lobReferenceFromDiffLine(line); sha != "" {
			referencedSHAs.Add(sha)
		}
	}
	cmd.Wait()

	// Must also not prune anything that's added but uncommitted
	cmd = exec.Command("git", "diff", "--cached", "--no-color", "-G", SHALineRegex)
	stdout, err = cmd.StdoutPipe()
	if err != nil {
		return make([]string, 0), errors.New("Unable to query git index for binary references: " + err.Error())
	}
	scanner = bufio.NewScanner(stdout)
	cmd.Start()
	for scanner.Scan() {
		line := scanner.Text()
		if sha := lobReferenceFromDiffLine(line); sha != "" {
			referencedSHAs.Add(sha)
		}
	}
	cmd.Wait()

	fileSHAs, err := getAllLocalLOBSHAs()
	if err == nil {

		toDelete := fileSHAs.Difference(referencedSHAs)
		ret := make([]string, 0, len(toDelete))
		for sha := range toDelete.Iter() {
			ret = append(ret, string(sha))
			if !dryRun {
				DeleteLOB(string(sha))
			}
		}
		return ret, nil
	} else {
		return make([]string, 0), errors.New("Unable to get list of binary files: " + err.Error())
	}

}

// Remove LOBs from the local store if they fall outside the range we would normally fetch for
func PruneOld(dryRun bool) {
	// TODO
	LogConsole("PSA: Prune functionality is not implemented yet")
}

// Prune the shared store of all LOBs with only 1 hard link (itself)
// DeleteLOB will do this for individual LOBs we prune, but if the user
// manually deletes a repo then unreferenced shared LOBs may never be cleaned up
func PruneSharedStore(dryRun bool) ([]string, error) {
	fileSHAs, err := getAllSharedLOBSHAs()
	if err == nil {
		ret := make([]string, 0, 10)
		for sha := range fileSHAs.Iter() {
			shareddir := GetSharedLOBDir(sha)
			names, err := filepath.Glob(filepath.Join(shareddir, fmt.Sprintf("%v*", sha)))
			if err != nil {
				return make([]string, 0), errors.New(fmt.Sprintf("Unable to glob shared files for %v: %v\n", sha, err))
			}
			var deleted bool = false
			for _, n := range names {
				links, err := GetHardLinkCount(n)
				if err == nil && links == 1 {
					// only 1 hard link means no other repo refers to this shared LOB
					// so it's safe to delete it
					deleted = true
					if !dryRun {
						err = os.Remove(n)
						if err != nil {
							// don't abort for 1 failure, report & carry on
							LogErrorf("Unable to delete file %v: %v\n", n, err)
						}
						LogDebugf("Deleted shared file %v\n", n)
					}
				}

			}
			if deleted {
				ret = append(ret, string(sha))
			}
		}
		return ret, nil
	} else {
		return make([]string, 0), err
	}

}
