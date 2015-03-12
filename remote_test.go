package main

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

		It("saves and restores a single entry", func() {

			sha := "b09bfdf65bb51bb50307f93ab930dd7708a5b6dc"

			push := ShouldPushBinariesForCommit_REMOVE(remote1Name, sha)
			Expect(push).To(BeTrue(), "Should want to push unknown SHA")

			err := SuccessfullyPushedBinariesForCommit_REMOVE(remote1Name, sha)
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			push = ShouldPushBinariesForCommit_REMOVE(remote1Name, sha)
			Expect(push).To(BeFalse(), "Shouldn't push again")

			push = ShouldPushBinariesForCommit_REMOVE(remote2Name, sha)
			Expect(push).To(BeTrue(), "Should push for other remote")

			// Now undo
			err = ResetPushedBinaryState(remote1Name)
			Expect(err).To(BeNil(), "Shouldn't be an error undoing pushed")
			push = ShouldPushBinariesForCommit_REMOVE(remote1Name, sha)
			Expect(push).To(BeTrue(), "Should push again after undo")
		})

		It("saves and restores multiple entries in the same file", func() {
			// Deliberately create a number of SHAs that will end up in the same file (same 1st 15 chars)
			// In practice this will only happen on larger repos
			shas := []string{"b09bfdf65bbfaa31000000000000000000000000",
				"b09bfdf65bbfaa32000000000000000000000000",
				"b09bfdf65bbfaa33000000000000000000000000",
				"b09bfdf65bbfaa34000000000000000000000000",
				"b09bfdf65bbfaa35000000000000000000000000",
				"b09bfdf65bbfaa36000000000000000000000000"}

			for _, s := range shas {
				push := ShouldPushBinariesForCommit_REMOVE(remote1Name, s)
				Expect(push).To(BeTrue(), "Should want to push unknown SHA %v", s)
			}

			// Add out of order so we can test insert before / after
			err := SuccessfullyPushedBinariesForCommit_REMOVE(remote1Name, shas[1])
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			// Insert at start
			err = SuccessfullyPushedBinariesForCommit_REMOVE(remote1Name, shas[0])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			// Insert at end
			err = SuccessfullyPushedBinariesForCommit_REMOVE(remote1Name, shas[4])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			// Insert in middle
			err = SuccessfullyPushedBinariesForCommit_REMOVE(remote1Name, shas[2])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			// Insert at end again
			err = SuccessfullyPushedBinariesForCommit_REMOVE(remote1Name, shas[5])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			// Insert in middle
			err = SuccessfullyPushedBinariesForCommit_REMOVE(remote1Name, shas[3])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			for _, s := range shas {
				push := ShouldPushBinariesForCommit_REMOVE(remote1Name, s)
				Expect(push).To(BeFalse(), "Should not want to push %v", s)
			}

			// Do a couple of duplicates to make sure we don't double-register
			err = SuccessfullyPushedBinariesForCommit_REMOVE(remote1Name, shas[4])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")
			err = SuccessfullyPushedBinariesForCommit_REMOVE(remote1Name, shas[2])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			for _, s := range shas {
				push := ShouldPushBinariesForCommit_REMOVE(remote1Name, s)
				Expect(push).To(BeFalse(), "Should not want to push %v", s)
			}

			// Verify file content is minimal & sorted
			filename := getRemoteStateCacheFileForCommit(remote1Name, shas[0])
			Expect(err).To(BeNil(), "Shouldn't error when reading state file")
			filebytes, err := ioutil.ReadFile(filename)
			// Don't use backtick literal string as will be CRLF on Windows and we always use LF
			// Remember LF at end
			expectedfilebytes := []byte("b09bfdf65bbfaa31000000000000000000000000\nb09bfdf65bbfaa32000000000000000000000000\nb09bfdf65bbfaa33000000000000000000000000\nb09bfdf65bbfaa34000000000000000000000000\nb09bfdf65bbfaa35000000000000000000000000\nb09bfdf65bbfaa36000000000000000000000000\n")
			Expect(filebytes).To(Equal(expectedfilebytes), "File content should be minimal and sorted")

			// Now undo
			err = ResetPushedBinaryState(remote1Name)
			Expect(err).To(BeNil(), "Shouldn't be an error undoing pushed")
			for _, s := range shas {
				push := ShouldPushBinariesForCommit_REMOVE(remote1Name, s)
				Expect(push).To(BeTrue(), "Should want to push %v", s)
			}

		})

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
			sha, err := FindLatestAncestorWhereBinariesPushed_REMOVE(remote, headSHA)
			Expect(err).To(BeNil())
			Expect(sha).To(Equal(""), "Should be no pushed binaries at start")

			for i, pushedCommit := range commitPoints {
				// Say we've pushed at this point, then test from HEAD
				SuccessfullyPushedBinariesForCommit_REMOVE(remote, pushedCommit)

				sha, err := FindLatestAncestorWhereBinariesPushed_REMOVE(remote, headSHA)
				Expect(err).To(BeNil())
				Expect(sha).To(Equal(pushedCommit), "Should detect %v as pushed commit (iteration %d)", pushedCommit, i)
			}

		})

	})
})
