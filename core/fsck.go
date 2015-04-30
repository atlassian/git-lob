package core

import (
	"bitbucket.org/sinbad/git-lob/util"
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
	var shaSet util.StringSet
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
		shaSet = util.NewStringSetFromSlice(shas)
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
		percent := int(float32(i+1) * 100 / float32(len(shaSet)))
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
					delerr := DeleteLOBInBaseDir(sha, basedir)
					if delerr != nil {
						// Log but don't abort for this
						util.LogErrorf("fsck error: Unable to delete bad LOB %v from %v: %v", sha, basedir, delerr.Error())
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
						util.LogErrorf("fsck error: Unable to delete %v: %v", e.Filename, delerr.Error())
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
