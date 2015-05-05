package core

import (
	"bitbucket.org/sinbad/git-lob/util"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Callback can report skip,transfer (on complete), error
type CheckoutCallback func(t util.ProgressCallbackType, filelob *FileLOB, err error)

// Populate local placeholders with real content, if available. Do entire working copy unless limited to pathspecs
func Checkout(pathspecs []string, dryRun bool, callback CheckoutCallback) error {
	// We're going to scan for missing git-lob content not just by checking the working copy, but
	// getting the expected content from git first. This is in case the working copy has had files
	// deleted for example. We still check the content of the working copy if the file IS there
	// in order to not overwrite modified files.

	util.LogDebug("Checking for missing binary files in working copy")

	// firstly convert any pathspecs to the root of the repo, in case this is being executed in a sub-folder
	reporoot, _, err := util.GetRepoRoot()
	if err != nil {
		return err
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
	var modifiedfiles []string
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
			if !dryRun {
				err = checkoutFile(absfile, filelob.SHA)
				if err != nil {
					if IsNotFoundError(err) {
						// most common issue, log nicely
						callback(util.ProgressNotFound, filelob,
							NewNotFoundError(fmt.Sprintf("%v: content not available, placeholder used [%v]", filelob.Filename, filelob.SHA[:7]),
								filelob.Filename))
					} else {
						// Still not fatal but log full detail
						callback(util.ProgressError, filelob,
							errors.New(fmt.Sprintf("Can't retrieve content for %v: %v", filelob.Filename, err.Error())))
					}
				} else {
					// Success
					callback(util.ProgressTransferBytes, filelob, nil)
				}
				// In all cases, we've changed the content of the file. It's important we note this for later
				modifiedfiles = append(modifiedfiles, filelob.Filename)
			} else {
				// Dry run, still call back as if we did it
				callback(util.ProgressTransferBytes, filelob, nil)
			}

		} else {
			callback(util.ProgressSkip, filelob, nil)
		}

	}

	var retErr error
	if len(modifiedfiles) > 0 {
		// Modifying files, even to a state that would show as unmodified in 'git diff' (because our filters
		// make sure that it is so) confuses git because the cached stat() info it stores no longer agrees with the file
		// So 'git status' would report the files modified even though 'git diff' wouldn't. Confusing for the user!
		// Cause git to refresh its index
		retErr = GitRefreshIndexForFiles(modifiedfiles)
	}

	if retErr == nil {
		util.LogDebug("Successfully checked the working copy")
	}

	return retErr

}

// Checkout a single file to a specific path
func checkoutFile(path, sha string) error {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return errors.New(fmt.Sprintf("Can't create parent directory of %v: %v\n", path, err.Error()))
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return errors.New(fmt.Sprintf("Can't open %v for writing: %v", path, err.Error()))
	}
	defer f.Close()
	_, err = RetrieveLOB(sha, f)
	if err != nil {
		// We already truncated the file so we need to re-write the placeholder contents
		ioutil.WriteFile(path, []byte(getLOBPlaceholderContent(sha)), 0644)
		return err
	}

	return nil

}
