package core

import (
	"bytes"
	"os"
	"path"

	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	. "github.com/atlassian/git-lob/util"
)

var _ = Describe("Filter", func() {

	root := path.Join(os.TempDir(), "StorageTest")
	var oldwd string
	BeforeEach(func() {
		// Set up git repo with some subfolders
		CreateGitRepoForTest(root)
		oldwd, _ = os.Getwd()
		os.Chdir(root)

	})

	AfterEach(func() {
		os.Chdir(oldwd)
		// Delete repo
		err := ForceRemoveAll(root)
		if err != nil {
			Fail(err.Error())
		}
	})

	Describe("Smudge filter", func() {

		It("doesn't alter non-LOB content", func() {
			nonLOBString := `This is some data
in a string
that we should absolutely not mess with`
			inBuffer := bytes.NewBufferString(nonLOBString)
			var outBuffer bytes.Buffer
			res := SmudgeFilterWithReaderWriter(inBuffer, &outBuffer, "testfile.txt")
			Expect(res).To(Equal(0), "smudge filter should succeed")
			Expect(outBuffer.String()).To(BeEquivalentTo(nonLOBString), "non LOB should not be modified by smudge")
		})

		It("doesn't alter LOB content when LOB isn't present in object store & no autodownloading", func() {
			GlobalOptions.AutoFetchEnabled = false
			// Made up SHA that doesn't exist
			lobString := SHAPrefix + "0123456789abcdef0123456789abcdef01234567"
			inBuffer := bytes.NewBufferString(lobString)
			var outBuffer bytes.Buffer
			res := SmudgeFilterWithReaderWriter(inBuffer, &outBuffer, "testfile.txt")
			Expect(res).To(Equal(0), "smudge filter should succeed")
			Expect(outBuffer.String()).To(BeEquivalentTo(lobString), "non existent LOB should not be modified by smudge")
		})

		It("writes real LOB data for small file", func() {
			lobinfo := CreateSmallTestLOBDataForRetrieval()
			lobString := SHAPrefix + lobinfo.SHA
			inBuffer := bytes.NewBufferString(lobString)
			var outBuffer bytes.Buffer
			res := SmudgeFilterWithReaderWriter(inBuffer, &outBuffer, "testfile.txt")
			Expect(res).To(Equal(0), "smudge filter should succeed")
			Expect(outBuffer.Len()).To(BeEquivalentTo(lobinfo.Size), "extracted LOB data should be correct size")
		})

		It("writes real LOB data for large file [LONGTEST]", func() {
			lobinfo := CreateLargeTestLOBDataForRetrieval()
			lobString := SHAPrefix + lobinfo.SHA
			inBuffer := bytes.NewBufferString(lobString)
			var outBuffer bytes.Buffer
			res := SmudgeFilterWithReaderWriter(inBuffer, &outBuffer, "testfile.txt")
			Expect(res).To(Equal(0), "smudge filter should succeed")
			Expect(outBuffer.Len()).To(BeEquivalentTo(lobinfo.Size), "extracted LOB data should be correct size")
		})

	})

	Describe("Clean filter", func() {

		It("doesn't change unexpanded LOB content", func() {
			// This is where a git-lob reference didn't find the binary in the store so just wrote the
			// committed LOB reference to the working copy
			lobString := SHAPrefix + "0123456789abcdef0123456789abcdef01234567"
			inBuffer := bytes.NewBufferString(lobString)
			var outBuffer bytes.Buffer
			res := CleanFilterWithReaderWriter(inBuffer, &outBuffer, "testfile.txt")
			Expect(res).To(Equal(0), "clean filter should succeed")
			Expect(outBuffer.String()).To(BeEquivalentTo(lobString), "unexpanded LOB should not be modified by clean")

		})

		It("writes LOB data to store and outputs reference", func() {
			testFileName := path.Join(root, "small.dat")
			info := CreateSmallTestLOBFileForStoring(testFileName)
			in, _ := os.OpenFile(testFileName, os.O_RDONLY, 0644)
			var outBuffer bytes.Buffer
			res := CleanFilterWithReaderWriter(in, &outBuffer, "testfile.txt")
			Expect(res).To(Equal(0), "clean filter should succeed")
			Expect(outBuffer.String()).To(BeEquivalentTo(SHAPrefix+info.SHA), "clean filter should output SHA reference")
			readinfo, _ := GetLOBInfo(info.SHA)
			Expect(readinfo).To(Equal(info))

		})

	})

})
