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
			latestFeature2CommitDate := time.Now().AddDate(0, -1, 0)
			latestFeature3CommitDate := refsExcludedDate.AddDate(0, -1, 0) // will be excluded
			headCommitsIncludedDate := latestHEADCommitDate.AddDate(0, 0, -defaultOptions.FetchCommitsPeriodHEAD).Add(time.Hour)
			headCommitsExcludedDate := headCommitsIncludedDate.Add(-time.Hour * 2)
			feature1CommitsIncludedDate := latestFeature1CommitDate.AddDate(0, 0, -defaultOptions.FetchCommitsPeriodOther).Add(time.Hour)
			feature2CommitsIncludedDate := latestFeature2CommitDate.AddDate(0, 0, -defaultOptions.FetchCommitsPeriodOther).Add(time.Hour)

			// Simple constant size for all files (not testing chunks)
			sz := int64(300)

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
			pushedSHA, err := FindLatestAncestorWhereBinariesPushed("origin", mastersha)
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
			os.RemoveAll(root)
			os.RemoveAll(originRoot)
			os.RemoveAll(originBinStore)
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
			RunGitCommandForTest(true, "checkout", setupOutputs[0].commit)
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(len(setupOutputs[0].lobSHAs)*2), "File count check should be right")
			filesTransferred = 0
			// This should have initialised ALL the push state (because no local LOBs) including all other branches, so reset it
			// then manually set it to [0]. We're trying to replicate the case where more than one fetch has taken place when the
			// git repo has had new commits pulled
			ResetPushedBinaryState("origin")
			MarkBinariesAsPushed("origin", setupOutputs[0].commit, "")
			// now fetch for commit 1, this should update the push state to this SHA
			RunGitCommandForTest(true, "checkout", setupOutputs[1].commit)
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(len(setupOutputs[1].lobSHAs)*2), "File count check should be right")
			pushed, err := FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[1].commit), "Fetch should have updated pushed state to the latest fetched point")
			filesTransferred = 0

			// Now test leaving a gap unpushed but present on remote
			// Commit 5 is > 2 days ahead of previous commit so this will only fetch 5
			// but will actually get 2 sets; the change at 5 and the change before 5
			RunGitCommandForTest(true, "checkout", setupOutputs[5].commit)
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(len(setupOutputs[5].lobSHAs)*4), "File count check should be right (2 commits)")
			pushed, err = FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[5].commit), "Fetch should have updated pushed state to the latest fetched point because files in gap are already on remote")
			filesTransferred = 0

			// Now try that again, but this time for a commit in the gap which is not fetched, make a file be missing on remote
			// this should mean that the push state is not moved, because we need to keep checking until someone pushes the missing LOB
			// (might be this client or any other)
			// Put the pushed state back to 1
			ResetPushedBinaryState("origin")
			MarkBinariesAsPushed("origin", setupOutputs[1].commit, "")
			// this test is for a missing file in the GAP, not a file we would fetch
			origRemoteBinary := filepath.Join(originBinStore, getLOBChunkRelativePath(setupOutputs[3].lobSHAs[0], 0))
			renamedRemoteBinary := origRemoteBinary + "_old"
			os.Rename(origRemoteBinary, renamedRemoteBinary)
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(0), "Should be nothing new to fetch (already done)")
			pushed, err = FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[1].commit), "Fetch should not have updated pushed state because remote files were missing in the gap")
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
			origRemoteBinary = filepath.Join(originBinStore, getLOBChunkRelativePath(setupOutputs[6].lobSHAs[0], 0))
			renamedRemoteBinary = origRemoteBinary + "_old"
			os.Rename(origRemoteBinary, renamedRemoteBinary)
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(len(setupOutputs[6].lobSHAs)*2+len(setupOutputs[7].lobSHAs)*2-1),
				"Should be 2 more commits to fetch, minus one which is missing")
			pushed, err = FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[1].commit), "Fetch should not have updated pushed state because remote files were missing in the gap")
			filesTransferred = 0
			// put this file back so it's not stopping push state update any more
			os.Rename(renamedRemoteBinary, origRemoteBinary)

			// now confirm that once the missing files are resolved, push state updates
			err = Fetch(provider, "origin", []*GitRefSpec{}, false, false, false, callback)
			Expect(err).To(BeNil(), "Should be no error fetching")
			Expect(filesTransferred).To(BeEquivalentTo(1), "Should fetch the 1 file that was missing")
			pushed, err = FindLatestAncestorWhereBinariesPushed("origin", "master")
			Expect(err).To(BeNil(), "Should be no error finding pushed ancestor")
			Expect(pushed).To(Equal(setupOutputs[7].commit), "Fetch should now have updated the push state to master")
			filesTransferred = 0

		})
	})

})
