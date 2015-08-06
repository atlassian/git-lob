package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/cloudflare/bm"
	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"github.com/atlassian/git-lob/core"
	"github.com/atlassian/git-lob/providers/smart"
)

var _ = Describe("git-lob-serve tests", func() {
	Context("Test individual server requests with test data", func() {

		var config *Config
		var repopath string
		var testchunkdata []byte
		testsha := "5e0865e76e8956900c3ef6fec2d2af1c05f31ec4"
		metacontent := `{"SHA":"5e0865e76e8956900c3ef6fec2d2af1c05f31ec4","Size":21982,"NumChunks":4}`
		testchunkdatasz := int64(16384)
		testchunkidx := 3

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

		It("Uploads & downloads simple files (client + reference server)", func() {
			cli, srv := net.Pipe()
			var outerr bytes.Buffer

			// 'Serve' is the real server function, usually connected to stdin/stdout but to pipe for test
			go Serve(srv, srv, &outerr, config, repopath)
			defer cli.Close()

			trans := smart.NewPersistentTransport(cli)

			// in this test we'll upload totally 'wrong' LOBs but we're just testing plumbing
			exists, _, err := trans.MetadataExists(testsha)
			Expect(err).To(BeNil(), "Should not be an error in MetadataExists")
			Expect(exists).To(BeFalse(), "Metadata should not exist yet")

			exists, _, err = trans.ChunkExists(testsha, testchunkidx)
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

			exists, sz, err := trans.MetadataExists(testsha)
			Expect(err).To(BeNil(), "Should not be an error in MetadataExists")
			Expect(exists).To(BeTrue(), "Metadata should now exist")
			Expect(sz).To(BeEquivalentTo(len(metacontent)), "Metadata should report right size")

			// Upload chunk (no callback used, that's tested in client tests)
			callback := func(bytesDone, totalBytes int64) {}
			chunkrdr := bytes.NewReader(testchunkdata)
			err = trans.UploadChunk(testsha, testchunkidx, testchunkdatasz, chunkrdr, callback)
			Expect(err).To(BeNil(), "Should not be an error in UploadChunk")
			Expect(chunkrdr.Len()).To(BeZero(), "Server should have read all bytes")
			s, err = os.Stat(getLOBChunkFilePath(testsha, testchunkidx, config, repopath))
			Expect(err).To(BeNil(), "Should not be an error stat'ing chunk")
			Expect(s.Size()).To(BeEquivalentTo(testchunkdatasz), "Server should have saved chunk at right size")

			exists, sz, err = trans.ChunkExists(testsha, testchunkidx)
			Expect(err).To(BeNil(), "Should not be an error in ChunkExists")
			Expect(exists).To(BeTrue(), "Chunk should now exist")
			Expect(sz).To(BeEquivalentTo(testchunkdatasz), "Server should report chunk at right size")

			exists, err = trans.ChunkExistsAndIsOfSize(testsha, testchunkidx, testchunkdatasz)
			Expect(err).To(BeNil(), "Should not be an error in ChunkExistsAndIsOfSize")
			Expect(exists).To(BeTrue(), "Chunk should now exist & be correct size")

			// Now try to download same data
			var buf bytes.Buffer
			err = trans.DownloadMetadata(testsha, &buf)
			Expect(err).To(BeNil(), "Should not be an error in DownloadMetadata")
			Expect(string(buf.Bytes())).To(Equal(metacontent), "Should download expected metadata content")

			buf.Reset()
			err = trans.DownloadChunk(testsha, testchunkidx, &buf, callback)
			Expect(err).To(BeNil(), "Should not be an error in DownloadChunk")
			Expect(buf.Len()).To(BeEquivalentTo(testchunkdatasz), "Should download the correct number of bytes")
			// Just check start & end of buffers
			contentbytes := buf.Bytes()
			Expect(contentbytes[:20]).To(Equal(testchunkdata[:20]), "Start of downloaded buffer should match")
			Expect(contentbytes[testchunkdatasz-20:]).To(Equal(testchunkdata[testchunkdatasz-20:]), "Start of downloaded buffer should match")

			// Make sure it fails safely when asking for the wrong SHA
			buf.Reset()
			err = trans.DownloadMetadata("0000000000000000000000000000000000000000", &buf)
			Expect(err).ToNot(BeNil(), "Should be an error in DownloadMetadata when asking for incorrect sha")
			Expect(buf.Len()).To(BeEquivalentTo(0), "Nothing should have been downloaded")
			err = trans.DownloadChunk("0000000000000000000000000000000000000000", 0, &buf, callback)
			Expect(err).ToNot(BeNil(), "Should be an error in DownloadChunk when asking for incorrect sha")
			Expect(buf.Len()).To(BeEquivalentTo(0), "Nothing should have been downloaded")

		})

	})

	Context("Delta tests which require valid binaries", func() {
		var config *Config
		var repopath string
		var oldChunkSize int64

		BeforeEach(func() {
			config = NewConfig()
			config.BasePath = filepath.Join(os.TempDir(), "git-lob-serve-test")
			config.DeltaCachePath = filepath.Join(os.TempDir(), "git-lob-serve-test-deltacache")
			os.MkdirAll(config.BasePath, 0755)
			repopath = "test/repo/with/deltas"

			// Alter the chunk size just for testing
			oldChunkSize = core.ChunkSize
			core.ChunkSize = 512

		})
		AfterEach(func() {
			os.RemoveAll(config.BasePath)
			os.RemoveAll(config.DeltaCachePath)
			core.ChunkSize = oldChunkSize
		})

		It("Uploads and downloads deltas", func() {
			cli, srv := net.Pipe()
			var outerr bytes.Buffer

			// 'Serve' is the real server function, usually connected to stdin/stdout but to pipe for test
			go Serve(srv, srv, &outerr, config, repopath)
			defer cli.Close()
			trans := smart.NewPersistentTransport(cli)

			// Firstly we upload a base LOB (must be complete)
			// Use NewBuffer with capacity but zero size to pre-size
			buf := bytes.NewBuffer(make([]byte, 0, core.ChunkSize*3+100))
			buf.Write(bytes.Repeat([]byte{'0'}, 128))
			buf.Write(bytes.Repeat([]byte{'1'}, 128))
			buf.Write(bytes.Repeat([]byte{'2'}, 128))
			buf.Write(bytes.Repeat([]byte{'3'}, 128)) // end of chunk 1
			buf.Write(bytes.Repeat([]byte{'4'}, 128))
			buf.Write(bytes.Repeat([]byte{'5'}, 128))
			buf.Write(bytes.Repeat([]byte{'6'}, 128))
			buf.Write(bytes.Repeat([]byte{'7'}, 128)) // end of chunk 2
			buf.Write(bytes.Repeat([]byte{'8'}, 128))
			buf.Write(bytes.Repeat([]byte{'9'}, 128))
			buf.Write(bytes.Repeat([]byte{'A'}, 128))
			buf.Write(bytes.Repeat([]byte{'B'}, 128)) // end of chunk 3
			buf.Write(bytes.Repeat([]byte{'C'}, 100)) // end of file

			// calculate SHA & generate metadata
			shacalc := sha1.New()
			shacalc.Write(buf.Bytes())
			sha := fmt.Sprintf("%x", string(shacalc.Sum(nil)))

			info := &core.LOBInfo{SHA: sha, Size: int64(buf.Len()), NumChunks: 4}
			infobytes, _ := json.Marshal(info)

			// Now upload
			metardr := bytes.NewReader(infobytes)
			err := trans.UploadMetadata(sha, int64(len(infobytes)), metardr)
			Expect(err).To(BeNil(), "Should not be an error in UploadMetadata")
			Expect(metardr.Len()).To(BeZero(), "Server should have read all bytes")

			chunkrdr := bytes.NewReader(buf.Bytes())
			callback := func(bytesDone, totalBytes int64) {}
			err = trans.UploadChunk(sha, 0, core.ChunkSize, chunkrdr, callback) // chunk 0
			Expect(err).To(BeNil(), "Should not be an error in UploadChunk")
			Expect(chunkrdr.Len()).To(BeEquivalentTo(int64(buf.Len())-core.ChunkSize), "Server should have read a chunk")
			err = trans.UploadChunk(sha, 1, core.ChunkSize, chunkrdr, callback) // chunk 1
			Expect(err).To(BeNil(), "Should not be an error in UploadChunk")
			Expect(chunkrdr.Len()).To(BeEquivalentTo(int64(buf.Len())-core.ChunkSize*2), "Server should have read a chunk")
			err = trans.UploadChunk(sha, 2, core.ChunkSize, chunkrdr, callback) // chunk 2
			Expect(err).To(BeNil(), "Should not be an error in UploadChunk")
			Expect(chunkrdr.Len()).To(BeEquivalentTo(int64(buf.Len())-core.ChunkSize*3), "Server should have read a chunk")
			err = trans.UploadChunk(sha, 3, 100, chunkrdr, callback) // final chunk
			Expect(err).To(BeNil(), "Should not be an error in UploadChunk")
			Expect(chunkrdr.Len()).To(BeZero(), "Server should have read final chunk")

			// Server should have LOB now
			exists, sz, err := trans.LOBExists(sha)
			Expect(err).To(BeNil(), "Should not be an error in LOBExists")
			Expect(exists).To(BeTrue(), "LOB should exist")
			Expect(sz).To(BeEquivalentTo(len(buf.Bytes())), "LOB should be right size")

			// Now let's make a revised version of this file
			// We'll change some bytes inside the file, across a chunk boundary, and add some data on at the end
			buf2 := bytes.NewBuffer(make([]byte, 0, core.ChunkSize*3+120))
			buf2.Write(bytes.Repeat([]byte{'0'}, 128))
			buf2.Write(bytes.Repeat([]byte{'1'}, 128))
			buf2.Write(bytes.Repeat([]byte{'2'}, 128))
			buf2.Write(bytes.Repeat([]byte{'3'}, 128)) // end of chunk 1
			buf2.Write(bytes.Repeat([]byte{'4'}, 128))
			buf2.Write(bytes.Repeat([]byte{'5'}, 128))
			buf2.Write(bytes.Repeat([]byte{'D'}, 128)) // changed
			buf2.Write(bytes.Repeat([]byte{'E'}, 128)) // changed // end of chunk 2
			buf2.Write(bytes.Repeat([]byte{'F'}, 128)) // changed
			buf2.Write(bytes.Repeat([]byte{'G'}, 128)) // changed
			buf2.Write(bytes.Repeat([]byte{'A'}, 128))
			buf2.Write(bytes.Repeat([]byte{'B'}, 128)) // end of chunk 3
			buf2.Write(bytes.Repeat([]byte{'C'}, 100))
			buf2.Write(bytes.Repeat([]byte{'Q'}, 20)) // added

			// calculate SHA & generate metadata
			shacalc2 := sha1.New()
			shacalc2.Write(buf2.Bytes())
			sha2 := fmt.Sprintf("%x", string(shacalc2.Sum(nil)))

			info2 := &core.LOBInfo{SHA: sha2, Size: int64(buf2.Len()), NumChunks: 4}
			infobytes2, _ := json.Marshal(info2)

			// Now let's look to upload the delta
			// First make sure that server picks it when given the option
			possibleSHAs := []string{"0022334455667788992200223344556677889922", sha, "99DDFFAA@@883322001199DDFFAA@@8833220011"}
			pickedsha, err := trans.GetFirstCompleteLOBFromList(possibleSHAs)
			Expect(err).To(BeNil(), "Should not be an error in GetFirstCompleteLOBFromList")
			Expect(pickedsha).To(Equal(sha), "Should have picked the correct sha out of the list")

			// let's generate a delta (not using client code right now, just VCDIFF directly)
			comp := bm.NewCompressor()
			var deltabuf bytes.Buffer
			// Create a dictionary
			baseDict := &bm.Dictionary{Dict: buf.Bytes()}
			// Use SetDictionary to set on compressor, this computes the hashes
			comp.SetDictionary(baseDict)
			// Set the delta buffer as the output
			comp.SetWriter(&deltabuf)
			// Now we just write the contents of the changed file to the compressor, then close to compress
			_, err = io.Copy(comp, buf2)
			Expect(err).To(BeNil(), "Should not be an error copying bytes to compressor")
			// This does the compression
			err = comp.Close()
			Expect(err).To(BeNil(), "Should not be an error finishing compression")
			deltabytes := deltabuf.Bytes()
			Expect(len(deltabytes)).ToNot(BeZero(), "Length of delta bytes should not be zero")
			deltardr := bytes.NewReader(deltabytes)
			ok, err := trans.UploadDelta(sha, sha2, int64(len(deltabytes)), deltardr, callback)
			Expect(err).To(BeNil(), "Should not be an error in UploadDelta")
			Expect(ok).To(BeTrue(), "Delta should have been uploaded ok")

			// Now download the target version & validate (not deltas yet)
			var downloadbuf bytes.Buffer
			err = trans.DownloadMetadata(sha2, &downloadbuf)
			Expect(err).To(BeNil(), "Should not be an error in DownloadMetadata")
			Expect(string(downloadbuf.Bytes())).To(Equal(string(infobytes2)), "Should download expected metadata content")
			// Just check one of the changed chunks
			downloadbuf.Reset()
			err = trans.DownloadChunk(sha2, 1, &downloadbuf, callback)
			Expect(err).To(BeNil(), "Should not be an error in DownloadChunk")
			Expect(downloadbuf.Len()).To(BeEquivalentTo(core.ChunkSize), "Should download the correct number of bytes")
			Expect(downloadbuf.Bytes()).To(Equal(buf2.Bytes()[core.ChunkSize:core.ChunkSize*2]), "Second chunk buffer should match")

			// Check that cache was saved
			fi, err := ioutil.ReadDir(config.DeltaCachePath)
			Expect(err).To(BeNil(), "Should not be an error reading delta cache path")
			Expect(fi).To(HaveLen(1), "Should be one file in cache")
			Expect(fi[0].Size()).To(BeEquivalentTo(len(deltabytes)), "Delta file should match")

			// Now request delta download, should use cache
			downloadbuf.Reset()
			// Check nothing happens when size limit exceeded
			ok, err = trans.DownloadDelta(sha, sha2, 10, &downloadbuf, callback)
			Expect(err).To(BeNil(), "Should not be an error in DownloadDelta")
			Expect(ok).To(BeFalse(), "Delta should not have happened becuase size is too big")

			downloadbuf.Reset()
			ok, err = trans.DownloadDelta(sha, sha2, 9999999, &downloadbuf, callback)
			Expect(err).To(BeNil(), "Should not be an error in DownloadDelta (cached)")
			Expect(ok).To(BeTrue(), "Delta should have happened (cached)")
			Expect(downloadbuf.Bytes()).To(Equal(deltabytes), "Delta should be identical (cached)")

			// Now request delta download again, but delete the cached item so it generates it again
			err = os.Remove(getLOBDeltaFilePath(sha, sha2, config, repopath))
			Expect(err).To(BeNil(), "Should not be an error deleting delta cache file")
			downloadbuf.Reset()
			ok, err = trans.DownloadDelta(sha, sha2, 9999999, &downloadbuf, callback)
			Expect(err).To(BeNil(), "Should not be an error in DownloadDelta (not cached)")
			Expect(ok).To(BeTrue(), "Delta should have happened (not cached)")
			Expect(downloadbuf.Bytes()).To(Equal(deltabytes), "Delta should be identical (not cached)")

			// Test that delta was re-cached after being generated
			s, err := os.Stat(getLOBDeltaFilePath(sha, sha2, config, repopath))
			Expect(err).To(BeNil(), "Delta should have been re-cached after calculation in DownloadDelta")
			Expect(s.Size()).To(BeEquivalentTo(len(deltabytes)), "Cached delta should be the same size")

		})

	})

})
