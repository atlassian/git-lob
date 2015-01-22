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
)

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
		LogErrorf("Unable to open LOB root: %v\n", err)
		return set, err
	}
	dir1, err := rootf.Readdir(0)
	if err != nil {
		LogErrorf("Unable to read first level LOB dir: %v\n", err)
		return set, err
	}
	for _, dir1fi := range dir1 {
		if dir1fi.IsDir() {
			dir1path := filepath.Join(lobroot, dir1fi.Name())
			dir1f, err := os.Open(dir1path)
			if err != nil {
				LogErrorf("Unable to open LOB dir: %v\n", err)
				return set, err
			}
			dir2, err := dir1f.Readdir(0)
			if err != nil {
				LogErrorf("Unable to read second level LOB dir: %v\n", err)
				return set, err
			}
			for _, dir2fi := range dir2 {
				if dir2fi.IsDir() {
					dir2path := filepath.Join(dir1path, dir2fi.Name())
					dir2f, err := os.Open(dir2path)
					if err != nil {
						LogErrorf("Unable to open LOB dir: %v\n", err)
						return set, err
					}
					lobnames, err := dir2f.Readdirnames(0)
					if err != nil {
						LogErrorf("Unable to read innermost LOB dir: %v\n", err)
						return set, err
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
func PurgeUnreferenced(dryRun bool) ([]string, error) {
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

	// Must also not purge anything that's added but uncommitted
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

// Purge the shared store of all LOBs with only 1 hard link (itself)
// DeleteLOB will do this for individual LOBs we purge, but if the user
// manually deletes a repo then unreferenced shared LOBs may never be cleaned up
func PurgeSharedStore(dryRun bool) ([]string, error) {
	fileSHAs, err := getAllSharedLOBSHAs()
	if err == nil {
		ret := make([]string, 0, 10)
		for sha := range fileSHAs.Iter() {
			shareddir := GetSharedLOBDir(sha)
			names, err := filepath.Glob(filepath.Join(shareddir, fmt.Sprintf("%v*", sha)))
			if err != nil {
				LogErrorf("Unable to glob shared files for %v: %v\n", sha, err)
				return make([]string, 0), err
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
