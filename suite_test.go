package main

import (
	"bytes"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"log"
	"os"
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

// Create a small LOB file  ready for storing in the LOB area
// filename should be a subfolder of a test git repo
// SHA will have been calculated outside the software so can be validated
func CreateSmallTestLOBFileForStoring(filename string) (correctInfo *LOBInfo) {
	// This was calculated with 'shasum' on Mac OS X with this file content
	correctLOBInfo := &LOBInfo{SHA: "772157c6ef480852edf921f5924b1ca582b0d78f", NumChunks: 1, Size: 128 * 255 * 16}

	// Create binary file
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0777)
	if err != nil {
		Fail(fmt.Sprintf("Can't create test file %v: %v", filename, err))
	}
	for i := 0; i < 128; i++ {
		var j byte
		for j = 0; j < 255; j++ {
			f.Write(bytes.Repeat([]byte{j}, 16))
		}
	}
	f.Close()
	return correctLOBInfo

}

// As CreateSmallTestLOBFileForStoring but will create a larger file which will need multiple chunks
func CreateLargeTestLOBFileForStoring(filename string) (correctInfo *LOBInfo) {
	// This was calculated with 'shasum' on Mac OS X with this file content
	correctLOBInfo := &LOBInfo{SHA: "6dc61e7c7d33e87592da1e534063052a17bf8f3c", NumChunks: 4, Size: 25000 * 255 * 16}

	// Create binary file
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0777)
	if err != nil {
		Fail(fmt.Sprintf("Can't create test file %v: %v", filename, err))
	}
	for i := 0; i < 25000; i++ {
		var j byte
		for j = 0; j < 255; j++ {
			f.Write(bytes.Repeat([]byte{j}, 16))
		}
	}
	f.Close()
	return correctLOBInfo
}

// Manually insert small LOB file into the LOB store ready for retrieval
func CreateSmallTestLOBDataForRetrieval() (correctInfo *LOBInfo) {
	return nil
}

// Manually insert large multi-chunk LOB file into the LOB store ready for retrieval
func CreateLargeTestLOBDataForRetrieval() (correctInfo *LOBInfo) {
	return nil
}
