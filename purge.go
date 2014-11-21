package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

var (
	diffLOBReferenceRegex *regexp.Regexp
)

// Retrieve the full set of SHAs that currently have files locally (complete or not)
// returned as map[string]bool for fast lookup
func getAllLocalLOBSHAs() (StringSet, error) {
	// os.File.Readdirnames is the most efficient
	// os.File.Readdir retrieves extra info we don't need
	// ioutil.ReadDir and filepath.Walk do sorting which is unnecessary & inefficient

	set := NewStringSet()

	// We only need to support a 2-folder structure here & know that all files are at the bottom level
	lobroot := GetLOBRoot()
	rootf, err := os.Open(lobroot)
	if err != nil {
		LogErrorf("Unable to open LOB root: %v\n", err)
		return set, err
	}
	dir1names, err := rootf.Readdirnames(0)
	if err != nil {
		LogErrorf("Unable to read first level LOB dir: %v\n", err)
		return set, err
	}
	for _, dir1name := range dir1names {
		dir1path := filepath.Join(lobroot, dir1name)
		dir1f, err := os.Open(dir1path)
		if err != nil {
			LogErrorf("Unable to open LOB dir: %v\n", err)
			return set, err
		}
		dir2names, err := dir1f.Readdirnames(0)
		if err != nil {
			LogErrorf("Unable to read second level LOB dir: %v\n", err)
			return set, err
		}
		for _, dir2name := range dir2names {
			dir2path := filepath.Join(dir1path, dir2name)
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
				// Use the first 40 characters of the name as SHA
				sha := lobname[:SHALen]
				set.Add(sha)
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
	cmd := exec.Command("git", "log", "--all", "--no-color", "--oneline", "-p", "-G", "^git-lob: [A-Fa-f0-9]{40}$")
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
	cmd = exec.Command("git", "diff", "--cached", "--no-color", "-G", "^git-lob: [A-Fa-f0-9]{40}$")
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
