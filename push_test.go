package main

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
	checkLOBsExist := func(shas []string, path string) {
		for _, sha := range shas {
			meta := filepath.Join(path, getLOBMetaRelativePath(sha))
			_, err := os.Stat(meta)
			Expect(err).To(BeNil(), "Meta file should exist")
			// Assuming only one chunk for this test
			chunk := filepath.Join(path, getLOBChunkRelativePath(sha, 0))
			_, err = os.Stat(chunk)
			Expect(err).To(BeNil(), "Chunk file should exist")
		}

	}
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
		f, err := os.OpenFile(filepath.Join(".git", "config"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		Expect(err).To(BeNil(), "Should not error trying to open config file")
		f.WriteString(fmt.Sprintf(`[remote "origin"]
    url = %v
    fetch = +refs/heads/*:refs/remotes/origin/*
    git-lob-path = %v
    git-lob-provider = filesystem
[remote "fork"]
    url = %v
    fetch = +refs/heads/*:refs/remotes/fork/*
    git-lob-path = %v
    git-lob-provider = filesystem
`, originPathUrl, originBinStore, forkPathUrl, forkBinStore))
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
		// Reset any option changes
		GlobalOptions = NewOptions()
	})

	It("Pushes correctly (Basic)", func() {
		originprovider, err := GetProviderForRemote("origin")
		Expect(err).To(BeNil(), "Shouldn't be an issue getting provider")

		var filesTransferred int
		var filesSkipped int
		var filesFailed int
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
				filesFailed++
			}
			return false
		}
		// Start by pushing up to Tag1 so that we push only first 2 commits on master
		err = PushBasic(originprovider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "Tag1"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Push should succeed")
		// Files should equal 2 for each entry (meta + one chunk)
		expectedFileCount := (len(masterfilespercommit[0]) + len(masterfilespercommit[1])) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFileCount), "Should have transferred the right number of files")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should fail")
		// Confirm data exists on remote
		checkLOBsExist(mastershaspercommit[0], originBinStore)
		checkLOBsExist(mastershaspercommit[1], originBinStore)

		// Now push all of master, should skip previous & upload new
		filesTransferred = 0
		err = PushBasic(originprovider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "master"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Push should succeed")
		// Files should equal 2 for each entry (meta + one chunk)
		expectedFileCount = len(masterfilespercommit[2]) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFileCount), "Should have transferred the right number of files")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped") // because cache should prevent
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should fail")
		// Confirm data exists on remote
		checkLOBsExist(mastershaspercommit[2], originBinStore)

		// Now push a different branch
		// Now push all of master, should skip previous & upload new
		filesTransferred = 0
		err = PushBasic(originprovider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "branch2"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Push should succeed")
		// Files should equal 2 for each entry (meta + one chunk)
		expectedFileCount = (len(branch2filespercommit[0]) + len(branch2filespercommit[1])) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFileCount), "Should have transferred the right number of files")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped") // because cache should prevent
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should fail")
		// Confirm data exists on remote
		checkLOBsExist(branch2shaspercommit[0], originBinStore)
		checkLOBsExist(branch2shaspercommit[1], originBinStore)

		// Now push master to fork
		filesTransferred = 0
		err = PushBasic(originprovider, "fork", []*GitRefSpec{&GitRefSpec{Ref1: "master"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Push should succeed")
		// Files should equal 2 for each entry (meta + one chunk)
		expectedFileCount = (len(masterfilespercommit[0]) + len(masterfilespercommit[1]) + len(masterfilespercommit[2])) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFileCount), "Should have transferred the right number of files")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped") // because cache should prevent
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should fail")
		// Confirm data exists on remote
		checkLOBsExist(mastershaspercommit[0], forkBinStore)
		checkLOBsExist(mastershaspercommit[1], forkBinStore)

	})

})
