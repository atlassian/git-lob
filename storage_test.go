package main

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

var _ = Describe("Storage", func() {

	root := path.Join(os.TempDir(), "StorageTest")
	separateGitDir := path.Join(os.TempDir(), "StorageTestGitDir")
	folders := []string{
		path.Join(root, "folder1"),
		path.Join(root, "folder2"),
		path.Join(root, "folder3"),
		path.Join(root, "folder1/sub1"),
		path.Join(root, "folder1/sub2"),
		path.Join(root, "folder1/sub1/subsub1"),
		path.Join(root, "folder1/a/b/c/d/e/f/g/h/i/j/k/l")}

	Describe("Identifying git repo root", func() {
		Context("Valid git repo", func() {

			BeforeEach(func() {
				// Set up git repo with some subfolders
				CreateGitRepoForTest(root)

				for _, f := range folders {
					err := os.MkdirAll(f, 0777)
					if err != nil {
						fmt.Printf("Can't MkdirAll %v: %v", f, err)
					}
				}

			})

			AfterEach(func() {
				// Delete repo
				os.RemoveAll(root)
			})

			It("finds root git folder", func() {

				// Need to expand root for symlinks here in order to guarantee string comparison works
				// /var turns into /private/var on OS X for example
				// Can't use this for creating repos etc though, OS X doesn't like direct access
				expandedroot, _ := filepath.EvalSymlinks(root)

				for _, f := range folders {
					err := os.Chdir(f)
					if err != nil {
						Fail(fmt.Sprintf("Can't chdir to %v: %v", f, err))
					}
					testroot, sep := GetRepoRoot()
					Expect(testroot).To(Equal(expandedroot))
					Expect(sep).To(Equal(false))
				}
			})
		})
		Context("Invalid git repo", func() {
			It("Fails safely outside a git repo", func() {
				// Relies on temp dir not being a git repo, which should be valid assumption
				os.Chdir(os.TempDir())
				testroot, _ := GetRepoRoot()
				Expect(testroot).To(Equal(""))
			})

		})

	})

	Describe("Finding git dir", func() {
		Context("Git repo with standard git dir", func() {
			BeforeEach(func() {
				// Set up git repo with some subfolders
				CreateGitRepoForTest(root)

				for _, f := range folders {
					err := os.MkdirAll(f, 0777)
					if err != nil {
						fmt.Printf("Can't MkdirAll %v: %v", f, err)
					}
				}

			})

			AfterEach(func() {
				// Delete repo
				os.RemoveAll(root)
			})

			It("finds git dir", func() {

				// Need to expand root for symlinks here in order to guarantee string comparison works
				// /var turns into /private/var on OS X for example
				// Can't use this for creating repos etc though, OS X doesn't like direct access
				gitdir, _ := filepath.EvalSymlinks(path.Join(root, ".git"))

				for _, f := range folders {
					err := os.Chdir(f)
					if err != nil {
						Fail(fmt.Sprintf("Can't chdir to %v: %v", f, err))
					}
					testgitdir := GetGitDir()
					Expect(testgitdir).To(Equal(gitdir))
				}
			})

		})

		Context("Git repo with separate git dir", func() {
			BeforeEach(func() {
				// Set up git repo with some subfolders
				CreateGitRepoWithSeparateGitDirForTest(root, separateGitDir)

				for _, f := range folders {
					err := os.MkdirAll(f, 0777)
					if err != nil {
						fmt.Printf("Can't MkdirAll %v: %v", f, err)
					}
				}

			})

			AfterEach(func() {
				// Delete repo
				os.RemoveAll(root)
			})

			It("finds git dir", func() {

				// Need to expand root for symlinks here in order to guarantee string comparison works
				// /var turns into /private/var on OS X for example
				// Can't use this for creating repos etc though, OS X doesn't like direct access
				gitdir, _ := filepath.EvalSymlinks(separateGitDir)

				for _, f := range folders {
					err := os.Chdir(f)
					if err != nil {
						Fail(fmt.Sprintf("Can't chdir to %v: %v", f, err))
					}
					testgitdir := GetGitDir()
					Expect(testgitdir).To(Equal(gitdir))
				}
			})

		})
	})

	Describe("Storing a LOB", func() {
		// Common git repo
		BeforeEach(func() {
			// Set up git repo with some subfolders
			CreateGitRepoForTest(root)

			for _, f := range folders {
				err := os.MkdirAll(f, 0777)
				if err != nil {
					fmt.Printf("Can't MkdirAll %v: %v", f, err)
				}
			}
			os.Chdir(root)
		})

		AfterEach(func() {
			// Delete repo
			os.RemoveAll(root)
		})

		Context("Store small single chunk LOB", func() {
			testFileName := path.Join(folders[2], "small.dat")
			var correctLOBInfo *LOBInfo

			BeforeEach(func() {
				correctLOBInfo = CreateSmallTestLOBFileForStoring(testFileName)
			})
			AfterEach(func() {
				os.Remove(testFileName)
			})

			It("correctly stores a small file", func() {
				f, err := os.Open(testFileName)
				if err != nil {
					Fail(fmt.Sprintf("Can't reopen test file %v: %v", testFileName, err))
				}
				defer f.Close()
				// Need to read leader for consistency with real usage
				leader := make([]byte, SHALineLen)
				c, err := f.Read(leader)
				if err != nil {
					Fail(fmt.Sprintf("Can't read leader of test file %v: %v", testFileName, err))
				}
				lobinfo, err := StoreLOB(f, leader[:c])
				Expect(err).To(BeNil(), "Shouldn't be error storing LOB")
				Expect(lobinfo).To(Equal(correctLOBInfo))
				fileinfo, err := os.Stat(getLOBChunkFilename(lobinfo.SHA, 0))
				Expect(err).To(BeNil(), "Shouldn't be error opening stored LOB")
				Expect(fileinfo.Size()).To(Equal(lobinfo.Size), "Stored LOB should be correct size")
			})

		})

		Context("Store large multiple chunk LOB [LONGTEST]", func() {

			testFileName := path.Join(folders[2], "large.dat")
			var correctLOBInfo *LOBInfo

			BeforeEach(func() {
				correctLOBInfo = CreateLargeTestLOBFileForStoring(testFileName)
			})
			AfterEach(func() {
				os.Remove(testFileName)
			})

			It("correctly stores a large file", func() {
				f, err := os.Open(testFileName)
				if err != nil {
					Fail(fmt.Sprintf("Can't reopen test file %v: %v", testFileName, err))
				}
				defer f.Close()
				// Need to read leader for consistency with real usage
				leader := make([]byte, SHALineLen)
				c, err := f.Read(leader)
				if err != nil {
					Fail(fmt.Sprintf("Can't read leader of test file %v: %v", testFileName, err))
				}
				lobinfo, err := StoreLOB(f, leader[:c])
				Expect(err).To(BeNil(), "Shouldn't be error storing LOB")
				Expect(lobinfo).To(Equal(correctLOBInfo))
				for i := 0; i < lobinfo.NumChunks; i++ {
					fileinfo, err := os.Stat(getLOBChunkFilename(lobinfo.SHA, i))
					Expect(err).To(BeNil(), "Shouldn't be error opening stored LOB #%v", i)
					if i+1 < lobinfo.NumChunks {
						Expect(fileinfo.Size()).To(BeEquivalentTo(CHUNKLIMIT), "Stored LOB #%v should be chunk limit size", i)
					} else {
						Expect(fileinfo.Size()).To(BeEquivalentTo(lobinfo.Size%CHUNKLIMIT), "Stored LOB #%v should be correct size", i)
					}

				}
			})

		})

	})

	Describe("Retrieving a LOB", func() {
		// Common git repo
		BeforeEach(func() {
			// Set up git repo with some subfolders
			CreateGitRepoForTest(root)

			for _, f := range folders {
				err := os.MkdirAll(f, 0777)
				if err != nil {
					fmt.Printf("Can't MkdirAll %v: %v", f, err)
				}
			}

		})

		AfterEach(func() {
			// Delete repo
			os.RemoveAll(root)
		})

		Context("Retrieve small single chunk LOB", func() {
			var correctLOBInfo *LOBInfo

			BeforeEach(func() {
				correctLOBInfo = CreateSmallTestLOBDataForRetrieval()
			})

			It("correctly retrieves small LOB file", func() {
				// output to a temp file
				out, err := ioutil.TempFile("", "lobsmall.dat")
				Expect(err).To(BeNil(), "Shouldn't be error creating temp file")
				outFilename := out.Name()
				info, err := RetrieveLOB(correctLOBInfo.SHA, out)

				Expect(err).To(BeNil(), "Shouldn't be error retrieving LOB")
				out.Close()

				Expect(info).To(Equal(correctLOBInfo), "Metadata should agree")
				// Check output file
				stat, err := os.Stat(outFilename)
				Expect(err).To(BeNil(), "Shouldn't be error checking output file")
				Expect(stat.Size()).To(Equal(info.Size), "Size on disk should agree with metadata")

				os.Remove(outFilename)

			})

		})
		Context("Retrieve large multiple chunk LOB [LONGTEST]", func() {
			var correctLOBInfo *LOBInfo

			BeforeEach(func() {
				correctLOBInfo = CreateLargeTestLOBDataForRetrieval()
			})

			It("correctly retrieves large LOB file", func() {
				// output to a temp file
				out, err := ioutil.TempFile("", "loblarge.dat")
				Expect(err).To(BeNil(), "Shouldn't be error creating temp file")
				outFilename := out.Name()
				info, err := RetrieveLOB(correctLOBInfo.SHA, out)

				Expect(err).To(BeNil(), "Shouldn't be error retrieving LOB")
				out.Close()

				Expect(info).To(Equal(correctLOBInfo), "Metadata should agree")
				// Check output file
				stat, err := os.Stat(outFilename)
				Expect(err).To(BeNil(), "Shouldn't be error checking output file")
				Expect(stat.Size()).To(Equal(info.Size), "Size on disk should agree with metadata")

				os.Remove(outFilename)

			})

		})

	})
})
