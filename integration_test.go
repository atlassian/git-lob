package main

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	//. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
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
			[]byte(`*.png filter=lob -crlf
*.jpg filter=lob -crlf
*.zip filter=lob -crlf
*.tiff filter=lob -crlf
*.tga filter=lob -crlf
*.dds filter=lob -crlf
*.bmp filter=lob -crlf
*.mov filter=lob -crlf`), 0644)
		f, err := os.OpenFile(filepath.Join(root, ".git", "config"), os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			Fail(fmt.Sprintf("Can't write gitconfig: %v", err.Error()))
		}
		// Here we have to assume that go test is running in the root source folder
		cwd, _ := os.Getwd()
		gitlobbinary := filepath.Join(cwd, "git-lob")
		f.WriteString(fmt.Sprintf(`
[filter "lob"]
  clean = "%v filter-clean %f"
  smudge = "%v filter-smudge %f"
`, gitlobbinary, gitlobbinary))
		f.Close()

		fmt.Println("Decided that git-lob is located at ", gitlobbinary)
	}

	Context("Integration tests", func() {
		// All actual tests nested here
		root := filepath.Join(os.TempDir(), "IntegrationTest")
		var oldwd string
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
		})

	})
})
