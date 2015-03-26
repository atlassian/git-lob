package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
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
	// Placeholder present, content corrupted (use fsck)
	MissingCorrupt MissingCallbackType = iota
	// Placeholder present, content not available (commit details included)
	MissingBlamed MissingCallbackType = iota
	// Placeholder present, but file is untracked or modified & therefore it's local user's fault
	// resolution is probably to delete or reset/checkout this file again
	MissingModified MissingCallbackType = iota
	// Some other error was encountered
	MissingError MissingCallbackType = iota
)

// Collected callback data for a missing operation
type MissingCallbackData struct {
	// What stage of the process this is for
	Type MissingCallbackType
	// Path to the file (relative to working dir)
	Path string
	// Commit summary for MissingBlamed
	CommitSummary *GitCommitSummary
	// Error details for MissingError
	Error error
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

	anyErrors := false
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
			LogConsolef("  Blame: %v(%v) [%v] %v\n", data.CommitSummary.CommitterName, data.CommitSummary.CommitterEmail,
				data.CommitSummary.ShortSHA, data.CommitSummary.Subject)
		case MissingError:
			LogConsoleErrorf("Error: %v\n", data.Error.Error())
			anyErrors = true // still continue
		case MissingWorking:
			LogConsoleDebugf("Checking %v\n", data.Path)
		}
		// Display progress always (fixed line width always large enough)
		LogConsoleSpinner("Searching: ")
		// Always continue
		return false
	}
	// Add newlines to messages since progress doesn't
	Missing(optCheckout, paths, callback)
	LogConsoleSpinnerFinish("Searching: ")
	if anyErrors {
		return 12
	}
	return 0
}

// Check for placeholders
func Missing(checkout bool, paths []string, callback func(data *MissingCallbackData) (quit bool)) {
	// Make sure we're in a git repo
	_, _, err := GetRepoRoot()
	if err != nil {
		callback(&MissingCallbackData{Type: MissingError, Error: err})
		return
	}

	if len(paths) > 0 {
		for _, path := range paths {
			matches, err := filepath.Glob(path)
			if err != nil {
				if callback(&MissingCallbackData{Type: MissingError, Path: path,
					Error: fmt.Errorf("Unable to glob %v: %v\n", path, err)}) {
					return
				}
			}
			for _, match := range matches {
				stat, err := os.Stat(match)
				if err != nil {
					if callback(&MissingCallbackData{Type: MissingError, Path: match,
						Error: fmt.Errorf("Unable to stat %v: %v\n", match, err)}) {
						return
					}
				}
				var quit bool
				if stat.IsDir() {
					// Matched a dir, so just cascade
					quit = missingCheckDir(match, checkout, callback)
				} else {
					quit = missingCheckFile(match, stat, checkout, callback)
				}
				if quit {
					return
				}
			}
		}
	} else {
		// cascade from working dir
		missingCheckDir(".", checkout, callback)
	}

	return
}

// Check the contents of a directory for placeholders
// path is relative to the working dir & we'll use that as-is
func missingCheckDir(dir string, checkout bool, callback func(data *MissingCallbackData) (quit bool)) (quit bool) {
	// Never cascade into git dir
	if filepath.Base(dir) == ".git" {
		return false
	}
	// os.File.Readdirnames is the most efficient but having Stat() output is quickest way to identify potential placeholders
	// os.File.Readdir retrieves Stat() which lets us check size & whether dir (to cascade)
	if callback(&MissingCallbackData{Type: MissingWorking, Path: dir}) {
		return true
	}

	dirf, err := os.Open(dir)
	if err != nil {
		return callback(&MissingCallbackData{Type: MissingError, Path: dir,
			Error: fmt.Errorf("Unable to open dir %v: %v\n", dir, err)})
	}

	defer dirf.Close()

	contents, err := dirf.Readdir(0)
	if err != nil {
		return callback(&MissingCallbackData{Type: MissingError, Path: dir,
			Error: fmt.Errorf("Unable to read read dir %v: %v\n", dir, err)})
	}
	for _, entry := range contents {
		relpath := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			quit = missingCheckDir(relpath, checkout, callback)
		} else {
			quit = missingCheckFile(relpath, entry, checkout, callback)
		}
	}

	return quit
}

// Check a file to see if it's a placeholder & what to do about it if so
// path is relative to the working dir & we'll use that as-is
func missingCheckFile(path string, fi os.FileInfo, checkout bool, callback func(data *MissingCallbackData) (quit bool)) (quit bool) {
	if callback(&MissingCallbackData{Type: MissingWorking, Path: path}) {
		return true
	}

	// Smoke test on file size
	if fi.Size() == int64(SHALineLen) {
		// It's the right size for a placeholder
		filebytes, err := ioutil.ReadFile(path)
		if err != nil {
			return callback(&MissingCallbackData{Type: MissingError, Path: path,
				Error: fmt.Errorf("Unable to read file %v: %v\n", path, err)})
		}
		shaRegex := regexp.MustCompile(SHALineMatchRegexStr)
		if match := shaRegex.FindStringSubmatch(string(filebytes)); match != nil {
			// Definitely a placeholder
			sha := match[1]
			err := CheckLOBFilesForSHA(sha, GetLocalLOBRoot(), false)
			if err != nil {
				if IsIntegrityError(err) {
					if callback(&MissingCallbackData{Type: MissingCorrupt, Path: path}) {
						return true
					}
				} else if IsNotFoundError(err) {
					// LOB not available, find out who committed this or if it's modified
					// extract latest change
					// first, root the filename for use in Git since path is relative to working dir
					root, _, err := GetRepoRoot()
					if err != nil {
						callback(&MissingCallbackData{Type: MissingError, Path: path, Error: err})
						return true // cannot continue
					}
					var absfilename string
					if filepath.IsAbs(path) {
						absfilename = path
					} else {
						wd, _ := os.Getwd()
						absfilename = filepath.Join(wd, path)
					}
					rootedfilename, _ := filepath.Rel(root, absfilename)
					summary, lobshaincommit, err := GetGitLatestLOBChangeDetails(rootedfilename, "HEAD")
					if err != nil {
						return callback(&MissingCallbackData{Type: MissingError, Path: path,
							Error: fmt.Errorf("Unable to get latest commit for file %v: %v\n", path, err)})
					}
					if lobshaincommit == sha {
						// unmodified, so blame case
						if callback(&MissingCallbackData{Type: MissingBlamed, Path: path, CommitSummary: summary}) {
							return true
						}
					} else {
						// sha in file doesn't agree with SHA in last commit, so modified
						if callback(&MissingCallbackData{Type: MissingModified, Path: path, CommitSummary: summary}) {
							return true
						}
					}

				} else {
					// Some other error
					return callback(&MissingCallbackData{Type: MissingError, Path: path,
						Error: fmt.Errorf("Error checking binary %v for file %v: %v\n", sha, path, err)})
				}
			} else {
				// LOB is present
				if checkout {
					err := checkoutFile(path, sha)
					if err != nil {
						return callback(&MissingCallbackData{Type: MissingError, Path: path,
							Error: fmt.Errorf("Unable to checkout %v to file %v: %v\n", sha, path, err)})
					}
					// checked out OK
					if callback(&MissingCallbackData{Type: MissingFixed, Path: path}) {
						return true
					}

				} else {
					// no checkout, just notify it's there
					if callback(&MissingCallbackData{Type: MissingAvailable, Path: path}) {
						return true
					}
				}
			}

		}

	}
	return false
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
