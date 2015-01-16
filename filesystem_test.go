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
	testfiles := GetRandomListOfFilesForTest(3, 3, 2)
	//remotefiles := GetRandomListOfFilesForTest(5, 3, 2)

	// Create utility functions as vars to avoid polluting function space
	testCreateFiles := func(basedir string) {
		for _, file := range testfiles {
			// generate of random size
			sz := rand.Intn(10000)
			fullpath := filepath.Join(basedir, file)
			os.MkdirAll(filepath.Dir(fullpath), 0755)
			f, err := os.OpenFile(fullpath, os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				Fail(err.Error())
			}
			f.Write(bytes.Repeat([]byte{255}, sz))
			f.Close()
		}

	}
	testUpload := func(files []string, fromDir, toDir string) {

		// Hack the git config to mock destination
		GlobalOptions.GitConfig["remote.origin.git-lob-path"] = toDir

		fsync := FileSystemSyncProvider{}
		// Record of callbacks for files
		var filesUploaded []string = make([]string, 0, len(files))
		var filesSkipped []string
		callback := func(filename string, progressType ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
			if bytesDone == totalBytes {
				if progressType == ProgressSkip {
					filesSkipped = append(filesSkipped, filename)
				} else {
					filesUploaded = append(filesUploaded, filename)
				}
			}
			return false
		}
		err := fsync.Upload("origin", files, fromDir, false, callback)
		Expect(err).To(BeNil(), "Should not have error uploading")
		Expect(filesUploaded).To(Equal(files), "Callback should have seen all the files at 100%")
		Expect(filesSkipped).To(BeEmpty(), "No files should be skipped")
		// Check files exist & are correct size
		for _, file := range files {
			fulllocalpath := filepath.Join(fromDir, file)
			fullremotepath := filepath.Join(toDir, file)

			remotestat, err := os.Stat(fullremotepath)
			Expect(err).To(BeNil(), "Remote file should exist")
			localstat, err := os.Stat(fulllocalpath)
			Expect(err).To(BeNil(), "Local file should exist")
			Expect(remotestat.Size()).To(Equal(localstat.Size()), "Remote file should be the same size as local")
		}

		// Now check nothing is uploaded when we repeat without force
		filesUploaded = nil
		err = fsync.Upload("origin", files, fromDir, false, callback)
		Expect(err).To(BeNil(), "Should not have error uploading")
		Expect(filesUploaded).To(BeEmpty(), "No files should be uploaded a second time")
		Expect(filesSkipped).To(Equal(files), "All files should have been skipped")

		// Now check that with force we do it over again
		filesUploaded = make([]string, 0, len(files))
		filesSkipped = nil
		err = fsync.Upload("origin", files, fromDir, true, callback)
		Expect(err).To(BeNil(), "Should not have error uploading")
		Expect(filesUploaded).To(Equal(files), "Files should be overwritten in force mode")
		Expect(filesSkipped).To(BeEmpty(), "No files should be skipped in force mode")

		// Now corrupt a few files on the remote and make sure it's detected
		filesToCorrupt := []string{files[1], files[4], files[7], files[15]}
		for _, file := range filesToCorrupt {
			fullremotepath := filepath.Join(toDir, file)
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
		filesSkipped = make([]string, 0, len(files))
		err = fsync.Upload("origin", files, fromDir, false, callback)
		Expect(err).To(BeNil(), "Should not have error uploading")
		Expect(filesUploaded).To(Equal(filesToCorrupt), "Corrupt files should have been updated")
		Expect(filesSkipped).To(HaveLen(len(files)-len(filesToCorrupt)), "Non-corrupt files should be skipped")
	}

	testDownload := func(files []string, fromDir, toDir string) {

		// Hack the git config to mock destination
		GlobalOptions.GitConfig["remote.origin.git-lob-path"] = fromDir

		fsync := FileSystemSyncProvider{}
		// Record of callbacks for files
		var filesDownloaded []string = make([]string, 0, len(files))
		var filesSkipped []string
		callback := func(filename string, progressType ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
			if bytesDone == totalBytes {
				if progressType == ProgressSkip {
					filesSkipped = append(filesSkipped, filename)
				} else {
					filesDownloaded = append(filesDownloaded, filename)
				}
			}
			return false
		}
		err := fsync.Download("origin", files, toDir, false, callback)
		Expect(err).To(BeNil(), "Should not have error downloading")
		Expect(filesDownloaded).To(Equal(files), "Callback should have seen all the files at 100%")
		Expect(filesSkipped).To(BeEmpty(), "No files should be skipped")
		// Check files exist & are correct size
		for _, file := range files {
			fulllocalpath := filepath.Join(fromDir, file)
			fullremotepath := filepath.Join(toDir, file)

			remotestat, err := os.Stat(fullremotepath)
			Expect(err).To(BeNil(), "Remote file should exist")
			localstat, err := os.Stat(fulllocalpath)
			Expect(err).To(BeNil(), "Local file should exist")
			Expect(remotestat.Size()).To(Equal(localstat.Size()), "Remote file should be the same size as local")
		}

		// Now check nothing is downloaded when we repeat without force
		filesDownloaded = nil
		filesToDownload := []string{files[3], files[5], files[9], files[12]}
		err = fsync.Download("origin", filesToDownload, toDir, false, callback)
		Expect(err).To(BeNil(), "Should not have error uploading")
		Expect(filesDownloaded).To(BeEmpty(), "No files should be uploaded a second time")
		Expect(filesSkipped).To(Equal(filesToDownload), "All files should have been skipped")

		// Now check that with force we do it over again
		filesDownloaded = make([]string, 0, len(files))
		filesSkipped = nil
		err = fsync.Download("origin", filesToDownload, toDir, true, callback)
		Expect(err).To(BeNil(), "Should not have error downloading")
		Expect(filesDownloaded).To(Equal(filesToDownload), "Correct files should be downloaded")
		Expect(filesSkipped).To(BeEmpty(), "No files should be skipped")

	}

	Context("Mocked remote tests", func() {
		// These tests don't actually use real remote mapped drives because that
		// requires more environmental setup. We just use a local drive
		mockremotepath := filepath.Join(os.TempDir(), "MockFileSystemRemote")

		Context("Upload", func() {
			BeforeEach(func() {
				os.MkdirAll(mockremotepath, 0755)

				// Create local files
				testCreateFiles(localpath)

			})
			AfterEach(func() {
				os.RemoveAll(mockremotepath)
				os.RemoveAll(localpath)
			})

			It("successfully uploads", func() {
				testUpload(testfiles, localpath, mockremotepath)
			})
		})

		Context("Download", func() {
			BeforeEach(func() {
				os.MkdirAll(mockremotepath, 0755)
				os.RemoveAll(localpath)
				os.MkdirAll(localpath, 0755)

				// Create remote files
				testCreateFiles(mockremotepath)

			})
			AfterEach(func() {
				os.RemoveAll(mockremotepath)
				os.RemoveAll(localpath)
			})

			It("successfully downloads", func() {
				testDownload(testfiles, mockremotepath, localpath)
			})
		})

	})

	Context("Real remote tests [REMOTETEST]", func() {
		// To do real tests with real remote paths, define environment vars:
		// GITLOB_TEST_REMOTE_SMBPATH - Samba
		// GITLOB_TEST_REMOTE_NFSPATH - NFS
		// GITLOB_TEST_REMOTE_AFPPATH - AFP
		// Standard watch script skips these, use ginkgo --focus=REMOTETEST to run
		// ***************************************************************************
		// WARNING: Contents of these directories WILL BE DELETED so USE WITH CAUTION!
		// ***************************************************************************
		smbpath := os.Getenv("GITLOB_TEST_REMOTE_SMBPATH")
		nfspath := os.Getenv("GITLOB_TEST_REMOTE_NFSPATH")
		afppath := os.Getenv("GITLOB_TEST_REMOTE_AFPPATH")

		Context("Upload", func() {
			BeforeEach(func() {
				if smbpath != "" {
					os.MkdirAll(smbpath, 0755)
				}
				if nfspath != "" {
					os.MkdirAll(nfspath, 0755)
				}
				if afppath != "" {
					os.MkdirAll(afppath, 0755)
				}
				// Create local files
				testCreateFiles(localpath)

			})
			AfterEach(func() {
				if smbpath != "" {
					os.RemoveAll(smbpath)
				}
				if nfspath != "" {
					os.RemoveAll(nfspath)
				}
				if afppath != "" {
					os.RemoveAll(afppath)
				}
				os.RemoveAll(localpath)
			})
			Describe("Real remote upload tests", func() {
				It("Uploads to SMB", func() {
					if smbpath != "" {
						testUpload(testfiles, localpath, smbpath)
					}
				})
				It("Uploads to NFS", func() {
					if nfspath != "" {
						testUpload(testfiles, localpath, nfspath)
					}
				})
				It("Uploads to AFP", func() {
					if afppath != "" {
						testUpload(testfiles, localpath, afppath)
					}

				})
			})
		})

		Context("Download", func() {
			BeforeEach(func() {
				if smbpath != "" {
					os.MkdirAll(smbpath, 0755)
				}
				if nfspath != "" {
					os.MkdirAll(nfspath, 0755)
				}
				if afppath != "" {
					os.MkdirAll(afppath, 0755)
				}
				os.RemoveAll(localpath)

			})
			AfterEach(func() {
				if smbpath != "" {
					os.RemoveAll(smbpath)
				}
				if nfspath != "" {
					os.RemoveAll(nfspath)
				}
				if afppath != "" {
					os.RemoveAll(afppath)
				}
				os.RemoveAll(localpath)
			})
			Describe("Real remote download tests", func() {
				It("Downloads to SMB", func() {
					if smbpath != "" {
						testCreateFiles(smbpath)
						testDownload(testfiles, smbpath, localpath)
					}
				})
				It("Downloads to NFS", func() {
					if nfspath != "" {
						testCreateFiles(nfspath)
						testDownload(testfiles, nfspath, localpath)
					}
				})
				It("Downloads to AFP", func() {
					if afppath != "" {
						testCreateFiles(afppath)
						testDownload(testfiles, afppath, localpath)
					}

				})
			})
		})
	})

})
