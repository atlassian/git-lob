package main

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
)

var _ = Describe("Purge", func() {
	Describe("Purge all unreferenced", func() {

		root := filepath.Join(os.TempDir(), "PurgeTest")
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
				shasToDelete, err := PurgeUnreferenced(false)
				Expect(err).To(BeNil(), "PurgeUnreferenced should succeed")
				Expect(shasToDelete).To(BeEmpty(), "Should report no files to purge")
			})
		})

		Context("Many files present", func() {
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
					metafile := getLOBMetaFilename(s)
					ioutil.WriteFile(metafile, []byte("meta something"), 0644)
					lobfiles = append(lobfiles, metafile)
					numChunks := rand.Intn(3) + 1
					for c := 0; c < numChunks; c++ {
						chunkfile := getLOBChunkFilename(s, c)
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
			Context("purges all files when no references", func() {
				// Because we've created no commits, all LOBs should be eligible for deletion
				It("lists files but doesn't act on it in dry run mode", func() {
					shasToDelete, err := PurgeUnreferenced(true)
					Expect(err).To(BeNil(), "PurgeUnreferenced should succeed")
					// Use sets to compare so ordering doesn't matter
					actualset := NewStringSetFromSlice(shasToDelete)
					Expect(actualset).To(Equal(lobshaset), "Should want to delete all files")

					for _, file := range lobfiles {
						exists, _ := FileOrDirExists(file)
						Expect(exists).To(Equal(true), "File %v should still exist", file)
					}

				})
				It("deletes files when not in dry run mode", func() {
					shasToDelete, err := PurgeUnreferenced(false)
					Expect(err).To(BeNil(), "PurgeUnreferenced should succeed")
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

					deletedSlice, err := PurgeUnreferenced(false)
					Expect(err).To(BeNil(), "PurgeUnreferenced should succeed")
					shasDidDelete := NewStringSetFromSlice(deletedSlice)

					Expect(shasDidDelete).To(Equal(shasShouldDelete), "Should delete the correct LOBs")
				})

			})

		})

	})

})
