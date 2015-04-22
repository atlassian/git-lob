package smart

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
)

var _ = Describe("Persistent Transport", func() {

	Context("Test JSON marshalling", func() {
		type TestStruct struct {
			Name      string
			Something int
		}
		It("Encodes JSON requests correctly", func() {

			params := &TestStruct{Name: "Steve", Something: 99}
			req, err := NewJsonRequest("TestMethod", params)
			Expect(err).To(BeNil(), "Should create request without error")
			reqbytes, err := json.Marshal(req)
			Expect(err).To(BeNil(), "Should marshal without error")
			Expect(string(reqbytes)).To(Equal(`{"Id":1,"Method":"TestMethod","Params":{"Name":"Steve","Something":99}}`), "Encoded JSON should be correct")

		})
		It("Decodes JSON requests correctly", func() {
			inputstruct := TestStruct{Name: "Steve", Something: 99}
			resp, err := NewJsonResponse(1, inputstruct)
			Expect(err).To(BeNil(), "Should create response without error")
			outputstruct := TestStruct{}
			// Now unmarshal nested result; need to extract json first
			innerbytes, err := resp.Result.MarshalJSON()
			Expect(err).To(BeNil(), "Extracting JSON from RawMessage should succeed")
			err = json.Unmarshal(innerbytes, &outputstruct)
			Expect(outputstruct).To(Equal(inputstruct), "Unmarshalled nested struct should match")
		})

	})

	Context("Test individual server requests", func() {
		metasThatExist := []string{
			"0000000000000000000000000000000000000000",
			"0000012300000004560000000000789000000000",
		}
		chunksThatExist := []string{
			"0000000000000000000000000000000000000000",
			"0000012300000004560000000000789000000000",
		}
		chunkIndexesThatExist := [][]int{
			[]int{0, 1},
			[]int{0, 1, 3, 4, 5, 7},
		}
		chunkSizes := [][]int64{ // only for first couple of chunks, testing only
			[]int64{16777216, 150},
			[]int64{16777216, 3210},
		}

		testsha := "5e0865e76e8956900c3ef6fec2d2af1c05f31ec4"
		metacontent := `{"SHA":"5e0865e76e8956900c3ef6fec2d2af1c05f31ec4","Size":21982,"NumChunks":4}`
		// Content doesn't actually matter here, just create some random data
		// Make sure it's big enough to require > 1 callback
		testchunkdatasz := PersistentTransportBufferSize*3 + 157
		testchunkidx := 3
		var testchunkdata []byte
		pickloblist := []string{"1234567890abcdef1234567890abcdef12345678", testsha, "0000000000000000000011111111112222222222"}
		deltaBaseSHA := "1234567890abcdef1234567890abcdef12345678"
		deltaTargetSHA := "5e0865e76e8956900c3ef6fec2d2af1c05f31ec4"

		BeforeEach(func() {
			testchunkdata = make([]byte, testchunkdatasz)
			// put something interesting in it so we can detect it at each end
			testchunkdata[0] = 'a'
			testchunkdata[1] = '1'
			testchunkdata[2] = '6'
			testchunkdata[testchunkdatasz-1] = 'z'
			testchunkdata[testchunkdatasz-2] = '2'
			testchunkdata[testchunkdatasz-3] = '5'

		})

		// Test server function here, just called over a pipe to test
		serve := func(conn net.Conn) {
			defer GinkgoRecover()
			defer conn.Close()
			// Run in a goroutine, be the server you seek
			// Read a request
			rdr := bufio.NewReader(conn)
			for {
				jsonbytes, err := rdr.ReadBytes(byte(0))
				if err != nil {
					if err == io.EOF {
						break
					}
					Fail(fmt.Sprintf("Test persistent server: unable to read from client: %v", err.Error()))
				}
				// slice off the terminator
				jsonbytes = jsonbytes[:len(jsonbytes)-1]
				var req JsonRequest
				err = json.Unmarshal(jsonbytes, &req)
				Expect(err).To(BeNil(), fmt.Sprintf("Test persistent server: unable to unmarshal json request from client:%v", string(jsonbytes)))
				var resp *JsonResponse
				allowedCaps := []string{"Feature1", "Feature2", "OMGSOAWESOME"}
				switch req.Method {
				case "QueryCaps":
					result := QueryCapsResponse{Caps: allowedCaps}
					resp, err = NewJsonResponse(req.Id, result)
					Expect(err).To(BeNil(), "Test persistent server: unable to create response")
				case "SetEnabledCaps":
					capsreq := SetEnabledCapsRequest{}
					extractStructFromJsonRawMessage(req.Params, &capsreq)
					result := SetEnabledCapsResponse{}
					resp, err = NewJsonResponse(req.Id, result)
					Expect(err).To(BeNil(), "Test persistent server: unable to create response")
					// test for error condition
					for _, c := range capsreq.EnableCaps {
						ok := false
						for _, s := range allowedCaps {
							if c == s {
								ok = true
								break
							}
						}
						if !ok {
							resp.Error = fmt.Sprintf("Unsupported capability: %v", c)
							break
						}

					}
				case "FileExists":
					fereq := FileExistsRequest{}
					var strerr string
					extractStructFromJsonRawMessage(req.Params, &fereq)
					result := FileExistsResponse{}
					if fereq.Type == "chunk" {
						for i, chunk := range chunksThatExist {
							if fereq.LobSHA == chunk {
								for _, chunkidx := range chunkIndexesThatExist[i] {
									if fereq.ChunkIdx == chunkidx {
										result.Result = true
										break
									}
								}
							}
							if result.Result == true {
								break
							}
						}
					} else if fereq.Type == "meta" {
						for _, meta := range metasThatExist {
							if fereq.LobSHA == meta {
								result.Result = true
								break
							}
						}

					} else {
						strerr = fmt.Sprintf("Unsupported type: %v", fereq.Type)
					}

					if strerr != "" {
						resp = NewJsonErrorResponse(req.Id, strerr)
					} else {
						resp, err = NewJsonResponse(req.Id, result)
						Expect(err).To(BeNil(), "Test persistent server: unable to create response")
					}

				case "FileExistsOfSize":
					fereq := FileExistsOfSizeRequest{}
					extractStructFromJsonRawMessage(req.Params, &fereq)
					result := FileExistsResponse{}
					for i, chunk := range chunksThatExist {
						if fereq.LobSHA == chunk {
							for chunkidx, sz := range chunkSizes[i] {
								if fereq.ChunkIdx == chunkidx {
									result.Result = (fereq.Size == sz)
									break
								}
							}
							break
						}
					}
					resp, err = NewJsonResponse(req.Id, result)
					Expect(err).To(BeNil(), "Test persistent server: unable to create response")
				case "UploadFile":
					upreq := UploadFileRequest{}
					extractStructFromJsonRawMessage(req.Params, &upreq)
					Expect(upreq.LobSHA).To(Equal(testsha), "Test persistent server: SHA incorrect")
					if upreq.Type == "chunk" {
						Expect(upreq.ChunkIdx).To(BeEquivalentTo(testchunkidx), "Test persistent server: Chunk index incorrect")
					}
					startresult := UploadFileStartResponse{}
					startresult.OKToSend = true
					// Send start response immediately
					resp, err = NewJsonResponse(req.Id, startresult)
					Expect(err).To(BeNil(), "Test persistent server: unable to create response")
					responseBytes, err := json.Marshal(resp)
					Expect(err).To(BeNil(), "Test persistent server: unable to marshal response")
					// null terminate response
					responseBytes = append(responseBytes, byte(0))
					conn.Write(responseBytes)
					// Next should by byte stream
					// Must read from buffered reader since bytes may have been read already
					receivedresult := UploadFileCompleteResponse{}
					receivedresult.ReceivedOK = true
					var receiveerr error
					// make pre-sized buffer
					contentbuf := bytes.NewBuffer(make([]byte, 0, upreq.Size))
					bytesLeft := upreq.Size
					for bytesLeft > 0 {
						c := PersistentTransportBufferSize
						if c > bytesLeft {
							c = bytesLeft
						}
						n, err := io.CopyN(contentbuf, rdr, c)
						bytesLeft -= int64(n)
						if err != nil {
							receivedresult.ReceivedOK = false
							receiveerr = fmt.Errorf("Test persistent server: unable to read data: %v", err.Error())
							break
						}
					}
					// Check we received what we expected to receive
					if upreq.Type == "meta" {
						if string(contentbuf.Bytes()) != metacontent {
							receiveerr = fmt.Errorf("Test persistent server: data didn't match")
						}
					} else {
						// test first & last 5 bytes
						contentbytes := contentbuf.Bytes()
						for i := 0; i < 5; i++ {
							if contentbytes[i] != testchunkdata[i] {
								receiveerr = fmt.Errorf("Test persistent server: data didn't match")
								break
							}
						}
						for i := len(contentbytes) - 5; i < len(contentbytes); i++ {
							if contentbytes[i] != testchunkdata[i] {
								receiveerr = fmt.Errorf("Test persistent server: data didn't match")
								break
							}
						}
					}
					// After we've read all the content bytes, send received response
					if receiveerr != nil {
						resp = NewJsonErrorResponse(req.Id, receiveerr.Error())
					} else {
						resp, _ = NewJsonResponse(req.Id, receivedresult)
					}
				case "DownloadFilePrepare":
					downreq := DownloadFilePrepareRequest{}
					extractStructFromJsonRawMessage(req.Params, &downreq)
					Expect(downreq.LobSHA).To(Equal(testsha), "Test persistent server: SHA incorrect")
					if downreq.Type == "chunk" {
						Expect(downreq.ChunkIdx).To(BeEquivalentTo(testchunkidx), "Test persistent server: Chunk index incorrect")
					}
					result := DownloadFilePrepareResponse{}
					if downreq.Type == "meta" {
						result.Size = int64(len(metacontent))
					} else {
						result.Size = int64(len(testchunkdata))
					}
					resp, err = NewJsonResponse(req.Id, result)
					Expect(err).To(BeNil(), "Test persistent server: unable to create response")
				case "DownloadFileStart":
					// Can't return any error responses here (byte stream response only), have to just fail
					downreq := DownloadFileStartRequest{}
					extractStructFromJsonRawMessage(req.Params, &downreq)
					// there is no response to this
					var sz int64
					var datasrc io.Reader
					if downreq.Type == "meta" {
						sz = int64(len(metacontent))
						datasrc = strings.NewReader(metacontent)
					} else {
						sz = int64(len(testchunkdata))
						datasrc = bytes.NewReader(testchunkdata)
					}
					// confirm size for testing
					Expect(sz).To(BeEquivalentTo(downreq.Size), "Test persistent server: download size incorrect")

					bytesLeft := sz
					for bytesLeft > 0 {
						c := PersistentTransportBufferSize
						if c > bytesLeft {
							c = bytesLeft
						}
						n, err := io.CopyN(conn, datasrc, c)
						bytesLeft -= int64(n)
						Expect(err).To(BeNil(), "Test persistent server: unable to read data")
					}
				case "PickCompleteLOB":
					params := GetFirstCompleteLOBFromListRequest{}
					extractStructFromJsonRawMessage(req.Params, &params)
					// check it's the list we expected
					Expect(params.LobSHAs).To(ConsistOf(pickloblist), "Server should receive correct params")
					result := GetFirstCompleteLOBFromListResponse{}
					result.FirstSHA = testsha
					resp, err = NewJsonResponse(req.Id, result)
					Expect(err).To(BeNil(), "Test persistent server: unable to create response")

				case "UploadDelta":
					upreq := UploadDeltaRequest{}
					extractStructFromJsonRawMessage(req.Params, &upreq)
					Expect(upreq.BaseLobSHA).To(Equal(deltaBaseSHA), "Test persistent server: base SHA incorrect")
					Expect(upreq.TargetLobSHA).To(Equal(deltaTargetSHA), "Test persistent server: base SHA incorrect")
					startresult := UploadDeltaStartResponse{}
					startresult.OKToSend = true
					// Send start response immediately
					resp, err = NewJsonResponse(req.Id, startresult)
					Expect(err).To(BeNil(), "Test persistent server: unable to create response")
					responseBytes, err := json.Marshal(resp)
					Expect(err).To(BeNil(), "Test persistent server: unable to marshal response")
					// null terminate response
					responseBytes = append(responseBytes, byte(0))
					conn.Write(responseBytes)
					// Next should by byte stream
					// Must read from buffered reader since bytes may have been read already
					receivedresult := UploadDeltaCompleteResponse{}
					receivedresult.ReceivedOK = true
					var receiveerr error
					// make pre-sized buffer
					contentbuf := bytes.NewBuffer(make([]byte, 0, upreq.Size))
					bytesLeft := upreq.Size
					for bytesLeft > 0 {
						c := PersistentTransportBufferSize
						if c > bytesLeft {
							c = bytesLeft
						}
						n, err := io.CopyN(contentbuf, rdr, c)
						bytesLeft -= int64(n)
						if err != nil {
							receivedresult.ReceivedOK = false
							receiveerr = fmt.Errorf("Test persistent server: unable to read data: %v", err.Error())
							break
						}
					}
					// Check we received what we expected to receive
					// test first & last 5 bytes
					contentbytes := contentbuf.Bytes()
					for i := 0; i < 5; i++ {
						if contentbytes[i] != testchunkdata[i] {
							receiveerr = fmt.Errorf("Test persistent server: data didn't match")
							break
						}
					}
					for i := len(contentbytes) - 5; i < len(contentbytes); i++ {
						if contentbytes[i] != testchunkdata[i] {
							receiveerr = fmt.Errorf("Test persistent server: data didn't match")
							break
						}
					}
					// After we've read all the content bytes, send received response
					if receiveerr != nil {
						resp = NewJsonErrorResponse(req.Id, receiveerr.Error())
					} else {
						resp, _ = NewJsonResponse(req.Id, receivedresult)
					}
				case "DownloadDeltaPrepare":
					downreq := DownloadDeltaPrepareRequest{}
					extractStructFromJsonRawMessage(req.Params, &downreq)
					Expect(downreq.BaseLobSHA).To(Equal(deltaBaseSHA), "Test persistent server: base SHA incorrect")
					Expect(downreq.TargetLobSHA).To(Equal(deltaTargetSHA), "Test persistent server: base SHA incorrect")
					result := DownloadDeltaPrepareResponse{}
					result.Size = int64(len(testchunkdata))
					resp, err = NewJsonResponse(req.Id, result)
					Expect(err).To(BeNil(), "Test persistent server: unable to create response")
				case "DownloadDeltaStart":
					// Can't return any error responses here (byte stream response only), have to just fail
					downreq := DownloadDeltaStartRequest{}
					extractStructFromJsonRawMessage(req.Params, &downreq)
					// there is no response to this
					Expect(downreq.BaseLobSHA).To(Equal(deltaBaseSHA), "Test persistent server: base SHA incorrect")
					Expect(downreq.TargetLobSHA).To(Equal(deltaTargetSHA), "Test persistent server: base SHA incorrect")
					sz := int64(len(testchunkdata))
					datasrc := bytes.NewReader(testchunkdata)
					// confirm size for testing
					Expect(sz).To(BeEquivalentTo(downreq.Size), "Test persistent server: download size incorrect")

					bytesLeft := sz
					for bytesLeft > 0 {
						c := PersistentTransportBufferSize
						if c > bytesLeft {
							c = bytesLeft
						}
						n, err := io.CopyN(conn, datasrc, c)
						bytesLeft -= int64(n)
						Expect(err).To(BeNil(), "Test persistent server: unable to read data")
					}
				default:
					resp = NewJsonErrorResponse(req.Id, fmt.Sprintf("Unknown method %v", req.Method))
				}
				if resp != nil {
					responseBytes, err := json.Marshal(resp)
					Expect(err).To(BeNil(), "Test persistent server: unable to marshal response")
					// null terminate response
					responseBytes = append(responseBytes, byte(0))
					conn.Write(responseBytes)
				}
			}
			conn.Close()
		}
		It("Queries capabilities (client)", func() {
			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			trans := NewPersistentTransport(cli)
			caps, err := trans.QueryCaps()
			Expect(err).To(BeNil(), "Should be no error")
			Expect(caps).To(ConsistOf([]string{"Feature1", "Feature2", "OMGSOAWESOME"}), "Capabilities should match server")

		})
		It("Sets capabilities (client)", func() {
			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			trans := NewPersistentTransport(cli)
			err := trans.SetEnabledCaps([]string{"OMGSOAWESOME", "Feature1"})
			Expect(err).To(BeNil(), "Should be no error")

		})
		It("Queries file existence (client)", func() {
			// This also tests multiple requests in sequence (JSON only)
			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			trans := NewPersistentTransport(cli)
			for _, meta := range metasThatExist {
				exists, err := trans.MetadataExists(meta)
				Expect(err).To(BeNil(), "Should be no error")
				Expect(exists).To(BeTrue(), "Metafile should exist")
			}
			for i, chunk := range chunksThatExist {
				for _, chunkidx := range chunkIndexesThatExist[i] {
					exists, err := trans.ChunkExists(chunk, chunkidx)
					Expect(err).To(BeNil(), "Should be no error")
					Expect(exists).To(BeTrue(), "Chunk should exist")
				}
			}

			// Now try a few that should fail
			exists, err := trans.MetadataExists("9999999999999999999999999999999999999999")
			Expect(err).To(BeNil(), "Should be no error")
			Expect(exists).To(BeFalse(), "Chunk should not exist")
			exists, err = trans.ChunkExists("9999999999999999999999999999999999999999", 0)
			Expect(err).To(BeNil(), "Should be no error")
			Expect(exists).To(BeFalse(), "Chunk should not exist")
			exists, err = trans.ChunkExists(chunksThatExist[0], 99)
			Expect(err).To(BeNil(), "Should be no error")
			Expect(exists).To(BeFalse(), "Chunk should not exist")

		})

		It("Queries file sizes (client)", func() {
			// This also tests multiple requests in sequence (JSON only)
			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			trans := NewPersistentTransport(cli)
			for i, chunksz := range chunkSizes {
				for chunkidx, sz := range chunksz {
					sha := chunksThatExist[i]
					exists, err := trans.ChunkExistsAndIsOfSize(sha, chunkidx, sz)
					Expect(err).To(BeNil(), "Should be no error")
					Expect(exists).To(BeTrue(), "Chunk should exist at this size")

					// Try a failure
					exists, err = trans.ChunkExistsAndIsOfSize(sha, chunkidx, sz+16)
					Expect(err).To(BeNil(), "Should be no error")
					Expect(exists).To(BeFalse(), "Chunk size shouldn't match")

				}
			}
			// Also test just a case of not being there at all
			exists, err := trans.ChunkExistsAndIsOfSize("9999999999999999999999999999999999999999", 0, 150)
			Expect(err).To(BeNil(), "Should be no error")
			Expect(exists).To(BeFalse(), "Chunk should not exist")

		})

		It("Detects errors", func() {
			// Request an invalid capability (as defined by this server)
			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			trans := NewPersistentTransport(cli)
			err := trans.SetEnabledCaps([]string{"Feature1", "THISISWRONG"})
			Expect(err).ToNot(BeNil(), "Should be an error")
		})

		It("Uploads metadata", func() {
			rdr := strings.NewReader(metacontent)

			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			trans := NewPersistentTransport(cli)
			err := trans.UploadMetadata(testsha, int64(len(metacontent)), rdr)
			Expect(err).To(BeNil(), "Should not be an error in UploadFile")
			Expect(rdr.Len()).To(BeZero(), "Server should have read all bytes")
		})

		It("Uploads chunk data", func() {
			rdr := bytes.NewReader(testchunkdata)

			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			numCallbacks := 0
			var totalBytesDone int64
			var totalBytesReported int64
			callback := func(bytesDone, totalBytes int64) {
				totalBytesDone = bytesDone
				totalBytesReported = totalBytes
				numCallbacks++
			}

			trans := NewPersistentTransport(cli)
			err := trans.UploadChunk(testsha, testchunkidx, testchunkdatasz, rdr, callback)
			Expect(err).To(BeNil(), "Should not be an error in UploadFile")
			Expect(rdr.Len()).To(BeZero(), "Server should have read all bytes")
			Expect(totalBytesDone).To(BeEquivalentTo(testchunkdatasz), "Callback should have reported all bytes done")
			Expect(totalBytesReported).To(BeEquivalentTo(testchunkdatasz), "Callback should have reported totalBytes correctly")
			Expect(numCallbacks).To(BeEquivalentTo(4), "Should have been 4 callbacks in total")

		})

		It("Downloads metadata", func() {
			var buf bytes.Buffer

			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			trans := NewPersistentTransport(cli)
			err := trans.DownloadMetadata(testsha, &buf)
			Expect(err).To(BeNil(), "Should not be an error in DownloadFile")
			Expect(string(buf.Bytes())).To(Equal(metacontent), "Should download expected metadata content")
		})
		It("Downloads chunk data", func() {
			var buf bytes.Buffer

			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			numCallbacks := 0
			var totalBytesDone int64
			var totalBytesReported int64
			callback := func(bytesDone, totalBytes int64) {
				totalBytesDone = bytesDone
				totalBytesReported = totalBytes
				numCallbacks++
			}

			trans := NewPersistentTransport(cli)
			err := trans.DownloadChunk(testsha, testchunkidx, &buf, callback)
			Expect(err).To(BeNil(), "Should not be an error in DownloadFile")
			Expect(buf.Len()).To(BeEquivalentTo(testchunkdatasz), "Should download the correct number of bytes")
			// Just check start & end of buffers
			contentbytes := buf.Bytes()
			Expect(contentbytes[:20]).To(Equal(testchunkdata[:20]), "Start of downloaded buffer should match")
			Expect(contentbytes[testchunkdatasz-20:]).To(Equal(testchunkdata[testchunkdatasz-20:]), "Start of downloaded buffer should match")
			Expect(totalBytesDone).To(BeEquivalentTo(testchunkdatasz), "Callback should have reported all bytes done")
			Expect(totalBytesReported).To(BeEquivalentTo(testchunkdatasz), "Callback should have reported totalBytes correctly")
			Expect(numCallbacks).To(BeEquivalentTo(4), "Should have been 4 callbacks in total")
		})

		It("Picks complete LOB from list", func() {
			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()
			trans := NewPersistentTransport(cli)
			sha, err := trans.GetFirstCompleteLOBFromList(pickloblist)
			Expect(err).To(BeNil(), "Should not be an error picking LOB")
			Expect(sha).To(Equal(testsha), "Should pick the correct sha")
		})

		It("Uploads a delta", func() {
			rdr := bytes.NewReader(testchunkdata)

			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			numCallbacks := 0
			var totalBytesDone int64
			var totalBytesReported int64
			callback := func(bytesDone, totalBytes int64) {
				totalBytesDone = bytesDone
				totalBytesReported = totalBytes
				numCallbacks++
			}

			trans := NewPersistentTransport(cli)
			ok, err := trans.UploadDelta(deltaBaseSHA, deltaTargetSHA, testchunkdatasz, rdr, callback)
			Expect(err).To(BeNil(), "Should not be an error in UploadDelta")
			Expect(ok).To(BeTrue(), "UploadDelta should have worked")
			Expect(rdr.Len()).To(BeZero(), "Server should have read all bytes")
			Expect(totalBytesDone).To(BeEquivalentTo(testchunkdatasz), "Callback should have reported all bytes done")
			Expect(totalBytesReported).To(BeEquivalentTo(testchunkdatasz), "Callback should have reported totalBytes correctly")
			Expect(numCallbacks).To(BeEquivalentTo(4), "Should have been 4 callbacks in total")

		})
		It("Downloads a delta", func() {
			var buf bytes.Buffer

			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			numCallbacks := 0
			var totalBytesDone int64
			var totalBytesReported int64
			callback := func(bytesDone, totalBytes int64) {
				totalBytesDone = bytesDone
				totalBytesReported = totalBytes
				numCallbacks++
			}

			trans := NewPersistentTransport(cli)
			ok, err := trans.DownloadDelta(deltaBaseSHA, deltaTargetSHA, 10000000, &buf, callback)
			Expect(err).To(BeNil(), "Should not be an error in DownloadFile")
			Expect(ok).To(BeTrue(), "DownloadDelta should have worked")
			Expect(buf.Len()).To(BeEquivalentTo(testchunkdatasz), "Should download the correct number of bytes")
			// Just check start & end of buffers
			contentbytes := buf.Bytes()
			Expect(contentbytes[:20]).To(Equal(testchunkdata[:20]), "Start of downloaded buffer should match")
			Expect(contentbytes[testchunkdatasz-20:]).To(Equal(testchunkdata[testchunkdatasz-20:]), "Start of downloaded buffer should match")
			Expect(totalBytesDone).To(BeEquivalentTo(testchunkdatasz), "Callback should have reported all bytes done")
			Expect(totalBytesReported).To(BeEquivalentTo(testchunkdatasz), "Callback should have reported totalBytes correctly")
			Expect(numCallbacks).To(BeEquivalentTo(4), "Should have been 4 callbacks in total")

			// Now check that a size limit works
			buf.Reset()
			ok, err = trans.DownloadDelta(deltaBaseSHA, deltaTargetSHA, 100, &buf, callback)
			Expect(err).To(BeNil(), "Should not be an error in DownloadFile")
			Expect(ok).To(BeFalse(), "DownloadDelta should return false when delta size is exceeded")
			Expect(buf.Len()).To(BeZero(), "Nothing should be downloaded when size exceeded")

		})

	})

	Context("Test chained server requests over one connection", func() {
		// TODO
	})

})
