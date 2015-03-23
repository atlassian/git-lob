package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MissingCallbackType int

const (
	// Process is just working through data (progress update)
	MissingWorking MissingCallbackType = iota
	// Placeholder present but content is available (not checked out)
	MissingAvailable MissingCallbackType = iota
	// Placeholder WAS present, content available & checked out
	MissingFixed MissingCallbackType = iota
	// Placeholder present, content not available (commit details included)
	MissingBlamed MissingCallbackType = iota
	// Placeholder present, but file is untracked or modified & therefore it's local user's fault
	// resolution is probably to delete or reset/checkout this file again
	MissingModified MissingCallbackType = iota
)

// Collected callback data for a fsck operation
type MissingCallbackData struct {
	// What stage of the process this is for
	Type MissingCallbackType
	// Path to the file (relative to working dir)
	Path string
	// Commit SHA of last update
	Commit string
	// Committer name of last update
	CommitterName string
	// Committer email of last update
	CommitterEmail string
	// 1-line description of commit of last update
	CommitSummary string
}

// Missing command line tool
func cmdMissing() int {

	// git-lob missing [--ignore-available] [--checkout] [path...]

	// Validate custom options
	errorList := validateCustomOptions(GlobalOptions, nil, []string{"ignore-available", "i", "checkout", "c"})
	if len(errorList) > 0 {
		LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	optIgnoreAvailable := GlobalOptions.BoolOpts.Contains("ignore-available") || GlobalOptions.BoolOpts.Contains("i")
	optCheckout := GlobalOptions.BoolOpts.Contains("checkout") || GlobalOptions.BoolOpts.Contains("c")

	var paths []string
	if len(GlobalOptions.Args) > 0 {
		paths = GlobalOptions.Args
	}

	callback := func(data *MissingCallbackData) (quit bool) {
		// Ensure we clear previous progress
		LogConsolef("\r")
		switch data.Type {
		case MissingAvailable:
			if !optIgnoreAvailable {
				LogConsolef("%v content is available, use checkout\n", data.Path)
			}
		case MissingFixed:
			LogConsolef("%v checked out\n", data.Path)
		case MissingModified:
			LogErrorf("%v is locally modified with no content, delete or reset/checkout to resolve\n", data.Path)
		case MissingBlamed:
			LogConsolef("%v no content available\n", data.Path)
			LogConsolef("  Blame: %v(%v) [%v] %v\n", data.CommitterName, data.CommitterEmail, data.Commit[:7], data.CommitSummary)
		case MissingWorking:
			// Do nothing, just progress below
		}
		// Display progress always (fixed line width always large enough)
		LogConsoleSpinner("Searching: ")
		// Always continue
		return false
	}
	// Add newlines to messages since progress doesn't
	err := Missing(optCheckout, paths, callback)
	LogConsoleSpinnerFinish("Searching: ")
	if err != nil {
		LogConsoleErrorf("Error: %v\n", err.Error())
		return 12
	}
	return 0
}

// Check for placeholders
func Missing(checkout bool, paths []string, callback func(data *MissingCallbackData) (quit bool)) error {
	if len(paths) > 0 {
		for _, path := range paths {
			matches, err := filepath.Glob(path)
			if err != nil {
				return err
			}
			for _, match := range matches {
				stat, err := os.Stat(match)
				if err != nil {
					return err
				}
				var quit bool
				if stat.IsDir() {
					// Matched a dir, so just cascade
					err, quit = missingCheckDir(match, checkout, callback)
				} else {
					err, quit = missingCheckFile(match, stat, checkout, callback)
				}
				if err != nil {
					return err
				}
				if quit {
					return nil
				}
			}
		}
	} else {
		// cascade from working dir
		missingCheckDir("", checkout, callback)
	}

	return nil
}

// Check the contents of a directory for placeholders
// path is relative to the working dir & we'll use that as-is
func missingCheckDir(dir string, checkout bool, callback func(data *MissingCallbackData) (quit bool)) (err error, quit bool) {
	// os.File.Readdirnames is the most efficient but having Stat() output is quickest way to identify potential placeholders
	// os.File.Readdir retrieves Stat() which lets us check size & whether dir (to cascade)
	if callback(&MissingCallbackData{Type: MissingWorking, Path: dir}) {
		return nil, true
	}

	dirf, err := os.Open(dir)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to open dir %v: %v\n", dir, err)), true
	}
	defer dirf.Close()

	contents, err := dirf.Readdir(0)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to read read dir %v: %v\n", dir, err)), true
	}
	for _, entry := range contents {
		relpath := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			err, quit = missingCheckDir(relpath, checkout, callback)
			if err != nil || quit {
				return err, quit
			}
		} else {
			err, quit = missingCheckFile(relpath, entry, checkout, callback)
			if err != nil || quit {
				return err, quit
			}
		}
	}

	return nil, false
}

// Check a file to see if it's a placeholder & what to do about it if so
// path is relative to the working dir & we'll use that as-is
func missingCheckFile(path string, fi os.FileInfo, checkout bool, callback func(data *MissingCallbackData) (quit bool)) (err error, quit bool) {
	// TODO
	return nil, false
}

func cmdMissingHelp() {
	LogConsole(`Usage: git-lob missing [options] [path...]

  Checks the state of the working copy for binary placeholders which haven't
  been expanded to the full content. If the content isn't available locally,
  because the person who added this binary hasn't pushed the content, reports
  who committed this version & when so you can chase them up.

  By default checks the current folder and all subfolders, unless you supply
  one or more paths.

Parameters:
  path...       Optional list of paths to check instead of checking the whole
                working copy. paths are treated relative to the working
                directory, and wildcard matching is supported.

Options:
  --ignore-available, -i  Ignore placeholders where content is available 
                          locally, it just isn't checked out right now.
  --checkout, -c          If we find content available, expand the placeholder
                          to the full content as per 'git lob checkout'.
  --quiet, -q             Print less output
  --verbose, -v           Print more output

`)
}
