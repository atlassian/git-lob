package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Args", func() {
	var args []string
	var opts *CommandLineOptions
	var ok bool

	Describe("Checking command", func() {
		It("fails when command is missing", func() {
			args = []string{"git-lob"}
			opts, ok = parseCommandLine(args)
			Expect(ok).To(Equal(false))
			// Command required, with other options
			args = []string{"git-lob", "--force", "-q"}
			opts, ok = parseCommandLine(args)
			Expect(ok).To(Equal(false))
		})
		It("succeeds when command is present", func() {
			args = []string{"git-lob", "lock"}
			opts, ok = parseCommandLine(args)
			Expect(ok).To(Equal(true))
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(false))
			Expect(opts.Quiet).To(Equal(false))
			Expect(opts.Verbose).To(Equal(false))
			Expect(opts.NonInteractive).To(Equal(false))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
	})

})
