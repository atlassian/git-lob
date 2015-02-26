package main

import (
	"fmt"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
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
	extracommitfilenames := []string{
		"supplement1.dat",
		"supplement2.dat",
		filepath.Join("some", "folder", "supplement3.dat"),
	}
	var extracommitshas []string
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
		// Now for the second commit which we'll put in another branch
		err := exec.Command("git", "checkout", "-b", "branch2").Run()
		if err != nil {
			Fail("Error creating branch: " + err.Error())
		}
		extracommitshas = []string{}
		for _, file := range extracommitfilenames {
			err := os.MkdirAll(filepath.Dir(file), 0755)
			if err != nil {
				Fail(err.Error())
			}
			// Constant size
			sz := int64(512)
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

			// Record the SHA we had so we can simulate it being missing
			extracommitshas = append(extracommitshas, info.SHA)
		}
		// Just 1 commit in second branch
		err = exec.Command("git", "commit", "-m", "Second branch Commit").Run()
		if err != nil {
			Fail("Error in git commit: " + err.Error())
		}
		// Go back to master
		err = exec.Command("git", "checkout", "master").Run()
		if err != nil {
			Fail("Error returning to master branch: " + err.Error())
		}

	})
	AfterEach(func() {
		os.Chdir(oldwd)
		os.RemoveAll(root)
	})

	It("Checks out all missing data on master", func() {
		// In this case we just checkout everything
		var filesOK int
		var filesSkipped int
		var filesFailed int
		var filesNotFound int
		testCallback := func(t ProgressCallbackType, filelob *FileLOB, err error) {
			switch t {
			case ProgressTransferBytes:
				filesOK++
			case ProgressError:
				filesFailed++
			case ProgressSkip:
				filesSkipped++
			case ProgressNotFound:
				filesNotFound++
			}

		}
		// Dry run test
		err := Checkout(nil, true, testCallback)
		Expect(err).To(BeNil(), "Shouldn't fail calling checkout")
		Expect(filesOK).To(BeEquivalentTo(len(filenames)), "All files should need to be updated")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should have failed")
		Expect(filesNotFound).To(BeEquivalentTo(0), "No files should have been missing")
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
		filesNotFound = 0
		err = Checkout(nil, false, testCallback)
		Expect(err).To(BeNil(), "Shouldn't fail calling checkout")
		Expect(filesOK).To(BeEquivalentTo(len(filenames)), "All files should be updated")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should have failed")
		Expect(filesNotFound).To(BeEquivalentTo(0), "No files should have been missing")
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
		filesNotFound = 0
		err = Checkout(nil, false, testCallback)
		Expect(err).To(BeNil(), "Shouldn't fail calling 2nd checkout")
		Expect(filesOK).To(BeEquivalentTo(0), "No files should be updated")
		Expect(filesSkipped).To(BeEquivalentTo(len(filenames)), "All files should be skipped")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should have failed")
		Expect(filesNotFound).To(BeEquivalentTo(0), "No files should have been missing")
		for i, file := range filenames {
			// All should be correct size
			sz := sizeForFile(i)
			stat, err := os.Stat(file)
			Expect(err).To(BeNil(), fmt.Sprintf("File %v should exist", file))
			Expect(stat.Size()).To(BeEquivalentTo(sz), fmt.Sprintf("File %v should be checked out & correct size", file))
		}
		// We shouldn't have any files from another branch
		for _, file := range extracommitfilenames {
			// All should be correct size
			_, err := os.Stat(file)
			Expect(err).ToNot(BeNil(), fmt.Sprintf("File %v should not exist on this branch", file))
		}

	})
	It("Deals with missing stored files gracefully & recovers later", func() {
		// Use the second branch as a way to simulate files coming in & out of scope
		// Hard to test this fully without setting up a git filter which we can't do in a test because binary is not built
		// Filter may or may not exist
		// First checkout current branch stuff (don't check)
		Checkout(nil, false, func(t ProgressCallbackType, filelob *FileLOB, err error) {})
		// We shouldn't have any files from another branch to start with
		for _, file := range extracommitfilenames {
			// All should be correct size
			_, err := os.Stat(file)
			Expect(err).ToNot(BeNil(), fmt.Sprintf("File %v should not exist on this branch", file))
		}
		err := exec.Command("git", "checkout", "branch2").Run()
		if err != nil {
			Fail("Error switching to branch2: " + err.Error())
		}
		// Because we don't know how much a filter configured on this test machine might have already done,
		// let's manually change the contents of the files to the state they'd be in if the data was not
		// available
		for i, file := range extracommitfilenames {
			// Now manually overwrite the file with the SHA line, as if it hadn't been available when checked out
			sha := extracommitshas[i]
			err = ioutil.WriteFile(file, []byte(getLOBPlaceholderContent(sha)), 0644)
			if err != nil {
				Fail("Error writing placeholder: " + err.Error())
			}

			// Also, temporarily make the content unavailable by renaming it
			// We know there's only 1 chunk here
			f := filepath.Join(GetLocalLOBDir(sha), getLOBMetaFilename(sha))
			err = os.Rename(f, f+"_bak")
			if err != nil {
				Fail("Error moving data: " + err.Error())
			}
			f = filepath.Join(GetLocalLOBDir(sha), getLOBChunkFilename(sha, 0))
			err = os.Rename(f, f+"_bak")
			if err != nil {
				Fail("Error moving data: " + err.Error())
			}
		}
		// Now run checkout, this shouldn't find the data
		var filesOK int
		var filesSkipped int
		var filesFailed int
		var filesNotFound int
		testCallback := func(t ProgressCallbackType, filelob *FileLOB, err error) {
			switch t {
			case ProgressTransferBytes:
				filesOK++
			case ProgressError:
				filesFailed++
			case ProgressSkip:
				filesSkipped++
			case ProgressNotFound:
				filesNotFound++
			}

		}
		err = Checkout(nil, false, testCallback)
		Expect(err).To(BeNil(), "Shouldn't fail checking out 2nd branch")
		Expect(filesOK).To(BeEquivalentTo(0), "No files should be updated because data is missing")
		Expect(filesNotFound).To(BeEquivalentTo(len(extracommitfilenames)), "All files should be missing")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should have failed")

		// now put files back
		for i, _ := range extracommitfilenames {
			sha := extracommitshas[i]
			f := filepath.Join(GetLocalLOBDir(sha), getLOBMetaFilename(sha))
			err = os.Rename(f+"_bak", f)
			if err != nil {
				Fail("Error moving data: " + err.Error())
			}
			f = filepath.Join(GetLocalLOBDir(sha), getLOBChunkFilename(sha, 0))
			err = os.Rename(f+"_bak", f)
			if err != nil {
				Fail("Error moving data: " + err.Error())
			}
		}
		// Checkout should now work
		filesNotFound = 0
		err = Checkout(nil, false, testCallback)
		Expect(err).To(BeNil(), "Shouldn't fail checking out 2nd branch")
		Expect(filesOK).To(BeEquivalentTo(len(extracommitfilenames)), "All files should now be updated because data is available")
		Expect(filesNotFound).To(BeEquivalentTo(0), "No files should be missing")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should have failed")

	})
	It("Respects pathspecs", func() {
		var filesDone []string
		var filesSkipped int
		var filesFailed int
		testCallback := func(t ProgressCallbackType, filelob *FileLOB, err error) {
			switch t {
			case ProgressTransferBytes:
				filesDone = append(filesDone, filelob.Filename)
			case ProgressError:
				filesFailed++
			case ProgressSkip:
				filesSkipped++
			}

		}
		pathspecs := []string{
			filepath.Join("some", "folder", "nested"),
			filepath.Join("second", "folder", "*6.*"),
		}
		// note callbacks always in git style ie forward slashes even on Windows
		// But we still accept Windows style as parameters
		correctFiles := []string{
			"some/folder/nested/file3.dat",
			"some/folder/nested/file31.dat",
			"some/folder/nested/file32.dat",
			"second/folder/file6.dat",
		}
		err := Checkout(pathspecs, false, testCallback)
		Expect(err).To(BeNil(), "Shouldn't fail calling checkout with pathspecs")
		Expect(filesDone).To(ConsistOf(correctFiles), "Files updated should match path specs")
		Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped")
		Expect(filesFailed).To(BeEquivalentTo(0), "No files should have failed")

	})
	Describe("Changed working dir", func() {
		BeforeEach(func() {
			// Change to a subfolder
			os.Chdir(filepath.Join(root, "some", "folder"))
		})
		AfterEach(func() {
			os.Chdir(root)
		})
		It("Checks out with path specs relative to current dir", func() {
			var filesDone []string
			var filesSkipped int
			var filesFailed int
			testCallback := func(t ProgressCallbackType, filelob *FileLOB, err error) {
				switch t {
				case ProgressTransferBytes:
					filesDone = append(filesDone, filelob.Filename)
				case ProgressError:
					filesFailed++
				case ProgressSkip:
					filesSkipped++
				}

			}
			pathspecs := []string{
				"*211.dat",
				"nested",
			}
			correctFiles := []string{
				"some/folder/file211.dat",
				"some/folder/nested/file3.dat",
				"some/folder/nested/file31.dat",
				"some/folder/nested/file32.dat",
			}
			err := Checkout(pathspecs, false, testCallback)
			Expect(err).To(BeNil(), "Shouldn't fail calling checkout with pathspecs")
			Expect(filesDone).To(ConsistOf(correctFiles), "Files updated should match path specs")
			Expect(filesSkipped).To(BeEquivalentTo(0), "No files should be skipped")
			Expect(filesFailed).To(BeEquivalentTo(0), "No files should have failed")
		})
	})

})
