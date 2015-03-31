package core

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"os"
	"path/filepath"
	"time"
)

var _ = Describe("Remote", func() {
	Context("Simple storage of remote state cache", func() {

		root := filepath.Join(os.TempDir(), "RemoteStateTest")
		remote1Name := "origin"
		remote2Name := "fork"
		var oldwd string
		BeforeEach(func() {
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)
		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
		})

		It("saves and restores push state", func() {

			sha := "b09bfdf65bb51bb50307f93ab930dd7708a5b6dc"
			sha2 := "c1234567890fdf651bb5f93ab930dd7708002341"
			sha3 := "d3f8734986de0f0a08b5f93ab930dd7708030dde"

			err := MarkBinariesAsPushed(remote1Name, sha, "")
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			pushed := GetPushedCommits(remote1Name)
			Expect(pushed).To(Equal([]string{sha}), "Should record pushed")

			err = MarkBinariesAsPushed(remote1Name, sha2, "")
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			pushed = GetPushedCommits(remote1Name)
			Expect(pushed).To(Equal([]string{sha, sha2}), "Should record pushed, in order")

			// Add a duplicate, confirm not added but replaced
			err = MarkBinariesAsPushed(remote1Name, sha, "")
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			pushed = GetPushedCommits(remote1Name)
			Expect(pushed).To(Equal([]string{sha, sha2}), "Should not add duplicates")

			// now replace sha with sha3
			err = MarkBinariesAsPushed(remote1Name, sha3, sha)
			Expect(err).To(BeNil(), "Shouldn't be an error marking pushed")

			pushed = GetPushedCommits(remote1Name)
			Expect(pushed).To(Equal([]string{sha2, sha3}), "Should record pushed, replacing first one, but in order")

			pushed = GetPushedCommits(remote2Name)
			Expect(pushed).To(Equal([]string{}), "Pushed should be empty for others")

			// Now undo
			err = ResetPushedBinaryState(remote1Name)
			Expect(err).To(BeNil(), "Shouldn't be an error undoing pushed")
			pushed = GetPushedCommits(remote1Name)
			Expect(pushed).To(Equal([]string{}), "Pushed should be empty after reset")
		})
	})

	Context("Real git repo tests", func() {
		root := filepath.Join(os.TempDir(), "RemoteStateTest")
		var oldwd string
		var setupInputs []*TestCommitSetupInput
		var setupOutputs []*CommitLOBRef
		var getOutputSubset = func(indexes ...int) []*CommitLOBRef {
			ret := make([]*CommitLOBRef, 0, len(indexes))
			for _, i := range indexes {
				ret = append(ret, setupOutputs[i])
			}
			return ret
		}
		BeforeEach(func() {
			CreateGitRepoForTest(root)
			oldwd, _ = os.Getwd()
			os.Chdir(root)

			now := time.Now()
			// Create a relatively interesting git repo with a few branches, merges
			setupInputs = []*TestCommitSetupInput{
				&TestCommitSetupInput{ // 0
					CommitDate: now.AddDate(0, 0, -29),
					Files:      []string{"data1.bin", filepath.Join("img", "image1.jpg")},
				},
				&TestCommitSetupInput{ // 1
					CommitDate: now.AddDate(0, 0, -28),
					Files:      []string{"data2.bin", filepath.Join("img", "image2.jpg")},
				},
				// branch, modify & add
				// this branch we'll leave hanging
				&TestCommitSetupInput{ // 2
					CommitDate: now.AddDate(0, 0, -27),
					Files:      []string{"data1.bin", filepath.Join("bigdata", "something.dat")},
					NewBranch:  "feature/hanging",
				},
				&TestCommitSetupInput{ // 3
					CommitDate:     now.AddDate(0, 0, -23),
					Files:          []string{"data3.bin", "data4.bin", "data5.bin"},
					ParentBranches: []string{"feature/hanging"},
				},
				// now back on master
				&TestCommitSetupInput{ // 4
					CommitDate:     now.AddDate(0, 0, -25),
					Files:          []string{"data2.bin", "newfile1.dat", "newfile2.dat"},
					ParentBranches: []string{"master"},
				},
				&TestCommitSetupInput{ // 5
					CommitDate: now.AddDate(0, 0, -24),
					Files:      []string{"mergedata.bin", "mergedata2.dat"},
					NewBranch:  "feature/tomerge",
				},
				// add a few parallel commits on master
				&TestCommitSetupInput{ // 6
					CommitDate:     now.AddDate(0, 0, -24),
					Files:          []string{"parallel.dat"},
					ParentBranches: []string{"master"}, // unnecessary but make sure
				},
				&TestCommitSetupInput{ // 7
					CommitDate:     now.AddDate(0, 0, -23),
					Files:          []string{"mergedata.bin", "mergedata3.dat"},
					ParentBranches: []string{"feature/tomerge"},
				},
				// add a few parallel commits on master
				&TestCommitSetupInput{ // 8
					CommitDate:     now.AddDate(0, 0, -23),
					Files:          []string{"parallel2.dat"},
					ParentBranches: []string{"master"}, // unnecessary but make sure
				},
				// now merge (no changes added except from merge)
				&TestCommitSetupInput{ // 9
					CommitDate:     now.AddDate(0, 0, -22),
					ParentBranches: []string{"master", "feature/tomerge"},
				},
				// one more commit on master
				&TestCommitSetupInput{ // 10
					CommitDate:     now.AddDate(0, 0, -20),
					Files:          []string{filepath.Join("img", "image30.jpg")},
					ParentBranches: []string{"master"}, // unnecessary but make sure
				},
				// commit with no files
				&TestCommitSetupInput{ // 11
					CommitDate:     now.AddDate(0, 0, -20),
					ParentBranches: []string{"master"}, // unnecessary but make sure
				},
				// now one more hanging branch ahead of  master
				&TestCommitSetupInput{ // 12
					CommitDate:     now.AddDate(0, 0, -10),
					Files:          []string{filepath.Join("img", "image31.jpg")},
					ParentBranches: []string{"master"}, // unnecessary but make sure
					NewBranch:      "feature/hanging_ahead",
				},
				// one more commit hanging
				&TestCommitSetupInput{ // 13
					CommitDate:     now.AddDate(0, 0, -9),
					Files:          []string{filepath.Join("img", "image40.jpg")},
					ParentBranches: []string{"feature/hanging_ahead"}, // unnecessary but make sure
				},
			}

			setupOutputs = SetupRepoForTest(setupInputs)

		})
		AfterEach(func() {
			os.Chdir(oldwd)
			os.RemoveAll(root)
		})

		It("traces pushed ancestors", func() {
			remote := "origin"

			// Check that no-match case works & terminates correctly
			sha, err := FindLatestAncestorWhereBinariesPushed(remote, "master")
			Expect(err).To(BeNil())
			Expect(sha).To(Equal(""), "Should be no pushed binaries at start")

			// Mark pushed at feature/hanging but not at tip
			err = MarkBinariesAsPushed(remote, setupOutputs[2].Commit, "")
			Expect(err).To(BeNil())
			// The ancestor that's pushed should be the common parent of hanging branch & master,
			// which is index 1
			sha, err = FindLatestAncestorWhereBinariesPushed(remote, "master")
			Expect(err).To(BeNil())
			Expect(sha).To(Equal(setupOutputs[1].Commit), "Should find common ancestor that's pushed")

			// Mark a commit on master as pushed too
			err = MarkBinariesAsPushed(remote, setupOutputs[4].Commit, "")
			Expect(err).To(BeNil())
			// The ancestor that's pushed from master should now be this one
			sha, err = FindLatestAncestorWhereBinariesPushed(remote, "master")
			Expect(err).To(BeNil())
			Expect(sha).To(Equal(setupOutputs[4].Commit), "Should find latest ancestor that's pushed")
			// but if measured from hanging branch this should be its own
			sha, err = FindLatestAncestorWhereBinariesPushed(remote, "feature/hanging")
			Expect(err).To(BeNil())
			Expect(sha).To(Equal(setupOutputs[2].Commit), "Should find hanging branch ancestor that's pushed")

			// Now mark a commit on the merge line as pushed, but not where it's merged
			// Again don't mark the tip to make the test more interesting
			err = MarkBinariesAsPushed(remote, setupOutputs[5].Commit, "")
			Expect(err).To(BeNil())
			// The ancestor that's pushed from master should now be the one in the middle of the merge
			sha, err = FindLatestAncestorWhereBinariesPushed(remote, "master")
			Expect(err).To(BeNil())
			Expect(sha).To(Equal(setupOutputs[5].Commit), "Should find merge ancestor that's pushed")
			// but should not affect hanging branch this should be its own
			sha, err = FindLatestAncestorWhereBinariesPushed(remote, "feature/hanging")
			Expect(err).To(BeNil())
			Expect(sha).To(Equal(setupOutputs[2].Commit), "Should find hanging branch ancestor that's pushed")
			// However even though merge ancestor is 'latest pushed' from master, we should still see commits on master in parallel with merge to push
			commits, err := GetCommitLOBsToPushForRef(remote, "master", false)
			Expect(err).To(BeNil())
			// don't include commits with no files e.g. 9(merge) & 11
			Expect(commits).To(ConsistOf(getOutputSubset(6, 7, 8, 10)), "Should be correct list to push")
			// now mark something in the middle on master as pushed & make sure remnants of merge branch still get picked up
			// Also replace the previously marked SHA on master because assume we picked this up from master (since merged) but didn't finish
			err = MarkBinariesAsPushed(remote, setupOutputs[8].Commit, setupOutputs[4].Commit)
			Expect(err).To(BeNil())
			// The ancestor that's pushed from master should now be this one
			sha, err = FindLatestAncestorWhereBinariesPushed(remote, "master")
			Expect(err).To(BeNil())
			Expect(sha).To(Equal(setupOutputs[8].Commit), "Should find master ancestor that's pushed")
			// Now test we pick up the commits from the merge branch on master, after what was marked there
			commits, err = GetCommitLOBsToPushForRef(remote, "master", false)
			Expect(err).To(BeNil())
			// don't include commits with no files e.g. 9(merge) & 11
			Expect(commits).To(ConsistOf(getOutputSubset(7, 10)), "Should be correct list to push")

		})

		It("consolidates pushed state", func() {
			// This time we're just inserting push markers and making sure that they are consolidated correctly
			remote := "origin"
			err := MarkBinariesAsPushed(remote, setupOutputs[0].Commit, "") // redundant since 4 is descendent
			Expect(err).To(BeNil())
			err = MarkBinariesAsPushed(remote, setupOutputs[4].Commit, "")
			Expect(err).To(BeNil())
			err = MarkBinariesAsPushed(remote, setupOutputs[3].Commit, "") // 3 is on a separate branch
			Expect(err).To(BeNil())
			CleanupPushState(remote)
			pushed := GetPushedCommits(remote)
			Expect(pushed).To(ConsistOf([]string{setupOutputs[4].Commit, setupOutputs[3].Commit}), "Should still record 2 pushed states")

			err = MarkBinariesAsPushed(remote, setupOutputs[7].Commit, "") // on merge path
			Expect(err).To(BeNil())
			err = MarkBinariesAsPushed(remote, setupOutputs[8].Commit, "") // in parallel on master (overrides 4)
			Expect(err).To(BeNil())
			CleanupPushState(remote)
			pushed = GetPushedCommits(remote)
			Expect(pushed).To(ConsistOf([]string{setupOutputs[8].Commit, setupOutputs[3].Commit, setupOutputs[7].Commit}),
				"Should record 3 pushed states; master, hanging and merge path")
			// Now once we say pushed after merge commit point, should resolve back to 2 again
			err = MarkBinariesAsPushed(remote, setupOutputs[10].Commit, "") // post-merge
			Expect(err).To(BeNil())
			CleanupPushState(remote)
			pushed = GetPushedCommits(remote)
			Expect(pushed).To(ConsistOf([]string{setupOutputs[10].Commit, setupOutputs[3].Commit}),
				"Should be nack to 2 pushed states; master & hanging")
		})

		It("copes with bad commit SHAs", func() {
			// Possible tha a SHA in the push list has been orphaned & gc'd so some commands including 'git log' will barf
			// Make sure that we can recover correctly
			remote := "origin"
			// Add valid post-merge push point (hanging is not pushed)
			err := MarkBinariesAsPushed(remote, setupOutputs[9].Commit, "")
			Expect(err).To(BeNil())
			// Now add a completely incorrect SHA
			err = MarkBinariesAsPushed(remote, "1111111111222222222233333333334444444444", "")
			Expect(err).To(BeNil())
			// Now let's check that we recover from the inevitable problem
			commits, err := GetCommitLOBsToPushForRef(remote, "master", false)
			Expect(err).To(BeNil())
			// Should report the commits on master/hanging_ahead past 9 which have LOBs
			Expect(commits).To(ConsistOf(getOutputSubset(10)), "Should correctly identify commits to push after bad SHA")
			commits, err = GetCommitLOBsToPushForRef(remote, "feature/hanging_ahead", false)
			Expect(err).To(BeNil())
			// Should report the commits on master/hanging_ahead past 9 which have LOBs
			Expect(commits).To(ConsistOf(getOutputSubset(10, 12, 13)), "Should correctly identify commits to push after bad SHA")

		})

	})

})
