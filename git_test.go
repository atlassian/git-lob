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

})
