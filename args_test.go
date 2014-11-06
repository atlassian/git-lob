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
		It("detects short options", func() {
			args = []string{"git-lob", "lock", "-q", "-v", "-f", "-n"}
			opts, ok = parseCommandLine(args)
			Expect(ok).To(Equal(true))
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(true))
			Expect(opts.Quiet).To(Equal(true))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.NonInteractive).To(Equal(true))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
		It("detects long options", func() {
			args = []string{"git-lob", "lock", "--quiet", "--verbose", "--force", "--noninteractive"}
			opts, ok = parseCommandLine(args)
			Expect(ok).To(Equal(true))
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(true))
			Expect(opts.Quiet).To(Equal(true))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.NonInteractive).To(Equal(true))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
		It("accepts additional options", func() {
			args = []string{"git-lob", "lock", "--verbose", "--option1=foo", "--option2=bar"}
			opts, ok = parseCommandLine(args)
			Expect(ok).To(Equal(true))
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(false))
			Expect(opts.Quiet).To(Equal(false))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.NonInteractive).To(Equal(false))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{"option1": "foo", "option2": "bar"}))
		})
		It("accepts additional arguments", func() {
			args = []string{"git-lob", "lock", "--verbose", "file/one/test.jpg", "file/two/another.png"}
			opts, ok = parseCommandLine(args)
			Expect(ok).To(Equal(true))
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(false))
			Expect(opts.Quiet).To(Equal(false))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.NonInteractive).To(Equal(false))
			Expect(opts.Args).To(Equal([]string{"file/one/test.jpg", "file/two/another.png"}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
	})

})
