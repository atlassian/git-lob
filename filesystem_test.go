package main

import (
	"bytes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"math/rand"
	"os"
	"path/filepath"
)

var _ = Describe("Filesystem", func() {

	localpath := filepath.Join(os.TempDir(), "MockFileSystemLocal")
	localfiles := GetRandomListOfFilesForTest(3, 3, 2)
	//remotefiles := GetRandomListOfFilesForTest(5, 3, 2)

	Context("Mocked remote tests", func() {
		// These tests don't actually use real remote mapped drives because that
		// requires more environmental setup. We just use a local drive
		mockremotepath := filepath.Join(os.TempDir(), "MockFileSystemRemote")

		BeforeEach(func() {
			os.MkdirAll(mockremotepath, 0755)
			// Manually configure globals
			GlobalOptions.GitConfig["remote.origin.git-lob-path"] = mockremotepath
		})
		AfterEach(func() {
			os.RemoveAll(mockremotepath)
		})

		It("successfully uploads", func() {

			// Create local files
			for _, file := range localfiles {
				// generate of random size
				sz := rand.Intn(10000)
				fullpath := filepath.Join(localpath, file)
				os.MkdirAll(filepath.Dir(fullpath), 0755)
				f, err := os.OpenFile(fullpath, os.O_CREATE|os.O_TRUNC, 0644)
				if err != nil {
					Fail(err.Error())
				}
				f.Write(bytes.Repeat([]byte{255}, sz))
				f.Close()
			}

			fsync := FileSystemSyncProvider{}
			// Record of callbacks for files
			var filesUploaded []string = make([]string, 0, len(localfiles))
			var filesSkipped []string
			callback := func(filename string, isSkipped bool, percent int) (abort bool) {
				if percent == 100 {
					if isSkipped {
						filesSkipped = append(filesSkipped, filename)
					} else {
						filesUploaded = append(filesUploaded, filename)
					}
				}
				return false
			}
			err := fsync.Upload("origin", localfiles, localpath, false, callback)
			Expect(err).To(BeNil(), "Should not have error uploading")
			Expect(filesUploaded).To(Equal(localfiles), "Callback should have seen all the files at 100%")
			Expect(filesSkipped).To(BeEmpty(), "No files should be skipped")
			// Check files exist & are correct size
			for _, file := range localfiles {
				fulllocalpath := filepath.Join(localpath, file)
				fullremotepath := filepath.Join(mockremotepath, file)

				remotestat, err := os.Stat(fullremotepath)
				Expect(err).To(BeNil(), "Remote file should exist")
				localstat, err := os.Stat(fulllocalpath)
				Expect(err).To(BeNil(), "Local file should exist")
				Expect(remotestat.Size()).To(Equal(localstat.Size()), "Remote file should be the same size as local")
			}

			// Now check nothing is uploaded when we repeat without force
			filesUploaded = nil
			err = fsync.Upload("origin", localfiles, localpath, false, callback)
			Expect(err).To(BeNil(), "Should not have error uploading")
			Expect(filesUploaded).To(BeEmpty(), "No files should be uploaded a second time")
			Expect(filesSkipped).To(Equal(localfiles), "All files should have been skipped")

			// Now check that with force we do it over again
			filesUploaded = make([]string, 0, len(localfiles))
			filesSkipped = nil
			err = fsync.Upload("origin", localfiles, localpath, true, callback)
			Expect(err).To(BeNil(), "Should not have error uploading")
			Expect(filesUploaded).To(Equal(localfiles), "Files should be overwritten in force mode")
			Expect(filesSkipped).To(BeEmpty(), "No files should be skipped in force mode")

			// Now corrupt a few files on the remote and make sure it's detected
			filesToCorrupt := []string{localfiles[1], localfiles[4], localfiles[7], localfiles[15]}
			for _, file := range filesToCorrupt {
				fullremotepath := filepath.Join(mockremotepath, file)
				stat, _ := os.Stat(fullremotepath)
				f, err := os.OpenFile(fullremotepath, os.O_TRUNC|os.O_WRONLY, 0644)
				Expect(err).To(BeNil(), "Should not be error corrupting file")
				if stat.Size() < 2 {
					f.Write(bytes.Repeat([]byte{0}, 5))
				} else {
					f.Write(bytes.Repeat([]byte{127}, int(stat.Size()/2)))
				}
				f.Close()
			}
			filesUploaded = make([]string, 0, len(filesToCorrupt))
			filesSkipped = make([]string, 0, len(localfiles))
			err = fsync.Upload("origin", localfiles, localpath, false, callback)
			Expect(err).To(BeNil(), "Should not have error uploading")
			Expect(filesUploaded).To(Equal(filesToCorrupt), "Corrupt files should have been updated")
			Expect(filesSkipped).To(HaveLen(len(localfiles)-len(filesToCorrupt)), "Non-corrupt files should be skipped")

		})

	})

	/*
		Context("Real remote tests [REMOTETEST]", func() {
			// To do real tests with real remote paths, define environment vars:
			// Contents of these directories WILL BE DELETED so use with caution
			// GITLOB_TEST_REMOTE_SMBPATH - Samba
			// GITLOB_TEST_REMOTE_NFSPATH - NFS
			// GITLOB_TEST_REMOTE_AFPPATH - AFP

			It("Uploads to as many services as configured", func() {
				// Run one set of tests for each protocol as supported by env
				// Has to use env because it's not a real test if we're not testing with a genuine remote
				envs := []string{
					"GITLOB_TEST_REMOTE_SMBPATH",
					"GITLOB_TEST_REMOTE_NFSPATH",
					"GITLOB_TEST_REMOTE_AFPPATH"}

			})
			smbpath := os.Getenv("GITLOB_TEST_REMOTE_SMBPATH")
			if smbpath == "" {
				Fail("Can't run SMB remote tests, define env var GITLOB_TEST_REMOTE_SMBPATH to run.")
			}

		})
	*/
})
