package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"log"
	"os/exec"
	"testing"
)

func TestAll(t *testing.T) {
	// Connect Ginkgo to Gomega
	RegisterFailHandler(Fail)

	// Set manual logging off
	loggingOff := true
	//loggingOff = false
	if loggingOff {
		errorFileLog = log.New(ioutil.Discard, "", 0)
		errorConsoleLog = log.New(ioutil.Discard, "", 0)
		debugLog = log.New(ioutil.Discard, "", 0)
		outputLog = log.New(ioutil.Discard, "", 0)
	}

	// Run everything
	RunSpecs(t, "Git Lob Test Suite")
}

// Utility methods
func CreateGitRepoForTest(path string) {
	cmd := exec.Command("git", "init", path)
	err := cmd.Run()
	if err != nil {
		Fail("Unable to create git repo: " + err.Error())
	}
}
func CreateGitRepoWithSeparateGitDirForTest(path string, gitDir string) {
	cmd := exec.Command("git", "init", "--separate-git-dir", gitDir, path)
	err := cmd.Run()
	if err != nil {
		Fail("Unable to create git repo: " + err.Error())
	}
}
