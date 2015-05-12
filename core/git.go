package core

import (
	"bitbucket.org/sinbad/git-lob/util"
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Git specification of a commit or range of commits (a reference or reference range)
type GitRefSpec struct {
	// First ref
	Ref1 string
	// Optional range operator if this is a range refspec (".." or "...")
	RangeOp string
	// Optional second ref
	Ref2 string
}

// Some top level information about a commit (only first line of message)
type GitCommitSummary struct {
	SHA            string
	ShortSHA       string
	Parents        []string
	CommitDate     time.Time
	AuthorDate     time.Time
	AuthorName     string
	AuthorEmail    string
	CommitterName  string
	CommitterEmail string
	Subject        string
}

type GitRefType int

const (
	GitRefTypeLocalBranch  = GitRefType(iota)
	GitRefTypeRemoteBranch = GitRefType(iota)
	GitRefTypeLocalTag     = GitRefType(iota)
	GitRefTypeRemoteTag    = GitRefType(iota)
	GitRefTypeHEAD         = GitRefType(iota) // current checkout
	GitRefTypeOther        = GitRefType(iota) // stash or unknown
)

// A git reference (branch, tag etc)
type GitRef struct {
	Name      string
	Type      GitRefType
	CommitSHA string
}

// Returns whether a GitRefSpec is a range or not
func (r *GitRefSpec) IsRange() bool {
	return (r.RangeOp == ".." || r.RangeOp == "...") &&
		r.Ref1 != "" && r.Ref2 != ""
}

// Returns whether a GitRefSpec is an empty range (using the same ref for start & end)
func (r *GitRefSpec) IsEmptyRange() bool {
	return (r.RangeOp == ".." || r.RangeOp == "...") &&
		r.Ref1 != "" && r.Ref1 == r.Ref2
}

func (r *GitRefSpec) String() string {
	if r.IsRange() {
		return fmt.Sprintf("%v%v%v", r.Ref1, r.RangeOp, r.Ref2)
	} else {
		return r.Ref1
	}
}

// A record of a set of LOB shas that are associated with a commit
type CommitLOBRef struct {
	Commit  string
	Parents []string
	// Bare LOBs
	LobSHAs []string
	// LOBs with file names
	FileLOBs []*FileLOB
}

func (self *CommitLOBRef) String() string {
	return fmt.Sprintf("Commit: %v\n  Files:%v\n", self.Commit, self.LobSHAs)
}

// A filename & LOB SHA pair
type FileLOB struct {
	// Filename relative to repository root
	Filename string
	// LOB SHA
	SHA string
}

// Convert a slice of FileLOBs to a map of lob sha to filename, eliminates duplicates
func ConvertFileLOBSliceToMap(slice []*FileLOB) map[string]string {
	ret := make(map[string]string, len(slice))
	for _, filelob := range slice {
		ret[filelob.SHA] = filelob.Filename
	}
	return ret
}

// Walk first parents starting from startSHA and call callback
// First call will be startSHA & its parent
// Parent will be blank string if there are no more parents & walk will stop after
// Optimises internally to call Git only for batches of 50
func WalkGitHistory(startSHA string, callback func(currentSHA, parentSHA string) (quit bool, err error)) error {

	quit := false
	currentLogHEAD := startSHA
	var callbackError error
	for !quit {
		// get 250 parents
		// format as <SHA> <PARENT> so we can detect the end of history
		cmd := exec.Command("git", "log", "--first-parent", "--topo-order",
			"-n", "250", "--format=%H %P", currentLogHEAD)

		outp, err := cmd.StdoutPipe()
		if err != nil {
			return errors.New(fmt.Sprintf("Unable to list commits from %v: %v", currentLogHEAD, err.Error()))
		}
		cmd.Start()
		scanner := bufio.NewScanner(outp)
		var currentLine string
		var parentSHA string
		for scanner.Scan() {
			currentLine = scanner.Text()
			currentSHA := currentLine[:40]
			// If we got here, we still haven't found an ancestor that was already marked
			// check next batch, provided there's a parent on the last one
			// 81 chars long, 2x40 SHAs + space
			if len(currentLine) >= 81 {
				parentSHA = strings.TrimSpace(currentLine[41:81])
			} else {
				parentSHA = ""
			}
			quit, callbackError = callback(currentSHA, parentSHA)
			if quit {
				cmd.Process.Kill()
				break
			}
		}
		cmd.Wait()
		// End of history
		if parentSHA == "" {
			break
		} else {
			currentLogHEAD = parentSHA
		}
	}
	return callbackError
}

// Walk forwards through a list of commits with LOB references based on refspec
// If refspec is a range, walks that specific range of commits regardless of whether it's been pushed
// If not, walks forwards from the oldest ancestor of refspec.Ref1 that's not pushed to the latest commit (including 'ref' if it includes LOBs)
// Walks all ancestors including second+ parents, in topological order
// remoteName can be a specific remote or "*" to count pushed ton *any* remote as OK
// If recheck=true then existing pushed records are ignored (all commits are walked)
func WalkGitCommitLOBsToPushForRefSpec(remoteName string, refspec *GitRefSpec, recheck bool, callback func(commitLOB *CommitLOBRef) (quit bool, err error)) error {
	if refspec.IsRange() {
		// Walk a specific range
		return walkGitCommitsReferencingLOBsInRange(refspec.Ref1, refspec.Ref2, true, false, []string{}, []string{}, callback)

	} else {
		// Walk everything that hasn't been pushed before Ref1
		return WalkGitCommitLOBsToPush(remoteName, refspec.Ref1, recheck, callback)
	}
}

// Walk a list of commits with LOB references which are ancestors of 'ref' which have not been pushed
// Walks forwards from the oldest commit to the latest commit (including 'ref' if it includes LOBs)
// Walks all ancestors including second+ parents, in topological order
// remoteName can be a specific remote or "*" to count pushed ton *any* remote as OK
func WalkGitCommitLOBsToPush(remoteName, ref string, recheck bool, callback func(commitLOB *CommitLOBRef) (quit bool, err error)) error {
	// We use git's ability to log all new commits up to ref but exclude any ancestors of pushed
	var pushedSHAs []string
	// If rechecking, then we just log the whole thing
	if !recheck {
		pushedSHAs = GetPushedCommits(remoteName)
	}
	// Loop to allow retry
	for {
		args := []string{"log", `--format=commitsha: %H %P`, "-p",
			"--topo-order",
			"--reverse",
			"-G", SHALineRegexStr,
			ref}

		for _, p := range pushedSHAs {
			// 'not reachable from pushed commits'
			args = append(args, fmt.Sprintf("^%v", p))
		}

		// format as <SHA> <PARENT> so we progressively work backward
		cmd := exec.Command("git", args...)

		outp, err := cmd.StdoutPipe()
		if err != nil {
			return errors.New(fmt.Sprintf("Unable to list commits from %v: %v", ref, err.Error()))
		}
		cmd.Start()

		quit, err := walkGitLogOutputForLOBReferences(outp, true, false, []string{}, []string{}, callback)

		if quit || err != nil {
			// Early abort
			cmd.Process.Kill()
		}

		procerr := cmd.Wait()
		if procerr != nil {
			if len(pushedSHAs) > 0 {
				// This can happen because one of the pushedSHAs has been completely removed from the repo
				// consolidate SHAs and try again, this deletes any non-existent SHAs
				consolidated := consolidateCommitsToLatestDescendants(pushedSHAs)
				if len(consolidated) != len(pushedSHAs) {
					// Store the refined state
					WritePushedState(remoteName, consolidated)
					pushedSHAs = consolidated
					// retry
					continue
				}
			}
		}

		return err

	}
}

// Internal utility for walking git-log output for git-lob references & calling callback
// Log output must be formated like this: `--format=commitsha: %H %P`
// outp must be output from a running git log task
func walkGitLogOutputForLOBReferences(outp io.Reader, additions, removals bool,
	includePaths, excludePaths []string, callback func(commitLOB *CommitLOBRef) (quit bool, err error)) (quit bool, err error) {
	// Sadly we still get more output than we actually need, but this is the minimum we can get
	// For each commit we'll get something like this:
	/*
	   commitsha: af2607421c9fee2e430cde7e7073a7dad07be559 22be911a626eb9cf2e2760b1b8b092441771cb9d

	   diff --git a/atheneNormalMap.png b/atheneNormalMap.png
	   new file mode 100644
	   index 0000000..272b5c1
	   --- /dev/null
	   +++ b/atheneNormalMap.png
	   @@ -0,0 +1 @@
	   +git-lob: b022770eab414c36575290c993c29799bc6610c3
	*/
	// There can be multiple diffs per commit (multiple binaries)
	// Also when a binary is changed the diff will include a '-' line for the old SHA
	// Depending on which direction in history the caller wants, they'll specify the
	// parameters 'additions' and 'removals' to determine which get included

	// Use 1 regex to capture all for speed
	var lobregex *regexp.Regexp
	if additions && !removals {
		lobregex = regexp.MustCompile(`^\+git-lob: ([A-Fa-f0-9]{40})`)
	} else if removals && !additions {
		lobregex = regexp.MustCompile(`^\-git-lob: ([A-Fa-f0-9]{40})`)
	} else {
		lobregex = regexp.MustCompile(`^[\+\-]git-lob: ([A-Fa-f0-9]{40})`)
	}
	fileHeaderRegex := regexp.MustCompile(`diff --git a\/(.+?)\s+b\/(.+)`)
	fileMergeHeaderRegex := regexp.MustCompile(`diff --cc (.+)`)
	commitHeaderRegex := regexp.MustCompile(`^commitsha: ([A-Fa-f0-9]{40})(?: ([A-Fa-f0-9]{40}))*`)

	scanner := bufio.NewScanner(outp)

	var currentCommit *CommitLOBRef
	var currentFilename string
	currentFileIncluded := true
	for scanner.Scan() {
		line := scanner.Text()
		if match := commitHeaderRegex.FindStringSubmatch(line); match != nil {
			// Commit header
			sha := match[1]
			parentSHAs := match[2:]
			// Set commit context
			if currentCommit != nil {
				if len(currentCommit.LobSHAs) > 0 {
					quit, err := callback(currentCommit)
					if err != nil {
						return quit, err
					} else if quit {
						return true, nil
					}
				}
				currentCommit = nil
			}
			currentCommit = &CommitLOBRef{Commit: sha, Parents: parentSHAs}
		} else if match := fileHeaderRegex.FindStringSubmatch(line); match != nil {
			// Finding a regular file header
			// Pertinent file name depends on whether we're listening to additions or removals
			if additions {
				currentFilename = match[2]
			} else {
				currentFilename = match[1]
			}
			currentFileIncluded = util.FilenamePassesIncludeExcludeFilter(currentFilename, includePaths, excludePaths)
		} else if match := fileMergeHeaderRegex.FindStringSubmatch(line); match != nil {
			// Git merge file header is a little different, only one file
			currentFilename = match[1]
			currentFileIncluded = util.FilenamePassesIncludeExcludeFilter(currentFilename, includePaths, excludePaths)
		} else if match := lobregex.FindStringSubmatch(line); match != nil {
			// This is a LOB reference (+/- already matched in variant of regex)
			sha := match[1]
			// Use filename context to include/exclude if paths were used
			if currentFileIncluded {
				currentCommit.LobSHAs = append(currentCommit.LobSHAs, sha)
				currentCommit.FileLOBs = append(currentCommit.FileLOBs, &FileLOB{Filename: currentFilename, SHA: sha})
			}
		}
	}
	// Final commit
	if currentCommit != nil {
		if len(currentCommit.LobSHAs) > 0 {
			quit, err := callback(currentCommit)
			if err != nil {
				return quit, err
			} else if quit {
				return true, nil
			}
		}
		currentCommit = nil
	}

	return false, nil
}

// Gets the default push remote for the working dir
// Determined from branch.*.remote configuration for the
// current branch if present, or defaults to origin.
func GetGitDefaultRemoteForPush() string {

	remote, ok := util.GlobalOptions.GitConfig[fmt.Sprintf("branch.%v.remote", GetGitCurrentBranch())]
	if ok {
		return remote
	}
	return "origin"

}

// Gets the default fetch remote for the working dir
// Determined from tracking state of current branch
// if present, or defaults to origin.
func GetGitDefaultRemoteForPull() string {

	remoteName, _ := GetGitUpstreamBranch(GetGitCurrentBranch())
	if remoteName != "" {
		return remoteName
	}
	return "origin"
}

// Get a list of git remotes
func GetGitRemotes() ([]string, error) {
	cmd := exec.Command("git", "remote")
	outp, err := cmd.StdoutPipe()
	if err != nil {
		return []string{}, fmt.Errorf("Error calling 'git remote': %v", err.Error())
	}
	scanner := bufio.NewScanner(outp)
	cmd.Start()
	var ret []string
	for scanner.Scan() {
		ret = append(ret, scanner.Text())
	}
	cmd.Wait()
	return ret, nil

}

func IsGitRemote(remoteName string) bool {
	remotes, err := GetGitRemotes()
	if err != nil {
		return false
	}
	sort.Strings(remotes)
	ret, _ := util.StringBinarySearch(remotes, remoteName)
	return ret
}

var cachedCurrentBranch string

// Get the name of the current branch
func GetGitCurrentBranch() string {
	// Use cache, we never switch branches ourselves within lifetime so save some
	// repeat calls if queried more than once
	if cachedCurrentBranch == "" {
		cmd := exec.Command("git", "branch")

		outp, err := cmd.StdoutPipe()
		if err != nil {
			util.LogErrorf("Unable to get current branch: %v", err.Error())
			return ""
		}
		cmd.Start()
		scanner := bufio.NewScanner(outp)
		found := false
		for scanner.Scan() {
			line := scanner.Text()

			if line[0] == '*' {
				cachedCurrentBranch = line[2:]
				found = true
				break
			}
		}
		cmd.Wait()

		// There's a special case in a newly initialised repository where 'git branch' returns nothing at all
		// In this case the branch really is 'master'
		if !found {
			cachedCurrentBranch = "master"
		}
	}

	return cachedCurrentBranch

}

// Parse a single git refspec string into a GitRefSpec structure ie identify ranges if present
// Does not perform any validation since refs can be symbolic anyway, up to the caller
// to check whether the returned refspec actually works
func ParseGitRefSpec(s string) *GitRefSpec {

	if idx := strings.Index(s, "..."); idx != -1 {
		// reachable from ref1 OR ref2, not both
		ref1 := strings.TrimSpace(s[:idx])
		ref2 := strings.TrimSpace(s[idx+3:])
		return &GitRefSpec{ref1, "...", ref2}
	} else if idx := strings.Index(s, ".."); idx != -1 {
		// range from ref1 -> ref2
		ref1 := strings.TrimSpace(s[:idx])
		ref2 := strings.TrimSpace(s[idx+2:])
		return &GitRefSpec{ref1, "..", ref2}
	} else {
		ref1 := strings.TrimSpace(s)
		return &GitRefSpec{Ref1: ref1}
	}

}

var IsSHARegex *regexp.Regexp = regexp.MustCompile("^[0-9A-Fa-f]{8,40}$")

// Return whether a single git reference (not refspec, so no ranges) is a full SHA or not
// SHAs can be used directly for things like lob lookup but other refs have too be converted
// This version requires a full length SHA (40 characters)
func GitRefIsFullSHA(ref string) bool {
	return len(ref) == 40 && IsSHARegex.MatchString(ref)
}

// Return whether a single git reference (not refspec, so no ranges) is a SHA or not
// SHAs can be used directly for things like lob lookup but other refs have too be converted
// This version accepts SHAs that are 8-40 characters in length, so accepts short SHAs
func GitRefIsSHA(ref string) bool {
	return IsSHARegex.MatchString(ref)
}

func GitRefToFullSHA(ref string) (string, error) {
	if GitRefIsFullSHA(ref) {
		return ref, nil
	}
	// Otherwise use Git to expand to full 40 character SHA
	cmd := exec.Command("git", "rev-parse", ref)
	outp, err := cmd.Output()
	if err != nil {
		return ref, fmt.Errorf("Unknown or ambiguous ref %v", ref)
	}
	return strings.TrimSpace(string(outp)), nil
}

// Returns whether a ref or SHA refers to a valid, existing commit or not by asking git to resolve it
func GitRefOrSHAIsValid(refOrSHA string) bool {
	// --verify doesn't actually verify commit object is valid, will return OK if it's just any 40-char SHA
	// Need to use <sha>^{commit} to verify it's a commit
	err := exec.Command("git", "rev-parse", "--verify",
		fmt.Sprintf("%v^{commit}", refOrSHA)).Run()
	return err == nil
}

// Return a list of all local branches
// Also FYI caches the current branch while we're at it so it's zero-cost to call
// GetGitCurrentBranch after this
func GetGitLocalBranches() ([]string, error) {
	cmd := exec.Command("git", "branch")

	outp, err := cmd.StdoutPipe()
	if err != nil {
		return []string{}, errors.New(fmt.Sprintf("Unable to get list local branches: %v", err.Error()))
	}
	cmd.Start()
	scanner := bufio.NewScanner(outp)
	foundcurrent := cachedCurrentBranch != ""
	var ret []string
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 2 {
			branch := line[2:]
			ret = append(ret, branch)
			// While we're at it, cache current branch
			if !foundcurrent && line[0] == '*' {
				cachedCurrentBranch = branch
				foundcurrent = true
			}

		}

	}
	cmd.Wait()

	return ret, nil

}

// Return a list of all remote branches for a given remote
// Note this doesn't retrieve mappings between local and remote branches, just a simple list
func GetGitRemoteBranches(remoteName string) ([]string, error) {
	cmd := exec.Command("git", "branch", "-r")

	outp, err := cmd.StdoutPipe()
	if err != nil {
		return []string{}, errors.New(fmt.Sprintf("Unable to get list remote branches: %v", err.Error()))
	}
	cmd.Start()
	scanner := bufio.NewScanner(outp)
	var ret []string
	prefix := remoteName + "/"
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 2 {
			line := line[2:]
			if strings.HasPrefix(line, prefix) {
				// Make sure we terminate at space, line may include alias
				remotebranch := strings.Fields(line[len(prefix):])[0]
				if remotebranch != "HEAD" {
					ret = append(ret, remotebranch)
				}
			}
		}

	}
	cmd.Wait()

	return ret, nil

}

// Return a list of branches to push by default, based on push.default and local/remote branches
// See push.default docs at https://www.kernel.org/pub/software/scm/git/docs/git-config.html
func GetGitPushDefaultBranches(remoteName string) []string {
	pushdef := util.GlobalOptions.GitConfig["push.default"]
	if pushdef == "" {
		// Use the git 2.0 'simple' default
		pushdef = "simple"
	}

	if pushdef == "matching" {
		// Multiple branches, but only where remote branch name matches
		localbranches, err := GetGitLocalBranches()
		if err != nil {
			// will be logged, safe return
			return []string{}
		}
		remotebranches, err := GetGitRemoteBranches(remoteName)
		if err != nil {
			// will be logged, safe return
			return []string{}
		}
		// Probably sorted already but to be sure
		sort.Strings(remotebranches)
		var ret []string
		for _, branch := range localbranches {
			present, _ := util.StringBinarySearch(remotebranches, branch)

			if present {
				ret = append(ret, branch)
			}
		}
		return ret
	} else if pushdef == "current" || pushdef == "upstream" || pushdef == "simple" {
		// Current, upstream, simple (in ascending complexity)
		currentBranch := GetGitCurrentBranch()
		if pushdef == "current" {
			return []string{currentBranch}
		}
		// For upstream & simple we need to know what the upstream branch is
		upstreamRemote, upstreamBranch := GetGitUpstreamBranch(currentBranch)
		// Only proceed if the upstream is on this remote
		if upstreamRemote == remoteName && upstreamBranch != "" {
			if pushdef == "upstream" {
				// For upstream we don't care what the remote branch is called
				return []string{currentBranch}
			} else {
				// "simple"
				// In this case git would only push if remote branch matches as well
				if upstreamBranch == currentBranch {
					return []string{currentBranch}
				}
			}
		}
	}

	// "nothing", something we don't understand (safety), or fallthrough non-matched
	return []string{}

}

// Get the upstream branch for a given local branch, as defined in what 'git pull' would do by default
// returns the remote name and the remote branch separately for ease of use
func GetGitUpstreamBranch(localbranch string) (remoteName, remoteBranch string) {
	// Super-verbose mode gives us tracking branch info
	cmd := exec.Command("git", "branch", "-vv")

	outp, err := cmd.StdoutPipe()
	if err != nil {
		util.LogErrorf("Unable to get list branches: %v", err.Error())
		return "", ""
	}
	cmd.Start()
	scanner := bufio.NewScanner(outp)

	// Output is like this:
	//   branch1              387def9 [origin/branch1] Another new branch
	// * master               aec3297 [origin/master: behind 1] Master change
	// * feature1             e88c156 [origin/feature1: ahead 4, behind 6] Something something dark side
	//   nottrackingbranch    f33e451 Some message

	// Extract branch name and tracking branch (won't match branches with no tracking)
	// Stops at ']' or ':' in tracking branch to deal with ahead/behind markers
	trackRegex := regexp.MustCompile(`^[* ] (\S+)\s+[a-fA-F0-9]+\s+\[([^/]+)/([^\:]+)[\]:]`)

	for scanner.Scan() {
		line := scanner.Text()
		if match := trackRegex.FindStringSubmatch(line); match != nil {
			lbranch := match[1]
			if lbranch == localbranch {
				return match[2], match[3]
			}
		}

	}
	cmd.Wait()

	// no tracking for this branch
	return "", ""

}

// Returns list of commits which have LOB SHAs referenced in them, in a given commit range
// Commits will be in ASCENDING order (parents before children) unlike WalkGitHistory
// Either of from, to or both can be blank to have an unbounded range of commits based on current HEAD
// It is required that if both are supplied, 'from' is an ancestor of 'to'
// Range is exclusive of 'from' and inclusive of 'to'
func GetGitCommitsReferencingLOBsInRange(from, to string, includePaths, excludePaths []string) ([]*CommitLOBRef, error) {
	// We want '+' lines
	return getGitCommitsReferencingLOBsInRange(from, to, true, false, includePaths, excludePaths)
}

// Returns list of commits which have LOB SHAs referenced in them, in a given commit range
// Range is exclusive of 'from' and inclusive of 'to'
// additions/removals controls whether we report only diffs with '+' lines of git-lob, '-' lines, or both
func getGitCommitsReferencingLOBsInRange(from, to string, additions, removals bool, includePaths, excludePaths []string) ([]*CommitLOBRef, error) {
	var ret []*CommitLOBRef
	callback := func(commit *CommitLOBRef) (quit bool, err error) {
		ret = append(ret, commit)
		return false, nil
	}
	err := walkGitCommitsReferencingLOBsInRange(from, to, additions, removals, includePaths, excludePaths, callback)
	return ret, err
}

// Walks a list of commits in ascending order which have LOB SHAs referenced in them, in a given commit range
// Range is exclusive of 'from' and inclusive of 'to'
// additions/removals controls whether we report only diffs with '+' lines of git-lob, '-' lines, or both
func walkGitCommitsReferencingLOBsInRange(from, to string, additions, removals bool, includePaths, excludePaths []string,
	callback func(commit *CommitLOBRef) (quit bool, err error)) error {

	args := []string{"log", `--format=commitsha: %H %P`, "-p",
		"--topo-order", "--first-parent",
		"--reverse", // we want to list them in ascending order
		"-G", SHALineRegexStr}

	if from != "" && to != "" {
		args = append(args, fmt.Sprintf("%v..%v", from, to))
	} else {
		if to != "" {
			args = append(args, to)
		} else if from != "" {
			args = append(args, fmt.Sprintf("%v..HEAD", from))
		}
		// if from & to are both blank, just use default behaviour of git log
	}

	cmd := exec.Command("git", args...)
	outp, err := cmd.StdoutPipe()
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to call git-log: %v", err.Error()))
	}
	cmd.Start()

	_, err = walkGitLogOutputForLOBReferences(outp, additions, removals, includePaths, excludePaths, callback)

	cmd.Wait()

	return err

}

// Gets a list of LOB SHAs for all binary files that are needed when checking out any of
// the commits referred to by refspec.
// As opposed to GetGitCommitsReferencingLOBsInRange which only picks up changes to LOBs,
// this function returns the complete set of LOBs needed if you checked out a commit either at
// a single commit, or any in a range (if the refspec is a range; only .. range operator allowed)
// This means it will include any LOBs that were added in commits before the range, if they are still used,
// while GetGitCommitsReferencingLOBsInRange wouldn't mention those.
// Note that git ranges are start AND end inclusive in this case.
// Note that duplicate SHAs are not eliminated for efficiency, you must do it if you need it
func GetGitAllLOBsToCheckoutInRefSpec(refspec *GitRefSpec, includePaths, excludePaths []string) ([]string, error) {

	var snapshotref string
	if refspec.IsRange() {
		if refspec.RangeOp != ".." {
			return []string{}, errors.New("Only '..' range operator allowed in GetGitAllLOBsToCheckoutInRefSpec")
		}
		// snapshot at end of range, then look at diffs later
		snapshotref = refspec.Ref2
	} else {
		snapshotref = refspec.Ref1
	}

	ret, err := GetGitAllLOBsToCheckoutAtCommit(snapshotref, includePaths, excludePaths)
	if err != nil {
		return ret, err
	}

	if refspec.IsRange() {
		// Now we have all LOBs at the snapshot, find any extra ones earlier in the range
		// to do this, we look for diffs in the commit range that start with "-git-lob:"
		// because a removal means it was referenced before that commit therefore we need it
		// to go back to that state
		// git log is range start exclusive, but that's actually OK since a -git-lob diff line
		// represents the state one commit earlier, giving us an inclusive start range
		commits, err := getGitCommitsReferencingLOBsInRange(refspec.Ref1, refspec.Ref2, false, true, includePaths, excludePaths)
		if err != nil {
			return ret, err
		}
		for _, commit := range commits {
			// possible to end up with duplicates here if same SHA referenced more than once
			// caller to resolve if they need uniques
			ret = append(ret, commit.LobSHAs...)
		}

	}

	return ret, nil

}

// Gets a list of LOB SHAs with their filenames for all binary files that are needed when checking out any of
// the commits referred to by refspec.
// As opposed to GetGitCommitsReferencingLOBsInRange which only picks up changes to LOBs,
// this function returns the complete set of LOBs needed if you checked out a commit either at
// a single commit, or any in a range (if the refspec is a range; only .. range operator allowed)
// This means it will include any LOBs that were added in commits before the range, if they are still used,
// while GetGitCommitsReferencingLOBsInRange wouldn't mention those.
// Note that git ranges are start AND end inclusive in this case.
// Note that duplicate SHAs are not eliminated for efficiency, you must do it if you need it
func GetGitAllFilesAndLOBsToCheckoutInRefSpec(refspec *GitRefSpec, includePaths, excludePaths []string) ([]*FileLOB, error) {

	var snapshotref string
	if refspec.IsRange() {
		if refspec.RangeOp != ".." {
			return nil, errors.New("Only '..' range operator allowed in GetGitAllLOBsToCheckoutInRefSpec")
		}
		// snapshot at end of range, then look at diffs later
		snapshotref = refspec.Ref2
	} else {
		snapshotref = refspec.Ref1
	}

	ret, err := GetGitAllFilesAndLOBsToCheckoutAtCommit(snapshotref, includePaths, excludePaths)
	if err != nil {
		return ret, err
	}

	if refspec.IsRange() {
		// Now we have all LOBs at the snapshot, find any extra ones earlier in the range
		// to do this, we look for diffs in the commit range that start with "-git-lob:"
		// because a removal means it was referenced before that commit therefore we need it
		// to go back to that state
		// git log is range start exclusive, but that's actually OK since a -git-lob diff line
		// represents the state one commit earlier, giving us an inclusive start range
		commits, err := getGitCommitsReferencingLOBsInRange(refspec.Ref1, refspec.Ref2, false, true, includePaths, excludePaths)
		if err != nil {
			return ret, err
		}
		for _, commit := range commits {
			// possible to end up with duplicates here if same SHA referenced more than once
			// caller to resolve if they need uniques
			ret = append(ret, commit.FileLOBs...)
		}

	}

	return ret, nil

}

// Get all the LOB SHAs that you would need to have available to check out a commit, and any other
// ancestor of it within a number of days of that commit date (not today's date)
// Note that if a LOB was modified to the same SHA more than once, duplicates may appear in the return
// They are not routinely eliminated for performance, so perform your own dupe removal if you need it
// as well as a list of LOBs, returns the commit SHA of the earliest change that was included in the scan.
// Since this is the first *change* included (which would be removing the previous SHA), the earliest LOB
// SHA included is from the *parent* of this commit.
func GetGitAllLOBsToCheckoutAtCommitAndRecent(commit string, days int, includePaths,
	excludePaths []string) (lobs []string, earliestChangeCommit string, reterr error) {
	// All LOBs at the commit itself
	shasAtCommit, err := GetGitAllLOBsToCheckoutAtCommit(commit, includePaths, excludePaths)
	if err != nil {
		return nil, "", err
	}

	// days == 0 means we only snapshot latest
	if days == 0 {
		earliest := commit
		if !GitRefIsFullSHA(earliest) {
			earliest, _ = GitRefToFullSHA(earliest)
		}
		return shasAtCommit, earliest, nil
	} else {
		ret := shasAtCommit
		earliestCommit := commit
		callback := func(lobcommit *CommitLOBRef) (quit bool, err error) {
			ret = append(ret, lobcommit.LobSHAs...)
			earliestCommit = lobcommit.Commit
			return false, nil
		}
		err := walkGitAllLOBsInRecentCommits(commit, days, includePaths, excludePaths, callback)

		return ret, earliestCommit, err
	}

}

// Get all the Filenames & LOB SHAs that you would need to have available to check out a commit, and any other
// ancestor of it within a number of days of that commit date (not today's date)
// Note that if a LOB was modified to the same SHA more than once, duplicates may appear in the return
// They are not routinely eliminated for performance, so perform your own dupe removal if you need it
// as well as a list of LOBs, returns the commit SHA of the earliest change that was included in the scan.
// Since this is the first *change* included (which would be removing the previous SHA), the earliest LOB
// SHA included is from the *parent* of this commit.
func GetGitAllFileLOBsToCheckoutAtCommitAndRecent(commit string, days int, includePaths,
	excludePaths []string) (filelobs []*FileLOB, earliestChangeCommit string, reterr error) {
	// All LOBs at the commit itself
	fileshasAtCommit, err := GetGitAllFilesAndLOBsToCheckoutAtCommit(commit, includePaths, excludePaths)
	if err != nil {
		return nil, "", err
	}

	// days == 0 means we only snapshot latest
	if days == 0 {
		earliest := commit
		if !GitRefIsFullSHA(earliest) {
			earliest, _ = GitRefToFullSHA(earliest)
		}
		return fileshasAtCommit, earliest, nil
	} else {
		ret := fileshasAtCommit
		earliestCommit := commit
		callback := func(lobcommit *CommitLOBRef) (quit bool, err error) {
			ret = append(ret, lobcommit.FileLOBs...)
			earliestCommit = lobcommit.Commit
			return false, nil
		}
		err := walkGitAllLOBsInRecentCommits(commit, days, includePaths, excludePaths, callback)

		return ret, earliestCommit, err
	}

}

// Walk backwards in history looking for all ancestors and references to LOBs in the '-' side of the diff
func walkGitAllLOBsInRecentCommits(startcommit string, days int, includePaths, excludePaths []string,
	callback func(lobcommit *CommitLOBRef) (quit bool, err error)) error {
	// get the commit date
	commitDetails, err := GetGitCommitSummary(startcommit)
	if err != nil {
		return err
	}
	sinceDate := commitDetails.CommitDate.AddDate(0, 0, -days)
	// Now use git log to scan backwards
	// We use git log from commit backwards, not commit^ (parent) because
	// we're looking for *previous* SHAs, which means we're looking for diffs
	// with a '-' line. So SHAs replaced in the latest commit are old versions too
	// that we haven't included yet in fileshasAtCommit
	args := []string{"log", `--format=commitsha: %H %P`, "-p",
		fmt.Sprintf("--since=%v", FormatGitDate(sinceDate)),
		"-G", SHALineRegexStr,
		startcommit}

	cmd := exec.Command("git", args...)
	outp, err := cmd.StdoutPipe()
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to call git-log: %v", err.Error()))
	}
	cmd.Start()

	// Looking backwards, so removals
	walkGitLogOutputForLOBReferences(outp, false, true, includePaths, excludePaths, callback)

	cmd.Wait()

	return nil
}

// Return a slice of LOB SHAs representing versions of filename, ordered by latest first
// history is from all heads not just checked out
// if shatoskip is supplied, this sha is excluded from the return if found
func GetGitAllLOBHistoryForFile(filename, shatoskip string) ([]string, error) {

	// Scan ALL history for this filename that includes a git-lob marker
	// not just history from checked out
	args := []string{"log", `--format=commitsha: %H %P`, "-p",
		"--all", "--topo-order", // ALL history in reverse order
		"-G", SHALineRegexStr,
		"--", filename}

	cmd := exec.Command("git", args...)
	outp, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Unable to call git-log: %v", err.Error()))
	}
	cmd.Start()

	// We'll just look for additions ever, walking backwards
	var ret []string
	callback := func(commitLOB *CommitLOBRef) (quit bool, err error) {
		// Already filtered by filename so there can only be one entry, but be sure
		if len(commitLOB.FileLOBs) == 1 {
			sha := commitLOB.FileLOBs[0].SHA
			if sha != shatoskip {
				ret = append(ret, sha)
			}
		}
		return false, nil
	}
	walkGitLogOutputForLOBReferences(outp, true, false, nil, nil, callback)

	cmd.Wait()

	return ret, nil

}

// Get all the binary files & their LOB SHAs that you would need to check out at a given commit (not changed in that commit)
func GetGitAllFilesAndLOBsToCheckoutAtCommit(commit string, includePaths, excludePaths []string) ([]*FileLOB, error) {
	var ret []*FileLOB
	err := WalkGitAllLOBsToCheckoutAtCommit(commit, includePaths, excludePaths, func(filelob *FileLOB) {
		ret = append(ret, filelob)
	})
	return ret, err
}

// Get all the LOB SHAs that you would need to check out at a given commit (not changed in that commit)
func GetGitAllLOBsToCheckoutAtCommit(commit string, includePaths, excludePaths []string) ([]string, error) {
	var ret []string
	err := WalkGitAllLOBsToCheckoutAtCommit(commit, includePaths, excludePaths, func(filelob *FileLOB) {
		ret = append(ret, filelob.SHA)
	})
	return ret, err
}

// Utility function to walk through all the LOBs which are present if checked out at a specific commit
func WalkGitAllLOBsToCheckoutAtCommit(commit string, includePaths, excludePaths []string,
	callback func(filelob *FileLOB)) error {

	// Snapshot using ls-tree
	args := []string{"ls-tree",
		"-r",          // recurse
		"-l",          // report object size (we'll need this)
		"--full-tree", // start at the root regardless of where we are in it
		commit}

	lstreecmd := exec.Command("git", args...)
	outp, err := lstreecmd.StdoutPipe()
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to call git ls-tree: %v", err.Error()))
	}
	defer outp.Close()
	lstreecmd.Start()
	lstreescanner := bufio.NewScanner(outp)

	// We will look for objects that are *exactly* the size of the git-lob line
	regex := regexp.MustCompile(fmt.Sprintf(`^\d+\s+blob\s+([0-9a-zA-Z]{40})\s+%d\s+(.*)$`, SHALineLen))
	// This will give us object SHAs of content which is exactly the right size, we must
	// then use cat-file (in batch mode) to get the content & parse out anything that's really
	// a git-lob reference.
	// Start git cat-file in parallel and feed its stdin
	catfilecmd := exec.Command("git", "cat-file", "--batch")
	catout, err := catfilecmd.StdoutPipe()
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to call git cat-file: %v", err.Error()))
	}
	defer catout.Close()
	catin, err := catfilecmd.StdinPipe()
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to call git cat-file: %v", err.Error()))
	}
	defer catin.Close()
	catfilecmd.Start()
	catscanner := bufio.NewScanner(catout)

	for lstreescanner.Scan() {
		line := lstreescanner.Text()
		if match := regex.FindStringSubmatch(line); match != nil {
			objsha := match[1]
			filename := match[2]
			// Apply filter
			if !util.FilenamePassesIncludeExcludeFilter(filename, includePaths, excludePaths) {
				continue
			}
			// Now feed object sha to cat-file to get git-lob SHA if any
			// remember we're already only finding files of exactly the right size (49 bytes)
			_, err := catin.Write([]byte(objsha))
			if err != nil {
				return errors.New(fmt.Sprintf("Unable to write to cat-file stream: %v", err.Error()))
			}
			_, err = catin.Write([]byte{'\n'})
			if err != nil {
				return errors.New(fmt.Sprintf("Unable to write to cat-file stream: %v", err.Error()))
			}

			// Now read back response - first line is report of object sha, type & size
			// second line is content in our case
			if !catscanner.Scan() || !catscanner.Scan() {
				return errors.New(fmt.Sprintf("Couldn't read response from cat-file stream: %v", catscanner.Err()))
			}

			// object SHA is the last 40 characters, after the prefix
			line := catscanner.Text()
			if len(line) == SHALineLen {
				lobsha := line[len(SHAPrefix):]
				// call callback to process result
				callback(&FileLOB{filename, lobsha})
			}

		}
	}
	lstreecmd.Wait()
	catfilecmd.Process.Kill()

	return nil

}

// Parse a Git date formatted in ISO 8601 format (%ci/%ai)
func ParseGitDate(str string) (time.Time, error) {

	// Unfortunately Go and Git don't overlap in their builtin date formats
	// Go's time.RFC1123Z and Git's %cD are ALMOST the same, except that
	// when the day is < 10 Git outputs a single digit, but Go expects a leading
	// zero - this is enough to break the parsing. Sigh.

	// Format is for 2 Jan 2006, 15:04:05 -7 UTC as per Go
	return time.Parse("2006-01-02 15:04:05 -0700", str)
}

// Format a date into Git format
func FormatGitDate(t time.Time) string {
	// Git format is "Fri Jun 21 20:26:41 2013 +0900" but no zero-leading for day
	return t.Format("Mon Jan 2 15:04:05 2006 -0700")
}

// Get summary information about a commit
func GetGitCommitSummary(commit string) (*GitCommitSummary, error) {
	cmd := exec.Command("git", "show", "-s",
		`--format=%H|%h|%P|%ai|%ci|%ae|%an|%ce|%cn|%s`, commit)

	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := fmt.Sprintf("Error calling git show: %v", err.Error())
		return nil, errors.New(msg)
	}

	// At most 10 substrings so subject line is not split on anything
	fields := strings.SplitN(string(out), "|", 10)
	// Cope with the case where subject is blank
	if len(fields) >= 9 {
		ret := &GitCommitSummary{}
		// Get SHAs from output, not commit input, so we can support symbolic refs
		ret.SHA = fields[0]
		ret.ShortSHA = fields[1]
		ret.Parents = strings.Split(fields[2], " ")
		// %aD & %cD (RFC2822) matches Go's RFC1123Z format
		ret.AuthorDate, _ = ParseGitDate(fields[3])
		ret.CommitDate, _ = ParseGitDate(fields[4])
		ret.AuthorEmail = fields[5]
		ret.AuthorName = fields[6]
		ret.CommitterEmail = fields[7]
		ret.CommitterName = fields[8]
		if len(fields) > 9 {
			ret.Subject = strings.TrimRight(fields[9], "\n")
		}
		return ret, nil
	} else {
		msg := fmt.Sprintf("Unexpected output from git show: %v", out)
		return nil, errors.New(msg)
	}

}

// Get a list of refs (branches, tags) that have received commits in the last numdays, ordered
// by most recent first
// You can also set numdays to -1 to not have any limit but still get them in reverse order
// remoteName is optional but if specified and includeRemoteBranches is true, will only include
// remote branches on that remote
func GetGitRecentRefs(numdays int, includeRemoteBranches bool, remoteName string) ([]*GitRef, error) {
	// Include %(objectname) AND %(*objectname), the latter only returns something if it's a tag
	// and that will be the dereferenced SHA ie the actual commit SHA instead of the tag SHA
	cmd := exec.Command("git", "for-each-ref",
		`--sort=-committerdate`,
		`--format=%(refname) %(objectname) %(*objectname)`,
		"refs")
	outp, err := cmd.StdoutPipe()
	if err != nil {
		msg := fmt.Sprintf("Unable to call git for-each-ref: %v", err.Error())
		return []*GitRef{}, errors.New(msg)
	}
	cmd.Start()
	scanner := bufio.NewScanner(outp)

	// Output is like this:
	// refs/heads/master 69d144416abf89b79f6a6fd21c2621dd9c13ead1
	// refs/remotes/origin/master ad3b29b773e46ad6870fdf08796c33d97190fe93
	// refs/tags/blah fa392f757dddf9fa7c3bb1717d0bf0c4762326fc c34b29b773e46ad6870fdf08796c33d97190fe93
	// note the second SHA when it's a tag but not otherwise

	// Output is ordered by latest commit date first, so we can stop at the threshold
	var earliestDate time.Time
	if numdays >= 0 {
		earliestDate = time.Now().AddDate(0, 0, -numdays)
	}

	regex := regexp.MustCompile(`^(refs/[^/]+/\S+)\s+([0-9A-Za-z]{40})(?:\s+([0-9A-Za-z]{40}))?`)

	var ret []*GitRef
	for scanner.Scan() {
		line := scanner.Text()
		if match := regex.FindStringSubmatch(line); match != nil {
			fullref := match[1]
			sha := match[2]
			// test for dereferenced tags, use commit SHA
			if len(match) > 3 && match[3] != "" {
				sha = match[3]
			}
			reftype, ref := ParseGitRefToTypeAndName(fullref)
			if reftype == GitRefTypeRemoteBranch || reftype == GitRefTypeRemoteTag {
				if !includeRemoteBranches {
					continue
				}
				if remoteName != "" && !strings.HasPrefix(ref, remoteName+"/") {
					continue
				}
			}
			// This is a ref we might use
			if numdays >= 0 {
				// Check the date
				commit, err := GetGitCommitSummary(ref)
				if err != nil {
					return ret, err
				}
				if commit.CommitDate.Before(earliestDate) {
					// the end
					break
				}
			}
			ret = append(ret, &GitRef{ref, reftype, sha})
		}
	}
	cmd.Wait()

	return ret, nil
}

// Tell the index to refresh for files which we've modified outside of git commands
// This is necessary because git caches stat() info to provide a fast way to detect
// modifications for git-status and so can consider files modified when they're actually not
// when we've changed things that the filter would consider unmodified when called via git-diff.
// 'files' is a list of files with paths relative to the repo root
func GitRefreshIndexForFiles(files []string) error {
	var retErr error
	// Since we don't know how many there will be, potentially split into many commands
	errorFunc := func(args []string, output string, err error) (abort bool) {
		// exit status 1 is not important, it's just '<filename> needs update'
		if !strings.HasSuffix(err.Error(), "exit status 1") {
			// We actually continue anyway to make sure we try to update all files
			// but note this one because it's odd
			if retErr == nil {
				retErr = fmt.Errorf("Post-checkout index refresh failed: %v", err.Error())
			} else {
				retErr = fmt.Errorf("%v\n%v", retErr.Error(), err.Error())
			}
		}
		return false // don't abort
	}
	// Need to make file list (which files are relative to repo root) relative to cwd for git's purposes
	relfiles := util.MakeRepoFileListRelativeToCwd(files)
	util.ExecForManyFilesSplitIfRequired(relfiles, errorFunc,
		"git", "update-index", "-q", "--really-refresh", "--")

	return retErr

}

// Get the type & name of a git reference
func ParseGitRefToTypeAndName(fullref string) (t GitRefType, name string) {
	const localPrefix = "refs/heads/"
	const remotePrefix = "refs/remotes/"
	const remoteTagPrefix = "refs/remotes/tags/"
	const localTagPrefix = "refs/tags/"

	if fullref == "HEAD" {
		name = fullref
		t = GitRefTypeHEAD
	} else if strings.HasPrefix(fullref, localPrefix) {
		name = fullref[len(localPrefix):]
		t = GitRefTypeLocalBranch
	} else if strings.HasPrefix(fullref, remotePrefix) {
		name = fullref[len(remotePrefix):]
		t = GitRefTypeRemoteBranch
	} else if strings.HasPrefix(fullref, remoteTagPrefix) {
		name = fullref[len(remoteTagPrefix):]
		t = GitRefTypeRemoteTag
	} else if strings.HasPrefix(fullref, localTagPrefix) {
		name = fullref[len(localTagPrefix):]
		t = GitRefTypeLocalTag
	} else {
		name = fullref
		t = GitRefTypeOther
	}
	return
}

// get all refs in the repo (branches, tags, stashes)
func GetGitAllRefs() ([]*GitRef, error) {
	cmd := exec.Command("git", "show-ref", "--head", "--dereference")
	outp, err := cmd.StdoutPipe()
	if err != nil {
		return []*GitRef{}, fmt.Errorf("Failure in git-show-ref: %v", err.Error())
	}
	scanner := bufio.NewScanner(outp)
	var ret []*GitRef
	cmd.Start()

	// Output is like this:
	// <sha> HEAD
	// <sha> refs/heads/<branch>
	// <sha> refs/tags/<tag>
	// <sha> refs/tags/<tag>^{}     <- dereferenced tag, should use this one instead of original
	// <sha> refs/remotes/<remotebranch>
	// <sha> refs/stash (skipped)

	for scanner.Scan() {
		line := scanner.Text()

		f := strings.Fields(line)
		if len(f) == 2 {
			sha := f[0]
			fullref := f[1]
			t, name := ParseGitRefToTypeAndName(fullref)
			if t == GitRefTypeOther {
				// skip all others (including Stash)
				continue
			}

			// Special case dereferenced tags. Non-lightweight tags refer to the tag
			// object, not the commit, but --dereference shows you the actual commit
			// with an extra ref after the tag object, called <tagname>^{}
			// This must take precedence to report the commit it applies to
			if t == GitRefTypeLocalTag && strings.HasSuffix(name, "^{}") {
				name = name[:len(name)-3]
				// now overwrite the previous tag object entry (they always come before)
				for _, ref := range ret {
					if ref.Name == name {
						ref.CommitSHA = sha
					}
				}
			} else {
				// Otherwise, new ref
				ret = append(ret, &GitRef{Name: name, Type: t, CommitSHA: sha})
			}

		}

	}
	cmd.Wait()

	return ret, nil
}

// Returns whether commit a (sha or ref) is an ancestor of commit b (sha or ref)
func GitIsAncestor(a, b string) (bool, error) {

	if !GitRefIsSHA(a) {
		var err error
		a, err = GitRefToFullSHA(a)
		if err != nil {
			return false, err
		}
	}
	if !GitRefIsSHA(b) {
		var err error
		b, err = GitRefToFullSHA(b)
		if err != nil {
			return false, err
		}
	}
	cmd := exec.Command("git", "merge-base", a, b)
	outp, err := cmd.Output()
	if err != nil {
		return false, err
	}
	base := strings.TrimSpace(string(outp))

	return base == a, nil

}

// Returns the 'best' ancestor of all the passed in refs (as a SHA)
// If a ref is listed twice the 'best' ancestor will be itself
func GetGitBestAncestor(refs []string) (ancestor string, err error) {
	args := []string{"merge-base"}
	args = append(args, refs...)
	cmd := exec.Command("git", args...)
	outp, err := cmd.Output()
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(string(outp))
	return base, nil
}

// Gets the latest change to a specific LOB file at ref, returning the SHA and the commit details
func GetGitLatestLOBChangeDetails(filename, ref string) (summary *GitCommitSummary, lobsha string, err error) {
	cmd := exec.Command("git", "log", "-p",
		"-n", "1", // one commit
		"-G", SHALineRegexStr, // if this file was ever embedded verbatim, ignore those
		`--format=commit:%H|%h|%P|%ai|%ci|%ae|%an|%ce|%cn|%s`, // standard summary info
		ref, "--", filename)
	outp, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", errors.New(fmt.Sprintf("Unable to get latest commit from %v: %v", ref, err.Error()))
	}
	cmd.Start()
	scanner := bufio.NewScanner(outp)
	summary = &GitCommitSummary{}
	lobsha = ""
	lobsharegex := regexp.MustCompile(`^\+git-lob: ([A-Fa-f0-9]{40})`)
	err = nil
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "commit:") {
			// At most 10 substrings so subject line is not split on anything
			fields := strings.SplitN(string(line[7:]), "|", 10)
			// Cope with the case where subject is blank
			if len(fields) >= 9 {
				// Get SHAs from output, not commit input, so we can support symbolic refs
				summary.SHA = fields[0]
				summary.ShortSHA = fields[1]
				summary.Parents = strings.Split(fields[2], " ")
				// %aD & %cD (RFC2822) matches Go's RFC1123Z format
				summary.AuthorDate, _ = ParseGitDate(fields[3])
				summary.CommitDate, _ = ParseGitDate(fields[4])
				summary.AuthorEmail = fields[5]
				summary.AuthorName = fields[6]
				summary.CommitterEmail = fields[7]
				summary.CommitterName = fields[8]
				if len(fields) > 9 {
					summary.Subject = strings.TrimRight(fields[9], "\n")
				}
			} else {
				msg := fmt.Sprintf("Unexpected output from git log: %v", line)
				return nil, "", errors.New(msg)
			}
		} else if match := lobsharegex.FindStringSubmatch(line); match != nil {
			lobsha = match[1]
		}
	}
	return

}
