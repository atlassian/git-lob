package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Args", func() {
	var args []string
	var opts *Options
	var errors []string

	Describe("Checking command", func() {
		BeforeEach(func() {
			opts = NewOptions()
		})
		It("fails when command is missing", func() {
			args = []string{"git-lob"}
			opts := NewOptions()
			errors = parseCommandLine(opts, args)
			Expect(errors).ToNot(BeEmpty())
			// Command required, with other options
			args = []string{"git-lob", "--force", "-q"}
			errors = parseCommandLine(opts, args)
			Expect(errors).ToNot(BeEmpty())
		})
		It("succeeds when command is present", func() {
			args = []string{"git-lob", "lock"}
			errors = parseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(false))
			Expect(opts.Quiet).To(Equal(false))
			Expect(opts.Verbose).To(Equal(false))
			Expect(opts.DryRun).To(Equal(false))
			Expect(opts.NonInteractive).To(Equal(false))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
		It("detects short options", func() {
			args = []string{"git-lob", "lock", "-q", "-v", "-f", "-n"}
			errors = parseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(true))
			Expect(opts.Quiet).To(Equal(true))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.DryRun).To(Equal(false))
			Expect(opts.NonInteractive).To(Equal(true))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
		It("detects long options", func() {
			args = []string{"git-lob", "lock", "--quiet", "--verbose", "--force", "--noninteractive"}
			errors = parseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(true))
			Expect(opts.Quiet).To(Equal(true))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.DryRun).To(Equal(false))
			Expect(opts.NonInteractive).To(Equal(true))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
		It("accepts additional options", func() {
			args = []string{"git-lob", "lock", "--verbose", "--option1=foo", "--option2=bar"}
			errors = parseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(false))
			Expect(opts.Quiet).To(Equal(false))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.DryRun).To(Equal(false))
			Expect(opts.NonInteractive).To(Equal(false))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{"option1": "foo", "option2": "bar"}))
		})
		It("accepts additional arguments", func() {
			args = []string{"git-lob", "lock", "--verbose", "--dry-run", "file/one/test.jpg", "file/two/another.png"}
			errors = parseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Force).To(Equal(false))
			Expect(opts.Quiet).To(Equal(false))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.DryRun).To(Equal(true))
			Expect(opts.NonInteractive).To(Equal(false))
			Expect(opts.Args).To(Equal([]string{"file/one/test.jpg", "file/two/another.png"}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
		It("fails with invalid short option", func() {
			args = []string{"git-lob", "lock", "-x"}
			errors = parseCommandLine(opts, args)
			Expect(errors).ToNot(BeEmpty())
		})
		It("fails with invalid long option", func() {
			args = []string{"git-lob", "lock", "--invalidoption"}
			errors = parseCommandLine(opts, args)
			Expect(errors).ToNot(BeEmpty())
		})

	})

})
