package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Args", func() {
	var args []string
	var opts *CommandLineOptions
	var errors []string

	Describe("Checking command", func() {
		It("fails when command is missing", func() {
			args = []string{"git-lob"}
			opts, errors = parseCommandLine(args)
			Expect(errors).ToNot(BeEmpty())
			// Command required, with other options
			args = []string{"git-lob", "--force", "-q"}
			opts, errors = parseCommandLine(args)
			Expect(errors).ToNot(BeEmpty())
		})
		It("succeeds when command is present", func() {
			args = []string{"git-lob", "lock"}
			opts, errors = parseCommandLine(args)
			Expect(errors).To(BeEmpty())
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
			opts, errors = parseCommandLine(args)
			Expect(errors).To(BeEmpty())
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
			opts, errors = parseCommandLine(args)
			Expect(errors).To(BeEmpty())
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
			opts, errors = parseCommandLine(args)
			Expect(errors).To(BeEmpty())
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
			opts, errors = parseCommandLine(args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(false))
			Expect(opts.Quiet).To(Equal(false))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.NonInteractive).To(Equal(false))
			Expect(opts.Args).To(Equal([]string{"file/one/test.jpg", "file/two/another.png"}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
		It("fails with invalid short option", func() {
			args = []string{"git-lob", "lock", "-x"}
			opts, errors = parseCommandLine(args)
			Expect(errors).ToNot(BeEmpty())
		})
		It("fails with invalid long option", func() {
			args = []string{"git-lob", "lock", "--invalidoption"}
			opts, errors = parseCommandLine(args)
			Expect(errors).ToNot(BeEmpty())
		})

	})

})
