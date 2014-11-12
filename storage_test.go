package main

import (
	"bytes"
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

		})

		AfterEach(func() {
			// Delete repo
			os.RemoveAll(root)
		})

		Context("Store small single chunk LOB", func() {
			testFileName := path.Join(folders[2], "small.dat")
			// This was calculated with 'shasum' on Mac OS X with this file content
			correctLOBInfo := &LOBInfo{SHA: "772157c6ef480852edf921f5924b1ca582b0d78f", NumChunks: 1, Size: 128 * 255 * 16}

			BeforeEach(func() {
				// Create binary file
				f, err := os.OpenFile(testFileName, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0777)
				if err != nil {
					Fail(fmt.Sprintf("Can't create test file %v: %v", testFileName, err))
				}
				for i := 0; i < 128; i++ {
					var j byte
					for j = 0; j < 255; j++ {
						f.Write(bytes.Repeat([]byte{j}, 16))
					}
				}
				f.Close()
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
			// This was calculated with 'shasum' on Mac OS X with this file content
			correctLOBInfo := &LOBInfo{SHA: "6dc61e7c7d33e87592da1e534063052a17bf8f3c", NumChunks: 4, Size: 25000 * 255 * 16}

			BeforeEach(func() {
				// Create binary file
				f, err := os.OpenFile(testFileName, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0777)
				if err != nil {
					Fail(fmt.Sprintf("Can't create test file %v: %v", testFileName, err))
				}
				for i := 0; i < 25000; i++ {
					var j byte
					for j = 0; j < 255; j++ {
						f.Write(bytes.Repeat([]byte{j}, 16))
					}
				}
				f.Close()
			})
			AfterEach(func() {
				//os.Remove(testFileName)
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
			correctLOBInfo := &LOBInfo{SHA: "772157c6ef480852edf921f5924b1ca582b0d78f", NumChunks: 1, Size: 128 * 255 * 16}

			BeforeEach(func() {
				err := storeLOBInfo(correctLOBInfo)
				Expect(err).To(BeNil(), "Shouldn't be error creating LOB meta file")

				lobFile := getLOBChunkFilename(correctLOBInfo.SHA, 0)
				f, err := os.OpenFile(lobFile, os.O_WRONLY|os.O_CREATE, 0666)
				Expect(err).To(BeNil(), "Shouldn't be error creating LOB file %v", lobFile)
				// Write test data
				for i := 0; i < 128; i++ {
					var j byte
					for j = 0; j < 255; j++ {
						f.Write(bytes.Repeat([]byte{j}, 16))
					}
				}
				f.Close()
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
	})
})
