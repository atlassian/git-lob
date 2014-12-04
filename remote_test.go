package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
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

		It("saves and restores a single entry", func() {

			sha := "b09bfdf65bb51bb50307f93ab930dd7708a5b6dc"

			push := ShouldPushBinariesForCommit(remote1Name, sha)
			Expect(push).To(BeTrue(), "Should want to push unknown SHA")

			err := SuccessfullyPushedBinariesForCommit(remote1Name, sha)
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			push = ShouldPushBinariesForCommit(remote1Name, sha)
			Expect(push).To(BeFalse(), "Shouldn't push again")

			push = ShouldPushBinariesForCommit(remote2Name, sha)
			Expect(push).To(BeTrue(), "Should push for other remote")

			// Now undo
			err = ResetPushedBinaryState(remote1Name)
			Expect(err).To(BeNil(), "Shouldn't be an error undoing pushed")
			push = ShouldPushBinariesForCommit(remote1Name, sha)
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
				push := ShouldPushBinariesForCommit(remote1Name, s)
				Expect(push).To(BeTrue(), "Should want to push unknown SHA %v", s)
			}

			// Add out of order so we can test insert before / after
			err := SuccessfullyPushedBinariesForCommit(remote1Name, shas[1])
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			// Insert at start
			err = SuccessfullyPushedBinariesForCommit(remote1Name, shas[0])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			// Insert at end
			err = SuccessfullyPushedBinariesForCommit(remote1Name, shas[4])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			// Insert in middle
			err = SuccessfullyPushedBinariesForCommit(remote1Name, shas[2])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			// Insert at end again
			err = SuccessfullyPushedBinariesForCommit(remote1Name, shas[5])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			// Insert in middle
			err = SuccessfullyPushedBinariesForCommit(remote1Name, shas[3])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			for _, s := range shas {
				push := ShouldPushBinariesForCommit(remote1Name, s)
				Expect(push).To(BeFalse(), "Should not want to push %v", s)
			}

			// Do a couple of duplicates to make sure we don't double-register
			err = SuccessfullyPushedBinariesForCommit(remote1Name, shas[4])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")
			err = SuccessfullyPushedBinariesForCommit(remote1Name, shas[2])
			Expect(err).To(BeNil(), "Shouldn't be an error inserting at start")

			for _, s := range shas {
				push := ShouldPushBinariesForCommit(remote1Name, s)
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
				push := ShouldPushBinariesForCommit(remote1Name, s)
				Expect(push).To(BeTrue(), "Should want to push %v", s)
			}

		})

	})
})