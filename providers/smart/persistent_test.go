package smart

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"bufio"
	"encoding/json"
	"fmt"
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
			req := NewJsonRequest("TestMethod", params)

			reqbytes, err := json.Marshal(req)
			Expect(err).To(BeNil(), "Should marshal without error")
			Expect(string(reqbytes)).To(Equal(`{"Id":1,"Method":"TestMethod","Params":{"Name":"Steve","Something":99}}`), "Encoded JSON should be correct")

		})
		It("Decodes JSON requests correctly", func() {

			resp := JsonResponse{}
			s := TestStruct{}
			b := []byte(`{"Id":1,"Result":{"Name":"Steve","Something":99}}`)
			err := json.Unmarshal(b, &resp)
			Expect(err).To(BeNil(), "Should unmarshal without error")
			// Now unmarshal nested result
			err = json.Unmarshal(resp.Result, &s)
			orig := TestStruct{Name: "Steve", Something: 99}
			Expect(s).To(Equal(orig), "Unmarshalled nested struct should match")
		})

	})

	Context("Test individual server requests", func() {
		serve := func(conn net.Conn) {
			defer conn.Close()
			// Run in a goroutine, be the server you seek
			// Read a request
			rdr := bufio.NewReader(conn)
			jsonbytes, err := rdr.ReadBytes(byte(0))
			if err != nil {
				Fail(fmt.Sprintf("Test persistent server: unable to read from client: %v", err.Error()))
			}
			// On the server we just work in JSON
			_ = jsonbytes
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
		It("Detects errors", func() {
			// TODO
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
