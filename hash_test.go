package main

import (
	"bytes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
)

var _ = Describe("Hash", func() {
	var testFileName string
	// This was calculated with 'shasum' on Mac OS X with this file content
	const correctSHA = "772157c6ef480852edf921f5924b1ca582b0d78f"

	BeforeEach(func() {
		// Create binary file
		f, err := ioutil.TempFile("", "hashtest")
		if err != nil {
			Fail("Unable to create test file")
		}
		for i := 0; i < 128; i++ {
			var j byte
			for j = 0; j < 255; j++ {
				f.Write(bytes.Repeat([]byte{j}, 16))
			}
		}
		testFileName = f.Name()
		f.Close()
	})
	AfterEach(func() {
		os.Remove(testFileName)
	})

	It("correctly calculates a SHA", func() {
		testSHA, err := CalculateFileSHA(testFileName)
		Expect(err).Should(BeNil())
		Expect(testSHA).To(Equal(correctSHA))

	})
	It("correctly fails on missing file", func() {
		_, err := CalculateFileSHA("/Users/imaginaryperson/this/does/not/exist")
		Expect(err).ShouldNot(BeNil())
	})
})
