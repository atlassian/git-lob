package main

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	. "bitbucket.org/sinbad/git-lob/core"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// This test file is about running git commands with git filters configured
// In order to do this, we have to actually run 'go build' to produce a git-lob
// binary, and configure filters & .gitattributes in a repo to do it
var _ = Describe("Integration", func() {

	BeforeEach(func() {
		// This singular BeforeEach will just perform the build
		outp, err := exec.Command("go", "build").CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to call 'go build': %v\n%v", err.Error(), string(outp)))
		}
	})

	var gitlobbinarypath string
	// Function to create a git repo configured with git-lob filters for integration testing
	createConfiguredRepoFunc := func(root string) {
		CreateGitRepoForTest(root)
		ioutil.WriteFile(filepath.Join(root, ".gitattributes"),
			[]byte(`*.png filter=testlob -crlf
*.jpg filter=testlob -crlf
*.zip filter=testlob -crlf
*.tiff filter=testlob -crlf
*.tga filter=testlob -crlf
*.dds filter=testlob -crlf
*.bmp filter=testlob -crlf
*.mov filter=testlob -crlf`), 0644)
		f, err := os.OpenFile(filepath.Join(root, ".git", "config"), os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			Fail(fmt.Sprintf("Can't write gitconfig: %v", err.Error()))
		}
		// Here we have to assume that go test is running in the root source folder
		cwd, _ := os.Getwd()
		gitlobbinarypath = filepath.Join(cwd, "git-lob")
		// For windows we need to convert \ to / since Git doesn't allow backslashes
		gitlobbinarypath = strings.Replace(gitlobbinarypath, "\\", "/", -1)
		f.WriteString(fmt.Sprintf(`
[filter "testlob"]
  clean = "%v filter-clean %%f"
  smudge = "%v filter-smudge %%f"
`, gitlobbinarypath, gitlobbinarypath))
		f.Close()
	}

	Context("Integration tests", func() {
		// All actual tests nested here
		root := filepath.Join(os.TempDir(), "IntegrationTest")
		var oldwd string
		filespercommit := [][]string{
			[]string{
				"img1.png", "img2.jpg",
				filepath.Join("movies", "movie1.mov"),
				filepath.Join("movies", "movie2.mov"),
				filepath.Join("other", "files", "windows.bmp"),
			},
			[]string{
				"img3.tga", "img4.tiff", "img5.png",
				filepath.Join("movies", "movie3.mov"),
				filepath.Join("movies", "movie4.mov"),
				filepath.Join("other", "files", "windows7.bmp"),
			},
			[]string{
				"img6.jpg",
				filepath.Join("other", "files", "windows10.bmp"),
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
		checkExistsAndRightSize := func(fileset int) {
			files := filespercommit[fileset]
			for i, file := range files {
				sz := sizeForFile(file, i)
				stat, err := os.Stat(file)
				Expect(err).To(BeNil(), fmt.Sprintf("%v should exist", file))
				Expect(stat.Size()).To(BeEquivalentTo(sz), fmt.Sprintf("%v should be correct size", file))
			}
		}
		checkExistsAndIsPlaceholder := func(fileset int) {
			files := filespercommit[fileset]
			for _, file := range files {
				stat, err := os.Stat(file)
				Expect(err).To(BeNil(), fmt.Sprintf("%v should exist", file))
				Expect(stat.Size()).To(BeEquivalentTo(SHALineLen), fmt.Sprintf("%v should be correct size", file))
			}
		}
		checkNotExists := func(fileset int) {
			files := filespercommit[fileset]
			for _, file := range files {
				_, err := os.Stat(file)
				Expect(err).ToNot(BeNil(), fmt.Sprintf("%v should not exist", file))
			}
		}
		checkGitStatusNotModified := func() {
			outp, err := exec.Command("git", "status", "--porcelain", "-uno").CombinedOutput()
			Expect(outp).To(HaveLen(0), "Should be no modified files")
			Expect(err).To(BeNil(), "git status should succeed")
		}
		moveAsideLOBs := func(shas []string) {
			for _, sha := range shas {
				meta := GetLocalLOBMetaPath(sha)
				err := os.Rename(meta, meta+"_bak")
				Expect(err).To(BeNil(), fmt.Sprintf("Rename should succeed for %v", meta))
				// Assuming only one chunk for this test
				chunk := GetLocalLOBChunkPath(sha, 0)
				err = os.Rename(chunk, chunk+"_bak")
				Expect(err).To(BeNil(), fmt.Sprintf("Rename should succeed for %v", chunk))
			}

		}
		restoreLOBs := func(shas []string) {
			for _, sha := range shas {
				meta := GetLocalLOBMetaPath(sha)
				err := os.Rename(meta+"_bak", meta)
				Expect(err).To(BeNil(), fmt.Sprintf("Rename should succeed for %v", meta))
				// Assuming only one chunk for this test
				chunk := GetLocalLOBChunkPath(sha, 0)
				err = os.Rename(chunk+"_bak", chunk)
				Expect(err).To(BeNil(), fmt.Sprintf("Rename should succeed for %v", chunk))
			}

		}

		BeforeEach(func() {
			createConfiguredRepoFunc(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)
		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
		})

		It("Git filters work", func() {

			diffregex := regexp.MustCompile("(?m)git-lob: ([A-Fa-f0-9]{40})")
			// Create commits
			for ci, commitfiles := range filespercommit {
				for i, file := range commitfiles {
					err := os.MkdirAll(filepath.Dir(file), 0755)
					Expect(err).To(BeNil(), "Shouldn't fail creating dir")
					sz := sizeForFile(file, i)
					// Create real content
					CreateRandomFileForTest(sz, file)
					// git add, will write placeholder & store via filter (or should)
					err = exec.Command("git", "add", file).Run()
					Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail in git add for %v", file))

					// Check content of index & that binaries created

					diffout, err := exec.Command("git", "diff", "--cached", file).CombinedOutput()
					Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail in git diff for %v", file))
					match := diffregex.FindStringSubmatch(string(diffout))
					Expect(match).ToNot(BeNil(), fmt.Sprintf("Should find git-lob ref in diff for %v", file))
					Expect(match).To(HaveLen(2), "Should be 2 matches")
					sha := match[1]

					// confirm that the file is stored correctly
					err = CheckLOBFilesForSHA(sha, GetLocalLOBRoot(), false)
					Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail checking storage for %v", file))
				}
				// Commit & tag
				err := exec.Command("git", "commit", "-m", fmt.Sprintf("Commit %d", ci)).Run()
				Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail commit %d", ci))
				err = exec.Command("git", "tag", "-a", "-m", "Nothing", fmt.Sprintf("Tag%d", ci)).Run()
				Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail tagging %d", ci))

				checkGitStatusNotModified()
			}

			// Now check out a few times to make sure working copy changes appropriately
			err := exec.Command("git", "checkout", "Tag0").Run()
			Expect(err).To(BeNil(), "Shouldn't fail to checkout")
			checkExistsAndRightSize(0)
			checkNotExists(1)
			checkNotExists(2)
			checkGitStatusNotModified()

			err = exec.Command("git", "checkout", "Tag1").Run()
			Expect(err).To(BeNil(), "Shouldn't fail to checkout")
			checkExistsAndRightSize(0)
			checkExistsAndRightSize(1)
			checkNotExists(2)
			checkGitStatusNotModified()

			err = exec.Command("git", "checkout", "Tag2").Run()
			Expect(err).To(BeNil(), "Shouldn't fail to checkout")
			checkExistsAndRightSize(0)
			checkExistsAndRightSize(1)
			checkExistsAndRightSize(2)
			checkGitStatusNotModified()

		})

		It("leaves files unmodified when placeholders present and after checkout", func() {
			diffregex := regexp.MustCompile("(?m)git-lob: ([A-Fa-f0-9]{40})")
			var shaspercommit [][]string
			// Create commits
			for ci, commitfiles := range filespercommit {
				var shas []string
				for i, file := range commitfiles {
					err := os.MkdirAll(filepath.Dir(file), 0755)
					Expect(err).To(BeNil(), "Shouldn't fail creating dir")
					sz := sizeForFile(file, i)
					// Create real content
					CreateRandomFileForTest(sz, file)
					// git add, will write placeholder & store via filter (or should)
					err = exec.Command("git", "add", file).Run()
					Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail in git add for %v", file))

					// Get SHA
					diffout, err := exec.Command("git", "diff", "--cached", file).CombinedOutput()
					Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail in git diff for %v", file))
					match := diffregex.FindStringSubmatch(string(diffout))
					Expect(match).ToNot(BeNil(), fmt.Sprintf("Should find git-lob ref in diff for %v", file))
					Expect(match).To(HaveLen(2), "Should be 2 matches")
					sha := match[1]

					shas = append(shas, sha)
				}
				// Commit & tag
				err := exec.Command("git", "commit", "-m", fmt.Sprintf("Commit %d", ci)).Run()
				Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail commit %d", ci))
				err = exec.Command("git", "tag", "-a", "-m", "Nothing", fmt.Sprintf("Tag%d", ci)).Run()
				Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail tagging %d", ci))

				shaspercommit = append(shaspercommit, shas)
			}

			// check out Tag0, thus deleting everything in Tag1/2
			err := exec.Command("git", "checkout", "Tag0").Run()
			Expect(err).To(BeNil(), "Shouldn't fail to checkout")
			checkNotExists(1)
			checkNotExists(2)
			checkGitStatusNotModified()

			// Now move aside storage for files in Tag1/2 so that they will be missing
			// when we try to check them out
			moveAsideLOBs(shaspercommit[1])
			moveAsideLOBs(shaspercommit[2])
			err = exec.Command("git", "checkout", "Tag2").Run()
			Expect(err).To(BeNil(), "Shouldn't fail to checkout")
			checkExistsAndIsPlaceholder(1)
			checkExistsAndIsPlaceholder(2)

			// now restore data for 1
			restoreLOBs(shaspercommit[1])
			// Now git-lob checkout - note we call direct not via "git lob" so we don't assume this test version is
			// on the path (it probably isn't). We ignore errors because Tag2 files are still missing so will return non-zero
			exec.Command(gitlobbinarypath, "checkout").Run()
			checkExistsAndRightSize(1)
			checkExistsAndIsPlaceholder(2)
			checkGitStatusNotModified()

			// now restore data for 2
			restoreLOBs(shaspercommit[2])
			// Now git-lob checkout - note we call direct not via "git lob" so we don't assume this test version is
			// on the path (it probably isn't). Should be no errors this time since complete data
			err = exec.Command(gitlobbinarypath, "checkout").Run()
			Expect(err).To(BeNil(), "Shouldn't fail to git-lob checkout")
			checkExistsAndRightSize(1)
			checkExistsAndRightSize(2)
			checkGitStatusNotModified()

		})

	})
})
