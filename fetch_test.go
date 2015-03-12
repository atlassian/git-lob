package main

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var _ = Describe("Fetch", func() {

	root := filepath.Join(os.TempDir(), "FetchTest")
	originRoot := filepath.Join(os.TempDir(), "FetchOriginTest")
	originBinStore := filepath.Join(os.TempDir(), "FetchOriginBinStoreTest")
	var oldwd string
	var lobshas []string
	var correctLOBsMaster []string
	var correctLOBsFeature1 []string
	var correctLOBsFeature2 []string

	BeforeEach(func() {
		CreateGitRepoForTest(root)
		oldwd, _ = os.Getwd()
		os.Chdir(root)

		defaultOptions := NewOptions()

		// The setup:
		// master, feature/1 and feature/2 are 'recent refs', 'feature/3' is not
		// master has one commit excluded from its range, the rest are included
		// feature/1 and feature/2 only have the tip included (default 0 days so no history)

		// add one hour forward to the threshold date so we always create commits within time of test run
		refsIncludedDate := time.Now().AddDate(0, 0, -defaultOptions.FetchRefsPeriodDays).Add(time.Hour)
		refsExcludedDate := refsIncludedDate.Add(-time.Hour * 2)
		// Commit inclusion is based on the latest commit made - so make sure latest commit is before today for test
		latestHEADCommitDate := time.Now().AddDate(0, -2, -3)
		latestFeature1CommitDate := time.Now().AddDate(0, 0, -4)
		latestFeature2CommitDate := time.Now().AddDate(0, -1, 0)
		latestFeature3CommitDate := refsExcludedDate.AddDate(0, -1, 0) // will be excluded
		headCommitsIncludedDate := latestHEADCommitDate.AddDate(0, 0, -defaultOptions.FetchCommitsPeriodHEAD).Add(time.Hour)
		headCommitsExcludedDate := headCommitsIncludedDate.Add(-time.Hour * 2)
		feature1CommitsIncludedDate := latestFeature1CommitDate.AddDate(0, 0, -defaultOptions.FetchCommitsPeriodOther).Add(time.Hour)
		feature2CommitsIncludedDate := latestFeature2CommitDate.AddDate(0, 0, -defaultOptions.FetchCommitsPeriodOther).Add(time.Hour)

		// Simple constant size for all files (not testing chunks)
		sz := int64(300)
		// Master branch (which will be HEAD)

		info := CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file1.txt"))
		lobshas = append(lobshas, info.SHA)
		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file2.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file1.txt", "file2.txt").Run()
		// exclude commit 1
		CommitAtDateForTest(headCommitsExcludedDate.Add(-time.Hour*24*30), "Initial")

		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file1.txt"))
		lobshas = append(lobshas, info.SHA)
		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file2.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file1.txt", "file2.txt").Run()
		// commit 2 will be excluded,
		CommitAtDateForTest(headCommitsExcludedDate.Add(-time.Hour*24*15), "Second commit")
		correctLOBsMaster = append(correctLOBsMaster, lobshas[2], lobshas[3])

		exec.Command("git", "tag", "start").Run()
		// Create a branch we're going to exclude
		exec.Command("git", "checkout", "-b", "feature/3").Run()
		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file20.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file20.txt").Run()
		// We'll never see this commit or the branch
		CommitAtDateForTest(latestFeature3CommitDate, "Feature 3 commit")
		// Back to master
		exec.Command("git", "checkout", "master").Run()

		// add another file & modify
		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file2.txt"))
		lobshas = append(lobshas, info.SHA)
		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file3.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file2.txt", "file3.txt").Run()
		// include commit 2
		CommitAtDateForTest(headCommitsIncludedDate.Add(time.Hour*24), "Third commit")
		correctLOBsMaster = append(correctLOBsMaster, lobshas[5], lobshas[6])
		// Also include commit that references NO shas
		CommitAtDateForTest(headCommitsIncludedDate.Add(time.Hour*48), "Non-LOB commit")

		// Create another feature branch that we'll include, but not all the commits
		exec.Command("git", "tag", "feature/1/start").Run()
		exec.Command("git", "checkout", "-b", "feature/1").Run()
		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file3.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file3.txt").Run()
		// We'll never see this commit but we will see the branch (commit later)
		CommitAtDateForTest(feature1CommitsIncludedDate.Add(-time.Hour*48), "Feature 1 excluded commit")
		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file3.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file3.txt").Run()
		CommitAtDateForTest(feature1CommitsIncludedDate.Add(-time.Hour*4), "Feature 1 included commit")

		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file3.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file3.txt").Run()
		// We'll see this commit because the next commit will be the tip & range will include it
		CommitAtDateForTest(latestFeature1CommitDate, "Feature 1 tip commit")
		correctLOBsFeature1 = append(correctLOBsFeature1, lobshas[9])
		// Also include unchanged file1.txt at this state and old state of file2.txt
		correctLOBsFeature1 = append(correctLOBsFeature1, lobshas[2], lobshas[5])
		exec.Command("git", "tag", "afeaturetag").Run()

		// Back to master
		exec.Command("git", "checkout", "master").Run()

		// Create another feature branch that we'll include, but not all the commits
		exec.Command("git", "tag", "feature/2/start").Run()
		exec.Command("git", "checkout", "-b", "feature/2").Run()
		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file4.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file4.txt").Run()
		// We'll never see this commit but we will see the branch (commit later)
		CommitAtDateForTest(feature2CommitsIncludedDate.Add(-time.Hour*24*3), "Feature 2 excluded commit")
		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file4.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file4.txt").Run()
		CommitAtDateForTest(feature2CommitsIncludedDate.Add(-time.Hour*24*2), "Feature 2 excluded commit")
		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file4.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file4.txt").Run()
		// We'll see this commit
		CommitAtDateForTest(latestFeature2CommitDate, "Feature 2 tip commit")
		correctLOBsFeature2 = append(correctLOBsFeature2, lobshas[12])
		// Also include unchanged files on this branch: file1-3.txt last state & included versions
		correctLOBsFeature2 = append(correctLOBsFeature2, lobshas[5], lobshas[6], lobshas[2])

		// Back to master to finish
		exec.Command("git", "checkout", "master").Run()

		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file6.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file6.txt").Run()
		CommitAtDateForTest(headCommitsIncludedDate.Add(time.Hour*24*3), "Master commit")
		correctLOBsMaster = append(correctLOBsMaster, lobshas[13])

		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file5.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file5.txt").Run()
		CommitAtDateForTest(refsIncludedDate.Add(time.Hour*5), "Master penultimate commit")
		correctLOBsMaster = append(correctLOBsMaster, lobshas[14])
		exec.Command("git", "tag", "aheadtag").Run()

		info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file5.txt"))
		lobshas = append(lobshas, info.SHA)
		exec.Command("git", "add", "file5.txt").Run()
		CommitAtDateForTest(latestHEADCommitDate, "Master tip commit")
		correctLOBsMaster = append(correctLOBsMaster, lobshas[15])

		// now that we've stored all the data locally, let's move it to a remote so we have to fetch it

		// Configure remote
		CreateBareGitRepoForTest(originRoot)

		// Make a file:// ref so we don't have hardlinks (more standard)
		originPathUrl := strings.Replace(originRoot, "\\", "/", -1)
		originPathUrl = "file://" + originPathUrl
		// Also replace backslashes with forward slashes for windows (git still expects forward)
		originBinStoreGit := strings.Replace(originBinStore, "\\", "/", -1)
		f, err := os.OpenFile(filepath.Join(".git", "config"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		Expect(err).To(BeNil(), "Should not error trying to open config file")
		f.WriteString(fmt.Sprintf(`
[remote "origin"]
    url = %v
    fetch = +refs/heads/*:refs/remotes/origin/*
    git-lob-path = %v
    git-lob-provider = filesystem
`, originPathUrl, originBinStoreGit))
		f.Close()

		// Need to load config to load remote but reset recent params
		LoadConfig(GlobalOptions)
		GlobalOptions.FetchCommitsPeriodHEAD = defaultOptions.FetchCommitsPeriodHEAD
		GlobalOptions.FetchCommitsPeriodOther = defaultOptions.FetchCommitsPeriodOther
		GlobalOptions.FetchRefsPeriodDays = defaultOptions.FetchRefsPeriodDays
		InitCoreProviders()

		// move data, so we have no data locally & it's all on remote
		err = os.Rename(GetLocalLOBRoot(), originBinStore)
		Expect(err).To(BeNil(), "Should not error moving local store to remote")

	})
	AfterEach(func() {
		os.Chdir(oldwd)
		os.RemoveAll(root)
		os.RemoveAll(originRoot)
		os.RemoveAll(originBinStore)
		// Reset any option changes
		GlobalOptions = NewOptions()
	})
	It("High level fetch test", func() {
		// Detailed querying of recent refs etc is already tested in git_test.go
		// Just do the high-level tests here to make sure that correct files are moved about
		provider, err := GetProviderForRemote("origin")
		Expect(err).To(BeNil(), "Shouldn't be an issue getting provider")

		var filesTransferred int
		var filesSkipped int
		var filesFailed int
		var filesNotFound int
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
				filesNotFound++
			}
			return false
		}
		// dry run first, with no params so all recents
		err = Fetch(provider, "origin", []*GitRefSpec{}, true, false, false, callback)
		Expect(err).To(BeNil(), "Should be no error fetching")
		Expect(FileExists(getLocalLOBMetaPath(correctLOBsMaster[0]))).To(BeFalse(), "Should not have downloaded anything")

		// Now do it
		err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, false, callback)
		Expect(err).To(BeNil(), "Should be no error fetching")
		// Get unique SHAs for all recent refs
		uniques := append(correctLOBsMaster, correctLOBsFeature1...)
		uniques = append(uniques, correctLOBsFeature2...)
		StringRemoveDuplicates(&uniques)
		expectedFiles := len(uniques) * 2
		Expect(filesTransferred).To(BeEquivalentTo(expectedFiles), "Should be correct number of files to transfer")
		Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped")
		Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
		Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")
		CheckLOBsExistForTest(correctLOBsMaster, GetLocalLOBRoot())
		CheckLOBsExistForTest(correctLOBsFeature1, GetLocalLOBRoot())
		CheckLOBsExistForTest(correctLOBsFeature2, GetLocalLOBRoot())

		// Should also have updated push state for origin since local store was blank
		mastersha, _ := GitRefToFullSHA("master")
		pushedSHA, err := FindLatestAncestorWhereBinariesPushed_REMOVE("origin", mastersha)
		Expect(err).To(BeNil(), "Should not be error finding latest pushed")
		Expect(pushedSHA).To(Equal(mastersha), "Should be marked as fully pushed after initial fetch")

		// Now do it again & confirm it does nothing
		filesTransferred = 0
		err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, false, callback)
		Expect(err).To(BeNil(), "Should be no error fetching")
		Expect(filesTransferred).To(BeEquivalentTo(0), "Should be no files transferred")
		Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped because no need to try to download them")
		Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
		Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")

		// Now repeat & force
		err = Fetch(provider, "origin", []*GitRefSpec{}, false, true, false, callback)
		Expect(err).To(BeNil(), "Should be no error fetching")
		Expect(filesTransferred).To(BeEquivalentTo(expectedFiles), "Should be all files transferred again (force)")
		Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped because no need to try to download them")
		Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
		Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")

		// Delete again & do single ref
		os.RemoveAll(GetLocalLOBRoot())
		filesTransferred = 0
		err = Fetch(provider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "master"}}, false, false, false, callback)
		Expect(err).To(BeNil(), "Should be no error fetching")
		// Count should be the files required *at* master, not in history ie file1-5.txt
		Expect(filesTransferred).To(BeEquivalentTo(5*2), "Should be just master files transferred")
		Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped because no need to try to download them")
		Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
		Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")

		// Test missing on remote, should error but still continue
		os.RemoveAll(GetLocalLOBRoot())
		RemoveLOBsForTest(correctLOBsFeature1, originBinStore)
		filesTransferred = 0
		err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, false, callback)
		Expect(err).To(BeNil(), "Should be no error fetching even though some missing")
		Expect(filesTransferred).To(BeEquivalentTo(expectedFiles-len(correctLOBsFeature1)*2), "Should be all files transferred again (force)")
		Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped because no need to try to download them")
		Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
		Expect(filesNotFound).To(BeEquivalentTo(len(correctLOBsFeature1)), "Should be some files not found (count = SHAs not files)")

	})

})
