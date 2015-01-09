package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// A git reference or reference range

type GitRefSpec struct {
	// First ref
	Ref1 string
	// Optional range operator if this is a range refspec (".." or "...")
	RangeOp string
	// Optional second ref
	Ref2 string
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
	commit  string
	lobSHAs []string
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
		// get 50 parents
		// format as <SHA> <PARENT> so we can detect the end of history
		cmd := exec.Command("git", "log", "--first-parent", "--topo-order",
			"-n", "50", "--format=%H %P", currentLogHEAD)

		outp, err := cmd.StdoutPipe()
		if err != nil {
			LogErrorf("Unable to list commits from %v: %v", currentLogHEAD, err.Error())
			return err
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

// Gets the default push remote for the working dir
// Determined from branch.*.remote configuration for the
// current branch if present, or defaults to origin.
func GetGitDefaultRemoteForPush() string {

	remote, ok := GlobalOptions.GitConfig[fmt.Sprintf("branch.%v.remote", GetGitCurrentBranch())]
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

var cachedCurrentBranch string

// Get the name of the current branch
func GetGitCurrentBranch() string {
	// Use cache, we never switch branches ourselves within lifetime so save some
	// repeat calls if queried more than once
	if cachedCurrentBranch == "" {
		cmd := exec.Command("git", "branch")

		outp, err := cmd.StdoutPipe()
		if err != nil {
			LogErrorf("Unable to get current branch: %v", err.Error())
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
		return ref, fmt.Errorf("Can't convert %v to a SHA: %v", ref, err.Error())
	}
	return strings.TrimSpace(string(outp)), nil
}

// Return a list of all local branches
// Also FYI caches the current branch while we're at it so it's zero-cost to call
// GetGitCurrentBranch after this
func GetGitLocalBranches() ([]string, error) {
	cmd := exec.Command("git", "branch")

	outp, err := cmd.StdoutPipe()
	if err != nil {
		LogErrorf("Unable to get list local branches: %v", err.Error())
		return []string{}, err
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
		LogErrorf("Unable to get list remote branches: %v", err.Error())
		return []string{}, err
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
	pushdef := GlobalOptions.GitConfig["push.default"]
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
			present, _ := StringBinarySearch(remotebranches, branch)

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
		LogErrorf("Unable to get list branches: %v", err.Error())
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
func GetGitCommitsReferencingLOBsInRange(from, to string) ([]CommitLOBRef, error) {

	args := []string{"log", `--format=commitsha: %H`, "-p",
		"--topo-order", "--first-parent",
		"--reverse", // we want to list them in ascending order
		"-G", "^git-lob: [A-Fa-f0-9]{40}$"}

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

	// Sadly we still get more output than we actually need, but this is the minimum we can get
	// For each commit we'll get something like this:
	/*
	   COMMITSHA:af2607421c9fee2e430cde7e7073a7dad07be559

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
	// So it's important that we only pull git-lob SHAs with a '+' prefix

	// Use 1 regex to capture all for speed
	regex := regexp.MustCompile(`^(commitsha|\+git-lob): ([A-Fa-f0-9]{40})`)

	cmd := exec.Command("git", args...)
	outp, err := cmd.StdoutPipe()
	if err != nil {
		LogErrorf("Unable to call git-log: %v", err.Error())
		return []CommitLOBRef{}, err
	}
	cmd.Start()
	scanner := bufio.NewScanner(outp)

	var currentCommit *CommitLOBRef
	var ret []CommitLOBRef
	for scanner.Scan() {
		line := scanner.Text()
		if match := regex.FindStringSubmatch(line); match != nil {
			sha := match[2]
			if match[1] == "commitsha" {
				if currentCommit != nil {
					ret = append(ret, *currentCommit)
					currentCommit = nil
				}
				currentCommit = &CommitLOBRef{commit: sha}
			} else { // git-lob is the only other possibility from regex, skip pointless test
				currentCommit.lobSHAs = append(currentCommit.lobSHAs, sha)
			}
		}
	}
	// Final commit
	if currentCommit != nil {
		ret = append(ret, *currentCommit)
		currentCommit = nil
	}

	return ret, nil

}

// Expands a refspec range into a list of commits (full SHAs) using 'git log'
// For empty ranges just returns refspec.Ref1 which will not be resolved to a full SHA
func GitExpandRefSpecRangeToCommits(refspec *GitRefSpec) ([]string, error) {
	if refspec.IsEmptyRange() {
		// For empty range just include the ref
		return []string{refspec.Ref1}, nil
	}

	var ret []string
	cmd := exec.Command("git", "log", "--topo-order",
		"--format=%H", refspec.String())

	outp, err := cmd.StdoutPipe()
	if err != nil {
		LogErrorf("Unable to list commits for %v: %v", refspec.String(), err.Error())
		return nil, err
	}
	cmd.Start()
	scanner := bufio.NewScanner(outp)
	for scanner.Scan() {
		currentLine := scanner.Text()
		if len(currentLine) >= 40 {
			ret = append(ret, currentLine[:40])
		}
	}
	cmd.Wait()

	return ret, nil

}

// Get a list of refs (branches, tags) that have received commits in the last numdays
func GetGitRecentRefs(numdays int) ([]string, error) {
	// TODO
	return nil, nil
}

// Get a refspec for a commit range representing all commits within numdays of when commit was made
func GetGitRecentCommitRange(commit string, numdays int) (*GitRefSpec, error) {
	// TODO
	return nil, nil
}
