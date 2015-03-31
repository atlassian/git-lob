package core

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	. "bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
)

var _ = Describe("Fsck", func() {

	root := filepath.Join(os.TempDir(), "FsckTest")
	shared := filepath.Join(os.TempDir(), "FsckShared")
	var oldwd string
	var smallLOBs []string
	var largeLOBs []string

	BeforeEach(func() {
		oldwd, _ = os.Getwd()
		CreateGitRepoForTest(root)
		os.Chdir(root)

		// Store a number of small LOBs
		// filename doesn't matter, we just want to store the data
		for i := 0; i < 10; i++ {
			info := CreateAndStoreLOBFileForTest(int64(rand.Intn(300)+50), "anything.dat")
			smallLOBs = append(smallLOBs, info.SHA)
		}
		// Store a couple of multi-chunk LOBs
		filename := "anything.dat"
		CreateFastFileForTest(ChunkSize+500, filename)
		info, err := StoreLOBForTest(filename)
		if err != nil {
			Fail(err.Error())
		}
		largeLOBs = append(largeLOBs, info.SHA)

		CreateFastFileForTest(ChunkSize*2+200, filename)
		info, err = StoreLOBForTest(filename)
		if err != nil {
			Fail(err.Error())
		}
		largeLOBs = append(largeLOBs, info.SHA)

	})
	AfterEach(func() {
		os.Chdir(oldwd)
		err := ForceRemoveAll(root)
		if err != nil {
			Fail(err.Error())
		}
		err = ForceRemoveAll(shared)
		if err != nil {
			Fail(err.Error())
		}
		GlobalOptions = NewOptions()
	})

	// Do everything in one test so we don't incur the setup cost more than once (large files)
	It("Fscks correctly", func() {
		var corruptFiles []string
		var missingFiles []string
		var wrongSizeFiles []string
		callback := func(data *FsckCallbackData) bool {
			switch data.Type {
			case FsckMissing:
				missingFiles = append(missingFiles, data.SHA)
			case FsckCorruptData:
				corruptFiles = append(corruptFiles, data.SHA)
			case FsckWrongSize:
				wrongSizeFiles = append(wrongSizeFiles, data.SHA)
			}
			return false
		}
		// First check all is well (shallow)
		err := Fsck(false, false, false, nil, callback)
		Expect(err).To(BeNil(), "Shouldn't be an error calling Fsck (shallow)")
		Expect(corruptFiles).To(BeEmpty(), "Should be no corrupt files (shallow)")
		Expect(missingFiles).To(BeEmpty(), "Should be no missing files (shallow)")
		Expect(wrongSizeFiles).To(BeEmpty(), "Should be no wrong size files (shallow)")
		// check again (deep)
		err = Fsck(true, false, false, nil, callback)
		Expect(err).To(BeNil(), "Shouldn't be an error calling Fsck (deep)")
		Expect(corruptFiles).To(BeEmpty(), "Should be no corrupt files (deep)")
		Expect(missingFiles).To(BeEmpty(), "Should be no missing files (deep)")
		Expect(wrongSizeFiles).To(BeEmpty(), "Should be no wrong size files (deep)")

		// now let's start breaking things
		var backupFile string
		var fileToBreak string
		// Make meta file missing
		fileToBreak = GetLocalLOBMetaPath(smallLOBs[0])
		backupFile = fileToBreak + "_bak"
		os.Rename(fileToBreak, backupFile)
		err = Fsck(false, false, false, nil, callback)
		Expect(err).ToNot(BeNil(), "Should be an error calling Fsck (shallow)")
		Expect(corruptFiles).To(BeEmpty(), "Should be no corrupt files (shallow)")
		Expect(missingFiles).To(ConsistOf([]string{smallLOBs[0]}), "Detect missing file (shallow)")
		Expect(wrongSizeFiles).To(BeEmpty(), "Should be no wrong size files (shallow)")
		// restore, then break a chunk & test again
		os.Rename(backupFile, fileToBreak)
		missingFiles = nil
		fileToBreak = GetLocalLOBChunkPath(smallLOBs[0], 0)
		backupFile = fileToBreak + "_bak"
		os.Rename(fileToBreak, backupFile)
		err = Fsck(false, false, false, nil, callback)
		Expect(err).ToNot(BeNil(), "Should be an error calling Fsck (shallow)")
		Expect(corruptFiles).To(BeEmpty(), "Should be no corrupt files (shallow)")
		Expect(missingFiles).To(ConsistOf([]string{smallLOBs[0]}), "Detect missing file (shallow)")
		Expect(wrongSizeFiles).To(BeEmpty(), "Should be no wrong size files (shallow)")
		// now try with a secondary chunk (leave smallLOBs[0] broken to test multiples)
		missingFiles = nil
		fileToBreak = GetLocalLOBChunkPath(largeLOBs[1], 1)
		backupFile = fileToBreak + "_bak"
		os.Rename(fileToBreak, backupFile)
		err = Fsck(false, false, false, nil, callback)
		Expect(err).ToNot(BeNil(), "Should be an error calling Fsck (shallow)")
		Expect(corruptFiles).To(BeEmpty(), "Should be no corrupt files (shallow)")
		Expect(missingFiles).To(ConsistOf([]string{smallLOBs[0], largeLOBs[1]}), "Detect missing file (shallow)")
		Expect(wrongSizeFiles).To(BeEmpty(), "Should be no wrong size files (shallow)")
		// restore large
		os.Rename(backupFile, fileToBreak)
		missingFiles = nil

		// Now test corruption of smallLOB[1] metadata
		// smallLOBs[0] still has missing data
		fileToBreak = GetLocalLOBMetaPath(smallLOBs[1])
		backupFile = fileToBreak + "_bak"
		os.Rename(fileToBreak, backupFile)
		ioutil.WriteFile(fileToBreak, []byte("{ Broken }"), 0644)
		err = Fsck(false, false, false, nil, callback)
		Expect(err).ToNot(BeNil(), "Should be an error calling Fsck (shallow)")
		Expect(corruptFiles).To(ConsistOf([]string{smallLOBs[1]}), "Detect corrupt file (shallow)")
		Expect(missingFiles).To(ConsistOf([]string{smallLOBs[0]}), "Detect missing file (shallow)")
		Expect(wrongSizeFiles).To(BeEmpty(), "Should be no wrong size files (shallow)")
		// restore
		os.Remove(fileToBreak)
		os.Rename(backupFile, fileToBreak)
		missingFiles = nil
		corruptFiles = nil

		// Now test 'wrong size' detection
		fileToBreak = GetLocalLOBChunkPath(smallLOBs[2], 0)
		fileToBreak2 := GetLocalLOBChunkPath(largeLOBs[0], 1)
		backupFile = fileToBreak + "_bak"
		backupFile2 := fileToBreak2 + "_bak"
		os.Rename(fileToBreak, backupFile)
		os.Rename(fileToBreak2, backupFile2)
		ioutil.WriteFile(fileToBreak, []byte{0, 1, 2, 3, 4}, 0644)
		ioutil.WriteFile(fileToBreak2, []byte{0, 1, 2, 3, 4}, 0644)
		err = Fsck(false, false, false, nil, callback)
		Expect(err).ToNot(BeNil(), "Should be an error calling Fsck (shallow)")
		Expect(wrongSizeFiles).To(ConsistOf([]string{smallLOBs[2], largeLOBs[0]}), "Detect wrong size file (shallow)")
		Expect(missingFiles).To(ConsistOf([]string{smallLOBs[0]}), "Detect missing file (shallow)")
		Expect(corruptFiles).To(BeEmpty(), "Should be no corrupt files (shallow)")
		// restore
		os.Remove(fileToBreak)
		os.Rename(backupFile, fileToBreak)
		os.Remove(fileToBreak2)
		os.Rename(backupFile2, fileToBreak2)
		missingFiles = nil
		wrongSizeFiles = nil

		// Now break the data by keeping it the right size but overwrite a few bytes with different data
		fileToBreak = GetLocalLOBChunkPath(smallLOBs[2], 0)
		fileToBreak2 = GetLocalLOBChunkPath(largeLOBs[0], 1)
		f, _ := os.OpenFile(fileToBreak, os.O_RDWR, 0644)
		f.Write([]byte{5, 4, 3, 2, 1})
		f.Close()
		f, _ = os.OpenFile(fileToBreak2, os.O_RDWR, 0644)
		f.Write([]byte{5, 4, 3, 2, 1})
		f.Close()
		// Prove that a shallow test won't pick up this change
		err = Fsck(false, false, false, nil, callback)
		Expect(err).ToNot(BeNil(), "Should be an error calling Fsck (shallow)")
		Expect(corruptFiles).To(BeEmpty(), "Corrupt files should not be detected by shallow test")
		Expect(missingFiles).To(ConsistOf([]string{smallLOBs[0]}), "Detect missing file (shallow)")
		Expect(wrongSizeFiles).To(BeEmpty(), "Should be no wrong size files (shallow)")
		missingFiles = nil
		// Now show a deep test will find it
		err = Fsck(true, false, false, nil, callback)
		Expect(err).ToNot(BeNil(), "Should be an error calling Fsck (deep)")
		Expect(corruptFiles).To(ConsistOf([]string{smallLOBs[2], largeLOBs[0]}), "Deep fsck should detect corrupt file")
		Expect(missingFiles).To(ConsistOf([]string{smallLOBs[0]}), "Detect missing file (deep)")
		Expect(wrongSizeFiles).To(BeEmpty(), "Should be no wrong size files (deep)")
		missingFiles = nil
		corruptFiles = nil

		// Check specific check for SHAs works (check only for small LOBs)
		err = Fsck(true, false, false, smallLOBs, callback)
		Expect(err).ToNot(BeNil(), "Should be an error calling Fsck (deep)")
		Expect(corruptFiles).To(ConsistOf([]string{smallLOBs[2]}), "Deep fsck but only for small LOBs")
		Expect(missingFiles).To(ConsistOf([]string{smallLOBs[0]}), "Detect missing file (deep)")
		Expect(wrongSizeFiles).To(BeEmpty(), "Should be no wrong size files (deep)")
		missingFiles = nil
		corruptFiles = nil

		// Now test deletion
		// Make a chunk the wrong size to test only that chunk gets deleted
		// also smallLOBs[2], largeLOBs[0] still corrupt from previous
		fileToBreak = GetLocalLOBChunkPath(largeLOBs[1], 2)
		ioutil.WriteFile(fileToBreak, []byte{0, 1, 2, 3, 4, 5}, 0644)
		err = Fsck(true, false, true, nil, callback)
		Expect(err).ToNot(BeNil(), "Should be an error calling Fsck (deep)")
		Expect(corruptFiles).To(ConsistOf([]string{smallLOBs[2], largeLOBs[0]}), "Deep fsck should detect corrupt file")
		Expect(missingFiles).To(ConsistOf([]string{smallLOBs[0]}), "Detect missing file (deep)")
		Expect(wrongSizeFiles).To(ConsistOf([]string{largeLOBs[1]}), "Detect a wrong size file (deep)")
		// Check deletion
		for i, s := range smallLOBs {
			// we already broke smallLOBs[0]
			if i == 0 {
				continue
			}
			if i == 2 {
				Expect(FileExists(GetLocalLOBMetaPath(s))).To(BeFalse(), "Corrupt file should be deleted")
				Expect(FileExists(GetLocalLOBChunkPath(s, 0))).To(BeFalse(), "Corrupt file should be deleted")
			} else {
				Expect(FileExists(GetLocalLOBMetaPath(s))).To(BeTrue(), fmt.Sprintf("Small meta file %d should still exist", i))
				Expect(FileExists(GetLocalLOBChunkPath(s, 0))).To(BeTrue(), fmt.Sprintf("Small chunk file %d should still exist", i))
			}
		}
		Expect(FileExists(GetLocalLOBChunkPath(largeLOBs[1], 2))).To(BeFalse(), "Wrong size file should be deleted")
		Expect(FileExists(GetLocalLOBMetaPath(largeLOBs[1]))).To(BeTrue(), "Metadata of wrong size chunk file should still exist")
		Expect(FileExists(GetLocalLOBChunkPath(largeLOBs[1], 0))).To(BeTrue(), "Earlier chunks of wrong size file should still exist")
		Expect(FileExists(GetLocalLOBChunkPath(largeLOBs[1], 1))).To(BeTrue(), "Earlier chunks of wrong size file should still exist")
		Expect(FileExists(GetLocalLOBMetaPath(largeLOBs[0]))).To(BeFalse(), "Corrupt file should be deleted")
		Expect(FileExists(GetLocalLOBChunkPath(largeLOBs[0], 0))).To(BeFalse(), "Corrupt file should be deleted")
		Expect(FileExists(GetLocalLOBChunkPath(largeLOBs[0], 1))).To(BeFalse(), "Corrupt file should be deleted")
		missingFiles = nil
		corruptFiles = nil
		wrongSizeFiles = nil

		// test shared
		GlobalOptions.SharedStore = shared
		err = os.Rename(GetLocalLOBRoot(), shared)
		Expect(err).To(BeNil(), fmt.Sprintf("Should be no error renaming to shared"))
		err = Fsck(false, true, false, nil, callback)
		Expect(err).ToNot(BeNil(), "Should be an error calling Fsck (shared)")
		Expect(corruptFiles).To(BeEmpty(), "No corruptions (they were deleted")
		Expect(missingFiles).To(ConsistOf([]string{smallLOBs[0], largeLOBs[1]}), "Detect missing file, including deleted wrong size files (shared)")
		Expect(wrongSizeFiles).To(BeEmpty(), "Should be no wrong size files (shared)")
		missingFiles = nil

	})

})
