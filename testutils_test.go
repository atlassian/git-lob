package main

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	"bitbucket.org/sinbad/git-lob/util"
	"bufio"
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Utility methods for testing only
// Sadly there's some duplication with the same file in the 'core' package; this is because you can't import shared packages when testing
// and still use _test.go to avoid polluting the non-testing namespace. Minimal duplication is a lesser evil than putting these utility functions
// in the non-test build
func CreateGitRepoForTest(path string) {
	// in case not previously deleted cleanly
	ForceRemoveAll(path)
	cmd := exec.Command("git", "init", path)
	err := cmd.Run()
	if err != nil {
		Fail("Unable to create git repo at " + path + ": " + err.Error())
	}
}

// Simplistic fire & forget running of git command - returns combined output
func RunGitCommandForTest(failureCheck bool, args ...string) string {
	outp, err := exec.Command("git", args...).CombinedOutput()
	if failureCheck && err != nil {
		Fail(fmt.Sprintf("Error running git command 'git %v': %v", strings.Join(args, " "), err.Error()))
	}
	return string(outp)

}

// Create a file with random data of size sz
func CreateRandomFileForTest(sz int64, filename string) {
	os.MkdirAll(filepath.Dir(filename), 0755)
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		Fail(fmt.Sprintf("Can't create test file %v: %v", filename, err))
	}
	defer f.Close()
	// random data
	fileWriter := bufio.NewWriter(f)
	_, err = io.CopyN(fileWriter, cryptorand.Reader, sz)
	fileWriter.Flush()
	if err != nil {
		Fail(fmt.Sprintf("Can't write random data to test file %v: %v", filename, err))
	}

}

// Delete a directory & all contents, overriding read-only flags
// BE VERY CAREFUL WITH THIS
func ForceRemoveAll(path string) error {
	// os.RemoveAll doesn't always work. Git marks some files within its structure as read-only
	// and some OS's then don't delete these files & return an error (e.g. Windows)
	err := os.RemoveAll(path)
	if err != nil && runtime.GOOS == "windows" {
		if path != "" && path != "\\" && util.DirExists(path) {
			// 'del' isn't an executable, it's a builtin of cmd
			cmd := exec.Command("cmd", "/C", "del", "/S", "/F", "/Q", path)
			err = cmd.Run()
		}
	}

	return err
}
