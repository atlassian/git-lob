package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAll(t *testing.T) {
	// Connect Ginkgo to Gomega
	RegisterFailHandler(Fail)

	// Set manual logging off
	loggingOff := true
	//loggingOff = false
	if loggingOff {
		consoleOut = ioutil.Discard
		errorLog = log.New(ioutil.Discard, "", 0)
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
	// This was calculated with 'shasum' on Mac OS X with this file content
	correctLOBInfo := &LOBInfo{SHA: "772157c6ef480852edf921f5924b1ca582b0d78f", NumChunks: 1, Size: 128 * 255 * 16}
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
	return correctLOBInfo
}

// Manually insert large multi-chunk LOB file into the LOB store ready for retrieval
func CreateLargeTestLOBDataForRetrieval() (correctInfo *LOBInfo) {
	// This was calculated with 'shasum' on Mac OS X with this file content
	correctFileSize := int64(25000 * 255 * 16)
	correctNumChunks := 4
	correctChunkSize := int64(32 * 1024 * 1024)
	correctLOBInfo := &LOBInfo{SHA: "6dc61e7c7d33e87592da1e534063052a17bf8f3c", NumChunks: correctNumChunks, Size: correctFileSize}

	err := storeLOBInfo(correctLOBInfo)
	Expect(err).To(BeNil(), "Shouldn't be error creating LOB meta file")

	// Write test data into 4 chunks
	var outf *os.File
	var currentChunkBytes int64
	var chunkIdx int

	for i := 0; i < 25000; i++ {
		var j byte
		for j = 0; j < 255; j++ {
			// We've specifically picked it so that this will exactly hit the end of a chunk
			if outf == nil || currentChunkBytes == correctChunkSize {
				if outf != nil {
					outf.Close()
				}
				lobFile := getLOBChunkFilename(correctLOBInfo.SHA, chunkIdx)
				chunkIdx++
				outf, err = os.OpenFile(lobFile, os.O_WRONLY|os.O_CREATE, 0666)
				Expect(err).To(BeNil(), "Shouldn't be error creating LOB file %v", lobFile)
				currentChunkBytes = 0
			}

			outf.Write(bytes.Repeat([]byte{j}, 16))
			currentChunkBytes += 16
		}
	}
	if outf != nil {
		outf.Close()
	}

	return correctLOBInfo
}

// generate a random list of SHAs for testing purposes
// these SHAs are random and don't correspond to any valid data
func GetListOfRandomSHAsForTest(num int) []string {
	ret := make([]string, 0, num)
	sha := sha1.New()
	for n := 0; n < num; n++ {
		randStr := strconv.Itoa(rand.Int())
		sha.Write([]byte(randStr))
		shaStr := fmt.Sprintf("%x", string(sha.Sum(nil)))
		ret = append(ret, shaStr)
	}
	return ret
}

// Create a single initial commit (no LOB references) to give us a base
func CreateInitialCommitForTest(path string) string {
	testfile := "test.txt"
	testfilepath := filepath.Join(path, testfile)
	ioutil.WriteFile(testfilepath, []byte("This is a test"), 0666)
	cmd := exec.Command("git", "add", testfile)
	cmd.Run()
	cmd = exec.Command("git", "commit", "-a", "-m \"Initial commit\"")
	outp, err := cmd.CombinedOutput()
	if err != nil {
		Fail("Unable to create initial commit: " + string(outp))
	}
	cmd = exec.Command("git", "rev-parse", "HEAD")
	sha, err := cmd.Output()
	if err != nil {
		Fail("Unable to read initial commit SHA: " + err.Error())
	}
	return strings.TrimSpace(string(sha))

}

func CreateCommitReferencingLOBsForTest(path string, filenamesBySha map[string]string) {
	for sha, filename := range filenamesBySha {
		testfilepath := filepath.Join(path, filename)
		ioutil.WriteFile(testfilepath, []byte(fmt.Sprintf("git-lob: %v", sha)), 0666)
		cmd := exec.Command("git", "add", filename)
		cmd.Run()
	}
	cmd := exec.Command("git", "commit", "-a", "-m \"Test commit\"")
	outp, err := cmd.CombinedOutput()
	if err != nil {
		Fail("Unable to create commit: " + string(outp))
	}
}
