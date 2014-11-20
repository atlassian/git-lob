package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
)

var _ = Describe("Purge", func() {
	Describe("Purge all unreferenced", func() {

		root := path.Join(os.TempDir(), "PurgeTest")
		var oldwd string
		BeforeEach(func() {
			// Set up git repo with some subfolders
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)
		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
		})

		Context("No files", func() {
			It("does nothing when no files present", func() {
				shasToDelete := PurgeUnreferenced(false)
				Expect(shasToDelete).To(BeEmpty(), "Should report no files to purge")
			})
		})

		Context("Many files present", func() {
			var lobshaset StringSet
			var lobfiles []string
			BeforeEach(func() {
				// Manually create a bunch of files, no need to really store things
				// since we just want to see what gets deleted
				lobfiles = make([]string, 0, 20)
				// Create a bunch of files, 20 SHAs
				lobshas := GetListOfRandomSHAsForTest(20)
				lobshaset = NewStringSetFromSlice(lobshas)
				for _, s := range lobshas {
					metafile := getLOBMetaFilename(s)
					ioutil.WriteFile(metafile, []byte("meta something"), 0666)
					lobfiles = append(lobfiles, metafile)
					numChunks := rand.Intn(3) + 1
					for c := 0; c < numChunks; c++ {
						chunkfile := getLOBChunkFilename(s, c)
						lobfiles = append(lobfiles, chunkfile)
						ioutil.WriteFile(chunkfile, []byte("data something"), 0666)
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
					shasToDelete := PurgeUnreferenced(true)
					// Use sets to compare so ordering doesn't matter
					actualset := NewStringSetFromSlice(shasToDelete)
					Expect(actualset).To(Equal(lobshaset), "Should want to delete all files")

					for _, file := range lobfiles {
						exists, _ := FileOrDirExists(file)
						Expect(exists).To(Equal(true), "File %v should still exist", file)
					}

				})
				It("deletes files when not in dry run mode", func() {
					shasToDelete := PurgeUnreferenced(false)
					// Use sets to compare so ordering doesn't matter
					actualset := NewStringSetFromSlice(shasToDelete)
					Expect(actualset).To(Equal(lobshaset), "Should want to delete all files")

					for _, file := range lobfiles {
						exists, _ := FileOrDirExists(file)
						Expect(exists).To(Equal(false), "File %v should have been deleted", file)
					}

				})

			})

		})

	})

})
