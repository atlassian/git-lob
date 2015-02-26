package main

import (
	"bytes"
	"fmt"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"io"
	"io/ioutil"
	"os"
)

var _ = Describe("Config", func() {
	Describe("Reading .gitconfig", func() {
		Context("Self-contained config", func() {

			configText := `[difftool "sourcetree"]
        path = 
        cmd = /usr/local/bin/ksdiff -w \"$LOCAL\" \"$REMOTE\"

    [mergetool "sourcetree"]
        cmd = /Applications/p4merge.app/Contents/MacOS/p4merge \"$BASE\" \"$LOCAL\" \"$REMOTE\" \"$MERGED\"
        trustExitCode = true

    [core]
        excludesfile = /Users/steve/gitignore_global
        quotepath = false

    # comment line
    # more comments

    [color]
      branch = auto
      diff = auto
      status = auto
    [color "branch"]
      current = yellow reverse
      local = yellow 
      remote = green
    [color "diff"]
      meta = yellow bold
      frag = magenta bold
      old = red bold
      new = green bold
    [color "status"]
      added = yellow
      changed = green
      untracked = cyan
    [mergetool]
        keepBackup = true
        
    [http]
        sslCAInfo = /Users/steve/.gitcertificates/lancelot.pem
    [init]
      templatedir = /Users/steve/temp/git-templates
      
    [user]
        name = Steve Streeting
        email = steve@stevestreeting.com
    [push]
        default = matching
    #[commit]
    #  template=/Users/steve/gitcommittemplate
    [filter "lob"] # Our large file plugin
      clean = "git-lob filter-clean %f" # test comment after line
      smudge = "git-lob filter-smudge %f" ; test comment after line
    `

			It("correctly reads well formatted simple sections", func() {
				in := bytes.NewBufferString(configText)
				config, err := ReadConfigStream(in, "")
				Expect(err).To(BeNil(), "Shouldn't encounter an error when reading config stream")
				Expect(len(config)).To(BeEquivalentTo(27), "Should be 27 leaf config nodes")
				Expect(config["difftool.sourcetree.path"]).To(Equal(""), "difftool.sourcetree.path")
				Expect(config["difftool.sourcetree.cmd"]).To(Equal(`/usr/local/bin/ksdiff -w \"$LOCAL\" \"$REMOTE\"`), "difftool.sourcetree.cmd")
				Expect(config["mergetool.sourcetree.cmd"]).To(Equal(`/Applications/p4merge.app/Contents/MacOS/p4merge \"$BASE\" \"$LOCAL\" \"$REMOTE\" \"$MERGED\"`), "mergetool.sourcetree.cmd")
				Expect(config["mergetool.sourcetree.trustexitcode"]).To(Equal("true"), "difftool.sourcetree.trustexitcode - lower case conversion")
				Expect(config["core.excludesfile"]).To(Equal("/Users/steve/gitignore_global"), "core.excludesfile")
				Expect(config["core.quotepath"]).To(Equal("false"), "core.quotepath")
				Expect(config["color.branch"]).To(Equal("auto"), "color.branch")
				Expect(config["color.diff"]).To(Equal("auto"), "color.diff")
				Expect(config["color.status"]).To(Equal("auto"), "color.status")
				Expect(config["color.branch.current"]).To(Equal("yellow reverse"), "color.branch.current")
				Expect(config["color.branch.local"]).To(Equal("yellow"), "color.branch.local")
				Expect(config["color.branch.remote"]).To(Equal("green"), "color.branch.remote")
				Expect(config["color.diff.meta"]).To(Equal("yellow bold"), "color.diff.meta")
				Expect(config["color.diff.frag"]).To(Equal("magenta bold"), "color.diff.frag")
				Expect(config["color.diff.old"]).To(Equal("red bold"), "color.diff.old")
				Expect(config["color.diff.new"]).To(Equal("green bold"), "color.diff.new")
				Expect(config["color.status.added"]).To(Equal("yellow"), "color.diff.added")
				Expect(config["color.status.changed"]).To(Equal("green"), "color.diff.changed")
				Expect(config["color.status.untracked"]).To(Equal("cyan"), "color.diff.untracked")
				Expect(config["mergetool.keepbackup"]).To(Equal("true"), "color.diff.keepbackup - lower case conversion")
				Expect(config["http.sslcainfo"]).To(Equal("/Users/steve/.gitcertificates/lancelot.pem"), "http.sslcainfo - lower case conversion")
				Expect(config["init.templatedir"]).To(Equal("/Users/steve/temp/git-templates"), "init.templatedir")
				Expect(config["user.name"]).To(Equal("Steve Streeting"), "user.name")
				Expect(config["user.email"]).To(Equal("steve@stevestreeting.com"), "user.email")
				Expect(config["push.default"]).To(Equal("matching"), "push.default")
				Expect(config["commit.template"]).To(Equal(""), "commit.template is commented out")
				Expect(config["filter.lob.clean"]).To(Equal(`"git-lob filter-clean %f"`), "filter.lob.clean - inline comments and quotes")
				Expect(config["filter.lob.smudge"]).To(Equal(`"git-lob filter-smudge %f"`), "filter.lob.smudge - inline comments and quotes")

			})
		})

		Context("Config with includes", func() {
			var includedTempFilename string
			BeforeEach(func() {
				includedText := `[user]
    name = Steve Streeting
    email = steve@stevestreeting.com
[push]
    default = matching
#[commit]
#  template=/Users/steve/gitcommittemplate
[filter "lob"] # Our large file plugin
  clean = "git-lob filter-clean %f" # test comment after line
  smudge = "git-lob filter-smudge %f" ; test comment after line
`
				tempfile, err := ioutil.TempFile("", "configinclude")
				if err != nil {
					Fail(err.Error())
				}
				io.WriteString(tempfile, includedText)
				includedTempFilename = tempfile.Name()
				tempfile.Close()
			})
			AfterEach(func() {
				os.Remove(includedTempFilename)
			})

			It("Correctly reads include files", func() {
				// Check that elements before are overridden and those after are not
				pretext := `[push]
    default = simple    
[http]
        sslCAInfo = /Users/steve/.gitcertificates/lancelot.pem
    [init]
      templatedir = /Users/steve/temp/git-templates
`
				includetext := fmt.Sprintf("[include]\n\tpath = %v\n", includedTempFilename)
				posttext := `[user]
    name = Steven J Streeting
[color]
      branch = auto
      diff = auto
      status = auto`
				fulltext := pretext + includetext + posttext
				in := bytes.NewBufferString(fulltext)
				config, err := ReadConfigStream(in, "")
				Expect(err).To(BeNil(), "Shouldn't encounter an error when reading config stream")
				// unchanged items from pretext
				Expect(config["http.sslcainfo"]).To(Equal("/Users/steve/.gitcertificates/lancelot.pem"), "http.sslcainfo - lower case conversion")
				Expect(config["init.templatedir"]).To(Equal("/Users/steve/temp/git-templates"), "init.templatedir")
				// include file
				Expect(config["push.default"]).To(Equal("matching"), "included file should have overridden push.default")
				Expect(config["user.email"]).To(Equal("steve@stevestreeting.com"), "user.email")
				Expect(config["commit.template"]).To(Equal(""), "commit.template is commented out")
				Expect(config["filter.lob.clean"]).To(Equal(`"git-lob filter-clean %f"`), "filter.lob.clean - inline comments and quotes")
				Expect(config["filter.lob.smudge"]).To(Equal(`"git-lob filter-smudge %f"`), "filter.lob.smudge - inline comments and quotes")
				// after include
				Expect(config["user.name"]).To(Equal("Steven J Streeting"), "user.name")
				Expect(config["color.branch"]).To(Equal("auto"), "color.branch")
				Expect(config["color.diff"]).To(Equal("auto"), "color.diff")
				Expect(config["color.status"]).To(Equal("auto"), "color.status")

			})
		})
	})
	Describe("Parsing multiple value options", func() {
		It("Parses fetch include/exclude", func() {
			configText := `[git-lob]
    fetch-include=include/path/one,include/path/glob*, left/a/space 
    fetch-exclude = wow/such/exclude  , something/something/dark-side, something with a space/inside it/here
`
			correctIncludes := []string{"include/path/one", "include/path/glob*", "left/a/space"}
			correctExcludes := []string{"wow/such/exclude", "something/something/dark-side", "something with a space/inside it/here"}
			in := bytes.NewBufferString(configText)
			config, err := ReadConfigStream(in, "")
			Expect(err).To(BeNil(), "Shouldn't encounter an error when reading config stream")
			opts := NewOptions()
			parseConfig(config, opts)
			Expect(opts.FetchIncludePaths).To(Equal(correctIncludes), "Includes should be correct")
			Expect(opts.FetchExcludePaths).To(Equal(correctExcludes), "Excludes should be correct")

		})

	})

})
