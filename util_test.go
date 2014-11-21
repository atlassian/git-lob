package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Util", func() {
	Describe("Size methods", func() {

		It("formats sizes", func() {

			var str string
			str = FormatSize(55)
			Expect(str).To(Equal("55"))
			str = FormatSize(1024)
			Expect(str).To(Equal("1KB"))
			str = FormatSize(2000)
			Expect(str).To(Equal("1.95KB"))
			str = FormatSize(1572864)
			Expect(str).To(Equal("1.5MB"))
			str = FormatSize(157286400)
			Expect(str).To(Equal("150MB"))
			str = FormatSize(44023414784)
			Expect(str).To(Equal("41GB"))
			str = FormatSize(44475414800)
			Expect(str).To(Equal("41.4GB"))
			str = FormatSize(1319413953331)
			Expect(str).To(Equal("1.2TB"))
			str = FormatSize(395824185999360)
			Expect(str).To(Equal("360TB"))
			str = FormatSize(2260595906707456)
			Expect(str).To(Equal("2.01PB"))

		})
		It("parses sizes", func() {
			var val int64
			var err error
			val, err = ParseSize("5a67")
			Expect(err).ToNot(BeNil(), "Should not parse")
			val, err = ParseSize("567")
			Expect(err).To(BeNil(), "Should parse without error")
			Expect(val).To(BeEquivalentTo(567))
			val, err = ParseSize("567B")
			Expect(err).To(BeNil(), "Should parse without error")
			Expect(val).To(BeEquivalentTo(567))
			val, err = ParseSize("567b")
			Expect(err).To(BeNil(), "Should parse without error")
			Expect(val).To(BeEquivalentTo(567))
			val, err = ParseSize(" 567 B ")
			Expect(err).To(BeNil(), "Should parse without error")
			Expect(val).To(BeEquivalentTo(567))
			val, err = ParseSize("1KB")
			Expect(err).To(BeNil(), "Should parse without error")
			Expect(val).To(BeEquivalentTo(1024))
			val, err = ParseSize("2.5KB")
			Expect(err).To(BeNil(), "Should parse without error")
			Expect(val).To(BeEquivalentTo(2560))
			val, err = ParseSize("5.25M")
			Expect(err).To(BeNil(), "Should parse without error")
			Expect(val).To(BeEquivalentTo(5505024))
			val, err = ParseSize("75.0Gb")
			Expect(err).To(BeNil(), "Should parse without error")
			Expect(val).To(BeEquivalentTo(80530636800))
			val, err = ParseSize("300T")
			Expect(err).To(BeNil(), "Should parse without error")
			Expect(val).To(BeEquivalentTo(329853488332800))
			val, err = ParseSize("1.5pb")
			Expect(err).To(BeNil(), "Should parse without error")
			Expect(val).To(BeEquivalentTo(1688849860263936))
		})

	})

})
