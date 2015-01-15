package main

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var _ = Describe("Git", func() {

	Describe("Walk history", func() {
		root := filepath.Join(os.TempDir(), "GitTest")
		var oldwd string
		BeforeEach(func() {
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)
		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
		})
		// Func var so as not to pollute namespace
		testWalk := func(count, quitAfter int) {
			// Create a bunch of empty commits, doesn't matter so long as message is different each time
			// so commit SHA is unique
			msgs := GetListOfRandomSHAsForTest(count)
			var commitPoints []string
			for i, msg := range msgs {
				cmd := exec.Command("git", "commit", "--allow-empty", "-m", msg)
				if err := cmd.Run(); err != nil {
					Fail(err.Error())
				}

				if quitAfter == -1 || i >= (count-quitAfter) {
					// Record commits & make sure we walk all of them
					// Get HEAD
					cmd = exec.Command("git", "rev-parse", "HEAD")
					outp, err := cmd.Output()
					if err != nil {
						Fail(err.Error())
					}
					commitPoints = append(commitPoints, strings.TrimSpace(string(outp)))
				}
			}
			headSHA := commitPoints[len(commitPoints)-1]

			var walkedCommits = make([]string, 0, len(commitPoints))
			var walkedParents = make([]string, 0, len(commitPoints))

			walkedCount := 0
			err := WalkGitHistory(headSHA, func(currentSHA, parentSHA string) (quit bool, err error) {
				walkedCommits = append(walkedCommits, currentSHA)
				if parentSHA != "" {
					walkedParents = append(walkedParents, parentSHA)
				}
				walkedCount++
				if quitAfter != -1 && walkedCount >= quitAfter {
					return true, nil
				}
				return false, nil
			})

			var expectedLen int
			var parentExpectedLen int
			if quitAfter != -1 {
				expectedLen = quitAfter
				parentExpectedLen = expectedLen

			} else {
				expectedLen = len(commitPoints)
				parentExpectedLen = expectedLen - 1
			}
			Expect(err).To(BeNil(), "Walk shouldn't report error")
			Expect(walkedCommits).To(HaveLen(expectedLen), "Should walk the same number of commits as we created")
			Expect(walkedParents).To(HaveLen(parentExpectedLen), "Should walk one less parent than the same number of commits we created")
			// We walk in reverse order
			walkedCommitTopIndex := expectedLen - 1
			walkedParentTopIndex := parentExpectedLen - 1

			for i, expected := range commitPoints {
				Expect(walkedCommits[walkedCommitTopIndex-i]).To(Equal(expected), "Walked SHA should be the same in reverse order")
				if i > 0 {
					if parentExpectedLen != expectedLen {
						Expect(walkedParents[walkedParentTopIndex-(i-1)]).To(Equal(commitPoints[i-1]), "Walked parent SHA should be the same in reverse order")
					} else {
						Expect(walkedParents[walkedParentTopIndex-i]).To(Equal(commitPoints[i-1]), "Walked parent SHA should be the same in reverse order")
					}
				}
			}

		}
		It("Walks short history", func() {
			testWalk(10, -1)
		})

		It("Walks long history", func() {
			// test continuation (50 batch right now)
			testWalk(105, -1)
		})

		It("Aborts walk when told to", func() {
			// Callback aborts 20 in
			testWalk(105, 20)
		})

	})
	Describe("ParseGitRefSpec", func() {
		It("Parses non-range", func() {
			r := ParseGitRefSpec("master")
			Expect(r).To(Equal(&GitRefSpec{"master", "", ""}))
			r = ParseGitRefSpec("79a32558d986e35c080dd3000fb4c7608b67fb46")
			Expect(r).To(Equal(&GitRefSpec{"79a32558d986e35c080dd3000fb4c7608b67fb46", "", ""}))
		})

		It("Parses .. range", func() {
			r := ParseGitRefSpec("feature1..master")
			Expect(r).To(Equal(&GitRefSpec{"feature1", "..", "master"}))
			r = ParseGitRefSpec("0de56..HEAD^1")
			Expect(r).To(Equal(&GitRefSpec{"0de56", "..", "HEAD^1"}))
			r = ParseGitRefSpec("40940fde248a07aadf414500db594107f7d5499d..e84486d69ef5c960c5ed4b0912da919a6d2d74d8")
			Expect(r).To(Equal(&GitRefSpec{"40940fde248a07aadf414500db594107f7d5499d", "..", "e84486d69ef5c960c5ed4b0912da919a6d2d74d8"}))
		})
		It("Parses ... range", func() {
			r := ParseGitRefSpec("feature1...master")
			Expect(r).To(Equal(&GitRefSpec{"feature1", "...", "master"}))
			r = ParseGitRefSpec("40940fde248a07aadf414500db594107f7d5499d...e84486d69ef5c960c5ed4b0912da919a6d2d74d8")
			Expect(r).To(Equal(&GitRefSpec{"40940fde248a07aadf414500db594107f7d5499d", "...", "e84486d69ef5c960c5ed4b0912da919a6d2d74d8"}))
		})
	})

	Describe("GitRefIsSHA", func() {
		It("Identifies SHAs", func() {
			Expect(GitRefIsSHA("40940fde248a07aadf414500db594107f7d5499d")).To(BeTrue(), "Long SHA is SHA")
			Expect(GitRefIsFullSHA("40940fde248a07aadf414500db594107f7d5499d")).To(BeTrue(), "Long SHA is full SHA")
			Expect(GitRefIsSHA("40940fde")).To(BeTrue(), "Short SHA is SHA")
			Expect(GitRefIsFullSHA("40940fde")).To(BeFalse(), "Short SHA is not full SHA")
			Expect(GitRefIsSHA("something something something")).To(BeFalse(), "Non-SHA is not SHA")
			Expect(GitRefIsFullSHA("something something something")).To(BeFalse(), "Non-SHA is not full SHA")
			Expect(GitRefIsSHA("")).To(BeFalse(), "Blank is not SHA")
			Expect(GitRefIsFullSHA("")).To(BeFalse(), "Blank is not full SHA")
			Expect(GitRefIsSHA("40940fde248a07aadf 14500db594107f7d5499d")).To(BeFalse(), "2 short SHAs is not SHA")
			Expect(GitRefIsFullSHA("40940fde248a07aadf 14500db594107f7d5499d")).To(BeFalse(), "2 short SHAs is not full SHA")
			Expect(GitRefIsSHA("40940fdg248a07aadfe14500db594x07f7d5y99d")).To(BeFalse(), "Corrupted SHA is not SHA")
			Expect(GitRefIsFullSHA("40940fdg248a07aadfe14500db594x07f7d5y99d")).To(BeFalse(), "Corrupted SHA is not full SHA")
		})

	})

	Describe("GetGitCurrentBranch", func() {
		root := filepath.Join(os.TempDir(), "GitTest")
		var oldwd string
		BeforeEach(func() {
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)
			cachedCurrentBranch = ""
		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
			cachedCurrentBranch = ""
		})
		It("Identifies current branch", func() {
			Expect(GetGitCurrentBranch()).To(Equal("master"), "Before 1st commit should be master branch")
			cachedCurrentBranch = ""
			exec.Command("git", "commit", "--allow-empty", "-m", "First commit").Run()
			Expect(GetGitCurrentBranch()).To(Equal("master"), "After 1st commit should be master branch")
			cachedCurrentBranch = ""
			CreateBranchForTest("feature1")
			CheckoutForTest("feature1")
			Expect(GetGitCurrentBranch()).To(Equal("feature1"), "After creating new branch current branch should be updated")
			exec.Command("git", "commit", "--allow-empty", "-m", "Second commit").Run()
			cachedCurrentBranch = ""
			Expect(GetGitCurrentBranch()).To(Equal("feature1"), "After creating new branch & committing current branch should be updated")
			//cachedCurrentBranch = "" - note NOT clearing cache to test it
			exec.Command("git", "checkout", "master").Run()
			Expect(GetGitCurrentBranch()).To(Equal("feature1"), "Without clearing cache, current branch should be previous value")
			cachedCurrentBranch = ""
			Expect(GetGitCurrentBranch()).To(Equal("master"), "After clearing cache, current branch should be updated")

		})

	})
	Describe("GetGitCurrentBranch", func() {
		root := filepath.Join(os.TempDir(), "GitTest")
		var oldwd string
		BeforeEach(func() {
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)
		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
		})
		It("Lists branches", func() {
			exec.Command("git", "commit", "--allow-empty", "-m", "First commit").Run()
			CreateBranchForTest("feature/ABC")
			CheckoutForTest("feature/ABC")
			exec.Command("git", "commit", "--allow-empty", "-m", "Second commit").Run()
			CreateBranchForTest("feature/DEF")
			CheckoutForTest("feature/DEF")
			exec.Command("git", "commit", "--allow-empty", "-m", "3rd commit").Run()
			CheckoutForTest("master")
			CreateBranchForTest("release/1.1")
			CreateBranchForTest("release/1.2")
			CreateBranchForTest("something")

			branches, err := GetGitLocalBranches()
			Expect(err).To(BeNil(), "Should not error in GetGitLocalBranches")
			Expect(branches).To(HaveLen(6), "Should be 6 branches")
			Expect(branches).To(ContainElement("master"))
			Expect(branches).To(ContainElement("feature/ABC"))
			Expect(branches).To(ContainElement("feature/DEF"))
			Expect(branches).To(ContainElement("release/1.1"))
			Expect(branches).To(ContainElement("release/1.2"))
			Expect(branches).To(ContainElement("something"))

		})

	})
	Describe("Remote branches & tracking", func() {
		root := filepath.Join(os.TempDir(), "GitTest")
		remotePath := filepath.Join(os.TempDir(), "GitTestRemote")
		var oldwd string
		BeforeEach(func() {
			CreateGitRepoForTest(root)
			CreateBareGitRepoForTest(remotePath)
			oldwd, _ = os.Getwd()
			os.Chdir(root)
			// Make a file:// ref so we don't have hardlinks (more standard)
			remotePathUrl := strings.Replace(remotePath, "\\", "/", -1)
			remotePathUrl = "file://" + remotePathUrl
			exec.Command("git", "remote", "add", "origin", remotePathUrl).Run()
		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
			os.RemoveAll(remotePath)
		})

		It("Reports remote branches correctly", func() {

			// Create a bunch of local branches
			exec.Command("git", "commit", "--allow-empty", "-m", "First commit").Run()
			CreateBranchForTest("feature/ABC")
			CheckoutForTest("feature/ABC")
			exec.Command("git", "commit", "--allow-empty", "-m", "Second commit").Run()
			CreateBranchForTest("feature/DEF")
			CheckoutForTest("feature/DEF")
			exec.Command("git", "commit", "--allow-empty", "-m", "3rd commit").Run()
			CheckoutForTest("master")
			CreateBranchForTest("release/1.1")
			CreateBranchForTest("release/1.2")
			CreateBranchForTest("something")
			// Push some of those branches & set up tracking
			exec.Command("git", "push", "--set-upstream", "origin", "master:master").Run()
			exec.Command("git", "push", "--set-upstream", "origin", "feature/ABC:feature/ABC").Run()
			exec.Command("git", "push", "--set-upstream", "origin", "feature/DEF:feature/DEFchangedonremote").Run()
			// Push one that we DON'T set tracking branch for
			exec.Command("git", "push", "origin", "something").Run()
			// List remote branches
			remoteBranches, err := GetGitRemoteBranches("origin")
			Expect(err).To(BeNil(), "Should not error listing remote branches")
			Expect(remoteBranches).To(HaveLen(4), "Should be 3 remote branches")
			Expect(remoteBranches).To(ContainElement("master"), "Remote branch list check")
			Expect(remoteBranches).To(ContainElement("feature/ABC"), "Remote branch list check")
			Expect(remoteBranches).To(ContainElement("feature/DEFchangedonremote"), "Remote branch list check")
			Expect(remoteBranches).To(ContainElement("something"), "Remote branch list check")

			// now check tracking
			remote, branch := GetGitUpstreamBranch("master")
			Expect(remote).To(Equal("origin"), "Remote should be origin in tracking")
			Expect(branch).To(Equal("master"), "Master should track master")
			remote, branch = GetGitUpstreamBranch("feature/ABC")
			Expect(remote).To(Equal("origin"), "Remote should be origin in tracking")
			Expect(branch).To(Equal("feature/ABC"), "feature/ABC should track feature/ABC")
			remote, branch = GetGitUpstreamBranch("feature/DEF")
			Expect(remote).To(Equal("origin"), "Remote should be origin in tracking")
			Expect(branch).To(Equal("feature/DEFchangedonremote"), "feature/DEF should track feature/DEFchangedonremote")
			remote, branch = GetGitUpstreamBranch("something")
			Expect(remote).To(Equal(""), "Should be no remote for untracked branch")
			Expect(branch).To(Equal(""), "Should be no branch for untracked branch")
			remote, branch = GetGitUpstreamBranch("release/1.1")
			Expect(remote).To(Equal(""), "Should be no remote for untracked branch")
			Expect(branch).To(Equal(""), "Should be no branch for untracked branch")

			// Check tracking works with ahead / behind
			// Make 2 local commits on master, test ahead only
			exec.Command("git", "commit", "--allow-empty", "-m", "4th commit").Run()
			exec.Command("git", "commit", "--allow-empty", "-m", "5th commit").Run()
			remote, branch = GetGitUpstreamBranch("master")
			Expect(remote).To(Equal("origin"), "Remote should be origin in tracking when ahead")
			Expect(branch).To(Equal("master"), "Master should track master when ahead")
			// Push these to remote so we can test behind
			exec.Command("git", "push", "origin", "master:master").Run()
			// now reset 1 commit back so we're ahead 1, behind 1
			exec.Command("git", "reset", "--hard", "HEAD^").Run()
			remote, branch = GetGitUpstreamBranch("master")
			Expect(remote).To(Equal("origin"), "Remote should be origin in tracking when ahead and behind")
			Expect(branch).To(Equal("master"), "Master should track master when ahead and behind")
			// now reset 1 MORE commit back so we're behind 2
			exec.Command("git", "reset", "--hard", "HEAD^").Run()
			remote, branch = GetGitUpstreamBranch("master")
			Expect(remote).To(Equal("origin"), "Remote should be origin in tracking when behind")
			Expect(branch).To(Equal("master"), "Master should track master when behind")

		})

	})

	Context("Commit LOB references", func() {

		root := filepath.Join(os.TempDir(), "GitTest")
		var oldwd string
		lobshas := GetListOfRandomSHAsForTest(10)
		var correctSHAs [][]string

		BeforeEach(func() {
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)

			// Add a few files with some lob SHAs (fake content, no store)
			ioutil.WriteFile(filepath.Join(root, "file1.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[0])), 0644)
			ioutil.WriteFile(filepath.Join(root, "file2.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[1])), 0644)
			exec.Command("git", "add", "file1.txt", "file2.txt").Run()
			exec.Command("git", "commit", "-m", "Initial").Run()
			// Tag at useful points
			exec.Command("git", "tag", "tag1").Run()
			// add another file & modify
			ioutil.WriteFile(filepath.Join(root, "file2.txt"), // replacement
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[2])), 0644)
			ioutil.WriteFile(filepath.Join(root, "file3.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[3])), 0644)
			exec.Command("git", "add", "file2.txt", "file3.txt").Run()
			exec.Command("git", "commit", "-m", "2nd commit").Run()
			exec.Command("git", "tag", "tag2").Run()
			// Also include commit that references NO shas
			exec.Command("git", "commit", "--allow-empty", "-m", "Non-LOB commit").Run()

			ioutil.WriteFile(filepath.Join(root, "file4.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[4])), 0644)
			exec.Command("git", "add", "file4.txt").Run()
			exec.Command("git", "commit", "-m", "3rd commit").Run()
			exec.Command("git", "tag", "tag3").Run()
			ioutil.WriteFile(filepath.Join(root, "file1.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[5])), 0644)
			exec.Command("git", "add", "file1.txt").Run()
			exec.Command("git", "commit", "-m", "4th commit").Run()
			exec.Command("git", "tag", "tag4").Run()
			ioutil.WriteFile(filepath.Join(root, "file5.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[6])), 0644)
			exec.Command("git", "add", "file5.txt").Run()
			exec.Command("git", "commit", "-m", "5th commit").Run()
			exec.Command("git", "tag", "tag5").Run()
			// Now create a separate branch from tag3 for 7-9 shas
			exec.Command("git", "checkout", "-b", "feature/1", "tag3").Run()
			ioutil.WriteFile(filepath.Join(root, "file2.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[7])), 0644)
			ioutil.WriteFile(filepath.Join(root, "file3.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[8])), 0644)
			exec.Command("git", "add", "file2.txt", "file3.txt").Run()
			exec.Command("git", "commit", "-m", "Feature commit 1").Run()
			ioutil.WriteFile(filepath.Join(root, "file10.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[9])), 0644)
			exec.Command("git", "add", "file10.txt").Run()
			exec.Command("git", "commit", "-m", "Feature commit 2").Run()
			// return to master
			exec.Command("git", "checkout", "master").Run()

			correctSHAs = [][]string{
				{lobshas[0], lobshas[1]}, // tag1, master & feature
				{lobshas[2], lobshas[3]}, // tag2, master & feature
				{lobshas[4]},             // tag3, master & feature
				{lobshas[5]},             // tag4, master only
				{lobshas[6]},             // tag5, master only
				{lobshas[7], lobshas[8]}, // feature only
				{lobshas[9]},             // feature only
			}

		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
		})

		Describe("Query commit LOB references", func() {
			It("Retrieves LOB references", func() {
				// Now let's retrieve LOBs
				// Entire history on current branch
				commitlobs, err := GetGitCommitsReferencingLOBsInRange("", "")
				Expect(err).To(BeNil(), "Should not fail calling GetGitCommitsReferencingLOBsInRange")
				// There are 6 commits on the master branch, but only 5 reference LOBs
				Expect(commitlobs).To(HaveLen(5), "Master branch should have 5 commits referencing LOBs")
				for i, commit := range commitlobs {
					Expect(commit.lobSHAs).To(Equal(correctSHAs[i]), "Commit %d should have correct SHAs", i)
				}
				// Just feature branch
				commitlobs, err = GetGitCommitsReferencingLOBsInRange("tag3", "feature/1")
				Expect(err).To(BeNil(), "Should not fail calling GetGitCommitsReferencingLOBsInRange")
				// 2 commits from tag3 to feature/1, excluding tag3 itself
				Expect(commitlobs).To(HaveLen(2), "Feature branch should have 2 commits referencing LOBs")
				Expect(commitlobs[0].lobSHAs).To(Equal(correctSHAs[5]), "Commit should have correct SHAs")
				Expect(commitlobs[1].lobSHAs).To(Equal(correctSHAs[6]), "Commit should have correct SHAs")
				// Now just 'from' (on master)
				commitlobs, err = GetGitCommitsReferencingLOBsInRange("tag4", "")
				Expect(err).To(BeNil(), "Should not fail calling GetGitCommitsReferencingLOBsInRange")
				// 1 commit from tag4 to master, excluding tag4 itself
				Expect(commitlobs).To(HaveLen(1), "tag4 onwards is only 1 commit")
				Expect(commitlobs[0].lobSHAs).To(Equal(correctSHAs[4]), "Commit should have correct SHAs")
				// Now just 'to' (on master)
				commitlobs, err = GetGitCommitsReferencingLOBsInRange("", "tag2")
				Expect(err).To(BeNil(), "Should not fail calling GetGitCommitsReferencingLOBsInRange")
				// 2 commits up to tag2 to master, excluding tag4 itself
				Expect(commitlobs).To(HaveLen(2), "tag4 onwards is only 1 commit")
				Expect(commitlobs[0].lobSHAs).To(Equal(correctSHAs[0]), "Commit should have correct SHAs")
				Expect(commitlobs[1].lobSHAs).To(Equal(correctSHAs[1]), "Commit should have correct SHAs")

			})

		})

		Describe("Get all LOBs at a commit and refspec range", func() {

			It("Gets LOB references at varying ranges", func() {
				// Get all LOBs referenced ever at master
				shas, err := GetGitAllLOBsToCheckoutInRefSpec(&GitRefSpec{"tag1", "..", "master"})
				// Because it's a range this will also include any which were later overwritten
				Expect(err).To(BeNil(), "Should be no error")
				Expect(shas).To(ConsistOf(lobshas[:7]), "Start to master should include first 7 file SHAs")

				// At tag 2, file2.txt was overwritten with a different SHA so the previous SHA (lobshas[1]) should be missing
				correct := lobshas[:1]
				correct = append(correct, lobshas[2:7]...)
				shas, err = GetGitAllLOBsToCheckoutInRefSpec(&GitRefSpec{"tag2", "..", "master"})
				Expect(err).To(BeNil(), "Should be no error")
				Expect(shas).To(ConsistOf(correct), "tag2 to master should include first 7 file SHAs minus one overwritten SHA")

			})

		})
	})
	Describe("Git commit summary", func() {

		root := filepath.Join(os.TempDir(), "GitTest")
		var oldwd string

		BeforeEach(func() {
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)
		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
		})

		It("Correctly queries commit summaries", func() {
			exec.Command("git",
				"-c", "user.name=Joe Bloggs",
				"-c", "user.email=joe@bloggs.com",
				"commit", "--allow-empty", "-m", "This is a commit",
				"--author=A N Author <author@something.com>",
				"--date=2010-03-01T14:12:00+00:00",
			).Run()
			now := time.Now()

			commit, err := GetGitCommitSummary("HEAD")
			Expect(err).To(BeNil(), "Should not be error calling git show")
			headsha, _ := GitRefToFullSHA("HEAD")
			Expect(commit.SHA).To(Equal(headsha), "SHA should be correct")
			Expect(commit.ShortSHA).To(Equal(headsha[0:7]), "Short SHA should be correct")
			Expect(commit.AuthorName).To(Equal("A N Author"), "Author should be correct")
			Expect(commit.AuthorEmail).To(Equal("author@something.com"), "Author email should be correct")
			Expect(commit.CommitterName).To(Equal("Joe Bloggs"), "Committer should be correct")
			Expect(commit.CommitterEmail).To(Equal("joe@bloggs.com"), "Committer email should be correct")
			Expect(commit.Subject).To(Equal("This is a commit"), "Subject should be correct")
			Expect(commit.CommitDate).To(BeTemporally("~", now, time.Second), "Commit date should be within a second of now")
			Expect(commit.AuthorDate).To(BeTemporally("~", time.Date(2010, 03, 01, 14, 12, 0, 0, time.UTC), time.Millisecond), "Author date should be correct")

		})
		It("Correctly queries commit summaries when subject includes separator character", func() {
			exec.Command("git",
				"-c", "user.name=Joe Bloggs",
				"-c", "user.email=joe@bloggs.com",
				"commit", "--allow-empty", "-m", "This is |a commit|with pipes in it|",
				"--author=A N Author <author@something.com>",
				"--date=2010-03-01T14:12:00+00:00",
			).Run()
			now := time.Now()

			commit, err := GetGitCommitSummary("HEAD")
			Expect(err).To(BeNil(), "Should not be error calling git show")
			headsha, _ := GitRefToFullSHA("HEAD")
			Expect(commit.SHA).To(Equal(headsha), "SHA should be correct")
			Expect(commit.ShortSHA).To(Equal(headsha[0:7]), "Short SHA should be correct")
			Expect(commit.AuthorName).To(Equal("A N Author"), "Author should be correct")
			Expect(commit.AuthorEmail).To(Equal("author@something.com"), "Author email should be correct")
			Expect(commit.CommitterName).To(Equal("Joe Bloggs"), "Committer should be correct")
			Expect(commit.CommitterEmail).To(Equal("joe@bloggs.com"), "Committer email should be correct")
			Expect(commit.Subject).To(Equal("This is |a commit|with pipes in it|"), "Subject should be correct")
			Expect(commit.CommitDate).To(BeTemporally("~", now, time.Second), "Commit date should be within a second of now")
			Expect(commit.AuthorDate).To(BeTemporally("~", time.Date(2010, 03, 01, 14, 12, 0, 0, time.UTC), time.Millisecond), "Author date should be correct")

		})

	})

	Describe("Git recent refs and recent git-lob references", func() {

		// set GIT_COMMITTER_DATE environment var e.g. "Fri Jun 21 20:26:41 2013 +0900"

		root := filepath.Join(os.TempDir(), "GitTest")
		var oldwd string
		lobshas := GetListOfRandomSHAsForTest(16)
		var correctRefs []string
		var correctLOBsMaster []string
		var correctLOBsFeature1 []string
		var correctLOBsFeature2 []string

		BeforeEach(func() {
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)

			// The setup:
			// master, feature/1 and feature/2 are 'recent refs', 'feature/3' is not
			// master has one commit excluded from its range, the rest are included
			// feature/1 has one commit excluded and 2 included
			// feature/2 has 2 commits excluded and 1 included

			// add one hour forward to the threshold date so we always create commits within time of test run
			refsIncludedDate := time.Now().AddDate(0, 0, -GlobalOptions.RecentRefsPeriodDays).Add(time.Hour)
			refsExcludedDate := refsIncludedDate.Add(-time.Hour * 2)
			// Commit inclusion is based on the latest commit made - so make sure latest commit is before today for test
			latestHEADCommitDate := time.Now().AddDate(0, -2, -3)
			latestFeature1CommitDate := time.Now().AddDate(0, 0, -4)
			latestFeature2CommitDate := time.Now().AddDate(0, -1, 0)
			latestFeature3CommitDate := refsExcludedDate.AddDate(0, -1, 0) // will be excluded
			headCommitsIncludedDate := latestHEADCommitDate.AddDate(0, 0, -GlobalOptions.RecentCommitsPeriodHEAD).Add(time.Hour)
			headCommitsExcludedDate := headCommitsIncludedDate.Add(-time.Hour * 2)
			feature1CommitsIncludedDate := latestFeature1CommitDate.AddDate(0, 0, -GlobalOptions.RecentCommitsPeriodOther).Add(time.Hour)
			feature2CommitsIncludedDate := latestFeature2CommitDate.AddDate(0, 0, -GlobalOptions.RecentCommitsPeriodOther).Add(time.Hour)

			// Function to commit at a specific date
			commitAtDate := func(t time.Time, msg string) error {
				cmd := exec.Command("git", "commit", "--allow-empty", "-m", msg)
				env := os.Environ()
				// set GIT_COMMITTER_DATE environment var e.g. "Fri Jun 21 20:26:41 2013 +0900"
				env = append(env, fmt.Sprintf("GIT_COMMITTER_DATE=%v", FormatGitDate(t)))
				cmd.Env = env
				return cmd.Run()
			}
			// Master branch (which will be HEAD)
			ioutil.WriteFile(filepath.Join(root, "file1.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[0])), 0644) // excluded
			ioutil.WriteFile(filepath.Join(root, "file2.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[1])), 0644) // excluded
			exec.Command("git", "add", "file1.txt", "file2.txt").Run()
			// exclude commit 1
			commitAtDate(headCommitsExcludedDate.Add(-time.Hour*24*30), "Initial")

			ioutil.WriteFile(filepath.Join(root, "file1.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[2])), 0644) // included
			ioutil.WriteFile(filepath.Join(root, "file2.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[3])), 0644) // included
			exec.Command("git", "add", "file1.txt", "file2.txt").Run()
			// commit 2 will be excluded, but its state will 'overlap' into the valid date range as a -ve diff
			commitAtDate(headCommitsExcludedDate.Add(-time.Hour*24*15), "Second commit")
			correctLOBsMaster = append(correctLOBsMaster, lobshas[2], lobshas[3])

			exec.Command("git", "tag", "start").Run()
			// Create a branch we're going to exclude
			exec.Command("git", "checkout", "-b", "feature/3").Run()
			ioutil.WriteFile(filepath.Join(root, "file20.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[4])), 0644) // excluded
			exec.Command("git", "add", "file20.txt").Run()
			// We'll never see this commit or the branch
			commitAtDate(latestFeature3CommitDate, "Feature 3 commit")
			// Back to master
			exec.Command("git", "checkout", "master").Run()

			// add another file & modify
			ioutil.WriteFile(filepath.Join(root, "file2.txt"), // replacement
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[5])), 0644) // included
			ioutil.WriteFile(filepath.Join(root, "file3.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[6])), 0644) // included
			exec.Command("git", "add", "file2.txt", "file3.txt").Run()
			// include commit 2
			commitAtDate(headCommitsIncludedDate.Add(time.Hour*24), "Third commit")
			correctLOBsMaster = append(correctLOBsMaster, lobshas[5], lobshas[6])
			// Also include commit that references NO shas
			commitAtDate(headCommitsIncludedDate.Add(time.Hour*48), "Non-LOB commit")

			// Create another feature branch that we'll include, but not all the commits
			exec.Command("git", "tag", "feature/1/start").Run()
			exec.Command("git", "checkout", "-b", "feature/1").Run()
			ioutil.WriteFile(filepath.Join(root, "file3.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[7])), 0644) // excluded
			exec.Command("git", "add", "file3.txt").Run()
			// We'll never see this commit but we will see the branch (commit later)
			commitAtDate(feature1CommitsIncludedDate.Add(-time.Hour*48), "Feature 1 excluded commit")
			ioutil.WriteFile(filepath.Join(root, "file3.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[8])), 0644) // included
			exec.Command("git", "add", "file3.txt").Run()
			// We'll see this commit because the next commit will be the tip & range will include it
			commitAtDate(feature1CommitsIncludedDate.Add(-time.Hour*4), "Feature 1 included commit")
			correctLOBsFeature1 = append(correctLOBsFeature1, lobshas[8])

			ioutil.WriteFile(filepath.Join(root, "file3.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[9])), 0644) // included
			exec.Command("git", "add", "file3.txt").Run()
			// We'll see this commit because the next commit will be the tip & range will include it
			commitAtDate(latestFeature1CommitDate, "Feature 1 tip commit")
			correctLOBsFeature1 = append(correctLOBsFeature1, lobshas[9])
			// Also include unchanged file1.txt at this state and old state of file2.txt
			correctLOBsFeature1 = append(correctLOBsFeature1, lobshas[2], lobshas[5])

			// Back to master
			exec.Command("git", "checkout", "master").Run()

			// Create another feature branch that we'll include, but not all the commits
			exec.Command("git", "tag", "feature/2/start").Run()
			exec.Command("git", "checkout", "-b", "feature/2").Run()
			ioutil.WriteFile(filepath.Join(root, "file4.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[10])), 0644) // excluded
			exec.Command("git", "add", "file4.txt").Run()
			// We'll never see this commit but we will see the branch (commit later)
			commitAtDate(feature2CommitsIncludedDate.Add(-time.Hour*24*3), "Feature 2 excluded commit")
			ioutil.WriteFile(filepath.Join(root, "file4.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[11])), 0644) // included
			exec.Command("git", "add", "file4.txt").Run()
			// We'll see this commit because the next commit will be the tip & range will include it
			commitAtDate(feature2CommitsIncludedDate.Add(-time.Hour*24*2), "Feature 2 excluded commit")
			correctLOBsFeature2 = append(correctLOBsFeature2, lobshas[11])
			ioutil.WriteFile(filepath.Join(root, "file4.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[12])), 0644) // included
			exec.Command("git", "add", "file4.txt").Run()
			// We'll see this commit
			commitAtDate(latestFeature2CommitDate, "Feature 2 tip commit")
			correctLOBsFeature2 = append(correctLOBsFeature2, lobshas[12])
			// Also include unchanged files on this branch: file1-3.txt last state & included versions
			correctLOBsFeature2 = append(correctLOBsFeature2, lobshas[5], lobshas[6], lobshas[2])

			// Back to master to finish
			exec.Command("git", "checkout", "master").Run()

			ioutil.WriteFile(filepath.Join(root, "file6.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[13])), 0644) // included
			exec.Command("git", "add", "file6.txt").Run()
			commitAtDate(headCommitsIncludedDate.Add(time.Hour*24*3), "Master commit")
			correctLOBsMaster = append(correctLOBsMaster, lobshas[13])

			ioutil.WriteFile(filepath.Join(root, "file5.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[14])), 0644) // included
			exec.Command("git", "add", "file5.txt").Run()
			commitAtDate(headCommitsIncludedDate.Add(time.Hour*24*4), "Master penultimate commit")
			correctLOBsMaster = append(correctLOBsMaster, lobshas[14])

			ioutil.WriteFile(filepath.Join(root, "file5.txt"),
				[]byte(fmt.Sprintf("git-lob: %v", lobshas[15])), 0644) // included
			exec.Command("git", "add", "file5.txt").Run()
			commitAtDate(latestHEADCommitDate, "Master tip commit")
			correctLOBsMaster = append(correctLOBsMaster, lobshas[15])

			correctRefs = []string{"master", "feature/1", "feature/2"}

		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
		})
		It("Retrieves recent git refs & LOBs", func() {
			recentrefs, err := GetGitRecentRefs(GlobalOptions.RecentRefsPeriodDays, false, "")
			Expect(err).To(BeNil(), "Should not error calling GetGitRecentRefs")
			Expect(recentrefs).To(ConsistOf(correctRefs), "Recent refs should be correct")

			lobs, err := GetGitAllLOBsToCheckoutAtCommitAndRecent("master", GlobalOptions.RecentCommitsPeriodHEAD)
			Expect(err).To(BeNil(), "Should not error getting lobs")
			Expect(lobs).To(ConsistOf(correctLOBsMaster), fmt.Sprintf("LOBs on master should be correct; all LOBS were:\n%v", strings.Join(lobshas, "\n")))

			// It's harder to visualise the feature branches because unchanged files from other branches are included
			lobs, err = GetGitAllLOBsToCheckoutAtCommitAndRecent("feature/1", GlobalOptions.RecentCommitsPeriodOther)
			Expect(err).To(BeNil(), "Should not error getting lobs")
			Expect(lobs).To(ConsistOf(correctLOBsFeature1), fmt.Sprintf("LOBs on feature/1 should be correct; all LOBS were:\n%v", strings.Join(lobshas, "\n")))
			lobs, err = GetGitAllLOBsToCheckoutAtCommitAndRecent("feature/2", GlobalOptions.RecentCommitsPeriodOther)
			Expect(err).To(BeNil(), "Should not error getting lobs")
			Expect(lobs).To(ConsistOf(correctLOBsFeature2), fmt.Sprintf("LOBs on feature/2 should be correct; all LOBS were:\n%v", strings.Join(lobshas, "\n")))
			// TODO remote branches
			// TODO test tags

			// TODO test LOBs

		})

	})
})
