package main

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

var _ = Describe("Checkout", func() {
	root := filepath.Join(os.TempDir(), "CheckoutTest")
	var oldwd string
	filenames := []string{
		"file1.dat",
		"file111.dat",
		"file112.dat",
		filepath.Join("some", "folder", "file2.dat"),
		filepath.Join("some", "folder", "file211.dat"),
		filepath.Join("some", "folder", "file212.dat"),
		filepath.Join("some", "folder", "nested", "file3.dat"),
		filepath.Join("some", "folder", "nested", "file31.dat"),
		filepath.Join("some", "folder", "nested", "file32.dat"),
		filepath.Join("second", "folder", "file4.dat"),
		filepath.Join("second", "folder", "file5.dat"),
		filepath.Join("second", "folder", "file6.dat"),
		filepath.Join("second", "folder", "file7.dat"),
		filepath.Join("spaced folder", "file8.dat"),
		filepath.Join("really", "really", "really", "really", "really", "really", "really", "really", "exceptionally", "very", "wow", "so", "much", "long", "folder", "withreallylongfilenameinit.barf"),
	}
	sizeForFile := func(i int) int64 {
		// Make a few files content exactly the same size as SHALineLen to test content check
		if i == 0 || i == 7 || i == 9 {
			return int64(SHALineLen)
		} else {
			return 500
		}
	}
	BeforeEach(func() {
		CreateGitRepoForTest(root)
		oldwd, _ = os.Getwd()
		os.Chdir(root)

		// In our test we have to actually create valid git commits referencing the data since
		// that's where checkout starts from
		// To avoid having to rely on clean filter setup when adding files, we'll manually store
		// the LOBs in the binary store then link them in files we add
		for i, file := range filenames {
			err := os.MkdirAll(filepath.Dir(file), 0755)
			if err != nil {
				Fail(err.Error())
			}
			sz := sizeForFile(i)
			CreateRandomFileForTest(sz, file)
			info, err := StoreLOBForTest(file)
			if err != nil {
				Fail("Error storing LOB: " + err.Error())
			}
			// Now manually overwrite the file with the SHA line, as if it hadn't been available when checked out
			err = ioutil.WriteFile(file, []byte(getLOBPlaceholderContent(info.SHA)), 0644)
			if err != nil {
				Fail("Error writing placeholder: " + err.Error())
			}
			// Need to commit the file (with placeholder)
			// If filter is enabled it should leave it alone anyway, but will also work if no filter is set up
			err = exec.Command("git", "add", file).Run()
			if err != nil {
				Fail("Error in git add: " + err.Error())
			}
			err = exec.Command("git", "commit", "-m", fmt.Sprintf("Commit %d", i)).Run()
			if err != nil {
				Fail("Error in git commit: " + err.Error())
			}

		}

	})
	AfterEach(func() {
		os.Chdir(oldwd)
		os.RemoveAll(root)
	})

	It("Checks out all missing data", func() {
		// In this case we just checkout everything
		var filesOK int
		var filesSkipped int
		var filesFailed int
		testCallback := func(t ProgressCallbackType, filelob *FileLOB, err error) {
			switch t {
			case ProgressTransferBytes:
				filesOK++
			case ProgressError:
				filesFailed++
			case ProgressSkip:
				filesSkipped++
			}

		}
		// Dry run test
		err := Checkout(nil, true, testCallback)
		Expect(err).To(BeNil(), "Shouldn't fail calling checkout")
		Expect(filesOK).To(BeEquivalentTo(len(filenames)), "All files should need to be updated")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should have failed")
		for _, file := range filenames {
			// All should be unchanged, still placeholders
			stat, err := os.Stat(file)
			Expect(err).To(BeNil(), fmt.Sprintf("File %v should still exist", file))
			Expect(stat.Size()).To(BeEquivalentTo(SHALineLen), fmt.Sprintf("File %v should be unchanged", file))
		}
		// Now the real call
		filesOK = 0
		filesSkipped = 0
		filesFailed = 0
		err = Checkout(nil, false, testCallback)
		Expect(err).To(BeNil(), "Shouldn't fail calling checkout")
		Expect(filesOK).To(BeEquivalentTo(len(filenames)), "All files should be updated")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should have failed")
		for i, file := range filenames {
			// All should be correct size
			sz := sizeForFile(i)
			stat, err := os.Stat(file)
			Expect(err).To(BeNil(), fmt.Sprintf("File %v should exist", file))
			Expect(stat.Size()).To(BeEquivalentTo(sz), fmt.Sprintf("File %v should be checked out & correct size", file))
		}
		// Second call should do nothing
		filesOK = 0
		filesSkipped = 0
		filesFailed = 0
		err = Checkout(nil, false, testCallback)
		Expect(err).To(BeNil(), "Shouldn't fail calling 2nd checkout")
		Expect(filesOK).To(BeEquivalentTo(0), "No files should be updated")
		Expect(filesSkipped).To(BeEquivalentTo(len(filenames)), "All files should be skipped")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should have failed")
		for i, file := range filenames {
			// All should be correct size
			sz := sizeForFile(i)
			stat, err := os.Stat(file)
			Expect(err).To(BeNil(), fmt.Sprintf("File %v should exist", file))
			Expect(stat.Size()).To(BeEquivalentTo(sz), fmt.Sprintf("File %v should be checked out & correct size", file))
		}

	})
	It("Respects pathspecs", func() {
		// TODO

	})

})
