package main

import (
	"bytes"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// hide unused error
var _ = fmt.Println

var _ = Describe("Config", func() {
	Describe("Reading .gitconfig", func() {
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
  clean = "git-lob filter-clean" # test comment after line
  smudge = "git-lob filter-smudge" ; test comment after line
`

		It("correctly reads well formatted simple sections", func() {
			in := bytes.NewBufferString(configText)
			config, err := ReadConfigStream(in)
			//fmt.Println(config)
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
			Expect(config["filter.lob.clean"]).To(Equal(`"git-lob filter-clean"`), "filter.lob.clean - inline comments and quotes")
			Expect(config["filter.lob.smudge"]).To(Equal(`"git-lob filter-smudge"`), "filter.lob.smudge - inline comments and quotes")

		})

		// TODO includes
	})

})
