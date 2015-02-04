package main

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
		gitlobbinary := filepath.Join(cwd, "git-lob")
		f.WriteString(fmt.Sprintf(`
[filter "testlob"]
  clean = "%v filter-clean %%f"
  smudge = "%v filter-smudge %%f"
`, gitlobbinary, gitlobbinary))
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

	})
})
