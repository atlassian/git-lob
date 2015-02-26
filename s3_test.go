package main

import (
	"fmt"
	"bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/mitchellh/goamz/aws"
	"bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/mitchellh/goamz/s3"
	"bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/mitchellh/goamz/testutil"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"io/ioutil"
	"os"
	"path/filepath"
)

var _ = Describe("S3", func() {
	Context("Mocked S3 tests", func() {
		var testServer *testutil.HTTPServer
		var auth = aws.Auth{"abc", "123", ""}
		var s3sync *S3SyncProvider
		BeforeEach(func() {
			// Mock server
			// No shutdown available so must only do this once
			if testServer == nil {
				testServer = testutil.NewHTTPServer()
				testServer.Start()
			}
			// Hack the git config to mock destination
			GlobalOptions.GitConfig["remote.origin.git-lob-s3-bucket"] = "thebucket"
			// Manually configure provider to use test server
			s3sync = &S3SyncProvider{}
			s3sync.S3Connection = s3.New(auth, aws.Region{Name: "test-region-1", S3Endpoint: testServer.URL})
		})
		AfterEach(func() {
			GlobalOptions = NewOptions()
			testServer.Flush()
		})

		It("Detects whether files exist", func() {
			// Not existing
			testServer.Response(404, nil, "")
			exists := s3sync.FileExists("origin", "notafile.txt")
			Expect(exists).To(BeFalse(), "File should not exist")
			req := testServer.WaitRequest()
			Expect(req.Method).To(Equal("HEAD"), "Should be correct HTTP method")
			Expect(req.URL.Path).To(Equal("/thebucket/notafile.txt"), "Requested path should be correct")
			// Not existing, checking size too
			testServer.Response(404, nil, "")
			exists = s3sync.FileExistsAndIsOfSize("origin", "notafile.txt", 150)
			Expect(exists).To(BeFalse(), "File should not exist")
			req = testServer.WaitRequest()
			Expect(req.Method).To(Equal("HEAD"), "Should be correct HTTP method")
			Expect(req.URL.Path).To(Equal("/thebucket/notafile.txt"), "Requested path should be correct")
			// Existing
			testServer.Response(200, map[string]string{"Content-Length": "100"}, "")
			exists = s3sync.FileExists("origin", "actuallyafile.txt")
			Expect(exists).To(BeTrue(), "File should exist")
			req = testServer.WaitRequest()
			Expect(req.Method).To(Equal("HEAD"), "Should be correct HTTP method")
			Expect(req.URL.Path).To(Equal("/thebucket/actuallyafile.txt"), "Requested path should be correct")
			// Existing, but wrong size
			testServer.Response(200, map[string]string{"Content-Length": "100"}, "")
			exists = s3sync.FileExistsAndIsOfSize("origin", "actuallyafile.txt", 150)
			Expect(exists).To(BeFalse(), "File exists but wrong size")
			req = testServer.WaitRequest()
			Expect(req.Method).To(Equal("HEAD"), "Should be correct HTTP method")
			Expect(req.URL.Path).To(Equal("/thebucket/actuallyafile.txt"), "Requested path should be correct")
			// Existing & right size too
			testServer.Response(200, map[string]string{"Content-Length": "150"}, "")
			exists = s3sync.FileExistsAndIsOfSize("origin", "actuallyafile.txt", 150)
			Expect(exists).To(BeTrue(), "File should exist")
			req = testServer.WaitRequest()
			Expect(req.Method).To(Equal("HEAD"), "Should be correct HTTP method")
			Expect(req.URL.Path).To(Equal("/thebucket/actuallyafile.txt"), "Requested path should be correct")

		})

		It("Uploads files to S3", func() {
			var filesUploaded []string
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

			// Need 3 responses (unfortunately we have to know the internals of the requests here)
			// 1 Check that bucket exists (OK)
			testServer.Response(200, nil, "")
			// 2 Check if file exists (404)
			testServer.Response(404, nil, "")
			// 3 Upload
			testServer.Response(200, nil, "")
			tmp, _ := ioutil.TempDir("", "s3test")
			filename := filepath.Join(tmp, "file1.txt")
			CreateRandomFileForTest(100, filename)
			err := s3sync.Upload("origin", []string{filepath.Base(filename)}, filepath.Dir(filename), false, callback)

			Expect(err).To(BeNil(), "Should not be error uploading")
			// Get 3rd response
			testServer.WaitRequest()
			testServer.WaitRequest()
			req := testServer.WaitRequest()
			Expect(req.Method).To(Equal("PUT"), "Should be correct HTTP method")
			Expect(req.URL.Path).To(Equal(fmt.Sprintf("/thebucket/%v", filepath.Base(filename))), "Requested path should be correct")
			Expect(req.Header["Content-Length"]).To(Equal([]string{"100"}), "Should be correct content length")
			Expect(filesSkipped).To(BeEmpty(), "No files should be skipped")
			Expect(filesUploaded).To(ConsistOf([]string{"file1.txt"}), "Correct files should be uploaded")

			// now test doesn't upload when already exists
			filesUploaded = []string{}
			// 1 Check that bucket exists (OK)
			testServer.Response(200, nil, "")
			// 2 Check if file exists OK & report size
			testServer.Response(200, map[string]string{"Content-Length": "100"}, "")

			err = s3sync.Upload("origin", []string{filepath.Base(filename)}, filepath.Dir(filename), false, callback)

			Expect(err).To(BeNil(), "Should not be error uploading")
			testServer.Flush()
			Expect(filesUploaded).To(BeEmpty(), "No files should be uploaded")
			Expect(filesSkipped).To(ConsistOf([]string{"file1.txt"}), "Correct files should be skipped")

			os.Remove(filename)
		})

		It("Downloads files from S3", func() {
			var filesDownloaded []string
			var filesSkipped []string
			var filesNotFound []string
			callback := func(filename string, progressType ProgressCallbackType, bytesDone, totalBytes int64) (abort bool) {
				if progressType == ProgressNotFound {
					filesNotFound = append(filesNotFound, filename)
				} else {
					if bytesDone == totalBytes {
						if progressType == ProgressSkip {
							filesSkipped = append(filesSkipped, filename)
						} else {
							filesDownloaded = append(filesDownloaded, filename)
						}
					}
				}
				return false
			}

			fileContent := "Hello from S3"
			tmp, _ := ioutil.TempDir("", "s3test")
			filename := "tests3file.txt"
			absfile := filepath.Join(tmp, filename)

			// First, missing file
			// 1 Check that bucket exists (OK)
			testServer.Response(200, nil, "")
			// 2 Check if file exists (404)
			testServer.Response(404, nil, "")
			err := s3sync.Download("origin", []string{filepath.Base(filename)}, tmp, false, callback)
			// No error even though file doesn't exist, should just be reported as missing & continue
			Expect(err).To(BeNil(), "Should not be error downloading")
			testServer.WaitRequest()
			testServer.WaitRequest()
			Expect(filesSkipped).To(BeEmpty(), "No files should be skipped")
			Expect(filesDownloaded).To(BeEmpty(), "No files should be downloaded")
			Expect(filesNotFound).To(ConsistOf([]string{"tests3file.txt"}), "Correct files should be not found")

			// reset
			filesNotFound = []string{}
			testServer.Flush()

			// Now test when really there
			// 1 Check that bucket exists (OK)
			testServer.Response(200, nil, "")
			// 2 Check if file exists OK & report size
			testServer.Response(200, map[string]string{"Content-Length": fmt.Sprintf("%d", len(fileContent))}, "")
			// 3 Download
			testServer.Response(200, map[string]string{"Content-Length": fmt.Sprintf("%d", len(fileContent))}, fileContent)

			err = s3sync.Download("origin", []string{filepath.Base(filename)}, tmp, false, callback)
			// No error even though file doesn't exist, should just be reported as missing & continue
			Expect(err).To(BeNil(), "Should not be error downloading")
			testServer.WaitRequest()
			testServer.WaitRequest()
			testServer.WaitRequest()
			Expect(filesSkipped).To(BeEmpty(), "No files should be skipped")
			Expect(filesNotFound).To(BeEmpty(), "No files should be not found")
			Expect(filesDownloaded).To(ConsistOf([]string{"tests3file.txt"}), "Correct files should be downloaded")
			// Check file content
			dl, err := ioutil.ReadFile(absfile)
			Expect(err).To(BeNil(), "Should not be error checking downloaded file content")
			Expect(string(dl)).To(Equal(fileContent), "Downloaded file content should be correct")

			// Test skips when repeated without force
			filesDownloaded = []string{}
			testServer.Flush()

			// 1 Check that bucket exists (OK)
			testServer.Response(200, nil, "")
			// 2 Check if file exists OK & report size
			testServer.Response(200, map[string]string{"Content-Length": fmt.Sprintf("%d", len(fileContent))}, "")
			// (download shouldn't be called)

			err = s3sync.Download("origin", []string{filepath.Base(filename)}, tmp, false, callback)
			// No error even though file doesn't exist, should just be reported as missing & continue
			Expect(err).To(BeNil(), "Should not be error downloading")
			testServer.WaitRequest()
			testServer.WaitRequest()
			Expect(filesDownloaded).To(BeEmpty(), "No files should be downloaded")
			Expect(filesNotFound).To(BeEmpty(), "No files should be not found")
			Expect(filesSkipped).To(ConsistOf([]string{"tests3file.txt"}), "Correct files should be skipped")

			os.Remove(absfile)
		})

	})

	/*
		It("Simple S3 smoke tests", func() {
			sync := S3SyncProvider{}
			sync.initS3()
			bucket := sync.S3Connection.Bucket("git-lob.test")
			key, err := bucket.GetKey("test.txt")
			Expect(err).To(BeNil(), "Should be there")
			Expect(key).ToNot(BeNil(), "Should be there")
			Expect(key.Size).To(BeEquivalentTo(14), "Correct size?")

			head, err := bucket.Head("/")
			Expect(err).To(BeNil(), "Should be there")
			Expect(head).ToNot(BeNil(), "Should be there")

			bucket2 := sync.S3Connection.Bucket("not-here")
			head, err = bucket2.Head("/")
			Expect(err).ToNot(BeNil(), "Should not be there")
			Expect(head).To(BeNil(), "Should not be there")

		})
	*/
})
