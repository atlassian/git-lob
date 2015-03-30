package core

import (
	"bitbucket.org/sinbad/git-lob/util"
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Do we have a remote state cache for this remote yet?
func hasRemoteStateCache(remoteName string) bool {
	dir := filepath.Join(util.GetGitDir(), "git-lob", "state", "remotes", remoteName)
	return util.DirExists(dir)
}

// Gets the root directory of the remote state cache for a given remote
func getRemoteStateCacheRoot(remoteName string) string {
	ret := filepath.Join(util.GetGitDir(), "git-lob", "state", "remotes", remoteName)
	err := os.MkdirAll(ret, 0755)
	if err != nil {
		util.LogErrorf("Unable to create remote state cache folder at %v: %v", ret, err)
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
		util.LogErrorf("Unable to create remote state cache folder at %v: %v", dir, err)
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
		util.LogErrorf("Unable to create remote state cache folder at %v: %v", dir, err)
		panic(err)
	}
	file := filepath.Join(dir, "push_state")
	return file
}

// Initialise the 'pushed' markers for all recent commits, if we can be sure we can do it
// Most common case: just after clone
// Returns whether we met the requirements to do this
func InitSuccessfullyPushedCacheIfAppropriate() bool {
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
			util.LogErrorf("Unable to get remotes to mark as pushed %v\n", err.Error())
			return false
		}
		// Mark as pushed at all refs (local branches, remote branches, tags)
		refs, err := GetGitAllRefs()
		if err != nil {
			util.LogErrorf("Unable to get refs to mark as pushed %v\n", err.Error())
			return false
		}
		var shas []string
		for _, ref := range refs {
			shas = append(shas, ref.CommitSHA)
		}
		shas = consolidateCommitsToLatestDescendants(shas)
		for _, remote := range remotes {
			err := WritePushedState(remote, shas)
			if err != nil {
				util.LogErrorf("Unable to write push state for %v: %v\n", remote, err.Error())
				return false
			}
		}
		return true
	}
	return false

}

func MarkAllBinariesPushed(remoteName string) error {
	// Mark as pushed at all refs (local branches, remote branches, tags)
	refs, err := GetGitAllRefs()
	if err != nil {
		return err
	}
	var shas []string
	for _, ref := range refs {
		shas = append(shas, ref.CommitSHA)
	}
	shas = consolidateCommitsToLatestDescendants(shas)
	return WritePushedState(remoteName, shas)
}

// Record that binaries have been pushed to a given remote at a commit
// replaceCommitSHA can be blank, but if provided will replace a previously inserted SHA
// If you use replaceCommitSHA, it MUST BE an ancestor of commitSHA
func MarkBinariesAsPushed(remoteName, commitSHA, replaceCommitSHA string) error {
	if !GitRefIsFullSHA(commitSHA) {
		return fmt.Errorf("Invalid commit SHA, must be full 40 char SHA, not '%v'", commitSHA)
	}
	shas := GetPushedCommits(remoteName)

	// confirm not there already
	alreadyPresent, _ := util.StringBinarySearch(shas, commitSHA)
	if alreadyPresent {
		return nil
	}

	// insert or append, then re-sort
	if replaceCommitSHA != "" {
		//util.LogDebugf("Updating remote state for %v to mark %v as pushed (replaces %v)\n", remoteName, commitSHA, replaceCommitSHA)
		found, insertAt := util.StringBinarySearch(shas, replaceCommitSHA)
		if found {
			shas[insertAt] = commitSHA
		} else {
			shas = append(shas, commitSHA)
		}
	} else {
		//util.LogDebugf("Updating remote state for %v to mark %v as pushed\n", remoteName, commitSHA)
		shas = append(shas, commitSHA)
	}
	sort.Strings(shas)
	return WritePushedState(remoteName, shas)
}

// Overwrite entire pushed state for a remote
func WritePushedState(remoteName string, shas []string) error {

	filename := getRemoteStateCacheFile(remoteName)
	// we just write the whole thing, sorted
	sort.Strings(shas)
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to write cache file %v: %v", filename, err.Error()))
	}
	for _, sha := range shas {
		// Have to re-insert the line break
		f.WriteString(sha + "\n")
	}

	return nil
}

// Get a list of commits that have been pushed for a remote
// remoteName can be "*" to return pushed list for all remotes combined
func GetPushedCommits(remoteName string) []string {
	var shas []string
	if remoteName == "*" {
		remotes, err := GetGitRemotes()
		if err != nil {
			return []string{}
		}
		for _, remote := range remotes {
			rshas := GetPushedCommits(remote)
			shas = append(shas, rshas...)
		}

	} else {
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
		for scanner.Scan() {
			shas = append(shas, scanner.Text())
		}

	}
	return shas
}

// Minimise the amount of state we retain on pushed state
// As we add SHAs that are pushed we can create redundant records because some SHAs are
// parents of others. This makes the subsequent retrieval of commits to push slower
// So remove SHAs that are ancestors of others and just keep the later SHAs that are pushed
func CleanupPushState(remoteName string) {
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
				// Don't compare to self
				continue
			}
			if a == b {
				// Duplicate, remove earliest
				if i < j {
					redundant = true
					break
				} else {
					continue
				}
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

// Reset the cached information about which binaries we have cached for a given remote
// Warning: this will make the next push expensive while it recalculates
func ResetPushedBinaryState(remoteName string) error {
	return os.RemoveAll(getRemoteStateCacheRoot(remoteName))
}

// Do we have any pushed binary state recorded for a remote?
func HasPushedBinaryState(remoteName string) bool {
	return hasRemoteStateCache(remoteName)
}

// Find the most recent ancestor of ref (or itself) at which we believe we've
// already pushed all binaries. Returns a blank string if none have been pushed.
func FindLatestAncestorWhereBinariesPushed(remoteName, ref string) (string, error) {

	// Use the list of pushed SHAs plus this ref to determine the best common ancestor
	pushedSHAs := GetPushedCommits(remoteName)
	if len(pushedSHAs) == 0 {
		return "", nil
	}

	var refs = make([]string, 0, len(pushedSHAs)+1)
	refs = append(refs, ref)
	refs = append(refs, pushedSHAs...)
	best, err := GetGitBestAncestor(refs)
	return best, err
}

// Get a list of commits which have LOB SHAs to push, given a refspec, in forward ancestry order
// Only commits which have LOBs associated will be returned on the assumption that when
// child commits are marked as pushed it will also mark the parents
// If the refspec is itself a range, just queries that range for binary references
// If the refspec is a single ref, then finds the latest ancestor we think has been pushed already
// for this remote and returns the LOBs referred to in that range. If recheck is true,
// ignores the record of the last commit we think we pushed and scans entire history (slow)
func GetCommitLOBsToPushForRefSpec(remoteName string, refspec *GitRefSpec, recheck bool) ([]*CommitLOBRef, error) {
	var ret []*CommitLOBRef
	callback := func(commit *CommitLOBRef) (quit bool, err error) {
		ret = append(ret, commit)
		return false, nil
	}
	err := WalkGitCommitLOBsToPushForRefSpec(remoteName, refspec, recheck, callback)
	return ret, err
}

// Get a list of commits which have LOB SHAs to push, given a ref, in forward ancestry order
// Only commits which have LOBs associated will be returned on the assumption that when
// child commits are marked as pushed it will also mark the parents
// If the refspec is a single ref, then finds the latest ancestor we think has been pushed already
// for this remote and returns the LOBs referred to in that range. If recheck is true,
// ignores the record of the last commit we think we pushed and scans entire history (slow)
func GetCommitLOBsToPushForRef(remoteName string, ref string, recheck bool) ([]*CommitLOBRef, error) {
	var ret []*CommitLOBRef
	callback := func(commit *CommitLOBRef) (quit bool, err error) {
		ret = append(ret, commit)
		return false, nil
	}
	err := WalkGitCommitLOBsToPush(remoteName, ref, recheck, callback)
	return ret, err
}

// Check with a remote provider for the presence of all data required for a given LOB
// Return nil if all data is there, NotFoundErr if not
func CheckRemoteLOBFilesForSHA(sha string, provider SyncProvider, remoteName string) error {
	// We need LOB info to know size / how many chunks it had
	var info *LOBInfo
	info, err := GetLOBInfo(sha)
	meta := GetLOBMetaRelativePath(sha)
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
			return NewNotFoundError(fmt.Sprintf("Meta file %v missing from %v", meta, remoteName), meta)
		}
	}

	// Now we get the list of chunks & check they are present
	for i := 0; i < info.NumChunks; i++ {
		expectedSize := getLOBExpectedChunkSize(info, i)
		chunk := GetLOBChunkRelativePath(sha, i)
		if !provider.FileExistsAndIsOfSize(remoteName, chunk, expectedSize) {
			return NewNotFoundError(fmt.Sprintf("Chunk file %v missing from %v", chunk, remoteName), chunk)
		}
	}

	// All OK
	return nil

}
