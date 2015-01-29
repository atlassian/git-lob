package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Do we have a remote state cache for this remote yet?
func hasRemoteStateCache(remoteName string) bool {
	dir := filepath.Join(GetGitDir(), "git-lob", "state", "remotes", remoteName)
	return DirExists(dir)
}

// Gets the root directory of the remote state cache for a given remote
func getRemoteStateCacheRoot(remoteName string) string {
	ret := filepath.Join(GetGitDir(), "git-lob", "state", "remotes", remoteName)
	err := os.MkdirAll(ret, 0755)
	if err != nil {
		LogErrorf("Unable to create remote state cache folder at %v: %v", ret, err)
		panic(err)
	}
	return ret
}

// Gets the file name which will store a given commitSHA if binaries are thought to
// be up to date at that commit on that remote
func getRemoteStateCacheFileForCommit(remoteName, commitSHA string) string {

	// Use a simple DB format based on commit SHA
	// e.g. for SHA 37d1cd1e4bd8f4853002ef6a5c8211d89fc09be2
	// cacheroot/37d/1cd/1e4/bd8/f48.txt
	// Every commit that starts with 37d1cd1e4bd8f48 will be stored in that text file, sorted
	dir := filepath.Join(getRemoteStateCacheRoot(remoteName),
		commitSHA[:3], commitSHA[3:6], commitSHA[6:9], commitSHA[9:12])
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		LogErrorf("Unable to create remote state cache folder at %v: %v", dir, err)
		panic(err)
	}
	file := filepath.Join(dir, commitSHA[12:15])
	return file
}

// Based on local cached state, should we try to push binaries for a given commit?
func ShouldPushBinariesForCommit(remoteName, commitSHA string) bool {
	filename := getRemoteStateCacheFileForCommit(remoteName, commitSHA)
	f, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		// File missing
		return true
	}
	defer f.Close()
	// Read entire file into memory and binary search
	// Will already be sorted
	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	found, _ := StringBinarySearch(lines, commitSHA)
	return !found
}

// Update local cache to say that we believe we've updated the named remote at this commit
func recordRemoteBinariesUpToDateAtCommit(remoteName, commitSHA string) (alreadyMarked bool, err error) {
	filename := getRemoteStateCacheFileForCommit(remoteName, commitSHA)
	f, err := os.OpenFile(filename, os.O_EXCL|os.O_RDWR, 0644)
	if err != nil {
		// File did not exist, just write single line
		// For consistency in sizing, always include \n
		LogDebugf("Created new remote state cache file %v to mark %v as pushed", filename, commitSHA)
		return false, ioutil.WriteFile(filename, []byte(commitSHA+"\n"), 0644)
	} else {
		defer f.Close()

		// File is sorted, could in theory read & insert but almost certainly faster
		// to read all, insert in memory then rewrite out in bulk. We've split on
		// first 15 chars of SHA anyway, unlikely to be huge contention
		scanner := bufio.NewScanner(f)
		var shas []string
		for scanner.Scan() {
			shas = append(shas, scanner.Text())
		}

		found, insertAt := StringBinarySearch(shas, commitSHA)

		if !found {
			// Rather than spend the time inserting in shas, just re-write from after insertion
			// Line length is the 40 char SHA plus (Unix) newline
			lineLen := SHALen + 1
			seekTo := int64(lineLen * insertAt)
			_, err = f.Seek(seekTo, os.SEEK_SET)
			if err != nil {
				return false, errors.New(fmt.Sprintf("Unable to seek to %v in %v", seekTo, filename))
			}
			// Insert the new entry
			f.WriteString(commitSHA + "\n")
			// Now write all the entries after that (we'll stay sorted)
			for i := insertAt; i < len(shas); i++ {
				// Have to re-insert the line break
				f.WriteString(shas[i] + "\n")
			}
			LogDebugf("Updated remote state cache file %v to mark %v as pushed", filename, commitSHA)

			return false, nil
		}

		// Was already recorded
		return true, nil
	}
}

// Say that we've successfully pushed binaries for a remote at a commit (and all ancestors)
func SuccessfullyPushedBinariesForCommit(remoteName, commitSHA string) error {

	if !GitRefIsFullSHA(commitSHA) {
		return fmt.Errorf("Invalid commit SHA, must be full 40 char SHA, not '%v'", commitSHA)
	}

	alreadyMarked, err := recordRemoteBinariesUpToDateAtCommit(remoteName, commitSHA)
	if err != nil {
		return err
	}
	// Check ancestors (stop at first commit already marked, transitive)
	// Retrieve them in bulk so we don't have to issue a git call for every one

	// Limit how far we go back for this though; we go back so if someone branches
	// at an old commit we still know which range of commits we need to search for
	// new binaries, but we don't want to waste time going back forever
	const historyLimit = 500

	// Start with first parent
	parent := commitSHA + "^"
	ancestorCount := 0
	err = WalkGitHistory(parent, func(currentSHA, parentSHA string) (bool, error) {
		alreadyMarked, err = recordRemoteBinariesUpToDateAtCommit(remoteName, currentSHA)
		if err != nil {
			// quit
			return true, err
		}
		ancestorCount++
		// stop if we've hit a marked SHA or history limit
		return alreadyMarked || ancestorCount >= historyLimit, nil
	})
	return err
}

// Reset the cached information about which binaries we have cached for a given remote
// Warning: this will make the next push expensive while it recalculates
func ResetPushedBinaryState(remoteName string) error {
	return os.RemoveAll(getRemoteStateCacheRoot(remoteName))
}

// Do we have any pushed binary state recorded for a remote?
func HasPushedBinaryState(remoteName string) bool {
	return hasRemoteStateCache(remoteName)
}

// Find the most recent ancestor of commitSHA (or itself) at which we believe we've
// already pushed all binaries. Returns a blanks string if none have been pushed.
func FindLatestAncestorWhereBinariesPushed(remoteName, commitSHA string) (string, error) {

	// Check self first (avoid git log call if up to date)
	if !ShouldPushBinariesForCommit(remoteName, commitSHA) {
		return commitSHA, nil
	}

	// Now check ancestors
	parent := commitSHA + "^"
	var foundSHA string
	err := WalkGitHistory(parent, func(currentSHA, parentSHA string) (bool, error) {

		if !ShouldPushBinariesForCommit(remoteName, currentSHA) {
			foundSHA = currentSHA
			return true, nil
		}
		return false, nil
	})
	return foundSHA, err
}

// Get a list of commits which have LOB SHAs to push, given a refspec, in forward ancestry order
// Only commits which have LOBs associated will be returned on the assumption that when
// child commits are marked as pushed it will also mark the parents
// If the refspec is itself a range, just queries that range for binary references
// If the refspec is a single ref, then finds the latest ancestor we think has been pushed already
// for this remote and returns the LOBs referred to in that range. If recheck is true,
// ignores the record of the last commit we think we pushed and scans entire history (slow)
func GetCommitLOBsToPushForRefSpec(remoteName string, refspec *GitRefSpec, recheck bool) ([]CommitLOBRef, error) {
	if refspec.IsRange() {
		// Only need to deal with '..' range operator for push
		return GetGitCommitsReferencingLOBsInRange(refspec.Ref1, refspec.Ref2, nil, nil)

	} else {
		// Determine range from last pushed - must be full SHA basis
		commitSHA, err := GitRefToFullSHA(refspec.Ref1)
		if err != nil {
			return []CommitLOBRef{}, err
		}
		if recheck {
			// Scan for LOBs in entire history
			return GetGitCommitsReferencingLOBsInRange("", commitSHA, nil, nil)
		} else {
			lastPushed, err := FindLatestAncestorWhereBinariesPushed(remoteName, commitSHA)
			if err != nil {
				return []CommitLOBRef{}, err
			}
			return GetGitCommitsReferencingLOBsInRange(lastPushed, commitSHA, nil, nil)
		}

	}
	return []CommitLOBRef{}, nil

}
