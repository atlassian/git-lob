package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
	"path/filepath"
)

var _ = Describe("Links", func() {
	var target string
	var links []string
	BeforeEach(func() {
		tmp := os.TempDir()
		target = filepath.Join(tmp, "linktarget.txt")
		links = make([]string, 0, 5)
		links = append(links, filepath.Join(tmp, "link1.txt"))
		links = append(links, filepath.Join(tmp, "link2.txt"))
		links = append(links, filepath.Join(tmp, "link3.txt"))
		links = append(links, filepath.Join(tmp, "link4.txt"))
		links = append(links, filepath.Join(tmp, "link5.txt"))

		ioutil.WriteFile(target, []byte("Something something"), 0666)
	})

	AfterEach(func() {
		os.Remove(target)
		for _, f := range links {
			os.Remove(f)
		}

	})

	It("creates and counts hard links", func() {
		var err error
		var lc int

		lc, err = GetHardLinkCount(target)
		Expect(err).To(BeNil(), "GetHardLinkCount should not fail")
		Expect(lc).To(BeEquivalentTo(1), "Hard link count should be 1 to start with")

		err = CreateHardLink(target, links[0])
		Expect(err).To(BeNil(), "CreateHardLink should not fail")
		lc, err = GetHardLinkCount(target)
		Expect(err).To(BeNil(), "GetHardLinkCount should not fail")
		Expect(lc).To(BeEquivalentTo(2), "Hard link count incorrect")

		err = CreateHardLink(target, links[1])
		Expect(err).To(BeNil(), "CreateHardLink should not fail")
		lc, err = GetHardLinkCount(target)
		Expect(err).To(BeNil(), "GetHardLinkCount should not fail")
		Expect(lc).To(BeEquivalentTo(3), "Hard link count incorrect")

		err = CreateHardLink(target, links[2])
		Expect(err).To(BeNil(), "CreateHardLink should not fail")
		lc, err = GetHardLinkCount(target)
		Expect(err).To(BeNil(), "GetHardLinkCount should not fail")
		Expect(lc).To(BeEquivalentTo(4), "Hard link count incorrect")

		err = CreateHardLink(target, links[3])
		Expect(err).To(BeNil(), "CreateHardLink should not fail")
		lc, err = GetHardLinkCount(target)
		Expect(err).To(BeNil(), "GetHardLinkCount should not fail")
		Expect(lc).To(BeEquivalentTo(5), "Hard link count incorrect")

		err = CreateHardLink(target, links[4])
		Expect(err).To(BeNil(), "CreateHardLink should not fail")
		lc, err = GetHardLinkCount(target)
		Expect(err).To(BeNil(), "GetHardLinkCount should not fail")
		Expect(lc).To(BeEquivalentTo(6), "Hard link count incorrect")

		os.Remove(links[2])
		os.Remove(links[4])
		lc, err = GetHardLinkCount(target)
		Expect(err).To(BeNil(), "GetHardLinkCount should not fail")
		Expect(lc).To(BeEquivalentTo(4), "Hard link count incorrect after removal")

		os.Remove(target)
		lc, err = GetHardLinkCount(links[0])
		Expect(err).To(BeNil(), "GetHardLinkCount should not fail")
		Expect(lc).To(BeEquivalentTo(3), "Hard link count incorrect after removal of target")

	})
})
