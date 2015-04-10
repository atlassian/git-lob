package smart

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"encoding/json"
)

var _ = Describe("Persistent Transport", func() {

	Context("Test JSON marshalling", func() {
		type TestStruct struct {
			JsonRequest
			Name      string
			Something int
		}
		It("Encodes JSON requests correctly", func() {

			req := &TestStruct{Name: "Steve", Something: 99}
			InitJsonRequest(&req.JsonRequest)

			reqbytes, err := json.Marshal(req)
			Expect(err).To(BeNil(), "Should marshal without error")
			Expect(string(reqbytes)).To(Equal(`{"Id":1,"Method":"","Name":"Steve","Something":99}`), "Encoded JSON should be correct")

		})
		It("Decodes JSON requests correctly", func() {
			t := &TestStruct{}
			var i interface{}
			i = t

			b := []byte(`{"Id":1,"Method":"","Name":"Steve","Something":99}`)
			err := json.Unmarshal(b, i)
			Expect(err).To(BeNil(), "Should unmarshal without error")
			req := &TestStruct{Name: "Steve", Something: 99}
			req.Id = 1
			Expect(i).To(Equal(req), "Unmarshalled should match")
		})

	})

})
