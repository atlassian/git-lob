package main

import (
	"bufio"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
				LogErrorf("Unable to seek to %v in %v", seekTo, filename)
				return false, err
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
	currentSHA := commitSHA + "^"
	ancestorCount := 0
	for !alreadyMarked && ancestorCount < historyLimit {
		// get 50 parents
		// format as <SHA> <PARENT> so we can detect the end of history
		cmd := exec.Command("git", "log", "--first-parent", "--topo-order",
			"-n", "50", "--format=%H %P", currentSHA)
		ancestorCount += 50

		outp, err := cmd.StdoutPipe()
		if err != nil {
			LogErrorf("Unable to list commits: " + err.Error())
			return err
		}
		cmd.Start()
		scanner := bufio.NewScanner(outp)
		var currentLine string
		for scanner.Scan() {
			currentLine = scanner.Text()
			currentSHA := currentLine[:40]
			alreadyMarked, err = recordRemoteBinariesUpToDateAtCommit(remoteName, currentSHA)
			if err != nil {
				return err
			}
			if alreadyMarked {
				break
			}
		}
		cmd.Wait()
		// If we got here, we still haven't found an ancestor that was already marked
		// check next batch, provided there's a parent on the last one
		// 81 chars long, 2x40 SHAs + space
		if len(currentLine) >= 81 {
			currentSHA = strings.TrimSpace(currentLine[41:81])
			if currentSHA == "" {
				break
			}
		} else {
			break
		}

	}
	return nil
}

// Reset the cached information about which binaries we have cached for a given remote
// Warning: this will make the next push expensive while it recalculates
func ResetPushedBinaryState(remoteName string) error {
	return os.RemoveAll(getRemoteStateCacheRoot(remoteName))
}

// Find the most recent ancestor of commitSHA (or itself) at which we believe we've
// already pushed all binaries. Returns a blanks string if none have been pushed.
func FindLatestAncestorWhereBinariesPushed(remoteName, commitSHA string) (string, error) {

	// Use git log to list 50 at a time
	// Only follow first parent and always in topo order
	currentHEAD := commitSHA
	for {
		cmd := exec.Command("git", "log", "--first-parent", "--topo-order",
			"-n", "50", "--format=%H %P", currentHEAD)
		outp, err := cmd.StdoutPipe()
		if err != nil {
			LogErrorf("Unable to list commits: " + err.Error())
			return "", err
		}
		cmd.Start()
		scanner := bufio.NewScanner(outp)
		var currentLine string
		for scanner.Scan() {
			currentLine = scanner.Text()
			currentSHA := currentLine[:40]
			if !ShouldPushBinariesForCommit(remoteName, currentSHA) {
				return currentSHA, nil
			}
		}
		cmd.Wait()
		// If we got here, none of this batch succeeded
		// check next batch, provided there's a parent on the last one
		// 81 chars long, 2x40 SHAs + space
		if len(currentLine) >= 81 {
			currentHEAD = strings.TrimSpace(currentLine[41:81])
			if currentHEAD == "" {
				break
			}
		} else {
			break
		}

	}
	// No match
	return "", nil
}
