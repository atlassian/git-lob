package main

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var _ = Describe("Prune", func() {
	Describe("Prune old references", func() {
		// Test critieria
		// Doesn't prune when all commits in range
		// Prunes pushed and not non-pushed
		// Remote filtering for push test
		// Picks up multiple branches
		// Merges?

		root := filepath.Join(os.TempDir(), "PruneOldTest")
		var oldwd string
		var setupInputs []*TestCommitSetupInput
		var setupOutputs []*CommitLOBRef
		BeforeEach(func() {
			// Just be explicit
			GlobalOptions = NewOptions()

			// Set up git repo with some subfolders
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)

			// Create a single commit (not referencing any SHA)
			now := time.Now()

			setupInputs = []*TestCommitSetupInput{
				&TestCommitSetupInput{ // 0
					CommitDate: now.AddDate(0, 0, -9).Add(time.Hour),
					// data1.bin is never overwritten so will remain regardless of date
					Files: []string{"data1.bin", filepath.Join("img", "image1.jpg")},
				},
				&TestCommitSetupInput{ // 1
					CommitDate: now.AddDate(0, 0, -8).Add(time.Hour),
					Files:      []string{"data2.bin", filepath.Join("img", "image2.jpg")},
				},
				// branch, modify & add
				// this branch we'll leave hanging
				&TestCommitSetupInput{ // 2
					CommitDate: now.AddDate(0, 0, -7).Add(time.Hour),
					Files:      []string{"data3.bin", filepath.Join("bigdata", "something.dat")},
					FileSizes:  []int64{150, 2000},
					NewBranch:  "feature/hanging",
				},
				&TestCommitSetupInput{ // 3
					CommitDate:     now.AddDate(0, 0, -3).Add(time.Hour),
					Files:          []string{"data3.bin", filepath.Join("bigdata", "something.dat")},
					ParentBranches: []string{"feature/hanging"},
				},
				// Tip commit includes no files; this is because including LOB changes here will
				// cause previous state to also be preserved since 1 day back would need old LOB state
				&TestCommitSetupInput{ // 4
					CommitDate:     now.AddDate(0, 0, -3).Add(time.Hour),
					ParentBranches: []string{"feature/hanging"},
				},
				// now back on master
				&TestCommitSetupInput{ // 5
					CommitDate:     now.AddDate(0, 0, -5).Add(time.Hour),
					Files:          []string{"data2.bin", "newfile1.dat", "newfile2.dat"},
					ParentBranches: []string{"master"},
				},
				// another new branch, this one we'll merge
				// deliberately include changes which will overwrite each other
				// to ensure that we pick up the cancelled LOB changes to retain
				// in this case 'mergedata.bin' change at this commit is overwritten by the
				// next commit when it comes to merging to master
				&TestCommitSetupInput{ // 6
					CommitDate: now.AddDate(0, 0, -3).Add(time.Hour),
					Files:      []string{"mergedata.bin", "mergedata2.dat"},
					NewBranch:  "feature/tomerge",
				},
				&TestCommitSetupInput{ // 7
					CommitDate:     now.AddDate(0, 0, -2).Add(time.Hour),
					Files:          []string{"mergedata.bin", "mergedata3.dat"},
					ParentBranches: []string{"feature/tomerge"},
				},
				// now merge (no changes added except from merge)
				&TestCommitSetupInput{ // 8
					CommitDate:     now.AddDate(0, 0, -1).Add(time.Hour),
					ParentBranches: []string{"master", "feature/tomerge"},
				},
				// one more commit on master
				&TestCommitSetupInput{ // 9
					CommitDate:     now,
					Files:          []string{filepath.Join("img", "image30.jpg")},
					ParentBranches: []string{"master"}, // unnecessary but make sure
				},
			}

			setupOutputs = SetupRepoForTest(setupInputs)
		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
			GlobalOptions = NewOptions()
		})

		It("Removes based on retention and pushed flags", func() {
			GlobalOptions.RetentionRefsPeriod = 30
			GlobalOptions.RetentionCommitsPeriodHEAD = 7
			GlobalOptions.RetentionCommitsPeriodOther = 1 // not the default, retain by date
			// we want to test that the older commit on the 'hanging' branch is retained when it's 1 day back, not because it's not pushed

			lobsdeleted := 0
			lobsretainedbydate := 0
			lobsretainednotpushed := 0
			callback := func(t PruneCallbackType, sha string) {
				switch t {
				case PruneRetainByDate:
					lobsretainedbydate++
				case PruneRetainNotPushed:
					lobsretainednotpushed++
				case PruneDeleted:
					lobsdeleted++
				}
			}
			filestoretain := 0
			for _, out := range setupOutputs {
				filestoretain += len(out.lobSHAs)
			}
			//fmt.Println(setupOutputs)
			deleted, err := PruneOld(false, callback)
			Expect(err).To(BeNil(), "Should be no error pruning")
			Expect(deleted).To(BeEmpty(), "No files should be deleted, all within range")
			Expect(lobsdeleted).To(BeZero(), "No deletion callbacks should be made")
			Expect(lobsretainedbydate).To(BeEquivalentTo(filestoretain), "Should be correct number of file SHAs retained by date")
			Expect(lobsretainednotpushed).To(BeEquivalentTo(0), "Should not need to rely on not pushed to retain anything")
			lobsretainedbydate = 0

			// Now retain no extra versions on other branches, just latest versions
			GlobalOptions.RetentionCommitsPeriodOther = 0
			deleted, err = PruneOld(false, callback)
			// However, not pushed flag should stop them being deleted
			Expect(lobsretainednotpushed).To(BeEquivalentTo(2), "Should have kept a couple of files because not pushed")
			Expect(err).To(BeNil(), "Should be no error pruning")
			Expect(deleted).To(BeEmpty(), "No files should be deleted")
			Expect(lobsdeleted).To(BeZero(), "No deletion callbacks should be made")
			Expect(lobsretainedbydate).To(BeEquivalentTo(filestoretain-2), "Should be correct number of file SHAs retained by date")
			lobsretainedbydate = 0
			lobsretainednotpushed = 0

			// now mark that branch as pushed so should delete
			MarkBinariesAsPushed("origin", setupOutputs[4].commit, "")
			deleted, err = PruneOld(false, callback)
			// However, not pushed flag should stop them being deleted
			Expect(err).To(BeNil(), "Should be no error pruning")
			Expect(lobsretainednotpushed).To(BeEquivalentTo(0), "All files that would be deleted are pushed")
			Expect(deleted).To(ConsistOf(setupOutputs[2].lobSHAs), "2 files should be deleted, old versions on non-HEAD")
			Expect(lobsdeleted).To(BeEquivalentTo(len(setupOutputs[2].lobSHAs)), "2 deletion callbacks should be made")
			filestoretain -= 2
			Expect(lobsretainedbydate).To(BeEquivalentTo(filestoretain), "Should be correct number of file SHAs retained by date")
			lobsretainedbydate = 0
			lobsdeleted = 0

			// Now retain less on current branch, and retain no other branches
			GlobalOptions.RetentionCommitsPeriodHEAD = 2
			GlobalOptions.RetentionRefsPeriod = 0
			// Retention should be all files to be checked out within last 2 days, which is latest plus any '-' changes in 2 days
			// needs to take account of what files were overwritten & which remained the same
			// commits 7,8 & 9 will be included, 7 being a commit that gets merged
			// final state from other commits bleeds through
			var lobstoretain []string
			var lobstodelete []string
			// first determine the complete list of files ever committed
			fileset := NewStringSet()
			for _, c := range setupInputs {
				// ignore hanging branch though
				if c.NewBranch == "feature/hanging" || (len(c.ParentBranches) > 0 && c.ParentBranches[0] == "feature/hanging") {
					continue
				}
				for _, l := range c.Files {
					fileset.Add(l)
				}
			}
			// Now add changes to keep
			for i := 7; i <= 9; i++ {
				out := setupOutputs[i]
				for _, l := range out.lobSHAs {
					lobstoretain = append(lobstoretain, l)
				}
			}
			// Now add the latest version for all prior to that
			for i := 6; i >= 0; i-- {
				// Note we already deleted LOBs in [2] above
				if i == 2 {
					continue
				}

				in := setupInputs[i]
				out := setupOutputs[i]
				for j, f := range in.Files {
					if fileset.Contains(f) {
						lobstoretain = append(lobstoretain, out.lobSHAs[j])
						// only record the latest
						fileset.Remove(f)
					} else {
						lobstodelete = append(lobstodelete, out.lobSHAs[j])
					}
				}
			}
			// mark master as pushed so should delete
			// hanging branch [4] was already marked as pushed, but now other refs are not being retained also
			MarkBinariesAsPushed("origin", setupOutputs[9].commit, "")
			deleted, err = PruneOld(false, callback)
			// However, not pushed flag should stop them being deleted
			Expect(err).To(BeNil(), "Should be no error pruning")
			Expect(lobsretainednotpushed).To(BeEquivalentTo(0), "All files that would be deleted are pushed")
			Expect(deleted).To(ConsistOf(lobstodelete), "Correct files should be deleted, all non-HEADs and old HEADs")
			Expect(lobsdeleted).To(BeEquivalentTo(len(lobstodelete)), "Correct deletion callbacks should be made")
			Expect(lobsretainedbydate).To(BeEquivalentTo(len(lobstoretain)), "Should be correct number of file SHAs retained by date")

		})

	})

	Describe("Prune all unreferenced", func() {

		root := filepath.Join(os.TempDir(), "PruneTest")
		var initialCommit string
		var oldwd string
		BeforeEach(func() {
			// Set up git repo with some subfolders
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)

			// Create a single commit (not referencing any SHA)
			initialCommit = CreateInitialCommitForTest(root)
		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
		})

		Context("No files", func() {
			It("does nothing when no files present", func() {
				lobsdeleted := 0
				lobsreferenced := 0
				callback := func(t PruneCallbackType, sha string) {
					switch t {
					case PruneRetainReferenced:
						lobsreferenced++
					case PruneDeleted:
						lobsdeleted++
					}
				}
				shasToDelete, err := PruneUnreferenced(false, callback)
				Expect(err).To(BeNil(), "PruneUnreferenced should succeed")
				Expect(shasToDelete).To(BeEmpty(), "Should report no files to prune")
				Expect(lobsdeleted).To(BeZero(), "Should be no deletion callbacks")
				Expect(lobsreferenced).To(BeZero(), "Should be no referenced callbacks")
			})
		})

		Context("Local storage only", func() {
			var lobshaset StringSet
			var lobshas []string
			var lobfiles []string
			BeforeEach(func() {
				// Manually create a bunch of files, no need to really store things
				// since we just want to see what gets deleted
				lobfiles = make([]string, 0, 20)
				// Create a bunch of files, 20 SHAs
				lobshas = GetListOfRandomSHAsForTest(20)
				lobshaset = NewStringSetFromSlice(lobshas)
				for _, s := range lobshas {
					metafile := getLocalLOBMetaPath(s)
					ioutil.WriteFile(metafile, []byte("meta something"), 0644)
					lobfiles = append(lobfiles, metafile)
					numChunks := rand.Intn(3) + 1
					for c := 0; c < numChunks; c++ {
						chunkfile := getLocalLOBChunkPath(s, c)
						lobfiles = append(lobfiles, chunkfile)
						ioutil.WriteFile(chunkfile, []byte("data something"), 0644)
					}
				}

			})
			AfterEach(func() {
				for _, l := range lobfiles {
					os.Remove(l)
				}
			})
			Context("prunes all files when no references", func() {
				// Because we've created no commits, all LOBs should be eligible for deletion
				It("lists files but doesn't act on it in dry run mode", func() {
					lobsdeleted := 0
					lobsreferenced := 0
					callback := func(t PruneCallbackType, sha string) {
						switch t {
						case PruneRetainReferenced:
							lobsreferenced++
						case PruneDeleted:
							lobsdeleted++
						}
					}
					shasToDelete, err := PruneUnreferenced(true, callback)
					Expect(err).To(BeNil(), "PruneUnreferenced should succeed")
					Expect(lobsreferenced).To(BeZero(), "Should be no LOB referenced")
					Expect(lobsdeleted).To(BeEquivalentTo(len(lobshas)), "Should be correct number deleted in callback")
					// Use sets to compare so ordering doesn't matter
					actualset := NewStringSetFromSlice(shasToDelete)
					Expect(actualset).To(Equal(lobshaset), "Should want to delete all files")

					for _, file := range lobfiles {
						exists, _ := FileOrDirExists(file)
						Expect(exists).To(Equal(true), "File %v should still exist", file)
					}

				})
				It("deletes files when not in dry run mode", func() {
					shasToDelete, err := PruneUnreferenced(false, func(PruneCallbackType, string) {})
					Expect(err).To(BeNil(), "PruneUnreferenced should succeed")
					// Use sets to compare so ordering doesn't matter
					actualset := NewStringSetFromSlice(shasToDelete)
					Expect(actualset).To(Equal(lobshaset), "Should want to delete all files")

					for _, file := range lobfiles {
						exists, _ := FileOrDirExists(file)
						Expect(exists).To(Equal(false), "File %v should have been deleted", file)
					}

				})
			})
			Context("some files referenced", func() {
				It("correctly identifies referenced and unreferenced files", func() {
					// Manually create commits of files which reference the first few SHAs but not
					// the latter few. Also spread these references across several different branches,
					// and put at least one of them in a file modification

					// First 3 SHAs, create in one branch with different files
					CreateCommitReferencingLOBsForTest(root, map[string]string{
						lobshas[0]: "test1.png",
						lobshas[1]: "test2.png",
						lobshas[2]: "test3.png"})

					// 4th SHA is a modification
					CreateCommitReferencingLOBsForTest(root, map[string]string{lobshas[3]: "test1.png"})

					// next 3, create in a different branch, 2 modifications & 1 new
					exec.Command("git", "branch", "branch2").Run()
					CreateCommitReferencingLOBsForTest(root, map[string]string{
						lobshas[4]: "test2.png",
						lobshas[5]: "test3.png"})
					CreateCommitReferencingLOBsForTest(root, map[string]string{lobshas[6]: "test4.png"})

					// Back to main branch
					exec.Command("git", "checkout", "master").Run()
					CreateCommitReferencingLOBsForTest(root, map[string]string{lobshas[7]: "test10.png"})

					// next 3, create in a different branch, 2 modifications & 1 new
					exec.Command("git", "branch", "branch3").Run()
					CreateCommitReferencingLOBsForTest(root, map[string]string{
						lobshas[8]: "test1.png",
						lobshas[9]: "test12.png"})
					CreateCommitReferencingLOBsForTest(root, map[string]string{
						lobshas[10]: "test2.png",
						lobshas[11]: "test12.png"})

					exec.Command("git", "checkout", "master").Run()
					// Last one, reference 12 & 13 in index, not committed.
					ioutil.WriteFile(filepath.Join(root, "test3.png"), []byte(fmt.Sprintf("git-lob: %v", lobshas[12])), 0644)
					ioutil.WriteFile(filepath.Join(root, "test20.png"), []byte(fmt.Sprintf("git-lob: %v", lobshas[13])), 0644)
					exec.Command("git", "add", "test3.png", "test20.png").Run()

					// At this point we should be deleting SHAs 14-19 but not the others
					//shasShouldKeep := NewStringSetFromSlice(lobshas[0:14])
					shasShouldDelete := NewStringSetFromSlice(lobshas[14:])

					lobsdeleted := 0
					lobsreferenced := 0
					callback := func(t PruneCallbackType, sha string) {
						switch t {
						case PruneRetainReferenced:
							lobsreferenced++
						case PruneDeleted:
							lobsdeleted++
						}
					}
					deletedSlice, err := PruneUnreferenced(false, callback)
					Expect(err).To(BeNil(), "PruneUnreferenced should succeed")
					shasDidDelete := NewStringSetFromSlice(deletedSlice)

					Expect(shasDidDelete).To(Equal(shasShouldDelete), "Should delete the correct LOBs")
					Expect(lobsdeleted).To(BeEquivalentTo(len(shasShouldDelete)), "Callbacks for deletion should be correct")
					Expect(lobsreferenced).To(BeEquivalentTo(len(lobshas)-len(shasShouldDelete)), "Callbacks for retention should be correct")
					// Make sure files don't exist
					for sha := range shasShouldDelete.Iter() {
						matches, err := filepath.Glob(fmt.Sprintf("%v*", filepath.Join(GetLocalLOBDir(sha), sha)))
						Expect(err).To(BeNil(), "Should not be error in glob checking")
						Expect(matches).To(BeEmpty(), "All local SHAs should be deleted")
					}

				})

			})

		})
		Context("Shared storage - deleting at same time as local", func() {
			var lobshaset StringSet
			var lobshas []string
			var lobfiles []string
			sharedStore := filepath.Join(os.TempDir(), "PruneTest_SharedStore")

			BeforeEach(func() {
				os.MkdirAll(sharedStore, 0755)
				GlobalOptions.SharedStore = sharedStore
				// Manually create a bunch of files, no need to really store things
				// since we just want to see what gets deleted
				lobfiles = make([]string, 0, 20)
				// Create a bunch of files, 20 SHAs
				lobshas = GetListOfRandomSHAsForTest(20)
				lobshaset = NewStringSetFromSlice(lobshas)
				for _, s := range lobshas {
					metafile := getSharedLOBMetaPath(s)
					ioutil.WriteFile(metafile, []byte("meta something"), 0644)
					lobfiles = append(lobfiles, metafile)
					// link shared locally
					metalinkfile := getLocalLOBMetaPath(s)
					CreateHardLink(metafile, metalinkfile)
					lobfiles = append(lobfiles, metalinkfile)
					numChunks := rand.Intn(3) + 1
					for c := 0; c < numChunks; c++ {
						chunkfile := getSharedLOBChunkPath(s, c)
						lobfiles = append(lobfiles, chunkfile)
						ioutil.WriteFile(chunkfile, []byte("data something"), 0644)
						// link shared locally
						linkfile := getLocalLOBChunkPath(s, c)
						CreateHardLink(chunkfile, linkfile)
						lobfiles = append(lobfiles, linkfile)
					}
				}

			})
			AfterEach(func() {
				for _, l := range lobfiles {
					os.Remove(l)
				}
				os.RemoveAll(sharedStore)
				GlobalOptions.SharedStore = ""
			})
			Context("prunes all files when no references", func() {
				// Because we've created no commits, all LOBs should be eligible for deletion
				It("lists files but doesn't act on it in dry run mode", func() {
					shasToDelete, err := PruneUnreferenced(true, func(PruneCallbackType, string) {})
					Expect(err).To(BeNil(), "PruneUnreferenced should succeed")
					// Use sets to compare so ordering doesn't matter
					actualset := NewStringSetFromSlice(shasToDelete)
					Expect(actualset).To(Equal(lobshaset), "Should want to delete all files")

					// This includes both local links and shared files
					for _, file := range lobfiles {
						exists, _ := FileOrDirExists(file)
						Expect(exists).To(Equal(true), "File %v should still exist", file)
					}

				})
				It("deletes files when not in dry run mode", func() {
					shasToDelete, err := PruneUnreferenced(false, func(PruneCallbackType, string) {})
					Expect(err).To(BeNil(), "PruneUnreferenced should succeed")
					// Use sets to compare so ordering doesn't matter
					actualset := NewStringSetFromSlice(shasToDelete)
					Expect(actualset).To(Equal(lobshaset), "Should want to delete all files")

					// This includes both local links and shared files
					for _, file := range lobfiles {
						exists, _ := FileOrDirExists(file)
						Expect(exists).To(Equal(false), "File %v should have been deleted", file)
					}

				})
			})
			Context("some files referenced", func() {
				It("correctly identifies referenced and unreferenced files", func() {
					// Manually create commits of files which reference the first few SHAs but not
					// the latter few. Also spread these references across several different branches,
					// and put at least one of them in a file modification

					// First 3 SHAs, create in one branch with different files
					CreateCommitReferencingLOBsForTest(root, map[string]string{
						lobshas[0]: "test1.png",
						lobshas[1]: "test2.png",
						lobshas[2]: "test3.png"})

					// 4th SHA is a modification
					CreateCommitReferencingLOBsForTest(root, map[string]string{lobshas[3]: "test1.png"})

					// next 3, create in a different branch, 2 modifications & 1 new
					exec.Command("git", "branch", "branch2").Run()
					CreateCommitReferencingLOBsForTest(root, map[string]string{
						lobshas[4]: "test2.png",
						lobshas[5]: "test3.png"})
					CreateCommitReferencingLOBsForTest(root, map[string]string{lobshas[6]: "test4.png"})

					// Back to main branch
					exec.Command("git", "checkout", "master").Run()
					CreateCommitReferencingLOBsForTest(root, map[string]string{lobshas[7]: "test10.png"})

					// next 3, create in a different branch, 2 modifications & 1 new
					exec.Command("git", "branch", "branch3").Run()
					CreateCommitReferencingLOBsForTest(root, map[string]string{
						lobshas[8]: "test1.png",
						lobshas[9]: "test12.png"})
					CreateCommitReferencingLOBsForTest(root, map[string]string{
						lobshas[10]: "test2.png",
						lobshas[11]: "test12.png"})

					exec.Command("git", "checkout", "master").Run()
					// Last one, reference 12 & 13 in index, not committed.
					ioutil.WriteFile(filepath.Join(root, "test3.png"), []byte(fmt.Sprintf("git-lob: %v", lobshas[12])), 0644)
					ioutil.WriteFile(filepath.Join(root, "test20.png"), []byte(fmt.Sprintf("git-lob: %v", lobshas[13])), 0644)
					exec.Command("git", "add", "test3.png", "test20.png").Run()

					// At this point we should be deleting SHAs 14-19 but not the others
					//shasShouldKeep := NewStringSetFromSlice(lobshas[0:14])
					shasShouldDelete := NewStringSetFromSlice(lobshas[14:])

					deletedSlice, err := PruneUnreferenced(false, func(PruneCallbackType, string) {})
					Expect(err).To(BeNil(), "PruneUnreferenced should succeed")
					shasDidDelete := NewStringSetFromSlice(deletedSlice)

					Expect(shasDidDelete).To(Equal(shasShouldDelete), "Should delete the correct LOBs")

					// Make sure files don't exist
					for sha := range shasShouldDelete.Iter() {
						matches, err := filepath.Glob(fmt.Sprintf("%v*", filepath.Join(GetLocalLOBDir(sha), sha)))
						Expect(err).To(BeNil(), "Should not be error in glob checking")
						Expect(matches).To(BeEmpty(), "All local SHAs should be deleted")

						matches, err = filepath.Glob(fmt.Sprintf("%v*", filepath.Join(GetSharedLOBDir(sha), sha)))
						Expect(err).To(BeNil(), "Should not be error in glob checking")
						Expect(matches).To(BeEmpty(), "All shared SHAs should be deleted")
					}
				})

			})
		})

		Context("Shared storage - cleaning up", func() {
			var lobshaset StringSet
			var lobshas []string
			var sharedlobfiles []string
			sharedStore := filepath.Join(os.TempDir(), "PruneTest_SharedStore")

			BeforeEach(func() {
				os.MkdirAll(sharedStore, 0755)
				GlobalOptions.SharedStore = sharedStore
				// Manually create a bunch of files, no need to really store things
				// since we just want to see what gets deleted
				sharedlobfiles = make([]string, 0, 20)
				// Create a bunch of files, 20 SHAs, in shared area
				lobshas = GetListOfRandomSHAsForTest(20)
				lobshaset = NewStringSetFromSlice(lobshas)
				for _, s := range lobshas {
					metafile := getSharedLOBMetaPath(s)
					ioutil.WriteFile(metafile, []byte("meta something"), 0644)
					sharedlobfiles = append(sharedlobfiles, metafile)
					numChunks := rand.Intn(3) + 1
					for c := 0; c < numChunks; c++ {
						chunkfile := getSharedLOBChunkPath(s, c)
						sharedlobfiles = append(sharedlobfiles, chunkfile)
						ioutil.WriteFile(chunkfile, []byte("data something"), 0644)
					}
				}

			})
			AfterEach(func() {
				os.RemoveAll(sharedStore)
				GlobalOptions.SharedStore = ""
			})
			Context("prunes all files when no references", func() {
				// Because we've created no hard links to the shared store, everything should be available for deletion
				It("lists files but doesn't act on it in dry run mode", func() {
					shasToDelete, err := PruneSharedStore(true, func(PruneCallbackType, string) {})
					Expect(err).To(BeNil(), "PruneSharedStore should succeed")
					// Use sets to compare so ordering doesn't matter
					actualset := NewStringSetFromSlice(shasToDelete)
					Expect(actualset).To(Equal(lobshaset), "Should want to delete all files")

					// This includes both local links and shared files
					for _, file := range sharedlobfiles {
						exists, _ := FileOrDirExists(file)
						Expect(exists).To(Equal(true), "File %v should still exist", file)
					}

				})
				It("deletes files when not in dry run mode", func() {
					shasToDelete, err := PruneSharedStore(false, func(PruneCallbackType, string) {})
					Expect(err).To(BeNil(), "PruneSharedStore should succeed")
					// Use sets to compare so ordering doesn't matter
					actualset := NewStringSetFromSlice(shasToDelete)
					Expect(actualset).To(Equal(lobshaset), "Should want to delete all files")

					// This includes both local links and shared files
					for _, file := range sharedlobfiles {
						exists, _ := FileOrDirExists(file)
						Expect(exists).To(Equal(false), "File %v should have been deleted", file)
					}

				})
			})
			Context("some files referenced", func() {
				var locallobfiles []string
				const referenceUpTo = 10
				BeforeEach(func() {
					locallobfiles = make([]string, 0, 10)
					for i, sharedfile := range sharedlobfiles {
						// Just link into temp dir, doesn't matter where the link is
						localfile := filepath.Join(os.TempDir(), filepath.Base(sharedfile))
						CreateHardLink(sharedfile, localfile)
						locallobfiles = append(locallobfiles, localfile)

						// Only reference some
						if i >= referenceUpTo {
							break
						}

					}
				})
				AfterEach(func() {
					for _, l := range locallobfiles {
						os.Remove(l)
					}
				})

				It("does nothing in dry run mode", func() {
					PruneSharedStore(true, func(PruneCallbackType, string) {})
					for _, sharedfile := range sharedlobfiles {
						exists, _ := FileOrDirExists(sharedfile)
						Expect(exists).To(BeTrue(), "Should not have deleted %v", sharedfile)
					}
				})
				It("correctly identifies referenced and unreferenced shared files", func() {
					PruneSharedStore(false, func(PruneCallbackType, string) {})
					for i, sharedfile := range sharedlobfiles {
						exists, _ := FileOrDirExists(sharedfile)
						if i <= referenceUpTo {
							Expect(exists).To(BeTrue(), "Should not have deleted %v", sharedfile)
						} else {
							Expect(exists).To(BeFalse(), "Should have deleted %v", sharedfile)
						}
					}
				})

			})
		})
	})

})
