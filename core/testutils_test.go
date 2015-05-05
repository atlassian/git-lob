package core

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"bitbucket.org/sinbad/git-lob/util"
	"bufio"
	"bytes"
	cryptorand "crypto/rand"
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Utility methods for testing only
func CreateGitRepoForTest(path string) {
	cmd := exec.Command("git", "init", path)
	err := cmd.Run()
	if err != nil {
		Fail("Unable to create git repo at " + path + ": " + err.Error())
	}
}
func CreateBareGitRepoForTest(path string) {
	cmd := exec.Command("git", "init", "--bare", path)
	err := cmd.Run()
	if err != nil {
		Fail("Unable to create git repo at " + path + ": " + err.Error())
	}
}

func CreateGitRepoWithSeparateGitDirForTest(path string, gitDir string) {
	cmd := exec.Command("git", "init", "--separate-git-dir", gitDir, path)
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

// Create a small LOB file  ready for storing in the LOB area
// filename should be a subfolder of a test git repo
// SHA will have been calculated outside the software so can be validated
func CreateSmallTestLOBFileForStoring(filename string) (correctInfo *LOBInfo) {
	// This was calculated with 'shasum' on Mac OS X with this file content
	correctLOBInfo := &LOBInfo{SHA: "772157c6ef480852edf921f5924b1ca582b0d78f",
		NumChunks: 1, Size: 128 * 255 * 16}

	// Create binary file
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0755)
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
	correctLOBInfo := &LOBInfo{SHA: "6dc61e7c7d33e87592da1e534063052a17bf8f3c",
		NumChunks: 4, Size: 25000 * 255 * 16}

	// Create binary file
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0755)
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
	correctLOBInfo := &LOBInfo{SHA: "772157c6ef480852edf921f5924b1ca582b0d78f",
		NumChunks: 1, Size: 128 * 255 * 16}
	err := StoreLOBInfo(correctLOBInfo)
	Expect(err).To(BeNil(), "Shouldn't be error creating LOB meta file")

	var lobFile string
	if IsUsingSharedStorage() {
		lobFile = GetSharedLOBChunkPath(correctLOBInfo.SHA, 0)
	} else {
		lobFile = GetLocalLOBChunkPath(correctLOBInfo.SHA, 0)
	}
	f, err := os.OpenFile(lobFile, os.O_WRONLY|os.O_CREATE, 0644)
	Expect(err).To(BeNil(), "Shouldn't be error creating LOB file %v", lobFile)
	// Write test data
	for i := 0; i < 128; i++ {
		var j byte
		for j = 0; j < 255; j++ {
			f.Write(bytes.Repeat([]byte{j}, 16))
		}
	}
	f.Close()
	if IsUsingSharedStorage() {
		link := GetLocalLOBChunkPath(correctLOBInfo.SHA, 0)
		CreateHardLink(lobFile, link)
	}
	return correctLOBInfo
}

// Manually insert large multi-chunk LOB file into the LOB store ready for retrieval
func CreateLargeTestLOBDataForRetrieval() (correctInfo *LOBInfo) {
	// This was calculated with 'shasum' on Mac OS X with this file content
	correctFileSize := int64(25000 * 255 * 16)
	correctNumChunks := 4
	correctLOBInfo := &LOBInfo{SHA: "6dc61e7c7d33e87592da1e534063052a17bf8f3c",
		NumChunks: correctNumChunks, Size: correctFileSize}

	err := StoreLOBInfo(correctLOBInfo)
	Expect(err).To(BeNil(), "Shouldn't be error creating LOB meta file")

	// Write test data into 4 chunks
	var outf *os.File
	var currentChunkBytes int64
	var chunkIdx int

	for i := 0; i < 25000; i++ {
		var j byte
		for j = 0; j < 255; j++ {
			// We've specifically picked it so that this will exactly hit the end of a chunk
			if outf == nil || currentChunkBytes == ChunkSize {
				if outf != nil {
					dest := outf.Name()
					outf.Close()
					if IsUsingSharedStorage() {
						link := GetLocalLOBChunkPath(correctLOBInfo.SHA, chunkIdx-1)
						CreateHardLink(dest, link)
					}
				}
				var lobFile string
				if IsUsingSharedStorage() {
					lobFile = GetSharedLOBChunkPath(correctLOBInfo.SHA, chunkIdx)
				} else {
					lobFile = GetLocalLOBChunkPath(correctLOBInfo.SHA, chunkIdx)
				}
				chunkIdx++
				outf, err = os.OpenFile(lobFile, os.O_WRONLY|os.O_CREATE, 0644)
				Expect(err).To(BeNil(), "Shouldn't be error creating LOB file %v", lobFile)
				currentChunkBytes = 0
			}

			outf.Write(bytes.Repeat([]byte{j}, 16))
			currentChunkBytes += 16
		}
	}
	if outf != nil {
		dest := outf.Name()
		outf.Close()
		if IsUsingSharedStorage() {
			link := GetLocalLOBChunkPath(correctLOBInfo.SHA, chunkIdx-1)
			CreateHardLink(dest, link)
		}
	}

	return correctLOBInfo
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

// Create a file with less random data of size sz (faster than CreateRandomFileForTest)
// Data is still random but written in repeating blocks
func CreateFastFileForTest(sz int64, filename string) {
	// Use fully random method if size is too small
	if sz < 16*1024*5 {
		CreateRandomFileForTest(sz, filename)
		return
	}
	os.MkdirAll(filepath.Dir(filename), 0755)
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		Fail(fmt.Sprintf("Can't create test file %v: %v", filename, err))
	}
	defer f.Close()

	szLeft := sz
	// write in blocks of 16k
	blockSize := 16 * 1024
	blocks := [][]byte{
		bytes.Repeat([]byte{byte(rand.Intn(255))}, blockSize),
		bytes.Repeat([]byte{byte(rand.Intn(255))}, blockSize),
		bytes.Repeat([]byte{byte(rand.Intn(255))}, blockSize),
		bytes.Repeat([]byte{byte(rand.Intn(255))}, blockSize),
	}
	block := 0
	for szLeft > 0 {
		var data []byte
		if szLeft < int64(blockSize) {
			data = blocks[block][:szLeft]
		} else {
			data = blocks[block]
		}
		_, err := f.Write(data)
		if err != nil {
			Fail(fmt.Sprintf("Can't write data to test file %v: %v", filename, err))
		}
		szLeft -= int64(len(data))
		block = (block + 1) % len(blocks)
	}

}

// Store a random file LOB, then overwrite it with a placeholder ready for commit (without filters)
func CreateAndStoreLOBFileForTest(sz int64, filename string) *LOBInfo {
	CreateFastFileForTest(sz, filename)
	info, err := StoreLOBForTest(filename)
	if err != nil {
		Fail(fmt.Sprintf("Failed to store test LOB %v: %v", filename, err))
	}
	// now overwrite with placeholder ready for adding to git
	err = ioutil.WriteFile(filename,
		[]byte(fmt.Sprintf("git-lob: %v", info.SHA)), 0644)
	if err != nil {
		Fail(fmt.Sprintf("Failed to wite placeholder for %v: %v", filename, err))
	}
	return info
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
	ioutil.WriteFile(testfilepath, []byte("This is a test"), 0644)
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
		ioutil.WriteFile(testfilepath, []byte(fmt.Sprintf("git-lob: %v", sha)), 0644)
		cmd := exec.Command("git", "add", filename)
		cmd.Run()
	}
	cmd := exec.Command("git", "commit", "-a", "-m \"Test commit\"")
	outp, err := cmd.CombinedOutput()
	if err != nil {
		Fail("Unable to create commit: " + string(outp))
	}
}

func CreateBranchForTest(branch string) {
	cmd := exec.Command("git", "branch", branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		Fail("git branch error: " + string(out))
	}
}
func CheckoutForTest(ref string) {
	cmd := exec.Command("git", "checkout", "-f", ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		Fail("git checkout error: " + string(out))
	}
}

// Wrapper function to add a file to the LOB database (no git interaction)
// filename relative path of file to current dir
func StoreLOBForTest(filename string) (*LOBInfo, error) {
	f, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return StoreLOB(bufio.NewReader(f), []byte(""))
}

func CreateManyCommitsForTest(filespercommit [][]string, commitOffset int, sizeFunc func(filename string, index int) int64) (shaspercommit [][]string) {
	var ret [][]string
	for ci, commitfiles := range filespercommit {
		var shas []string
		for i, file := range commitfiles {
			err := os.MkdirAll(filepath.Dir(file), 0755)
			Expect(err).To(BeNil(), "Shouldn't fail creating dir")
			sz := sizeFunc(file, i)
			// Create real content
			CreateRandomFileForTest(sz, file)
			info, err := StoreLOBForTest(file)
			Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail to store LOB for %v", file))

			// Now overwrite with placeholder & add (what filter would have done)
			ioutil.WriteFile(file, []byte(fmt.Sprintf("git-lob: %v", info.SHA)), 0644)

			err = exec.Command("git", "add", file).Run()
			Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail in git add for %v", file))

			shas = append(shas, info.SHA)
		}

		// Commit & tag
		err := exec.Command("git", "commit", "-m", fmt.Sprintf("Commit %d", ci+commitOffset)).Run()
		Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail commit %d", ci+commitOffset))
		err = exec.Command("git", "tag", fmt.Sprintf("Tag%d", ci+commitOffset)).Run()
		Expect(err).To(BeNil(), fmt.Sprintf("Shouldn't fail tagging %d", ci+commitOffset))

		ret = append(ret, shas)
	}

	return ret

}

// Checks that a meta file and at least one chunk exists for the given shas
func CheckLOBsExistForTest(shas []string, rootlobpath string) {
	for _, sha := range shas {
		meta := filepath.Join(rootlobpath, GetLOBMetaRelativePath(sha))
		_, err := os.Stat(meta)
		Expect(err).To(BeNil(), "Meta file should exist")
		// Checking only one chunk for this test
		chunk := filepath.Join(rootlobpath, GetLOBChunkRelativePath(sha, 0))
		_, err = os.Stat(chunk)
		Expect(err).To(BeNil(), "Chunk file should exist")
	}

}
func RemoveLOBsForTest(shas []string, rootlobpath string) {
	for _, sha := range shas {
		meta := filepath.Join(rootlobpath, GetLOBMetaRelativePath(sha))
		os.Remove(meta)
		chunk := filepath.Join(rootlobpath, GetLOBChunkRelativePath(sha, 0))
		os.Remove(chunk)
	}

}

// Input struct for defining commits for test setup
type TestCommitSetupInput struct {
	// Date that we should commit on (optional, leave blank for 'now')
	CommitDate time.Time
	// List of files to have LOB contents inserted at this commit
	Files []string
	// Optional list of sizes for the above files; if not specified defaults to 100 bytes
	FileSizes []int64
	// List of parent branches (all branches must have been created in a previous)
	// Can be omitted to just use the parent of the previous commit
	ParentBranches []string
	// Name of a new branch we should create at this commit (optional - master not required)
	NewBranch string
	// Name of committer
	CommitterName string
	// Email of committer
	CommitterEmail string
}

func CommitAtDateForTest(t time.Time, committerName, committerEmail, msg string) error {
	var args []string
	if committerName != "" && committerEmail != "" {
		args = append(args, "-c", fmt.Sprintf("user.name=%v", committerName))
		args = append(args, "-c", fmt.Sprintf("user.email=%v", committerEmail))
	}
	args = append(args, "commit", "--allow-empty", "-m", msg)
	cmd := exec.Command("git", args...)
	env := os.Environ()
	// set GIT_COMMITTER_DATE environment var e.g. "Fri Jun 21 20:26:41 2013 +0900"
	if t.IsZero() {
		env = append(env, "GIT_COMMITTER_DATE=")
	} else {
		env = append(env, fmt.Sprintf("GIT_COMMITTER_DATE=%v", FormatGitDate(t)))
	}
	cmd.Env = env
	return cmd.Run()
}

func SetupRepoForTest(inputs []*TestCommitSetupInput) []*CommitLOBRef {
	// Used to check whether we need to checkout another commit before
	lastBranch := "master"
	outputs := make([]*CommitLOBRef, 0, len(inputs))

	for i, input := range inputs {
		output := &CommitLOBRef{}
		// first, are we on the correct branch
		if len(input.ParentBranches) > 0 {
			if input.ParentBranches[0] != lastBranch {
				RunGitCommandForTest(true, "checkout", input.ParentBranches[0])
				lastBranch = input.ParentBranches[0]
			}
		}
		// Is this a merge?
		if len(input.ParentBranches) > 1 {
			// Always take the *other* side in a merge so we adopt changes
			// also don't automatically commit, we'll do that below
			args := []string{"merge", "--no-ff", "--no-commit", "--strategy-option=theirs"}
			args = append(args, input.ParentBranches[1:]...)
			RunGitCommandForTest(false, args...)
		} else if input.NewBranch != "" {
			RunGitCommandForTest(true, "checkout", "-b", input.NewBranch)
			lastBranch = input.NewBranch
		}
		// Any files to write?
		for fi, filename := range input.Files {
			sz := int64(100)
			if len(input.FileSizes) > fi {
				sz = input.FileSizes[fi]
			}
			info := CreateAndStoreLOBFileForTest(sz, filename)
			output.LobSHAs = append(output.LobSHAs, info.SHA)
			output.FileLOBs = append(output.FileLOBs, &FileLOB{Filename: filename, SHA: info.SHA})
			RunGitCommandForTest(true, "add", filename)
		}
		// Now commit
		CommitAtDateForTest(input.CommitDate, input.CommitterName, input.CommitterEmail,
			fmt.Sprintf("Test commit %d", i))
		commit, err := GetGitCommitSummary("HEAD")
		if err != nil {
			Fail("Error determining commit SHA: " + err.Error())
		}
		output.Commit = commit.SHA
		output.Parents = commit.Parents
		outputs = append(outputs, output)
	}
	return outputs
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
			cmd.Run()
			// cannot trust exit code?
			return nil
		}
	}

	return err
}

// Delete a file, overriding read-only flags
// BE VERY CAREFUL WITH THIS
func ForceRemove(path string) error {
	err := os.Remove(path)
	if err != nil && runtime.GOOS == "windows" {
		// Windows totally sucks. Sometimes it decides that another process has locked a file, even though it hasn't
		// Tests will randomly flip between working and not working because of this
		// A delay & retry often works.
		retryCount := 0
		for err != nil && retryCount < 3 {
			time.Sleep(time.Second)
			err = os.Remove(path)
			retryCount++
		}
		if err != nil {
			// Last attempt, try a force delete
			if util.FileExists(path) {
				// 'del' isn't an executable, it's a builtin of cmd
				cmd := exec.Command("cmd", "/C", "del", "/F", "/Q", path)
				var out []byte
				out, err = cmd.CombinedOutput()
				// del return code is not reliable, check for output
				txtOut := strings.TrimSpace(string(out))
				if txtOut != "" {
					// Must warn tests that Windows is being a dick
					// This seems to happen *sometimes* when files are copied from temp to final LOB store location
					// you can't delete them immediately for some reason (ForceRemoveAll seems to work later)
					return fmt.Errorf("Can't delete file %v. This will break the test, but Windows does it randomly. You can probably ignore this, re-test with -focus", path)
				}
			}
		}
	}

	return err

}
