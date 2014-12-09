package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// FileSystemSyncProvider implements the BasicSyncProvider interface
type FileSystemSyncProvider struct {
}

func (*FileSystemSyncProvider) TypeID() string {
	return "filesystem"
}

func (*FileSystemSyncProvider) HelpTextSummary() string {
	return `filesystem: transfers binaries via mounted volumes / mapped drives`
}

func (*FileSystemSyncProvider) HelpTextDetail() string {
	return `The "filesystem" provider transfers files solely by copying them to/from locations
on the file system, i.e. to remotes via mounted volumes / mapped drives. You
are assumed to have the required permissions set up via the file system.

Required parameters in remote section of .gitconfig:
    git-lob-path    The filesystem path to use as a remote binary store

Example configuration:
    [remote "origin"]
        url = git@blah.com/your/usual/git/repo
        git-lob-provider = filesystem
        git-lob-path = /Volumes/shared/your/remote/binary/store

When uploading & downloading, to avoid partially written files when interrupted
a temporary file is created first, then moved to the final location on 
completion. While we clean up files on error and exit, if forcibly interrupted
temporary files may remain; these are called 'tempupload*' and 'tempdownload*'
in the target file structure and can be safely deleted if older than 24h.
`
}

func (*FileSystemSyncProvider) Upload(remoteName string, filenames []string, fromDir string,
	force bool) error {

	// Check config
	destpath, ok := GlobalOptions.GitConfig[fmt.Sprintf("remote.%v.git-lob-path", remoteName)]
	if !ok {
		return fmt.Errorf("Missing git-lob-path config parameter for remote '%v'", remoteName)
	}

	// clean up the path
	destpath = filepath.Clean(destpath)

	// Check dir exists & also extract permissions to use
	destpathfi, err := os.Stat(destpath)
	if err != nil || !destpathfi.IsDir() {
		return fmt.Errorf("git-lob-path '%v' for remote '%v' is not a valid directory", destpath, remoteName)
	}

	var errorList []string
	for _, filename := range filenames {
		// Check to see if the file is already there, right size
		srcfilename := filepath.Join(fromDir, filename)
		srcfi, err := os.Stat(srcfilename)
		if err != nil {
			msg := fmt.Sprintf("Unable to stat %v: %v", srcfilename, err)
			LogErrorf(msg)
			errorList = append(errorList, msg)
			// Keep going, upload as much as we can
			continue
		}

		destfilename := filepath.Join(destpath, filename)
		if !force {
			// Check existence & size before uploading
			if destfi, err := os.Stat(destfilename); err == nil {
				// File exists on remote, check the size
				if destfi.Size() == srcfi.Size() {
					// File already present and correct size, skip
					LogDebugf("Not updating %v on remote %v, already up to date", filename, remoteName)
					continue
				}
			}
		}

		// Make sure dest dir exists
		// Copy the permissions of root dest path
		parentDir := filepath.Dir(destfilename)
		err = os.MkdirAll(parentDir, destpathfi.Mode())
		if err != nil {
			msg := fmt.Sprintf("Unable to create dir %v: %v", parentDir, err)
			LogErrorf(msg)
			errorList = append(errorList, msg)
			continue
		}
		// Create a temporary file to copy, avoid issues with interruptions
		// Note this isn't a valid thing to do in security conscious cases but this isn't one
		// by opening the file we will get a unique temp file name (albeit a predictable one)
		outf, err := ioutil.TempFile(parentDir, "tempupload")
		if err != nil {
			msg := fmt.Sprintf("Unable to create temp file for upload in %v: %v", parentDir, err)
			LogErrorf(msg)
			errorList = append(errorList, msg)
			continue
		}
		tmpfilename := outf.Name()
		// We don't defer close here because we need to closely couple close/move and not defer
		// the move (interrupted is no good)
		inf, err := os.OpenFile(srcfilename, os.O_RDONLY, 0644)
		if err != nil {
			// Tidy up
			outf.Close()
			os.Remove(tmpfilename)

			msg := fmt.Sprintf("Unable to read input file for upload %v: %v", srcfilename, err)
			LogErrorf(msg)
			errorList = append(errorList, msg)
			continue
		}

		n, err := io.Copy(outf, inf)
		outf.Close()
		inf.Close()
		if n != srcfi.Size() || err != nil {
			os.Remove(tmpfilename)
			var msg string
			if err != nil {
				msg = fmt.Sprintf("Problem while uploading %v to %v: %v", srcfilename, remoteName, err)
			} else {
				msg = fmt.Sprintf("Upload error: number of bytes written to %v in upload of %v does not agree (%d/%d)",
					remoteName, srcfilename, n, srcfi.Size())
			}
			LogErrorf(msg)
			errorList = append(errorList, msg)
			continue
		}
		// Otherwise, file data is ok on remote
		// Move to correct location - remove before to deal with force or bad size cases
		os.Remove(destfilename)
		os.Rename(tmpfilename, destfilename)

	}

	if len(errorList) > 0 {
		return errors.New(strings.Join(errorList, "\n"))
	}

	return nil
}

func (*FileSystemSyncProvider) UploadForce(remoteName string, filenames []string, fromDir string) error {
	// TODO
	return nil
}

func (*FileSystemSyncProvider) Download(remoteName string, filenames []string, toDir string) error {
	// TODO
	return nil
}
