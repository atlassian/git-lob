package smart

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
				if err != nil {
					Fail(fmt.Sprintf("Test persistent server: unable to unmarshal json request from client:%v %v", string(jsonbytes), err.Error()))
				}
				var resp *JsonResponse
				allowedCaps := []string{"Feature1", "Feature2", "OMGSOAWESOME"}
				switch req.Method {
				case "QueryCaps":
					result := QueryCapsResponse{Caps: allowedCaps}
					resp, err = NewJsonResponse(req.Id, result)
					if err != nil {
						Fail(fmt.Sprintf("Test persistent server: unable to create response: %v", err.Error()))
					}
				case "SetEnabledCaps":
					capsreq := SetEnabledCapsRequest{}
					extractStructFromJsonRawMessage(req.Params, &capsreq)
					result := SetEnabledCapsResponse{}
					resp, err = NewJsonResponse(req.Id, result)
					if err != nil {
						Fail(fmt.Sprintf("Test persistent server: unable to create response: %v", err.Error()))
					}
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
						if err != nil {
							Fail(fmt.Sprintf("Test persistent server: unable to create response: %v", err.Error()))
						}
					}

				default:
					resp = NewJsonErrorResponse(req.Id, fmt.Sprintf("Unknown method %v", req.Method))
				}
				responseBytes, err := json.Marshal(resp)
				if err != nil {
					Fail(fmt.Sprintf("Test persistent server: unable to marshal response:%v %v", resp, err.Error()))
				}
				// null terminate response
				responseBytes = append(responseBytes, byte(0))
				conn.Write(responseBytes)
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

		It("Detects errors", func() {
			// Request an invalid capability (as defined by this server)
			cli, srv := net.Pipe()
			go serve(srv)
			defer cli.Close()

			trans := NewPersistentTransport(cli)
			err := trans.SetEnabledCaps([]string{"Feature1", "THISISWRONG"})
			Expect(err).ToNot(BeNil(), "Should be an error")
		})

		It("Deals with disconnection", func() {
			// ??
		})
		It("Deals with timeouts", func() {
			// ??
		})

	})

	Context("Test chained server requests over one connection", func() {
		// TODO
	})

})
