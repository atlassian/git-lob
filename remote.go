package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Command line low-level tool to manually mark a remote/commit combo as pushed
func cmdMarkPushed() int {
	// git-lob mark-pushed <remote> <ref>...

	// Validate custom options (none)
	errorList := validateCustomOptions(GlobalOptions, nil, []string{})
	if len(errorList) > 0 {
		LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	if len(GlobalOptions.Args) < 2 {
		LogConsoleError("Too few arguments; must supply remote and at least one ref")
		return 9
	}
	// first parameter must be remote
	remoteName := GlobalOptions.Args[0]
	// Check valid remote
	if !IsGitRemote(remoteName) {
		LogConsoleError(remoteName, "is not a valid remote name")
		return 9
	}

	// Remaining args are refs
	refs := GlobalOptions.Args[1:]
	var expandedrefs []string
	// expand refs
	shaRegex := regexp.MustCompile("^[A-Fa-f0-9]{40}$")
	for _, ref := range refs {
		if shaRegex.MatchString(ref) {
			// already a full sha
			expandedrefs = append(expandedrefs, ref)
		} else {
			expanded, err := GitRefToFullSHA(ref)
			if err != nil {
				LogConsoleErrorf("Invalid ref '%v': %v\n", ref, err.Error())
				return 12
			}
			expandedrefs = append(expandedrefs, expanded)
		}
	}

	// If all refs were ok, do it
	LogConsole("Marking", remoteName, "as pushed at", refs)

	for i, sha := range expandedrefs {
		err := SuccessfullyPushedBinariesForCommit_REMOVE(remoteName, sha)
		if err != nil {
			LogErrorf("Unable to mark %v as pushed at %v (%v): %v\n", remoteName, sha, refs[i], err.Error())
		} else {
			LogDebugf("Marked %v as pushed at %v (%v)\n", remoteName, sha, refs[i])
		}
	}

	return 0

}

func cmdMarkPushedHelp() {
	LogConsole(`Usage: git-lob mark-pushed [options] <remote> <ref>...

  Manually marks a commit as pushed for a remote (and all its ancestors)

  This is a low-level command which tells git-lob's remote state cache
  that as at a given commit, it should assume that all binaries have been
  pushed to the named remote. This means next time a 'git lob push' is 
  performed for that remote, history won't be scanned earlier than this
  commit. See HISTORY CHECKING in 'git lob push --help' for more details.

  The most common case of using this command would be directly after adding
  a secondary remote (e.g. a fork), assuming that the fork already has access
  to all the binaries you have locally. Without doing this, the first push
  of binaries to this fork will take much longer as it scans all history.

Parameters:
  <remote>: The name of the remote.

     <ref>: One or more refs or commit SHAs at which to record as pushed. 
     		Any ancestors of this ref are also assumed to be pushed. If
     		you have many (open) branches that you intend to push from later
     		then you should do this for each branch to avoid a full history
     		scan the first time you push.

Options:
  --quiet, -q   Print less output
  --verbose, -v Print more output

`)

}

// Command line low-level tool to manually reset the pushed state of a remote
func cmdResetPushed() int {
	// git-lob reset-pushed <remote>

	// Validate custom options (none)
	errorList := validateCustomOptions(GlobalOptions, nil, []string{})
	if len(errorList) > 0 {
		LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	if len(GlobalOptions.Args) < 1 {
		LogConsoleError("Too few arguments; must supply remote name")
		return 9
	}
	// first parameter must be remote
	remoteName := GlobalOptions.Args[0]

	// Check valid remote
	if !IsGitRemote(remoteName) {
		LogConsoleError(remoteName, "is not a valid remote name")
		return 9
	}

	err := ResetPushedBinaryState(remoteName)
	if err != nil {
		LogError("Unable to reset pushed marker for", remoteName, ": ", err.Error())
		return 12
	} else {
		LogConsole("Successfully reset pushed markers for", remoteName)
	}

	return 0

}

func cmdResetPushedHelp() {
	LogConsole(`Usage: git-lob reset-pushed [options] <remote>

  Manually resets the cached pushed state for a remote

  This is a low-level command which tells git-lob's remote state cache
  to be cleared for a given remote. This means that the next 'git lob push'
  for that remote will do a full history scan and so will take longer.

  An alternative to this is to use the --recheck command on the next push.

Parameters:
  <remote>: The name of the remote.

Options:
  --quiet, -q   Print less output
  --verbose, -v Print more output

`)

}

// Command line low-level tool to report the last pushed ancestor of a ref
func cmdLastPushed() int {
	// git-lob last-pushed <remote> <ref>

	// Validate custom options (none)
	errorList := validateCustomOptions(GlobalOptions, nil, []string{})
	if len(errorList) > 0 {
		LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	if len(GlobalOptions.Args) != 2 {
		LogConsoleError("Wrong number of arguments; must supply a remote name and a ref")
		return 9
	}
	// first parameter must be remote
	remoteName := GlobalOptions.Args[0]
	// Check valid remote
	if !IsGitRemote(remoteName) {
		LogConsoleError(remoteName, "is not a valid remote name")
		return 9
	}

	ref := GlobalOptions.Args[1]
	// Convert the ref into a SHA
	commitSHA, err := GitRefToFullSHA(ref)
	if err != nil {
		LogConsoleErrorf("Invalid ref: %v: %v\n", ref, err.Error())
		return 9
	}

	last, err := FindLatestAncestorWhereBinariesPushed_REMOVE(remoteName, commitSHA)
	if err != nil {
		LogErrorf("Unable to locate last pushed commit for %v at %v: %v\n", remoteName, ref, err.Error())
		return 12
	} else {
		if last == "" {
			LogConsolef("No ancestor of %v has been pushed to %v\n", ref, remoteName)
		} else {
			LogConsolef("Last ancestor of %v that has been pushed to %v: %v\n", ref, remoteName, last)
		}
	}

	return 0

}

func cmdLastPushedHelp() {
	LogConsole(`Usage: git-lob last-pushed [options] <remote> <ref>

  Reports the most recent ancestor of <ref> where binaries are considered
  to have been fully pushed to <remote>

  git-lob stores a remote state cache so it doesn't have to check the whole
  history when pushing. This command reports where git-lob will look back to
  when asked to push to a given remote. See HISTORY CHECKING in the 
  'git lob push --help' output for more information.


Parameters:
  <remote>: The name of the remote.

     <ref>: The ref at which you want to start scanning backwards for the
            last push marker. 

Options:
  --quiet, -q   Print less output
  --verbose, -v Print more output

`)
}

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
// REMOVE
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

// Gets the file name which will store when we last pushed binaries
func getRemoteStateCacheFile(remoteName string) string {

	// Use a simple DB format based on commit SHA
	// e.g. for SHA 37d1cd1e4bd8f4853002ef6a5c8211d89fc09be2
	// cacheroot/37d/1cd/1e4/bd8/f48.txt
	// Every commit that starts with 37d1cd1e4bd8f48 will be stored in that text file, sorted
	dir := getRemoteStateCacheRoot(remoteName)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		LogErrorf("Unable to create remote state cache folder at %v: %v", dir, err)
		panic(err)
	}
	file := filepath.Join(dir, "push_state")
	return file
}

// Based on local cached state, should we try to push binaries for a given commit?
func ShouldPushBinariesForCommit_REMOVE(remoteName, commitSHA string) bool {
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
func recordRemoteBinariesUpToDateAtCommit_REMOVE(remoteName, commitSHA string) (alreadyMarked bool, err error) {
	filename := getRemoteStateCacheFileForCommit(remoteName, commitSHA)
	f, err := os.OpenFile(filename, os.O_EXCL|os.O_RDWR, 0644)
	if err != nil {
		// File did not exist, just write single line
		// For consistency in sizing, always include \n
		LogDebugf("Created new remote state cache file %v to mark %v as pushed\n", filename, commitSHA)
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

// Initialise the 'pushed' markers for all recent commits, if we can be sure we can do it
// Most common case: just after clone
func InitSuccessfullyPushedCacheIfAppropriate() {
	// Things get complex when you can have a combination of binaries which need fetching and
	// which might need pushing. Our push cache errs on the side of caution since binaries may
	// have been added from multiple sources so we check we pushed (or don't need to) before
	// marking a commit as pushed.
	// Fetching doesn't generally mark all commits as pushed, because you can easily have the
	// case where fetch only goes back a certain distance in time, but there are still commits
	// further back in history which you haven't pushed the binaries for yet.
	// However, after first clone you don't want to have to check the entire history. A really
	// easy shortcut is that if there are no local binaries, then there can't be anything to
	// push. This is the case on first fetch after a clone, so this is where we call it for now
	// We can mark all known remotes as pushed.

	// Adding a new remote (e.g. a fork) will however cause everything to be checked again.
	if IsLocalLOBStoreEmpty() {
		// No binaries locally so everything can be marked as pushed
		remotes, err := GetGitRemotes()
		if err != nil {
			LogErrorf("Unable to get remotes to mark as pushed %v\n", err.Error())
			return
		}
		// Mark as pushed at all refs (local branches, remote branches, tags)
		refs, err := GetGitAllRefs()
		if err != nil {
			LogErrorf("Unable to get all refs from git %v\n", err.Error())
			return
		}
		for _, remote := range remotes {
			var shas []string
			for _, ref := range refs {
				shas = append(shas, ref.CommitSHA)
			}
			shas = consolidateCommitsToLatestDescendants(shas)
			WritePushedState(remote, shas)
		}
	}

}

// Record that binaries have been pushed to a given remote at a commit
// replaceCommitSHA can be blank, but if provided will replace a previously inserted SHA
// for an ancestor instead of adding this SHA to the list (to be potentially optimised out)
func MarkBinariesAsPushed(remoteName, commitSHA, replaceCommitSHA string) error {
	if !GitRefIsFullSHA(commitSHA) {
		return fmt.Errorf("Invalid commit SHA, must be full 40 char SHA, not '%v'", commitSHA)
	}

	filename := getRemoteStateCacheFile(remoteName)
	f, err := os.OpenFile(filename, os.O_EXCL|os.O_RDWR, 0644)
	if err != nil {
		// File did not exist, just write single line
		// For consistency in sizing, always include \n
		LogDebugf("Created new remote state cache file %v to mark %v as pushed\n", filename, commitSHA)
		return ioutil.WriteFile(filename, []byte(commitSHA+"\n"), 0644)
	} else {
		defer f.Close()

		// File is sorted, could in theory read & insert but almost certainly faster
		// to read all, insert in memory then rewrite out in bulk. We've split on
		// first 15 chars of SHA anyway, unlikely to be huge contention
		scanner := bufio.NewScanner(f)
		var shas []string
		for scanner.Scan() {
			c := scanner.Text()
			if c != replaceCommitSHA {
				shas = append(shas, c)
			}
		}

		found, insertAt := StringBinarySearch(shas, commitSHA)

		if !found {
			// Rather than spend the time inserting in shas, just re-write from after insertion
			// Line length is the 40 char SHA plus (Unix) newline
			lineLen := SHALen + 1
			seekTo := int64(lineLen * insertAt)
			_, err = f.Seek(seekTo, os.SEEK_SET)
			if err != nil {
				return errors.New(fmt.Sprintf("Unable to seek to %v in %v", seekTo, filename))
			}
			// Insert the new entry
			f.WriteString(commitSHA + "\n")
			// Now write all the entries after that (we'll stay sorted)
			for i := insertAt; i < len(shas); i++ {
				// Have to re-insert the line break
				f.WriteString(shas[i] + "\n")
			}
			LogDebugf("Updated remote state cache file %v to mark %v as pushed", filename, commitSHA)

			return nil
		}

		// Was already recorded
		return nil
	}
}

// Overwrite entire pushed state for a remote
func WritePushedState(remoteName string, shas []string) error {

	filename := getRemoteStateCacheFile(remoteName)
	// we just write the whole thing, sorted
	sort.Strings(shas)
	f, err := os.OpenFile(filename, os.O_EXCL|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to write cache file %v:", filename, err.Error()))
	}
	for _, sha := range shas {
		// Have to re-insert the line break
		f.WriteString(sha + "\n")
	}
	LogDebugf("Initialised remote state cache for %v", remoteName)

	return nil
}

// Get a list of commits that have been pushed for a remote
func GetPushedCommits(remoteName string) []string {
	filename := getRemoteStateCacheFile(remoteName)
	f, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		// File missing
		return []string{}
	}
	defer f.Close()
	// Read entire file into memory and binary search
	// Will already be sorted
	scanner := bufio.NewScanner(f)
	var shas []string
	for scanner.Scan() {
		shas = append(shas, scanner.Text())
	}
	return shas
}

// Minimise the amount of state we retain on pushed state
// As we add SHAs that are pushed we can create redundant records because some SHAs are
// parents of others. This makes the subsequent retrieval of commits to push slower
// So remove SHAs that are ancestors of others and just keep the later SHAs that are pushed
func cleanupPushState(remoteName string) {
	pushed := GetPushedCommits(remoteName)

	consolidated := consolidateCommitsToLatestDescendants(pushed)

	if len(consolidated) != len(pushed) {
		WritePushedState(remoteName, consolidated)
	}
}

// Take a list of commit SHAs and consolidate them into another list which excludes
// any commits which are ancestors of others, and those which are no longer valid
// Note that this makes up to N^2 + N git calls so call infrequently
func consolidateCommitsToLatestDescendants(in []string) []string {
	consolidated := make([]string, 0, len(in))
	for i, a := range in {
		// First check this is a valid ref still (if rebased & deleted, remove)
		if !GitRefOrSHAIsValid(a) {
			continue
		}
		// If any other pushed entry is a descendent of 'a' then no reason to store 'a'
		redundant := false
		for j, b := range in {
			if i == j {
				continue
			}
			isancestor, err := GitIsAncestor(a, b)
			if err != nil {
				// play safe & keep
				continue
			}
			if isancestor {
				redundant = true
				break
			}
		}
		if !redundant {
			consolidated = append(consolidated, a)
		}
	}
	return consolidated

}

// Say that we've successfully pushed binaries for a remote at a commit (and all ancestors)
func SuccessfullyPushedBinariesForCommit_REMOVE(remoteName, commitSHA string) error {

	if !GitRefIsFullSHA(commitSHA) {
		return fmt.Errorf("Invalid commit SHA, must be full 40 char SHA, not '%v'", commitSHA)
	}

	alreadyMarked, err := recordRemoteBinariesUpToDateAtCommit_REMOVE(remoteName, commitSHA)
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
		alreadyMarked, err = recordRemoteBinariesUpToDateAtCommit_REMOVE(remoteName, currentSHA)
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
func FindLatestAncestorWhereBinariesPushed_REMOVE(remoteName, commitSHA string) (string, error) {
	if len(commitSHA) != 40 {
		return "", errors.New("Invalid parameter, commitSHA must be full SHA not alias/ref")
	}
	// Check self first (avoid git log call if up to date)
	if !ShouldPushBinariesForCommit_REMOVE(remoteName, commitSHA) {
		return commitSHA, nil
	}

	// Now check ancestors
	parent := commitSHA + "^"
	var foundSHA string
	err := WalkGitHistory(parent, func(currentSHA, parentSHA string) (bool, error) {

		if !ShouldPushBinariesForCommit_REMOVE(remoteName, currentSHA) {
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
			lastPushed, err := FindLatestAncestorWhereBinariesPushed_REMOVE(remoteName, commitSHA)
			if err != nil {
				return []CommitLOBRef{}, err
			}
			return GetGitCommitsReferencingLOBsInRange(lastPushed, commitSHA, nil, nil)
		}

	}
	return []CommitLOBRef{}, nil

}

// Check with a remote provider for the presence of all data required for a given LOB
// Return nil if all data is there, NotFoundErr if not
func CheckRemoteLOBFilesForSHA(sha string, provider SyncProvider, remoteName string) error {
	// We need LOB info to know size / how many chunks it had
	var info *LOBInfo
	info, err := GetLOBInfo(sha)
	meta := getLOBMetaRelativePath(sha)
	if err != nil {
		// We have to actually download meta file in order to figure out what else is needed
		dlerr := provider.Download(remoteName, []string{meta}, os.TempDir(), false, DummySyncProgressCallback)
		if dlerr != nil {
			return dlerr
		}
		metafullpath := filepath.Join(os.TempDir(), meta)
		var parseerr error
		info, parseerr = parseLOBInfoFromFile(metafullpath)
		// delete from temp afterwards
		os.Remove(metafullpath)
		if parseerr != nil {
			return fmt.Errorf("Unable to parse metadata from file downloaded from %v for %v: %v", remoteName, sha, parseerr.Error())
		}
	} else {
		// We had the meta locally, so just check the file is on the remote
		if !provider.FileExists(remoteName, meta) {
			return NewNotFoundError(fmt.Sprintf("Meta file %v missing from %v", meta, remoteName))
		}
	}

	// Now we get the list of chunks & check they are present
	for i := 0; i < info.NumChunks; i++ {
		expectedSize := getLOBExpectedChunkSize(info, i)
		chunk := getLOBChunkRelativePath(sha, i)
		if !provider.FileExistsAndIsOfSize(remoteName, chunk, expectedSize) {
			return NewNotFoundError(fmt.Sprintf("Chunk file %v missing from %v", chunk, remoteName))
		}
	}

	// All OK
	return nil

}
