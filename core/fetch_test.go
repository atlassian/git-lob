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

var _ = Describe("Fetch", func() {

	Context("Main fetch test", func() {
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
			latestFeature2CommitDate := refsIncludedDate.AddDate(0, 0, 1)
			latestFeature3CommitDate := refsExcludedDate.AddDate(0, -2, 0) // will be excluded
			headCommitsIncludedDate := latestHEADCommitDate.AddDate(0, 0, -defaultOptions.FetchCommitsPeriodHEAD).Add(time.Hour)
			headCommitsExcludedDate := headCommitsIncludedDate.Add(-time.Hour * 2)
			feature1CommitsIncludedDate := latestFeature1CommitDate.AddDate(0, 0, -defaultOptions.FetchCommitsPeriodOther).Add(time.Hour)
			feature2CommitsIncludedDate := latestFeature2CommitDate.AddDate(0, 0, -defaultOptions.FetchCommitsPeriodOther).Add(time.Hour)

			// Simple constant size for all files (not testing chunks)
			sz := int64(300)

			info := CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file1.txt"))
			lobshas = append(lobshas, info.SHA) // 0
			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file2.txt"))
			lobshas = append(lobshas, info.SHA) // 1
			exec.Command("git", "add", "file1.txt", "file2.txt").Run()
			// exclude commit 1
			CommitAtDateForTest(headCommitsExcludedDate.Add(-time.Hour*24*30), "Fred", "fred@bloggs.com", "Initial")

			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file1.txt"))
			lobshas = append(lobshas, info.SHA) // 2
			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file2.txt"))
			lobshas = append(lobshas, info.SHA) // 3
			exec.Command("git", "add", "file1.txt", "file2.txt").Run()
			// commit 2 will be excluded,
			CommitAtDateForTest(headCommitsExcludedDate.Add(-time.Hour*24*15), "Fred", "fred@bloggs.com", "Second commit")
			correctLOBsMaster = append(correctLOBsMaster, lobshas[2], lobshas[3])

			exec.Command("git", "tag", "start").Run()
			// Create a branch we're going to exclude
			exec.Command("git", "checkout", "-b", "feature/3").Run()
			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file20.txt"))
			lobshas = append(lobshas, info.SHA) // 4
			exec.Command("git", "add", "file20.txt").Run()
			// We'll never see this commit or the branch
			CommitAtDateForTest(latestFeature3CommitDate, "Fred", "fred@bloggs.com", "Feature 3 commit")
			// Back to master
			exec.Command("git", "checkout", "master").Run()

			// add another file & modify
			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file2.txt"))
			lobshas = append(lobshas, info.SHA) // 5
			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file3.txt"))
			lobshas = append(lobshas, info.SHA) // 6
			exec.Command("git", "add", "file2.txt", "file3.txt").Run()
			// include commit 2
			CommitAtDateForTest(headCommitsIncludedDate.Add(time.Hour*24), "Fred", "fred@bloggs.com", "Third commit")
			correctLOBsMaster = append(correctLOBsMaster, lobshas[5], lobshas[6])
			// Also include commit that references NO shas
			CommitAtDateForTest(headCommitsIncludedDate.Add(time.Hour*48), "Fred", "fred@bloggs.com", "Non-LOB commit")

			// Create another feature branch that we'll include, but not all the commits
			exec.Command("git", "tag", "feature/1/start").Run()
			exec.Command("git", "checkout", "-b", "feature/1").Run()
			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file3.txt"))
			lobshas = append(lobshas, info.SHA) // 7
			exec.Command("git", "add", "file3.txt").Run()
			// We'll never see this commit but we will see the branch (commit later)
			CommitAtDateForTest(feature1CommitsIncludedDate.Add(-time.Hour*48), "Fred", "fred@bloggs.com", "Feature 1 excluded commit")
			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file3.txt"))
			lobshas = append(lobshas, info.SHA) // 8
			exec.Command("git", "add", "file3.txt").Run()
			CommitAtDateForTest(feature1CommitsIncludedDate.Add(-time.Hour*4), "Fred", "fred@bloggs.com", "Feature 1 included commit")

			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file3.txt"))
			lobshas = append(lobshas, info.SHA) // 9
			exec.Command("git", "add", "file3.txt").Run()
			// We'll see this commit because the next commit will be the tip & range will include it
			CommitAtDateForTest(latestFeature1CommitDate, "Fred", "fred@bloggs.com", "Feature 1 tip commit")
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
			lobshas = append(lobshas, info.SHA) // 10
			exec.Command("git", "add", "file4.txt").Run()
			// We'll never see this commit but we will see the branch (commit later)
			CommitAtDateForTest(feature2CommitsIncludedDate.Add(-time.Hour*24*3), "Fred", "fred@bloggs.com", "Feature 2 excluded commit")
			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file4.txt"))
			lobshas = append(lobshas, info.SHA) // 11
			exec.Command("git", "add", "file4.txt").Run()
			CommitAtDateForTest(feature2CommitsIncludedDate.Add(-time.Hour*24*2), "Fred", "fred@bloggs.com", "Feature 2 excluded commit")
			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file4.txt"))
			lobshas = append(lobshas, info.SHA) // 12
			exec.Command("git", "add", "file4.txt").Run()
			// We'll see this commit
			CommitAtDateForTest(latestFeature2CommitDate, "Fred", "fred@bloggs.com", "Feature 2 tip commit")
			correctLOBsFeature2 = append(correctLOBsFeature2, lobshas[12])
			// Also include unchanged files on this branch: file1-3.txt last state & included versions
			correctLOBsFeature2 = append(correctLOBsFeature2, lobshas[5], lobshas[6], lobshas[2])

			// Back to master to finish
			exec.Command("git", "checkout", "master").Run()

			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file6.txt"))
			lobshas = append(lobshas, info.SHA)
			exec.Command("git", "add", "file6.txt").Run()
			CommitAtDateForTest(headCommitsIncludedDate.Add(time.Hour*24*3), "Fred", "fred@bloggs.com", "Master commit")
			correctLOBsMaster = append(correctLOBsMaster, lobshas[13])

			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file5.txt"))
			lobshas = append(lobshas, info.SHA)
			exec.Command("git", "add", "file5.txt").Run()
			CommitAtDateForTest(refsIncludedDate.Add(time.Hour*5), "Fred", "fred@bloggs.com", "Master penultimate commit")
			correctLOBsMaster = append(correctLOBsMaster, lobshas[14])
			exec.Command("git", "tag", "aheadtag").Run()

			info = CreateAndStoreLOBFileForTest(sz, filepath.Join(root, "file5.txt"))
			lobshas = append(lobshas, info.SHA)
			exec.Command("git", "add", "file5.txt").Run()
			CommitAtDateForTest(latestHEADCommitDate, "Fred", "fred@bloggs.com", "Master tip commit")
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
			err := ForceRemoveAll(root)
			if err != nil {
				Fail(err.Error())
			}
			err = ForceRemoveAll(originRoot)
			if err != nil {
				Fail(err.Error())
			}
			err = ForceRemoveAll(originBinStore)
			if err != nil {
				Fail(err.Error())
			}
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
			err = Fetch(provider, "origin", []*GitRefSpec{}, true, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(FileExists(GetLocalLOBMetaPath(correctLOBsMaster[0]))).To(BeFalse(), "Should not have downloaded anything")

			// Now do it
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
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
			pushedSHA, err := FindLatestAncestorWhereBinariesPushed("origin", mastersha)
			Expect(err).To(BeNil(), "Should not be error finding latest pushed")
			Expect(pushedSHA).To(Equal(mastersha), "Should be marked as fully pushed after initial fetch")

			// Now do it again & confirm it does nothing
			filesTransferred = 0
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(0), "Should be no files transferred")
			Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped because no need to try to download them")
			Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
			Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")

			// Now repeat & force
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, true, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(expectedFiles), "Should be all files transferred again (force)")
			Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped because no need to try to download them")
			Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
			Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")

			// Delete again & do single ref
			ForceRemoveAll(GetLocalLOBRoot())
			filesTransferred = 0
			err = Fetch(provider, "origin", []*GitRefSpec{&GitRefSpec{Ref1: "master"}}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			// Count should be the files required *at* master, not in history ie file1-5.txt
			Expect(filesTransferred).To(BeEquivalentTo(5*2), "Should be just master files transferred")
			Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped because no need to try to download them")
			Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
			Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")

			// Test missing on remote, should error but still continue
			ForceRemoveAll(GetLocalLOBRoot())
			RemoveLOBsForTest(correctLOBsFeature1, originBinStore)
			filesTransferred = 0
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching even though some missing")
			Expect(filesTransferred).To(BeEquivalentTo(expectedFiles-len(correctLOBsFeature1)*2), "Should be all files transferred again (force)")
			Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped because no need to try to download them")
			Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
			Expect(filesNotFound).To(BeEquivalentTo(len(correctLOBsFeature1)), "Should be some files not found (count = SHAs not files)")

		})

	})

	Context("Fetch effects on push state", func() {

		root := filepath.Join(os.TempDir(), "FetchTest")
		originRoot := filepath.Join(os.TempDir(), "FetchOriginTest")
		originBinStore := filepath.Join(os.TempDir(), "FetchOriginBinStoreTest")
		var oldwd string
		var setupInputs []*TestCommitSetupInput
		var setupOutputs []*CommitLOBRef

		BeforeEach(func() {
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)
			now := time.Now()

			// Set up commits
			// Note how we always modify the same 2 files every time
			// this is to make it simpler to count files, since fetch will get
			// all the files needed at a commit, so if the files are different then
			// it will fetch them even if they hadn't changed (because still needed to complete working copy)
			setupInputs = []*TestCommitSetupInput{
				&TestCommitSetupInput{ // 0
					CommitDate: now.AddDate(0, 0, -20),
					Files:      []string{"file1.txt", "file2.txt"},
				},
				&TestCommitSetupInput{ // 1
					CommitDate: now.AddDate(0, 0, -19),
					Files:      []string{"file1.txt", "file2.txt"},
				},
				// feature1 branch
				&TestCommitSetupInput{ // 2
					CommitDate: now.AddDate(0, 0, -18),
					Files:      []string{"file1.txt", "file2.txt"},
					NewBranch:  "feature1",
				},
				&TestCommitSetupInput{ // 3
					CommitDate: now.AddDate(0, 0, -17),
					Files:      []string{"file1.txt", "file2.txt"},
				},
				// merge
				&TestCommitSetupInput{ // 4
					CommitDate:     now.AddDate(0, 0, -16),
					Files:          []string{"file1.txt", "file2.txt"},
					ParentBranches: []string{"master", "feature1"},
				},
				&TestCommitSetupInput{ // 5
					CommitDate:     now.AddDate(0, 0, -13),
					Files:          []string{"file1.txt", "file2.txt"},
					ParentBranches: []string{"master"},
				},
				&TestCommitSetupInput{ // 6
					CommitDate: now.AddDate(0, 0, -11),
					Files:      []string{"file1.txt", "file2.txt"},
				},
				&TestCommitSetupInput{ // 7
					CommitDate: now.AddDate(0, 0, -11),
					Files:      []string{"file1.txt", "file2.txt"},
				},
			}

			setupOutputs = SetupRepoForTest(setupInputs)

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

			LoadConfig(GlobalOptions)
			// only very recent fetch behaviour
			GlobalOptions.FetchCommitsPeriodHEAD = 2
			GlobalOptions.FetchCommitsPeriodOther = 0
			GlobalOptions.FetchRefsPeriodDays = 0
			InitCoreProviders()

			// move data, so we have no data locally & it's all on remote
			err = os.Rename(GetLocalLOBRoot(), originBinStore)
			Expect(err).To(BeNil(), "Should not error moving local store to remote")

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
			err = ForceRemoveAll(originBinStore)
			if err != nil {
				Fail(err.Error())
			}
			// Reset any option changes
			GlobalOptions = NewOptions()
		})
		It("Fetch test for push state", func() {
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
			// Firstly we'll fetch at commit 0 just for local
			RunGitCommandForTest(true, "checkout", setupOutputs[0].Commit)
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(len(setupOutputs[0].LobSHAs)*2), "File count check should be right")
			filesTransferred = 0
			// This should have initialised ALL the push state (because no local LOBs) including all other branches, so reset it
			// then manually set it to [0]. We're trying to replicate the case where more than one fetch has taken place when the
			// git repo has had new commits pulled
			err = ResetPushedBinaryState("origin")
			Expect(err).To(BeNil(), "Should be no error resetting pushed state")
			MarkBinariesAsPushed("origin", setupOutputs[0].Commit, "")
			pushed, err := FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[0].Commit), "Reset pushed state should work")
			// now fetch for commit 1, this should update the push state to this SHA
			RunGitCommandForTest(true, "checkout", setupOutputs[1].Commit)
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(len(setupOutputs[1].LobSHAs)*2), "File count check should be right")
			pushed, err = FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[1].Commit), "Fetch should have updated pushed state to the latest fetched point")
			filesTransferred = 0

			// Now test leaving a gap unpushed but present on remote
			// Commit 5 is > 2 days ahead of previous commit so this will only fetch 5
			// but will actually get 2 sets; the change at 5 and the change before 5
			RunGitCommandForTest(true, "checkout", setupOutputs[5].Commit)
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(len(setupOutputs[5].LobSHAs)*4), "File count check should be right (2 commits)")
			pushed, err = FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[5].Commit), "Fetch should have updated pushed state to the latest fetched point because files in gap are already on remote")
			filesTransferred = 0

			// Now try that again, but this time for a commit in the gap which is not fetched, make a file be missing on remote
			// this should mean that the push state is not moved, because we need to keep checking until someone pushes the missing LOB
			// (might be this client or any other)
			// Put the pushed state back to 1
			ResetPushedBinaryState("origin")
			MarkBinariesAsPushed("origin", setupOutputs[1].Commit, "")
			// this test is for a missing file in the GAP, not a file we would fetch
			origRemoteBinary := filepath.Join(originBinStore, GetLOBChunkRelativePath(setupOutputs[3].LobSHAs[0], 0))
			renamedRemoteBinary := origRemoteBinary + "_old"
			os.Rename(origRemoteBinary, renamedRemoteBinary)
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(0), "Should be nothing new to fetch (already done)")
			pushed, err = FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[1].Commit), "Fetch should not have updated pushed state because remote files were missing in the gap")
			filesTransferred = 0
			// put this file back so it's not stopping push state update any more
			os.Rename(renamedRemoteBinary, origRemoteBinary)

			// Now try that again, but this time for a commit that would be *fetched*, have a missing file on the remote
			// this should mean that the push state is not moved as well because someone needs to push that commit
			// It would be great for the push state to be rolled forward to just before that commit, but fetch doesn't work that way
			// So instead the push state is not updated at all until the missing remote files are resolved
			// Latest commit
			RunGitCommandForTest(true, "checkout", "master")

			// this test is for a missing file in the FETCH
			origRemoteBinary = filepath.Join(originBinStore, GetLOBChunkRelativePath(setupOutputs[6].LobSHAs[0], 0))
			renamedRemoteBinary = origRemoteBinary + "_old"
			os.Rename(origRemoteBinary, renamedRemoteBinary)
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(len(setupOutputs[6].LobSHAs)*2+len(setupOutputs[7].LobSHAs)*2-1),
				"Should be 2 more commits to fetch, minus one which is missing")
			pushed, err = FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[1].Commit), "Fetch should not have updated pushed state because remote files were missing in the gap")
			filesTransferred = 0
			// put this file back so it's not stopping push state update any more
			os.Rename(renamedRemoteBinary, origRemoteBinary)

			// now confirm that once the missing files are resolved, push state updates
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(1), "Should fetch the 1 file that was missing")
			pushed, err = FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[7].Commit), "Fetch should now have updated the push state to master")
			filesTransferred = 0

		})
	})

	Context("Delta fetch test", func() {
		root := filepath.Join(os.TempDir(), "FetchTest")
		originRoot := filepath.Join(os.TempDir(), "FetchOriginTest")
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

			// now that we've stored all the data locally, let's move it to a remote so we have to fetch it

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
			GlobalOptions.FetchDeltasAboveSize = 0

			// Now set up the dummy transport system which will fake remote comms
			// We need to give the server the deltas and metadata required for the fetch (revs 2 & 3)
			sha1 := setupOutputs[0].FileLOBs[0].SHA
			sha2 := setupOutputs[1].FileLOBs[0].SHA
			sha3 := setupOutputs[2].FileLOBs[0].SHA
			var delta12, delta13, delta23 bytes.Buffer
			err = GenerateLOBDelta(sha1, sha2, &delta12)
			Expect(err).To(BeNil(), "Should not error trying to generate delta")
			err = GenerateLOBDelta(sha2, sha3, &delta23)
			Expect(err).To(BeNil(), "Should not error trying to generate delta")
			err = GenerateLOBDelta(sha1, sha3, &delta13)
			Expect(err).To(BeNil(), "Should not error trying to generate delta")
			meta1, err := ioutil.ReadFile(GetLocalLOBMetaPath(sha1))
			Expect(err).To(BeNil(), "Should not error trying to read metafile")
			meta2, err := ioutil.ReadFile(GetLocalLOBMetaPath(sha2))
			Expect(err).To(BeNil(), "Should not error trying to read metafile")
			meta3, err := ioutil.ReadFile(GetLocalLOBMetaPath(sha3))
			Expect(err).To(BeNil(), "Should not error trying to read metafile")
			metamap := map[string][]byte{
				sha1: meta1, // need to include this so dummy remote knows it exists
				sha2: meta2,
				sha3: meta3,
			}
			// inlcude deltas from 1-2, 2-3 and 1-3
			deltamap := map[string][]byte{
				fmt.Sprintf("%v:%v", sha1, sha2): delta12.Bytes(),
				fmt.Sprintf("%v:%v", sha1, sha3): delta13.Bytes(),
				fmt.Sprintf("%v:%v", sha2, sha3): delta23.Bytes(),
			}
			smart.RegisterTransportFactory(&DummyFetchTransportFactory{metamap, deltamap})

			// now delete the second 2 revisions of LOBs, keeping only the first one
			// and relying on deltas from the dummy server for the other 2
			err = DeleteLOB(sha2)
			Expect(err).To(BeNil(), "Should not error trying to delete LOB")
			err = DeleteLOB(sha3)
			Expect(err).To(BeNil(), "Should not error trying to delete LOB")

			fileshas = append(fileshas, sha1, sha2, sha3)

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

		It("Fetches deltas", func() {
			var filesTransferred int
			var filesSkipped int
			var filesFailed int
			var filesNotFound int
			var messages []string
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
				messages = append(messages, data.Desc)
				return false
			}
			provider, err := GetProviderForRemote("origin")
			Expect(err).To(BeNil(), "Shouldn't be an issue getting provider")

			// dry run first, with no params so all recents
			err = Fetch(provider, "origin", []*GitRefSpec{}, true, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(FileExists(GetLocalLOBMetaPath(setupOutputs[1].FileLOBs[0].SHA))).To(BeFalse(), "Should not have downloaded anything")

			// First try fetching the entire range
			// Should try to generate deltas 1-2 and 1-3 in this case, because we only have SHA 1 locally
			// *technically* if we did things in order all the time we could use 1-2 and 2-3 in one fetch but that's more complex
			// results in some duplication when fetching many updates to the same file probably, but not worth it yet
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			//fmt.Println(messages)
			Expect(filesTransferred).To(BeEquivalentTo(2), "Should be correct number of files to transfer")
			Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped")
			Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
			Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")
			lobstocheck := []string{
				setupOutputs[1].FileLOBs[0].SHA,
				setupOutputs[2].FileLOBs[0].SHA,
			}
			CheckLOBsExistForTest(lobstocheck, GetLocalLOBRoot())
			filesTransferred = 0
			// now let's delete LOB 3 and fetch again so it uses 2-3 delta
			err = DeleteLOB(fileshas[2])
			Expect(err).To(BeNil(), "Should not error trying to delete LOB")
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			//fmt.Println(messages)
			Expect(filesTransferred).To(BeEquivalentTo(1), "Should be correct number of files to transfer")
			Expect(filesSkipped).To(BeEquivalentTo(0), "Should be no files skipped")
			Expect(filesFailed).To(BeEquivalentTo(0), "Should be no files failed")
			Expect(filesNotFound).To(BeEquivalentTo(0), "Should be no files not found")
			lobstocheck = []string{
				setupOutputs[2].FileLOBs[0].SHA,
			}
			CheckLOBsExistForTest(lobstocheck, GetLocalLOBRoot())

		})

	})

})

// We'll use a dummy smart remote that can only respond to the necessary methods
// Set up a dummy transport that talks over a pipe
type DummyFetchTransport struct {
	// map of LOB sha -> meta content
	MetaContentMap map[string][]byte
	// map of "BASE:TARGET" -> delta content
	DeltaContentMap map[string][]byte
}

func (*DummyFetchTransport) Release() {
}
func (*DummyFetchTransport) QueryCaps() ([]string, error) {
	return []string{"binary_delta"}, nil
}
func (*DummyFetchTransport) SetEnabledCaps(caps []string) error {
	return nil
}
func (self *DummyFetchTransport) MetadataExists(lobsha string) (ex bool, sz int64, e error) {
	meta, ok := self.MetaContentMap[lobsha]
	return ok, int64(len(meta)), nil
}
func (*DummyFetchTransport) ChunkExists(lobsha string, chunk int) (ex bool, sz int64, e error) {
	// We don't need this
	return true, 0, nil
}
func (*DummyFetchTransport) ChunkExistsAndIsOfSize(lobsha string, chunk int, sz int64) (bool, error) {
	// We don't need this
	return true, nil
}
func (*DummyFetchTransport) LOBExists(lobsha string) (ex bool, sz int64, e error) {
	// We don't need this
	return true, 0, nil
}
func (*DummyFetchTransport) UploadMetadata(lobsha string, sz int64, data io.Reader) error {
	// We don't need this
	return nil
}
func (*DummyFetchTransport) UploadChunk(lobsha string, chunk int, sz int64, data io.Reader, callback smart.TransportProgressCallback) error {
	// We don't need this
	return nil
}
func (self *DummyFetchTransport) DownloadMetadata(lobsha string, out io.Writer) error {
	meta, ok := self.MetaContentMap[lobsha]
	if ok {
		out.Write(meta)
		return nil
	}
	return fmt.Errorf("Meta not found: %v", lobsha)
}
func (*DummyFetchTransport) DownloadChunk(lobsha string, chunk int, out io.Writer, callback smart.TransportProgressCallback) error {
	// We don't need this and to call it is a test failure
	return fmt.Errorf("DownloadChunk Not implemented")
}
func (self *DummyFetchTransport) GetFirstCompleteLOBFromList(candidateSHAs []string) (string, error) {
	for _, sha := range candidateSHAs {
		_, metaok := self.MetaContentMap[sha]
		if metaok {
			return sha, nil
		}
	}
	return "", nil
}
func (*DummyFetchTransport) UploadDelta(baseSHA, targetSHA string, deltaSize int64, data io.Reader, callback smart.TransportProgressCallback) (bool, error) {
	// We don't need this
	return false, nil
}
func (self *DummyFetchTransport) DownloadDeltaPrepare(baseSHA, targetSHA string) (int64, error) {
	delta, ok := self.DeltaContentMap[fmt.Sprintf("%v:%v", baseSHA, targetSHA)]
	if ok {
		return int64(len(delta)), nil
	}
	return 0, fmt.Errorf("Not found delta %v->%v", baseSHA, targetSHA)
}
func (self *DummyFetchTransport) DownloadDelta(baseSHA, targetSHA string, sizeLimit int64, out io.Writer, callback smart.TransportProgressCallback) (bool, error) {
	delta, ok := self.DeltaContentMap[fmt.Sprintf("%v:%v", baseSHA, targetSHA)]
	if ok {
		// Manually call callback just for test
		totallen := int64(len(delta))
		callback(0, totallen)
		out.Write([]byte(delta))
		callback(totallen, totallen)
		return true, nil
	}
	return false, nil

}

type DummyFetchTransportFactory struct {
	// map of LOB sha -> meta content string
	MetaContentMap map[string][]byte
	// map of "BASE:TARGET" -> delta content
	DeltaContentMap map[string][]byte
}

func (self *DummyFetchTransportFactory) WillHandleUrl(u *url.URL) bool {
	return u.Scheme == "dummy"
}
func (self *DummyFetchTransportFactory) Connect(u *url.URL) (smart.Transport, error) {
	return &DummyFetchTransport{self.MetaContentMap, self.DeltaContentMap}, nil
}
