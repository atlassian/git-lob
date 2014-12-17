package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

		})

	})

})
