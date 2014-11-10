package main

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
	"path"
	"path/filepath"
)

var _ = Describe("Storage", func() {

	Describe("Identifying git repo root", func() {

		Context("Valid git repo", func() {

			root := path.Join(os.TempDir(), "StorageTest")
			folders := []string{
				path.Join(root, "folder1"),
				path.Join(root, "folder2"),
				path.Join(root, "folder3"),
				path.Join(root, "folder1/sub1"),
				path.Join(root, "folder1/sub2"),
				path.Join(root, "folder1/sub1/subsub1"),
				path.Join(root, "folder1/a/b/c/d/e/f/g/h/i/j/k/l")}

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

})
