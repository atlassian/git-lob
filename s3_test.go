package main

import (
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
	"github.com/mitchellh/goamz/testutil"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("S3", func() {

	Context("Mocked S3 tests", func() {
		var testServer *testutil.HTTPServer
		var auth = aws.Auth{"abc", "123", ""}
		var s3sync *S3SyncProvider
		BeforeEach(func() {
			// Mock server
			// Hack the git config to mock destination
			GlobalOptions.GitConfig["remote.origin.git-lob-s3-bucket"] = "thebucket"
			testServer = testutil.NewHTTPServer()
			testServer.Start()
			// Manually configure provider to use test server
			s3sync = &S3SyncProvider{}
			s3sync.S3Connection = s3.New(auth, aws.Region{Name: "test-region-1", S3Endpoint: testServer.URL})
		})
		AfterEach(func() {
			GlobalOptions = NewOptions()
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

	})

	Context("Real S3 tests", func() {

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
