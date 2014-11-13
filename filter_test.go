package main

import (
	"bytes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
	"path"
)

var _ = Describe("Filter", func() {

	root := path.Join(os.TempDir(), "StorageTest")
	BeforeEach(func() {
		// Set up git repo with some subfolders
		CreateGitRepoForTest(root)
		os.Chdir(root)

	})

	AfterEach(func() {
		// Delete repo
		os.RemoveAll(root)
	})

	Describe("Smudge filter", func() {

		It("doesn't alter non-LOB content", func() {
			nonLOBString := `This is some data
in a string
that we should absolutely not mess with`
			inBuffer := bytes.NewBufferString(nonLOBString)
			var outBuffer bytes.Buffer
			res := SmudgeFilterWithReaderWriter(inBuffer, &outBuffer)
			Expect(res).To(Equal(0), "smudge filter should succeed")
			Expect(outBuffer.String()).To(BeEquivalentTo(nonLOBString), "non LOB should not be modified by smudge")
		})

		It("doesn't alter LOB content when LOB isn't present in object store & no autodownloading", func() {
			// TODO this is when auto download is not implemented; turn it off when added
			// Made up SHA that doesn't exist
			lobString := SHAPrefix + "0123456789abcdef0123456789abcdef01234567"
			inBuffer := bytes.NewBufferString(lobString)
			var outBuffer bytes.Buffer
			res := SmudgeFilterWithReaderWriter(inBuffer, &outBuffer)
			Expect(res).To(Equal(0), "smudge filter should succeed")
			Expect(outBuffer.String()).To(BeEquivalentTo(lobString), "non existent LOB should not be modified by smudge")
		})

		It("writes real LOB data for small file", func() {
			lobinfo := CreateSmallTestLOBDataForRetrieval()
			lobString := SHAPrefix + lobinfo.SHA
			inBuffer := bytes.NewBufferString(lobString)
			var outBuffer bytes.Buffer
			res := SmudgeFilterWithReaderWriter(inBuffer, &outBuffer)
			Expect(res).To(Equal(0), "smudge filter should succeed")
			Expect(outBuffer.Len()).To(BeEquivalentTo(lobinfo.Size), "extracted LOB data should be correct size")
		})

		It("writes real LOB data for large file [LONGTEST]", func() {
			lobinfo := CreateLargeTestLOBDataForRetrieval()
			lobString := SHAPrefix + lobinfo.SHA
			inBuffer := bytes.NewBufferString(lobString)
			var outBuffer bytes.Buffer
			res := SmudgeFilterWithReaderWriter(inBuffer, &outBuffer)
			Expect(res).To(Equal(0), "smudge filter should succeed")
			Expect(outBuffer.Len()).To(BeEquivalentTo(lobinfo.Size), "extracted LOB data should be correct size")
		})

	})

	Describe("Clean filter", func() {

	})

})
