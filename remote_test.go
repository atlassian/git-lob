package main

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"os"
	"path/filepath"
)

var _ = Describe("Remote", func() {
	Describe("Storage of remote state cache", func() {

		root := filepath.Join(os.TempDir(), "RemoteStateTest")
		remote1Name := "origin"
		remote2Name := "fork"
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

		It("saves and restores push state", func() {

			sha := "b09bfdf65bb51bb50307f93ab930dd7708a5b6dc"
			sha2 := "c1234567890fdf651bb5f93ab930dd7708002341"
			sha3 := "d3f8734986de0f0a08b5f93ab930dd7708030dde"

			err := MarkBinariesAsPushed(remote1Name, sha, "")
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			pushed := GetPushedCommits(remote1Name)
			Expect(pushed).To(Equal([]string{sha}), "Should record pushed")

			err = MarkBinariesAsPushed(remote1Name, sha2, "")
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			pushed = GetPushedCommits(remote1Name)
			Expect(pushed).To(Equal([]string{sha, sha2}), "Should record pushed, in order")

			// Add a duplicate, confirm not added but replaced
			err = MarkBinariesAsPushed(remote1Name, sha, "")
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			pushed = GetPushedCommits(remote1Name)
			Expect(pushed).To(Equal([]string{sha, sha2}), "Should not add duplicates")

			// now replace sha with sha3
			err = MarkBinariesAsPushed(remote1Name, sha3, sha)
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			pushed = GetPushedCommits(remote1Name)
			Expect(pushed).To(Equal([]string{sha2, sha3}), "Should record pushed, replacing first one, but in order")

			pushed = GetPushedCommits(remote2Name)
			Expect(pushed).To(Equal([]string{}), "Pushed should be empty for others")

			// Now undo
			err = ResetPushedBinaryState(remote1Name)
			Expect(err).To(BeNil(), "Shouldn't be an error undoing pushed")
			pushed = GetPushedCommits(remote1Name)
			Expect(pushed).To(Equal([]string{}), "Pushed should be empty after reset")
		})

		/*
			It("finds pushed ancestor", func() {
				// Create a bunch of empty commits, doesn't matter so long as message is different each time
				// so commit SHA is unique
				// Just use random SHAs to generate different messages
				// We need > 100 to test 2 batches of commit probes
				msgs := GetListOfRandomSHAsForTest(100)
				var commitPoints []string
				for i, msg := range msgs {
					cmd := exec.Command("git", "commit", "--allow-empty", "-m", msg)
					if err := cmd.Run(); err != nil {
						Fail(err.Error())
					}

					// Record push points at useful test intervals
					// start & end, in middle & near boundaries
					if i == 0 || i == 16 || i == 50 || i == 90 || i == 99 {
						// Get HEAD
						cmd := exec.Command("git", "rev-parse", "HEAD")
						outp, err := cmd.Output()
						if err != nil {
							Fail(err.Error())
						}

						commitPoints = append(commitPoints, strings.TrimSpace(string(outp)))

					}
				}
				remote := "origin"
				// Get SHA of HEAD
				cmd := exec.Command("git", "rev-parse", "HEAD")
				outp, err := cmd.Output()
				if err != nil {
					Fail(err.Error())
				}
				headSHA := strings.TrimSpace(string(outp))

				// Check that no-match case works & terminates correctly
				sha, err := FindLatestAncestorWhereBinariesPushed(remote, headSHA)
				Expect(err).To(BeNil())
				Expect(sha).To(Equal(""), "Should be no pushed binaries at start")

				for i, pushedCommit := range commitPoints {
					// Say we've pushed at this point, then test from HEAD
					SuccessfullyPushedBinariesForCommit_REMOVE(remote, pushedCommit)

					sha, err := FindLatestAncestorWhereBinariesPushed(remote, headSHA)
					Expect(err).To(BeNil())
					Expect(sha).To(Equal(pushedCommit), "Should detect %v as pushed commit (iteration %d)", pushedCommit, i)
				}

			})
		*/

	})
})
