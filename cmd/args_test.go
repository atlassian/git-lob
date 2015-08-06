package cmd

import (
	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	. "github.com/atlassian/git-lob/util"
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
			errors = ParseCommandLine(opts, args)
			Expect(errors).ToNot(BeEmpty())
			// Command required, with other options
			args = []string{"git-lob", "--verbose", "-q"}
			errors = ParseCommandLine(opts, args)
			Expect(errors).ToNot(BeEmpty())
		})
		It("succeeds when command is present", func() {
			args = []string{"git-lob", "lock"}
			errors = ParseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Quiet).To(Equal(false))
			Expect(opts.Verbose).To(Equal(false))
			Expect(opts.DryRun).To(Equal(false))
			Expect(opts.NonInteractive).To(Equal(false))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
	})
	Describe("Option handling", func() {
		BeforeEach(func() {
			opts = NewOptions()
		})
		It("detects short options", func() {
			args = []string{"git-lob", "lock", "-q", "-v", "-f", "-n"}
			errors = ParseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Quiet).To(Equal(true))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.DryRun).To(Equal(false))
			Expect(opts.NonInteractive).To(Equal(true))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
		It("detects long options", func() {
			args = []string{"git-lob", "lock", "--quiet", "--verbose", "--noninteractive"}
			errors = ParseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Quiet).To(Equal(true))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.DryRun).To(Equal(false))
			Expect(opts.NonInteractive).To(Equal(true))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
		It("accepts additional options", func() {
			args = []string{"git-lob", "lock", "--verbose", "--option1=foo", "--option2=bar"}
			errors = ParseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Quiet).To(Equal(false))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.DryRun).To(Equal(false))
			Expect(opts.NonInteractive).To(Equal(false))
			Expect(opts.Args).To(Equal([]string{}))
			Expect(opts.StringOpts).To(Equal(map[string]string{"option1": "foo", "option2": "bar"}))
		})
		It("accepts additional arguments", func() {
			args = []string{"git-lob", "lock", "--verbose", "--dry-run", "file/one/test.jpg", "file/two/another.png"}
			errors = ParseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Quiet).To(Equal(false))
			Expect(opts.Verbose).To(Equal(true))
			Expect(opts.DryRun).To(Equal(true))
			Expect(opts.NonInteractive).To(Equal(false))
			Expect(opts.Args).To(Equal([]string{"file/one/test.jpg", "file/two/another.png"}))
			Expect(opts.StringOpts).To(Equal(map[string]string{}))
		})
		It("accepts custom boolean short options", func() {
			args = []string{"git-lob", "lock", "-x"}
			errors = ParseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.BoolOpts).To(HaveKey("x"))
		})
		It("accepts custom boolean long options", func() {
			args = []string{"git-lob", "lock", "--customoption"}
			errors = ParseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.BoolOpts).To(HaveKey("customoption"))
		})
		It("validates custom options", func() {
			args = []string{"git-lob", "lock", "--validbool", "--invalidbool", "--validvalue=1", "--invalidvalue=2", "-x", "-b", "-q"}
			errors = ParseCommandLine(opts, args)
			Expect(errors).To(BeEmpty())
			Expect(opts.Command).To(Equal("lock"))
			Expect(opts.Args).To(BeEmpty())
			Expect(opts.BoolOpts).To(HaveKey("validbool"))
			Expect(opts.BoolOpts).To(HaveKey("invalidbool"))
			Expect(opts.StringOpts).To(HaveKey("validvalue"))
			Expect(opts.StringOpts).To(HaveKey("invalidvalue"))
			Expect(opts.BoolOpts).To(HaveKey("x"))
			Expect(opts.BoolOpts).To(HaveKey("b"))
			Expect(opts.BoolOpts).ToNot(HaveKey("q")) // std option

			errors = validateCustomOptions(opts, []string{"validvalue"}, []string{"validbool", "x"})
			Expect(errors).To(HaveLen(3))

		})

	})

})
