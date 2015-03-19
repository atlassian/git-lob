package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type FsckCallbackType int

const (
	// Process is just working through data (progress update)
	FsckWorking FsckCallbackType = iota
	// A file was missing and not recovered from shared store (desc = name)
	FsckMissing FsckCallbackType = iota
	// A file was missing in the local store but was recovered from the shared store (desc = name)
	FsckRecovered FsckCallbackType = iota
	// A content file had the wrong size (desc = name)
	// This file will be deleted if --delete was specified
	FsckWrongSize FsckCallbackType = iota
	// A binary is corrupt - either the metadata is invalid, or the SHA doesn't match the content (only detected with --deep) (desc = SHA)
	// The files will be deleted if --delete was specified
	FsckCorruptData FsckCallbackType = iota
)

// Collected callback data for a fsck operation
type FsckCallbackData struct {
	// What stage of the process this is for, preparing, transferring or skipping something
	Type FsckCallbackType
	// What SHA is being checked
	SHA string
	// Either a general message or an item name (e.g. file name)
	Desc string
	// The percentage complete
	PercentComplete int
}

// Fsck command line tool
func cmdFsck() int {

	// git-lob fsck [--deep] [--shared]

	// Validate custom options
	errorList := validateCustomOptions(GlobalOptions, nil, []string{"deep", "shared", "delete"})
	if len(errorList) > 0 {
		LogConsoleError(strings.Join(errorList, "\n"))
		return 9
	}

	optDeep := GlobalOptions.BoolOpts.Contains("deep")
	optShared := GlobalOptions.BoolOpts.Contains("shared")
	optDelete := GlobalOptions.BoolOpts.Contains("delete")

	if optShared {
		// Check we have a shared store
		if GlobalOptions.SharedStore == "" {
			LogConsoleError("No shared store is configured for this repository, cannot use --shared")
			return 8
		}
		LogConsole("Checking shared store at", GlobalOptions.SharedStore)
	} else {
		LogConsole("Checking local binary store")
	}

	var shas []string
	if len(GlobalOptions.Args) > 0 {
		shas = GlobalOptions.Args
	}

	callback := func(data *FsckCallbackData) (quit bool) {
		// Ensure we clear previous progress
		LogConsolef("\r")
		switch data.Type {
		case FsckMissing:
			LogErrorf("fsck %v: %v has some missing data, try fetch/prune\n", data.SHA[:7], data.Desc)
		case FsckRecovered:
			LogDebugf("fsck %v: %v was recovered from shared store\n", data.SHA[:7], data.Desc)
		case FsckCorruptData:
			LogErrorf("fsck %v: content is corrupt (deleted: %v)\n", data.SHA[:7], optDelete)
		case FsckWrongSize:
			LogErrorf("fsck %v: %v is wrong size (deleted: %v)\n", data.SHA[:7], data.Desc, optDelete)
		case FsckWorking:
			// Do nothing, just progress below
		}
		// Display progress always (fixed line width always large enough)
		LogConsoleOverwrite(fmt.Sprintf("Progress: %d%", data.PercentComplete), 14)
		// Always continue
		return false
	}

	err := Fsck(optDeep, optShared, optDelete, shas, callback)
	if err != nil {
		LogError("Error(s) in fsck: %v", err.Error())
		return 12
	}
	LogConsole("Completed successfully, no problems found")
	return 0
}

// Validate local binary store
// deep = recaculate SHAs & check (takes longer) instead of just checking file size
// shared = check shared store instead of local store
// deleteBadFiles = delete files which are the wrong size or corrupted
// shas = specific list of binaries to check; if empty, checks entire store
// callback = for progress and file errors, return quit to abort (also skips deleting current item)
// The returned error will be nil if no files had any issues but the process will continue (with callbacks)
// even when missing/bad files are encountered until all have been checked
func Fsck(deep, shared, deleteBadFiles bool, shas []string, callback func(data *FsckCallbackData) (quit bool)) error {

	// When listing all LOBs it returns a set to eliminate dupes so use this across both
	// cheaper to construct a set from small number of arguments than a slice from potentially
	// unlimited number of items on the disk
	var shaSet StringSet
	if len(shas) == 0 {
		var err error
		if shared {
			shaSet, err = getAllSharedLOBSHAs()
		} else {
			shaSet, err = getAllLocalLOBSHAs()
		}
		if err != nil {
			return err
		}
	} else {
		shaSet = NewStringSetFromSlice(shas)
	}

	var basedir string
	if shared {
		basedir = GetSharedLOBRoot()
	} else {
		basedir = GetLocalLOBRoot()
	}
	i := 0
	var errorList []string
	for sha := range shaSet.Iter() {
		percent := int(float32(i) / float32(len(shas)))

		err := CheckLOBFilesForSHA(sha, basedir, deep)
		var quit bool
		if err != nil {
			switch e := err.(type) {
			case *NotFoundError:
				quit = callback(&FsckCallbackData{FsckMissing, sha, e.Path, percent})
			case *IntegrityError:
				quit = callback(&FsckCallbackData{FsckCorruptData, sha, sha, percent})
				if !quit && deleteBadFiles {
					// Delete all files for this LOB
					delerr := deleteLOBRelative(sha, basedir)
					if delerr != nil {
						// Log but don't abort for this
						LogErrorf("fsck error: Unable to delete bad LOB %v from %v: %v", sha, basedir, delerr.Error())
					}
				}
			case *WrongSizeError:
				quit = callback(&FsckCallbackData{FsckWrongSize, sha, e.Filename, percent})
				if !quit && deleteBadFiles {
					// in this case we only delete the single file which is bad (might just be one chunk of many)
					// others could be downloaded again later
					delerr := os.Remove(e.Filename)
					if delerr != nil {
						// Log but don't abort for this
						LogErrorf("fsck error: Unable to delete %v: %v", e.Filename, delerr.Error())
					}
				}
			default:
				// Something else, abort
				return fmt.Errorf("fsck aborted: %v", e.Error())
			}
			errorList = append(errorList, err.Error())
		} else {
			quit = callback(&FsckCallbackData{FsckWorking, sha, "", percent})
		}
		i++

		if quit {
			break
		}

	}

	if len(errorList) > 0 {
		return errors.New(strings.Join(errorList, "\n"))
	}
	return nil

}

func cmdFsckHelp() {
	LogConsole(`Usage: git-lob fsck [options] [SHA...]

  Validates that the local binary store is interally consistent. 

  This utility command checks the contents of the local binary store to make
  sure that each binary stored there is complete & correct. The basic mode
  just ensures that all the required file components are there and are of the
  correct size, wheras --deep mode also checks every every byte of the content
  to ensure it is correct (by checking that the SHA matches the content).

  If you're using a shared store across repos (see git-lob.sharedstore in 
  'git lob help config') and a missing local file is available in that shared 
  store, it will automatically be re-linked into your local repo to resolve the
  problem.

  The --delete option can be used to clean any files which are invalid. 
  Partially downloaded binaries where some chunks are missing are not deleted
  so you can resume downloading, but invalid files such as corrupt metadata, 
  incorrectly sized chunks, and content where the SHA doesn't agree (only
  checked with the --deep option) are deleted.

  This command doesn't check your working copy, use 'git lob status' for that,
  for example to help figure out why a binary file is still a placeholder.

Parameters:
  SHA...        If you supply one or more 40-character SHA arguments, only
                those binaries are checked rather than the entire store. The
                SHA is the identifier of the binary content itself, not a Git
                object.

Options:
  --deep        Recalculates the SHA of each binary file to ensure the contents
                are correct. Without this option, just checks that all files
                are present and the correct size.
  --shared      Checks the shared store instead of the local repo
  --delete      Delete files which are invalid. Doesn't delete partial binaries
                where 1 or more chunks are missing, but deletes files which are
                internally inconsistent; e.g. invalid meta files, partial 
                chunks, and all files where --deep is used and SHA doesn't 
                agree with content.
  --quiet, -q   Print less output
  --verbose, -v Print more output

`)
}
