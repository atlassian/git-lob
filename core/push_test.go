package core

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	. "bitbucket.org/sinbad/git-lob/providers"
	"bitbucket.org/sinbad/git-lob/providers/smart"
	. "bitbucket.org/sinbad/git-lob/util"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
		err := ForceRemoveAll(root)
		if err != nil {
			Fail(err.Error())
		}
		err = ForceRemoveAll(originRoot)
		if err != nil {
			Fail(err.Error())
		}
		err = ForceRemoveAll(forkRoot)
		if err != nil {
			Fail(err.Error())
		}
		err = ForceRemoveAll(originBinStore)
		if err != nil {
			Fail(err.Error())
		}
		err = ForceRemoveAll(forkBinStore)
		if err != nil {
			Fail(err.Error())
		}
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
		pushedSHA, err := FindLatestAncestorWhereBinariesPushed("origin", mastersha)
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

		pushedSHA, err = FindLatestAncestorWhereBinariesPushed("origin", mastersha)
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
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed("origin", branch2sha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(branch2sha), "Pushed marker should be at branch2")

		// Now push master to fork
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed("fork", mastersha)
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
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed("fork", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(mastersha), "Pushed marker should be at master")

		// now reset all the pushed data for origin
		err = ResetPushedBinaryState("origin")
		Expect(err).To(BeNil(), "Should not be error resetting pushed data")
		Expect(HasPushedBinaryState("origin")).To(BeFalse(), "Should not have pushed state for origin")
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed("origin", mastersha)
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
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed("origin", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(mastersha), "Pushed marker should be at master")

		// Reset all the state again, and this time we'll test with missing data locally that's
		// also missing on the remote
		err = ResetPushedBinaryState("origin")
		Expect(err).To(BeNil(), "Should not be error resetting pushed data")
		Expect(HasPushedBinaryState("origin")).To(BeFalse(), "Should not have pushed state for origin")
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed("origin", mastersha)
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
		pushedSHA, err = FindLatestAncestorWhereBinariesPushed("origin", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(tag0sha), "Pushed marker should have only been moved to the point before missing files on local & remote")

	})

	Context("Delta push test", func() {
		root := filepath.Join(os.TempDir(), "PushTest")
		originRoot := filepath.Join(os.TempDir(), "PushOriginTest")
		var oldwd string
		var setupInputs []*TestCommitSetupInput
		var setupOutputs []*CommitLOBRef
		var fileshas []string

		BeforeEach(func() {
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)
			now := time.Now()

			filebytes := make([][]byte, 3)

			filebytes[0] = []byte("kajdflkajgsklfjgalsfalsgeflkajsjdbaclksuegfkacjsdmcabslkdfaiusegkcajbdsckjabiabilweubcilaweubkjbsecilawef")
			filebytes[1] = []byte("kajdflkajgsklf34235falsgeflkajsjdbaclksuegfkacjsdmca22334455bslkdfaiusegkcajbdsckjabiabilweubcilaweubkjbsecilawef")
			filebytes[2] = []byte("kajdflkajgsklf34235falsgeflkajsjdbaclksuegfkacjsdmca22334455bslkdfaiusegkcajbNOWYOUSEEMEdsckjabiabilweubcilaweubkjbsecilawefSUFFIXTIME")

			// Set up commits
			// We're only going to modify a single file since we're testing for deltas
			setupInputs = []*TestCommitSetupInput{
				&TestCommitSetupInput{ // 0
					CommitDate: now.AddDate(0, 0, -5),
					Files:      []string{"file1.txt"},
					FileData:   [][]byte{filebytes[0]},
				},
				&TestCommitSetupInput{ // 1
					CommitDate: now.AddDate(0, 0, -4),
					Files:      []string{"file1.txt"},
					FileData:   [][]byte{filebytes[1]},
				},
				&TestCommitSetupInput{ // 1
					CommitDate: now.AddDate(0, 0, -3),
					Files:      []string{"file1.txt"},
					FileData:   [][]byte{filebytes[2]},
				},
			}

			setupOutputs = SetupRepoForTest(setupInputs)

			// Configure remote
			CreateBareGitRepoForTest(originRoot)

			// Make a file:// ref so we don't have hardlinks (more standard)
			originPathUrl := strings.Replace(originRoot, "\\", "/", -1)
			originPathUrl = "file://" + originPathUrl

			// We'll use a dummy smart remote that can only respond to the necessary methods
			// which provider delta data, then use a specific URL form to use it
			originBinDummyURL := "dummy://something"

			// Also replace backslashes with forward slashes for windows (git still expects forward)
			f, err := os.OpenFile(filepath.Join(".git", "config"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
			Expect(err).To(BeNil(), "Should not error trying to open config file")
			f.WriteString(fmt.Sprintf(`
[remote "origin"]
    url = %v
    fetch = +refs/heads/*:refs/remotes/origin/*
    git-lob-url = %v
    git-lob-provider = smart
`, originPathUrl, originBinDummyURL))
			f.Close()

			LoadConfig(GlobalOptions)
			InitCoreProviders()
			smart.InitCoreProviders()

			// We need to set the delta threshold low enough that we'll always get deltas
			GlobalOptions.PushDeltasAboveSize = 0

			// Now set up the dummy transport system which will fake remote comms
			// We need to give the server the sha1 base data so it will accept delta pushes for 2 & 3
			sha1 := setupOutputs[0].FileLOBs[0].SHA
			sha2 := setupOutputs[1].FileLOBs[0].SHA
			sha3 := setupOutputs[2].FileLOBs[0].SHA
			meta1, err := ioutil.ReadFile(GetLocalLOBMetaPath(sha1))
			Expect(err).To(BeNil(), "Should not error trying to read metafile")
			metamap := map[string][]byte{
				sha1: meta1,
			}
			// include content for 1
			contentmap := map[string][]byte{
				sha1: filebytes[0],
			}
			smart.RegisterTransportFactory(&DummyPushTransportFactory{metamap, contentmap})

			fileshas = append(fileshas, sha1, sha2, sha3)
			// mark first commit as pushed since we included the data in the transport
			MarkBinariesAsPushed("origin", setupOutputs[0].Commit, "")

		})
		AfterEach(func() {
			os.Chdir(oldwd)
			err := ForceRemoveAll(root)
			if err != nil {
				Fail(err.Error())
			}
			err = ForceRemoveAll(originRoot)
			if err != nil {
				Fail(err.Error())
			}
			// Reset any option changes
			GlobalOptions = NewOptions()
		})

		It("Pushes deltas", func() {
			var filesTransferred int
			var filesSkipped int
			var filesFailed int
			var filesNotFound int
			var deltasSeen int
			callback := func(data *ProgressCallbackData) (abort bool) {
				switch data.Type {
				case ProgressCalculate:
					//fmt.Println(data.Desc)
				case ProgressTransferBytes:
					//fmt.Printf("%v : [%d/%d] Overall [%d/%d]\n", data.Desc, data.ItemBytesDone, data.ItemBytes, data.TotalBytesDone, data.TotalBytes)
					if data.ItemBytesDone == data.ItemBytes {
						filesTransferred++
						if strings.HasPrefix(data.Desc, "Delta") {
							deltasSeen++
						}
					}
				case ProgressSkip:
					filesSkipped++
				case ProgressError:
					filesFailed++
				case ProgressNotFound:
					filesNotFound++
				}
				return false
			}
			provider, err := GetProviderForRemote("origin")
			Expect(err).To(BeNil(), "Shouldn't be an issue getting provider")

			// dry run first
			err = Push(provider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "master"}}, true, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error pushing")
			Expect(filesTransferred).To(BeEquivalentTo(0), "Should be no files to transfer")
			Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped")
			Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
			Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")

			// Now for real
			err = Push(provider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "master"}}, false, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error pushing")
			Expect(filesTransferred).To(BeEquivalentTo(2*2), "Should be correct no of files to transfer (2 meta, 2 deltas)")
			Expect(deltasSeen).To(BeEquivalentTo(2), "Should see 2 completed deltas")
			Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped")
			Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
			Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")

		})

	})

})

// We'll use a dummy smart remote that can only respond to the necessary methods
// Set up a dummy transport that talks over a pipe
type DummyPushTransport struct {
	// map of LOB sha -> meta content
	MetaContentMap map[string][]byte
	// map of LOB shas we already have
	ContentMap map[string][]byte
}

func (*DummyPushTransport) Release() {
}
func (*DummyPushTransport) QueryCaps() ([]string, error) {
	return []string{"binary_delta"}, nil
}
func (*DummyPushTransport) SetEnabledCaps(caps []string) error {
	return nil
}
func (self *DummyPushTransport) MetadataExists(lobsha string) (ex bool, sz int64, e error) {
	meta, ok := self.MetaContentMap[lobsha]
	return ok, int64(len(meta)), nil
}
func (*DummyPushTransport) ChunkExists(lobsha string, chunk int) (ex bool, sz int64, e error) {
	// We don't need this
	return true, 0, nil
}
func (*DummyPushTransport) ChunkExistsAndIsOfSize(lobsha string, chunk int, sz int64) (bool, error) {
	// We don't need this
	return true, nil
}
func (self *DummyPushTransport) LOBExists(lobsha string) (ex bool, sz int64, e error) {
	content, contentok := self.ContentMap[lobsha]
	_, metaok := self.MetaContentMap[lobsha]
	return metaok && contentok, int64(len(content)), nil
}
func (self *DummyPushTransport) UploadMetadata(lobsha string, sz int64, data io.Reader) error {
	var buf bytes.Buffer
	n, err := io.CopyN(&buf, data, sz)
	if err != nil {
		return err
	} else if n != sz {
		return fmt.Errorf("Wrong size of data read from meta reader, expected %d got %d", sz, n)
	}
	self.MetaContentMap[lobsha] = buf.Bytes()
	return nil
}
func (*DummyPushTransport) UploadChunk(lobsha string, chunk int, sz int64, data io.Reader, callback smart.TransportProgressCallback) error {
	// We don't need this and to call it is a test failure
	return fmt.Errorf("UploadChunk Not implemented")
}
func (self *DummyPushTransport) DownloadMetadata(lobsha string, out io.Writer) error {
	// Not needed
	return nil
}
func (*DummyPushTransport) DownloadChunk(lobsha string, chunk int, out io.Writer, callback smart.TransportProgressCallback) error {
	// Not needed
	return nil
}
func (self *DummyPushTransport) GetFirstCompleteLOBFromList(candidateSHAs []string) (string, error) {
	for _, sha := range candidateSHAs {
		exists, _, _ := self.LOBExists(sha)
		if exists {
			return sha, nil
		}
	}
	return "", nil
}
func (self *DummyPushTransport) UploadDelta(baseSHA, targetSHA string, deltaSize int64, data io.Reader, callback smart.TransportProgressCallback) (bool, error) {
	_, ok := self.ContentMap[baseSHA]
	if !ok {
		return false, nil
	}
	// Manually call callback just for test
	callback(0, deltaSize)
	var buf bytes.Buffer
	n, err := io.CopyN(&buf, data, deltaSize)
	if err != nil {
		return false, err
	} else if n != deltaSize {
		return false, fmt.Errorf("Wrong size of data read from delta reader, expected %d got %d", deltaSize, n)
	}
	// we won't actually apply the delta, just acknowledge & discard (test will check for this callback)
	callback(deltaSize, deltaSize)
	return true, nil

}
func (self *DummyPushTransport) DownloadDeltaPrepare(baseSHA, targetSHA string) (int64, error) {
	// We don't need this
	return 0, nil
}
func (self *DummyPushTransport) DownloadDelta(baseSHA, targetSHA string, sizeLimit int64, out io.Writer, callback smart.TransportProgressCallback) (bool, error) {
	// We don't need this
	return false, nil
}

type DummyPushTransportFactory struct {
	// map of LOB sha -> meta content
	MetaContentMap map[string][]byte
	// map of LOB shas we already have
	ContentMap map[string][]byte
}

func (self *DummyPushTransportFactory) WillHandleUrl(u *url.URL) bool {
	return u.Scheme == "dummy"
}
func (self *DummyPushTransportFactory) Connect(u *url.URL) (smart.Transport, error) {
	return &DummyPushTransport{self.MetaContentMap, self.ContentMap}, nil
}
