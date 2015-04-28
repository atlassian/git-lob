package main

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"bitbucket.org/sinbad/git-lob/providers/smart"
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
)

var _ = Describe("git-lob-serve tests", func() {
	Context("Test individual server requests", func() {

		var config *Config
		var repopath string
		var testchunkdata []byte
		testsha := "5e0865e76e8956900c3ef6fec2d2af1c05f31ec4"
		metacontent := `{"SHA":"5e0865e76e8956900c3ef6fec2d2af1c05f31ec4","Size":21982,"NumChunks":4}`
		testchunkdatasz := int64(16384)
		testchunkidx := 3
		// pickloblist := []string{"1234567890abcdef1234567890abcdef12345678", testsha, "0000000000000000000011111111112222222222"}
		// deltaBaseSHA := "1234567890abcdef1234567890abcdef12345678"
		// deltaTargetSHA := "5e0865e76e8956900c3ef6fec2d2af1c05f31ec4"

		BeforeEach(func() {
			config = NewConfig()
			config.BasePath = filepath.Join(os.TempDir(), "git-lob-serve-test")
			os.MkdirAll(config.BasePath, 0755)
			repopath = "test/repo"

			testchunkdata = make([]byte, testchunkdatasz)
			// put something interesting in it so we can detect it at each end
			testchunkdata[0] = '8'
			testchunkdata[1] = 'p'
			testchunkdata[2] = 'q'
			testchunkdata[3] = 'L'
			testchunkdata[testchunkdatasz-1] = 'z'
			testchunkdata[testchunkdatasz-2] = '2'
			testchunkdata[testchunkdatasz-3] = '5'
		})
		AfterEach(func() {
			os.RemoveAll(config.BasePath)
		})

		It("Queries capabilities (client + reference server)", func() {
			cli, srv := net.Pipe()
			var outerr bytes.Buffer
			// 'Serve' is the real server function, usually connected to stdin/stdout but to pipe for test
			go Serve(srv, srv, &outerr, config, repopath)
			defer cli.Close()

			trans := smart.NewPersistentTransport(cli)
			caps, err := trans.QueryCaps()
			Expect(err).To(BeNil(), "Should be no error")
			Expect(caps).To(ConsistOf([]string{"binary_delta"}))
			Expect(outerr.String()).To(HaveLen(0), "Nothing should be written to stderr")

		})

		It("Uploads simple files (client + reference server)", func() {
			cli, srv := net.Pipe()
			var outerr bytes.Buffer

			// 'Serve' is the real server function, usually connected to stdin/stdout but to pipe for test
			go Serve(srv, srv, &outerr, config, repopath)
			defer cli.Close()

			trans := smart.NewPersistentTransport(cli)

			// in this test we'll upload totally 'wrong' LOBs but we're just testing plumbing
			exists, err := trans.MetadataExists(testsha)
			Expect(err).To(BeNil(), "Should not be an error in MetadataExists")
			Expect(exists).To(BeFalse(), "Metadata should not exist yet")

			exists, err = trans.ChunkExists(testsha, testchunkidx)
			Expect(err).To(BeNil(), "Should not be an error in ChunkExists")
			Expect(exists).To(BeFalse(), "Chunk should not exist yet")

			// Upload metadata now
			metardr := strings.NewReader(metacontent)
			err = trans.UploadMetadata(testsha, int64(len(metacontent)), metardr)
			Expect(err).To(BeNil(), "Should not be an error in UploadMetadata")
			Expect(metardr.Len()).To(BeZero(), "Server should have read all bytes")
			s, err := os.Stat(getLOBMetaFilePath(testsha, config, repopath))
			Expect(err).To(BeNil(), "Should not be an error stat'ing metadata")
			Expect(s.Size()).To(BeEquivalentTo(len(metacontent)), "Server should have saved metacontent at right size")

			exists, err = trans.MetadataExists(testsha)
			Expect(err).To(BeNil(), "Should not be an error in MetadataExists")
			Expect(exists).To(BeTrue(), "Metadata should now exist")

			// Upload chunk (no callback used, that's tested in client tests)
			chunkrdr := bytes.NewReader(testchunkdata)
			err = trans.UploadChunk(testsha, testchunkidx, testchunkdatasz, chunkrdr, func(bytesDone, totalBytes int64) {})
			Expect(err).To(BeNil(), "Should not be an error in UploadChunk")
			Expect(chunkrdr.Len()).To(BeZero(), "Server should have read all bytes")
			s, err = os.Stat(getLOBChunkFilePath(testsha, testchunkidx, config, repopath))
			Expect(err).To(BeNil(), "Should not be an error stat'ing chunk")
			Expect(s.Size()).To(BeEquivalentTo(testchunkdatasz), "Server should have saved chunk at right size")

			exists, err = trans.ChunkExists(testsha, testchunkidx)
			Expect(err).To(BeNil(), "Should not be an error in ChunkExists")
			Expect(exists).To(BeTrue(), "Chunk should now exist")

			exists, err = trans.ChunkExistsAndIsOfSize(testsha, testchunkidx, testchunkdatasz)
			Expect(err).To(BeNil(), "Should not be an error in ChunkExistsAndIsOfSize")
			Expect(exists).To(BeTrue(), "Chunk should now exist & be correct size")
		})
	})

})
