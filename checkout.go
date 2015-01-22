package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

func cmdCheckout() int {

	// git-lob checkout [options] [<pathspec>...]

	// no custom options
	optDryRun := GlobalOptions.DryRun

	// All extra arguments must be <pathspec>
	var pathspecs []string
	for _, arg := range GlobalOptions.Args[1:] {
		p := filepath.Clean(arg)
		pathspecs = append(pathspecs, p)
	}

	err := Checkout(pathspecs, optDryRun)

	if err != nil {
		fmt.Fprintf(os.Stderr, "git-lob: checkout error - %v", err.Error())
		return 7
	}

	return 0
}

// Populate local placeholders with real content, if available. Do entire working copy unless limited to pathspecs
func Checkout(pathspecs []string, dryRun bool) error {
	// We're going to scan for missing git-lob content not just by checking the working copy, but
	// getting the expected content from git first. This is in case the working copy has had files
	// deleted for example. We still check the content of the working copy if the file IS there
	// in order to not overwrite modified files.

	// firstly convert any pathspecs to the root of the repo, in case this is being executed in a sub-folder
	reporoot, _ := GetRepoRoot()
	if reporoot == "" {
		return errors.New("Not in git repository?")
	}
	curdir, err := os.Getwd()
	if err != nil {
		return err
	}
	var rootedpathspecs []string
	for _, p := range pathspecs {
		var abs string
		if filepath.IsAbs(p) {
			abs = p
		} else {
			abs = filepath.Join(curdir, p)
		}
		reltoroot, err := filepath.Rel(reporoot, abs)
		if err != nil {
			return errors.New(fmt.Sprintf("Unable to make %v relative to repo root %v", p, reporoot))
		}
		rootedpathspecs = append(rootedpathspecs, reltoroot)
	}

	// Get what git thinks we should have
	filelobs, err := GetGitAllFilesAndLOBsToCheckoutAtCommit("HEAD", rootedpathspecs, nil)
	if err != nil {
		return err
	}
	var filesOK int
	var filesNotOK int
	for _, filelob := range filelobs {
		// Check each file, and if it's missing or contains the placeholder text, replace it with content
		// Otherwise, assume it's been locally modified and leave it alone (user can override this with git reset/checkout if they want)
		absfile := filepath.Join(reporoot, filelob.Filename)
		stat, err := os.Stat(absfile)
		replaceContent := false
		if err == nil {
			// File existed, check content (smoke test on size)
			if stat.Size() == int64(SHALineLen) {
				// File existed and is right size for placeholder, so check contents
				placeholderContent := getLOBPlaceholderContent(filelob.SHA)
				filebytes, err := ioutil.ReadFile(absfile)
				if err == nil && string(filebytes) == placeholderContent {
					// File content is placeholder, so replace
					replaceContent = true
				}
			}
		} else {
			// File did not exist
			replaceContent = true
		}

		if replaceContent {
			if dryRun {
				if GlobalOptions.Verbose {
					fmt.Println("Checkout:", filelob.Filename)
				}

			} else {
				err = os.MkdirAll(filepath.Dir(absfile), 0755)
				if err != nil {
					// This is not fatal but log it
					LogErrorf("ERROR: can't create parent directory of %v: %v\n", absfile, err.Error())
					filesNotOK++
					continue
				}
				f, err := os.OpenFile(absfile, os.O_CREATE|os.O_TRUNC, 0644)
				if err != nil {
					// This is not fatal but log it
					LogErrorf("ERROR: can't open %v for writing: %v", absfile, err.Error())
					filesNotOK++
					continue
				}
				_, err = RetrieveLOB(filelob.SHA, f)
				f.Close()
				if err != nil {
					// This is not fatal but log it
					LogErrorf("ERROR: can't retrieve content for %v: %v", filelob.Filename, err.Error())
					filesNotOK++
					continue
				}

			}
			filesOK++
		}

	}
	if !GlobalOptions.Quiet {
		if dryRun {
			fmt.Println(filesOK, "files need updating")
			fmt.Println("Run this command again without --dry-run to update these files.")
		} else {
			fmt.Println(filesOK, "files were updated")
			if filesNotOK > 0 {
				fmt.Println("WARNING:", filesNotOK, "failed to be updated, check errors above")
			}
		}
	}

	if filesNotOK > 0 {
		return errors.New(fmt.Sprintf("%d files failed", filesNotOK))
	}

	return nil

}

func cmdCheckoutHelp() {
	fmt.Println(`Usage: git-lob checkout [options] [<pathspec>...]

  Populate files in the working copy with binary content where they
  currently just have placeholder content, because the real content wasn't
  available.

  NOTE: You probably won't need to run this command yourself.

        Running 'git lob pull' will both fetch (download) AND checkout, so
        most of the time you should use 'git lob pull' instead. 

        Also 'git checkout' will populate the binary content correctly if
        you have it locally so you don't have to run this command after
        switching branches, unless you need to download extra content, in
        which case 'git lob pull' is once again a better bet.

  Because git-lob stores binary content separately from your git repository, 
  it's possible that when you perform a 'git checkout' or 'git clone', you did
  not have the binary content available locally to populate binary files in 
  your working copy. In this situation, git-lob creates placeholders in the
  working copy, whose content looks something like this:

  git-lob: <sha>

  Where <sha> is the identifier of the content of the binary file. Once you
  have downloaded the content (e.g. via 'git lob fetch'), you can then use
  'git lob checkout' to fill in these blanks.

  Specify <pathspec> to limit the checking to particular files or directories.

  Options:
    --quiet, -q   Print less output
    --verbose, -v Print more output
    --dry-run     Don't actually change any files, just report

`)
}
