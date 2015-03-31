package core

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	. "bitbucket.org/sinbad/git-lob/util"
	"io/ioutil"
	"os"
	"path/filepath"
)

var _ = Describe("Missing", func() {

	root := filepath.Join(os.TempDir(), "MissingTest")
	var oldwd string
	var setupInputs []*TestCommitSetupInput
	var setupOutputs []*CommitLOBRef

	BeforeEach(func() {
		oldwd, _ = os.Getwd()
		CreateGitRepoForTest(root)
		os.Chdir(root)

		setupInputs = []*TestCommitSetupInput{
			&TestCommitSetupInput{ // 0
				Files:          []string{"file1.bin", "file2.bin"},
				CommitterName:  "Barry",
				CommitterEmail: "baz@foo.com",
			},
			&TestCommitSetupInput{ // 1
				Files:          []string{filepath.Join("fld", "file3.bin"), filepath.Join("fld", "file4.bin")},
				CommitterName:  "Nigel",
				CommitterEmail: "nig@foo.com",
			},
			&TestCommitSetupInput{ // 2
				Files:          []string{"file1.bin"},
				CommitterName:  "Nigel",
				CommitterEmail: "nig@foo.com",
			},
			&TestCommitSetupInput{ // 3
				Files:          []string{filepath.Join("fld", "large", "large1.bin")},
				FileSizes:      []int64{ChunkSize + 30},
				CommitterName:  "Barry",
				CommitterEmail: "baz@foo.com",
			},
			&TestCommitSetupInput{ // 4
				Files:          []string{filepath.Join("fld", "file5.bin")},
				CommitterName:  "Barry",
				CommitterEmail: "baz@foo.com",
			},
			&TestCommitSetupInput{ // 5
				Files:          []string{"file4.bin", "file5.bin"},
				CommitterName:  "Barry",
				CommitterEmail: "baz@foo.com",
			},
		}

		setupOutputs = SetupRepoForTest(setupInputs)

		// At this point all files are placeholders, because that's how SetupRepoForTest works (to avoid filters)
		// Checkout all files before we break things
		Expect(Checkout(nil, false, func(t ProgressCallbackType, filelob *FileLOB, err error) {})).To(BeNil())
		// Now make some of these files placeholders
		// Case 1. placeholder for missing data (small file and missing chunk) - blamed
		// ./file1.bin
		Expect(ioutil.WriteFile(setupInputs[2].Files[0], []byte(getLOBPlaceholderContent(setupOutputs[2].LobSHAs[0])), 0644)).To(BeNil())
		Expect(os.Remove(GetLocalLOBMetaPath(setupOutputs[2].LobSHAs[0]))).To(BeNil())     // remove meta
		Expect(os.Remove(GetLocalLOBChunkPath(setupOutputs[2].LobSHAs[0], 0))).To(BeNil()) // remove chunk 1
		// ./fld/large/large1.bin
		Expect(ioutil.WriteFile(setupInputs[3].Files[0], []byte(getLOBPlaceholderContent(setupOutputs[3].LobSHAs[0])), 0644)).To(BeNil())
		Expect(os.Remove(GetLocalLOBChunkPath(setupOutputs[3].LobSHAs[0], 1))).To(BeNil()) // remove chunk 2 (partial)

		// Case 2. placeholder for corrupt data
		// for this one we just mess up the metadata
		// ./fld/file4.bin
		Expect(ioutil.WriteFile(setupInputs[1].Files[1], []byte(getLOBPlaceholderContent(setupOutputs[1].LobSHAs[1])), 0644)).To(BeNil())
		Expect(ioutil.WriteFile(GetLocalLOBMetaPath(setupOutputs[1].LobSHAs[1]), []byte("{ garbage }"), 0644)).To(BeNil()) // corrupt meta

		// Case 3. placeholder locally modified (random SHA)
		// ./file4.bin
		Expect(ioutil.WriteFile(setupInputs[5].Files[0], []byte(getLOBPlaceholderContent("1111111111111111111111111111111111111111")), 0644)).To(BeNil())

		// Case 4. placeholder with data available
		// ./file5.bin
		Expect(ioutil.WriteFile(setupInputs[5].Files[1], []byte(getLOBPlaceholderContent(setupOutputs[5].LobSHAs[1])), 0644)).To(BeNil())

	})
	AfterEach(func() {
		os.Chdir(oldwd)
		err := ForceRemoveAll(root)
		if err != nil {
			Fail(err.Error())
		}
	})

	It("Checks missing correctly", func() {
		var responses []*MissingCallbackData
		callback := func(data *MissingCallbackData) (quit bool) {
			if data.Type != MissingWorking {
				responses = append(responses, data)
			}
			return false
		}
		Missing(false, nil, callback)
		Expect(responses).To(HaveLen(5), "Should be 5 callbacks")
		// Doing a direct comparison with commit summaries is unusable when it fails because time.Time spits out
		// so much output it floods everything. So pick out specifics to test for
		findResponseForPath := func(path string) *MissingCallbackData {
			for _, resp := range responses {
				if resp.Path == path {
					return resp
				}
			}
			return nil
		}
		// Case 1a
		r := findResponseForPath(setupInputs[2].Files[0])
		Expect(r).ToNot(BeNil(), "Should find response for"+setupInputs[2].Files[0])
		Expect(r.Type).To(BeEquivalentTo(MissingBlamed), "Should be blamed")
		Expect(r.CommitSummary).ToNot(BeNil(), "Should have commit details")
		Expect(r.CommitSummary.SHA).To(Equal(setupOutputs[2].Commit), "Should blame correct commit")
		Expect(r.CommitSummary.CommitterName).To(Equal("Nigel"), "Should blame correct person")
		Expect(r.CommitSummary.CommitterEmail).To(Equal("nig@foo.com"), "Should blame correct person")
		// Case 1b
		r = findResponseForPath(setupInputs[3].Files[0])
		Expect(r).ToNot(BeNil(), "Should find response for"+setupInputs[3].Files[0])
		Expect(r.Type).To(BeEquivalentTo(MissingBlamed), "Should be blamed")
		Expect(r.CommitSummary).ToNot(BeNil(), "Should have commit details")
		Expect(r.CommitSummary.SHA).To(Equal(setupOutputs[3].Commit), "Should blame correct commit")
		Expect(r.CommitSummary.CommitterName).To(Equal("Barry"), "Should blame correct person")
		Expect(r.CommitSummary.CommitterEmail).To(Equal("baz@foo.com"), "Should blame correct person")
		// Case 2
		r = findResponseForPath(setupInputs[1].Files[1])
		Expect(r).ToNot(BeNil(), "Should find response for"+setupInputs[1].Files[1])
		Expect(r.Type).To(BeEquivalentTo(MissingCorrupt), "Should be corrupt")
		// Case 3
		r = findResponseForPath(setupInputs[5].Files[0])
		Expect(r).ToNot(BeNil(), "Should find response for"+setupInputs[5].Files[0])
		Expect(r.Type).To(BeEquivalentTo(MissingModified), "Should be locally modified")
		// Case 4
		r = findResponseForPath(setupInputs[5].Files[1])
		Expect(r).ToNot(BeNil(), "Should find response for"+setupInputs[5].Files[1])
		Expect(r.Type).To(BeEquivalentTo(MissingAvailable), "Should be a placeholder but available")

		content, err := ioutil.ReadFile(setupInputs[5].Files[1])
		Expect(err).To(BeNil(), "Should be ok to read placeholder")
		Expect(string(content)).To(Equal(getLOBPlaceholderContent(setupOutputs[5].LobSHAs[1])), "Available file should still be a placeholder")

		// Now do the same again but checkout=true
		responses = nil
		Missing(true, nil, callback)
		Expect(responses).To(HaveLen(5), "Should be 5 callbacks")
		// Case 1a
		r = findResponseForPath(setupInputs[2].Files[0])
		Expect(r).ToNot(BeNil(), "Should find response for"+setupInputs[2].Files[0])
		Expect(r.Type).To(BeEquivalentTo(MissingBlamed), "Should be blamed")
		Expect(r.CommitSummary).ToNot(BeNil(), "Should have commit details")
		Expect(r.CommitSummary.SHA).To(Equal(setupOutputs[2].Commit), "Should blame correct commit")
		Expect(r.CommitSummary.CommitterName).To(Equal("Nigel"), "Should blame correct person")
		Expect(r.CommitSummary.CommitterEmail).To(Equal("nig@foo.com"), "Should blame correct person")
		// Case 1b
		r = findResponseForPath(setupInputs[3].Files[0])
		Expect(r).ToNot(BeNil(), "Should find response for"+setupInputs[3].Files[0])
		Expect(r.Type).To(BeEquivalentTo(MissingBlamed), "Should be blamed")
		Expect(r.CommitSummary).ToNot(BeNil(), "Should have commit details")
		Expect(r.CommitSummary.SHA).To(Equal(setupOutputs[3].Commit), "Should blame correct commit")
		Expect(r.CommitSummary.CommitterName).To(Equal("Barry"), "Should blame correct person")
		Expect(r.CommitSummary.CommitterEmail).To(Equal("baz@foo.com"), "Should blame correct person")
		// Case 2
		r = findResponseForPath(setupInputs[1].Files[1])
		Expect(r).ToNot(BeNil(), "Should find response for"+setupInputs[1].Files[1])
		Expect(r.Type).To(BeEquivalentTo(MissingCorrupt), "Should be corrupt")
		// Case 3
		r = findResponseForPath(setupInputs[5].Files[0])
		Expect(r).ToNot(BeNil(), "Should find response for"+setupInputs[5].Files[0])
		Expect(r.Type).To(BeEquivalentTo(MissingModified), "Should be locally modified")
		// Case 4
		r = findResponseForPath(setupInputs[5].Files[1])
		Expect(r).ToNot(BeNil(), "Should find response for"+setupInputs[5].Files[1])
		Expect(r.Type).To(BeEquivalentTo(MissingFixed), "Available file should have been fixed")

		stat, err := os.Stat(setupInputs[5].Files[1])
		Expect(err).To(BeNil(), "Should be ok to read placeholder")
		Expect(stat.Size()).ToNot(BeEquivalentTo(SHALineLen), "Available file should have been replaced with real data")

		// Now test using path matching
		var filenames []string
		nameCallback := func(data *MissingCallbackData) (quit bool) {
			if data.Type != MissingWorking {
				filenames = append(filenames, data.Path)
			}
			return false
		}
		Missing(false, []string{"fld"}, nameCallback)
		Expect(filenames).To(ConsistOf([]string{
			filepath.Join("fld", "large", "large1.bin"),
			filepath.Join("fld", "file4.bin")}), "Should have correct filtered responses")

		// Now test using path matching - wildcard
		filenames = nil
		Missing(false, []string{"file*"}, nameCallback)
		// file5.bin has already been fixed, and shouldn't match anything in subdirs
		Expect(filenames).To(ConsistOf([]string{
			"file1.bin", "file4.bin"}), "Should have correct filtered responses")

		// Now test using path matching - wildcard with folder
		filenames = nil
		Missing(false, []string{"*/*.bin"}, nameCallback)
		// only 1 level down
		Expect(filenames).To(ConsistOf([]string{
			filepath.Join("fld", "file4.bin")}), "Should have correct filtered responses")
		//Now multiple matches
		filenames = nil
		Missing(false, []string{"fld", "file*.bin"}, nameCallback)
		// This should match everything
		Expect(filenames).To(ConsistOf([]string{
			filepath.Join("fld", "large", "large1.bin"),
			filepath.Join("fld", "file4.bin"),
			"file1.bin", "file4.bin",
		}), "Should have correct filtered responses")
	})

})
