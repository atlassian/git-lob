package main

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var _ = Describe("Push", func() {
	root := filepath.Join(os.TempDir(), "PushTest")
	originRoot := filepath.Join(os.TempDir(), "PushOriginTest")
	forkRoot := filepath.Join(os.TempDir(), "PushForkTest")
	originBinStore := filepath.Join(os.TempDir(), "PushOriginBinStoreTest")
	forkBinStore := filepath.Join(os.TempDir(), "PushForkBinStoreTest")
	var oldwd string

	masterfilespercommit := [][]string{
		[]string{
			"img1.png", "img2.jpg",
			filepath.Join("movies", "movie1.mov"),
			filepath.Join("movies", "movie2.mov"),
			filepath.Join("other", "files", "windows.bmp"),
		},
		[]string{
			"img2.tga", "img3.tiff", "img4.png",
			filepath.Join("movies", "movie2.mov"),
			filepath.Join("movies", "movie3.mov"),
			filepath.Join("other", "files", "windows7.bmp"),
		},
		[]string{
			"img6.jpg",
			filepath.Join("other", "files", "windows.bmp"),
		},
	}
	branch2filespercommit := [][]string{
		[]string{
			"img4.png", "img5.jpg",
			filepath.Join("movies", "movie3.mov"),
		},
		[]string{
			"img7.jpg",
			filepath.Join("other", "files", "windows8.bmp"),
		},
	}
	sizeForFile := func(filename string, i int) int64 {
		// not actually that big, we're not doing size tests here
		if strings.HasSuffix(filename, ".mov") {
			return int64(i%3*1000 + 2000)
		} else {
			return int64(i%3*100 + 300)
		}
	}
	var mastershaspercommit [][]string
	var branch2shaspercommit [][]string
	BeforeEach(func() {
		CreateGitRepoForTest(root)
		oldwd, _ = os.Getwd()
		os.Chdir(root)

		// Create 2 remotes
		CreateBareGitRepoForTest(originRoot)
		CreateBareGitRepoForTest(forkRoot)
		os.MkdirAll(originBinStore, 0755)
		os.MkdirAll(forkBinStore, 0755)

		// Make a file:// ref so we don't have hardlinks (more standard)
		originPathUrl := strings.Replace(originRoot, "\\", "/", -1)
		originPathUrl = "file://" + originPathUrl
		forkPathUrl := strings.Replace(forkRoot, "\\", "/", -1)
		forkPathUrl = "file://" + forkPathUrl
		// Also replace backslashes with forward slashes for windows (git still expects forward)
		originBinStoreGit := strings.Replace(originBinStore, "\\", "/", -1)
		forkBinStoreGit := strings.Replace(forkBinStore, "\\", "/", -1)

		f, err := os.OpenFile(filepath.Join(".git", "config"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		Expect(err).To(BeNil(), "Should not error trying to open config file")
		f.WriteString(fmt.Sprintf(`
[remote "origin"]
    url = %v
    fetch = +refs/heads/*:refs/remotes/origin/*
    git-lob-path = %v
    git-lob-provider = filesystem
[remote "fork"]
    url = %v
    fetch = +refs/heads/*:refs/remotes/fork/*
    git-lob-path = %v
    git-lob-provider = filesystem
`, originPathUrl, originBinStoreGit, forkPathUrl, forkBinStoreGit))
		f.Close()

		LoadConfig(GlobalOptions)
		InitCoreProviders()

		// Create 3 commits with binary file references on master
		mastershaspercommit = CreateManyCommitsForTest(masterfilespercommit, 0, sizeForFile)
		// create another branch, from Tag1 which previous call created on 2nd commit
		err = exec.Command("git", "checkout", "-b", "branch2", "Tag1").Run()
		Expect(err).To(BeNil(), "Didn't create branch")
		// Create 2 more commits on this branch
		branch2shaspercommit = CreateManyCommitsForTest(branch2filespercommit, 3, sizeForFile)
		// Go back
		err = exec.Command("git", "checkout", "master").Run()
		Expect(err).To(BeNil(), "Didn't checkout master")
		// Note that working copy won't have correct binary data because filters aren't necessarily configured
		// see integration_test.go for those tests

	})
	AfterEach(func() {
		os.Chdir(oldwd)
		os.RemoveAll(root)
		os.RemoveAll(originRoot)
		os.RemoveAll(forkRoot)
		os.RemoveAll(originBinStore)
		os.RemoveAll(forkBinStore)
		// Reset any option changes
		GlobalOptions = NewOptions()
	})

	It("Pushes correctly (Basic)", func() {
		originprovider, err := GetProviderForRemote("origin")
		Expect(err).To(BeNil(), "Shouldn't be an issue getting provider")

		var filesTransferred int
		var filesSkipped int
		var filesFailed int
		var commitsNotFound int
		callback := func(data *ProgressCallbackData) (abort bool) {
			switch data.Type {
			case ProgressTransferBytes:
				if data.ItemBytesDone == data.ItemBytes {
					filesTransferred++
				}
			case ProgressSkip:
				filesSkipped++
			case ProgressError:
				filesFailed++
			case ProgressNotFound:
				commitsNotFound++
			}
			return false
		}
		Expect(HasPushedBinaryState("origin")).To(BeFalse(), "Should not have pushed state for origin")
		Expect(HasPushedBinaryState("fork")).To(BeFalse(), "Should not have pushed state for fork")

		// Start by pushing up to Tag1 so that we push only first 2 commits on master
		err = Push(originprovider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "Tag1"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Push should succeed")
		// Files should equal 2 for each entry (meta + one chunk)
		expectedFileCount := (len(masterfilespercommit[0]) + len(masterfilespercommit[1])) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFileCount), "Should have transferred the right number of files")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should fail")
		Expect(commitsNotFound).To(BeEquivalentTo(0), "No files should be not found")

		// Should now have pushed state
		Expect(HasPushedBinaryState("origin")).To(BeTrue(), "Should have pushed state for origin")
		// Check it's at position expected
		mastersha, _ := GitRefToFullSHA("master")
		pushedSHA, err := FindLatestAncestorWhereBinariesPushed_REMOVE("origin", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		tag1sha, _ := GitRefToFullSHA("Tag1")
		Expect(pushedSHA).To(Equal(tag1sha), "Pushed marker should be at Tag1")

		// Confirm data exists on remote
		CheckLOBsExistForTest(mastershaspercommit[0], originBinStore)
		CheckLOBsExistForTest(mastershaspercommit[1], originBinStore)

		// Now push all of master, should skip previous & upload new
		filesTransferred = 0
		err = Push(originprovider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "master"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Push should succeed")
		// Files should equal 2 for each entry (meta + one chunk)
		expectedFileCount = len(masterfilespercommit[2]) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFileCount), "Should have transferred the right number of files")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped") // because cache should prevent
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should fail")
		Expect(commitsNotFound).To(BeEquivalentTo(0), "No files should be not found")
		// Confirm data exists on remote
		CheckLOBsExistForTest(mastershaspercommit[2], originBinStore)

		pushedSHA, err = FindLatestAncestorWhereBinariesPushed_REMOVE("origin", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(mastersha), "Pushed marker should be at master")

		// Now push a different branch
		// Now push all of master, should skip previous & upload new
		filesTransferred = 0
		err = Push(originprovider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "branch2"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Push should succeed")
		// Files should equal 2 for each entry (meta + one chunk)
		expectedFileCount = (len(branch2filespercommit[0]) + len(branch2filespercommit[1])) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFileCount), "Should have transferred the right number of files")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped") // because cache should prevent
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should fail")
		Expect(commitsNotFound).To(BeEquivalentTo(0), "No files should be not found")
		// Confirm data exists on remote
		CheckLOBsExistForTest(branch2shaspercommit[0], originBinStore)
		CheckLOBsExistForTest(branch2shaspercommit[1], originBinStore)

		branch2sha, _ := GitRefToFullSHA("branch2")
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed_REMOVE("origin", branch2sha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(branch2sha), "Pushed marker should be at branch2")

		// Now push master to fork
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed_REMOVE("fork", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(""), "Pushed marker should not be set for fork")

		filesTransferred = 0
		err = Push(originprovider, "fork", []*GitRefSpec{&GitRefSpec{Ref1: "master"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Push should succeed")
		// Files should equal 2 for each entry (meta + one chunk)
		expectedFileCount = (len(masterfilespercommit[0]) + len(masterfilespercommit[1]) + len(masterfilespercommit[2])) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFileCount), "Should have transferred the right number of files")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped") // because cache should prevent
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should fail")
		Expect(commitsNotFound).To(BeEquivalentTo(0), "No files should be not found")
		// Confirm data exists on remote
		CheckLOBsExistForTest(mastershaspercommit[0], forkBinStore)
		CheckLOBsExistForTest(mastershaspercommit[1], forkBinStore)
		Expect(HasPushedBinaryState("fork")).To(BeTrue(), "Should have pushed state for fork")
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed_REMOVE("fork", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(mastersha), "Pushed marker should be at master")

		// now reset all the pushed data for origin
		err = ResetPushedBinaryState("origin")
		Expect(err).To(BeNil(), "Should not be error resetting pushed data")
		Expect(HasPushedBinaryState("origin")).To(BeFalse(), "Should not have pushed state for origin")
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed_REMOVE("origin", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(""), "Pushed marker should not be set")

		// now delete some of the local LOBs to create a gap in our data
		// but leave the data on the remote; this simulates the case where user has fetched commits from
		// someone else but hasn't fetched LOBs, then tries to push their own binaries
		// this should still succeed, and because LOBs are on the remote then it's fine to
		// move the pushed pointer over these commits
		RemoveLOBsForTest(mastershaspercommit[1], GetLocalLOBRoot())
		// Also delete some *other* LOBs on the remote to make sure they get pushed
		RemoveLOBsForTest(mastershaspercommit[2], originBinStore)

		// now push master again, should be OK to skip over missing LOBs since on remote
		filesTransferred = 0
		err = Push(originprovider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "master"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Push should succeed")
		// Files should equal 2 for each entry (meta + one chunk)
		// We should transfer [2] because not on remote
		// We should do nothing with [1] - missing locally but OK because on remote
		// And should skip [0] since already on remote and local
		expectedFileCount = len(masterfilespercommit[2]) * 2
		expectedSkipFileCount := len(masterfilespercommit[0]) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFileCount), "Should have transferred the right number of files")
		Expect(filesSkipped).To(BeEquivalentTo(expectedSkipFileCount), "Should skip the files already on remote")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should fail")
		Expect(commitsNotFound).To(BeEquivalentTo(0), "Should have no 'not found' files since found on remote")
		// Confirm new data exists on remote
		CheckLOBsExistForTest(mastershaspercommit[2], originBinStore)
		// Check that push cache has been updated (because missing files were OK on remote)
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed_REMOVE("origin", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(mastersha), "Pushed marker should be at master")

		// Reset all the state again, and this time we'll test with missing data locally that's
		// also missing on the remote
		err = ResetPushedBinaryState("origin")
		Expect(err).To(BeNil(), "Should not be error resetting pushed data")
		Expect(HasPushedBinaryState("origin")).To(BeFalse(), "Should not have pushed state for origin")
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed_REMOVE("origin", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(""), "Pushed marker should not be set")

		// now delete some of the local AND remote LOBs to create a gap in our data
		// We should still push data we have but not update push cache state, should warn about missing
		RemoveLOBsForTest(mastershaspercommit[1], GetLocalLOBRoot())
		RemoveLOBsForTest(mastershaspercommit[1], originBinStore)
		// Also delete some *other* LOBs on the remote to make sure they get pushed
		RemoveLOBsForTest(mastershaspercommit[2], originBinStore)

		// now push master again, should be OK to skip over missing LOBs since on remote
		filesTransferred = 0
		filesSkipped = 0
		err = Push(originprovider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "master"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Push should succeed")
		// Files should equal 2 for each entry (meta + one chunk)
		// We should transfer [2] because not on remote & present locally
		// We should 'not found' on [1]
		// And should skip [0] since already on remote and local
		expectedFileCount = len(masterfilespercommit[2]) * 2
		expectedSkipFileCount = len(masterfilespercommit[0]) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFileCount), "Should have transferred the right number of files")
		Expect(filesSkipped).To(BeEquivalentTo(expectedSkipFileCount), "Should skip the files already on remote")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should fail")
		Expect(commitsNotFound).To(BeEquivalentTo(1), "One commit should have had missing files locally & on remote")
		// Confirm new data exists on remote
		CheckLOBsExistForTest(mastershaspercommit[2], originBinStore)
		// Check that push cache has been updated, but only to [0] (Tag0)
		tag0sha, _ := GitRefToFullSHA("Tag0")
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed_REMOVE("origin", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(tag0sha), "Pushed marker should have only been moved to the point before missing files on local & remote")

	})

})
