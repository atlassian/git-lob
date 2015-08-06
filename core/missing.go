package core

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/atlassian/git-lob/util"
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

// Check for placeholders
func Missing(checkout bool, paths []string, callback func(data *MissingCallbackData) (quit bool)) {
	// Make sure we're in a git repo
	_, _, err := util.GetRepoRoot()
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
					root, _, err := util.GetRepoRoot()
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
