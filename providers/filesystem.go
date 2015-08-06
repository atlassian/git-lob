package providers

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/atlassian/git-lob/util"
)

// FileSystemSyncProvider implements the basic SyncProvider interface
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

const FileSystemBufferSize = 131072

func (*FileSystemSyncProvider) ValidateConfig(remoteName string) error {
	pathsetting := fmt.Sprintf("remote.%v.git-lob-path", remoteName)
	path := util.GlobalOptions.GitConfig[pathsetting]
	if path == "" {
		return fmt.Errorf("Configuration invalid for 'filesystem', missing setting %v", pathsetting)
	}
	// Check it exists
	exists, isdir := util.FileOrDirExists(path)
	if !exists {
		return fmt.Errorf("Configuration invalid for 'filesystem', %v does not exist", path)
	}
	if !isdir {
		return fmt.Errorf("Configuration invalid for 'filesystem', %v is not a directory", path)
	}
	return nil
}

func (*FileSystemSyncProvider) Release() {
	// Nothing to do here
}

func (*FileSystemSyncProvider) uploadSingleFile(remoteName, filename, fromDir, toDir string, fileMode os.FileMode,
	force bool, callback SyncProgressCallback) (errorList []string, abort bool) {
	// Check to see if the file is already there, right size
	srcfilename := filepath.Join(fromDir, filename)
	srcfi, err := os.Stat(srcfilename)
	if err != nil {
		if callback != nil {
			if callback(filename, util.ProgressNotFound, 0, 0) {
				return errorList, true
			}
		}
		msg := fmt.Sprintf("Unable to stat %v: %v", srcfilename, err)
		errorList = append(errorList, msg)
		// Keep going with other files
		return errorList, false
	}

	destfilename := filepath.Join(toDir, filename)
	if !force {
		// Check existence & size before uploading
		if destfi, err := os.Stat(destfilename); err == nil {
			// File exists on remote, check the size
			if destfi.Size() == srcfi.Size() {
				// File already present and correct size, skip
				if callback != nil {
					if callback(filename, util.ProgressSkip, srcfi.Size(), srcfi.Size()) {
						return errorList, true
					}
				}
				return errorList, false
			}
		}
	}

	// Make sure dest dir exists
	// Copy the permissions of root dest path
	parentDir := filepath.Dir(destfilename)
	err = os.MkdirAll(parentDir, fileMode)
	if err != nil {
		msg := fmt.Sprintf("Unable to create dir %v: %v", parentDir, err)
		errorList = append(errorList, msg)
		return errorList, false
	}
	// Create a temporary file to copy, avoid issues with interruptions
	// Note this isn't a valid thing to do in security conscious cases but this isn't one
	// by opening the file we will get a unique temp file name (albeit a predictable one)
	outf, err := ioutil.TempFile(parentDir, "tempupload")
	if err != nil {
		msg := fmt.Sprintf("Unable to create temp file for upload in %v: %v", parentDir, err)
		errorList = append(errorList, msg)
		return errorList, false
	}
	tmpfilename := outf.Name()
	// This is safe to do even though we manually close & rename because both calls are no-ops if we succeed
	defer func() {
		outf.Close()
		os.Remove(tmpfilename)
	}()
	inf, err := os.OpenFile(srcfilename, os.O_RDONLY, 0644)
	if err != nil {
		msg := fmt.Sprintf("Unable to read input file for upload %v: %v", srcfilename, err)
		errorList = append(errorList, msg)
		return errorList, false
	}
	defer inf.Close()

	// Initial callback
	if callback != nil {
		if callback(filename, util.ProgressTransferBytes, 0, srcfi.Size()) {
			return errorList, true
		}
	}
	var copysize int64 = 0
	for {
		var n int64
		n, err = io.CopyN(outf, inf, FileSystemBufferSize)
		copysize += n
		if n > 0 && callback != nil && srcfi.Size() > 0 {
			if callback(filename, util.ProgressTransferBytes, copysize, srcfi.Size()) {
				return errorList, true
			}
		}
		if err != nil {
			break
		}
	}
	outf.Close()
	inf.Close()
	if copysize != srcfi.Size() {
		os.Remove(tmpfilename)
		var msg string
		if err != nil {
			msg = fmt.Sprintf("Problem while uploading %v to %v: %v", srcfilename, remoteName, err)
		} else {
			msg = fmt.Sprintf("Upload error: number of bytes written to %v in upload of %v does not agree (%d/%d)",
				remoteName, srcfilename, copysize, srcfi.Size())
		}
		errorList = append(errorList, msg)
		return errorList, false
	}
	// Otherwise, file data is ok on remote
	// Move to correct location - remove before to deal with force or bad size cases
	os.Remove(destfilename)
	os.Rename(tmpfilename, destfilename)
	return errorList, false

}

func (self *FileSystemSyncProvider) Upload(remoteName string, filenames []string, fromDir string,
	force bool, callback SyncProgressCallback) error {

	destpath, err := self.getRemoteRootPath(remoteName)
	if err != nil {
		return err
	}

	// Check dir exists & also extract permissions to use
	destpathfi, err := os.Stat(destpath)
	if err != nil || !destpathfi.IsDir() {
		return fmt.Errorf("git-lob-path '%v' for remote '%v' is not a valid directory", destpath, remoteName)
	}

	var errorList []string
	for _, filename := range filenames {
		// Allow aborting
		newerrs, abort := self.uploadSingleFile(remoteName, filename, fromDir, destpath,
			destpathfi.Mode(), force, callback)
		errorList = append(errorList, newerrs...)
		if abort {
			break
		}
	}

	if len(errorList) > 0 {
		return errors.New(strings.Join(errorList, "\n"))
	}

	return nil
}

func (*FileSystemSyncProvider) downloadSingleFile(remoteName, filename, fromDir, toDir string,
	force bool, callback SyncProgressCallback) (errorList []string, abort bool) {
	// Check to see if the file is already there, right size
	srcfilename := filepath.Join(fromDir, filename)
	srcfi, err := os.Stat(srcfilename)
	if err != nil {
		if callback != nil {
			if callback(filename, util.ProgressNotFound, 0, 0) {
				return errorList, true
			}
		}
		// Note how we don't add an error to the returned error list
		// As per provider docs, we simply tell callback it happened & treat it
		// as a skipped item otherwise, since caller can only request files & not know
		// if they're on the remote or not
		// Keep going with other files
		return errorList, false
	}

	destfilename := filepath.Join(toDir, filename)
	if !force {
		// Check existence & size before downloading
		if destfi, err := os.Stat(destfilename); err == nil {
			// File exists locally, check the size
			if destfi.Size() == srcfi.Size() {
				// File already present and correct size, skip
				if callback != nil {
					if callback(filename, util.ProgressSkip, srcfi.Size(), srcfi.Size()) {
						return errorList, true
					}
				}
				return errorList, false
			}
		}
	}

	// Make sure dest dir exists
	parentDir := filepath.Dir(destfilename)
	err = os.MkdirAll(parentDir, 0755)
	if err != nil {
		msg := fmt.Sprintf("Unable to create dir %v: %v", parentDir, err)
		errorList = append(errorList, msg)
		return errorList, false
	}
	// Create a temporary file to copy, avoid issues with interruptions
	// Note this isn't a valid thing to do in security conscious cases but this isn't one
	// by opening the file we will get a unique temp file name (albeit a predictable one)
	outf, err := ioutil.TempFile(parentDir, "tempdownload")
	if err != nil {
		msg := fmt.Sprintf("Unable to create temp file for download in %v: %v", parentDir, err)
		errorList = append(errorList, msg)
		return errorList, false
	}
	tmpfilename := outf.Name()
	// This is safe to do even though we manually close & rename because both calls are no-ops if we succeed
	defer func() {
		outf.Close()
		os.Remove(tmpfilename)
	}()
	inf, err := os.OpenFile(srcfilename, os.O_RDONLY, 0644)
	if err != nil {
		msg := fmt.Sprintf("Unable to read input file for download %v: %v", srcfilename, err)
		errorList = append(errorList, msg)
		return errorList, false
	}
	defer inf.Close()

	// Initial callback
	if callback != nil {
		if callback(filename, util.ProgressTransferBytes, 0, srcfi.Size()) {
			return errorList, true
		}
	}
	var copysize int64 = 0
	for {
		var n int64
		n, err = io.CopyN(outf, inf, FileSystemBufferSize)
		copysize += n
		if n > 0 && callback != nil && srcfi.Size() > 0 {
			if callback(filename, util.ProgressTransferBytes, copysize, srcfi.Size()) {
				return errorList, true
			}
		}
		if err != nil {
			break
		}
	}
	outf.Close()
	inf.Close()
	if copysize != srcfi.Size() {
		os.Remove(tmpfilename)
		var msg string
		if err != nil {
			msg = fmt.Sprintf("Problem while downloading %v from %v: %v", srcfilename, remoteName, err)
		} else {
			msg = fmt.Sprintf("Download error: number of bytes read from %v in download of %v does not agree (%d/%d)",
				remoteName, srcfilename, copysize, srcfi.Size())
		}
		errorList = append(errorList, msg)
		return errorList, false
	}
	// Otherwise, file data is ok on remote
	// Move to correct location - remove before to deal with force or bad size cases
	os.Remove(destfilename)
	os.Rename(tmpfilename, destfilename)
	return errorList, false

}

func (self *FileSystemSyncProvider) Download(remoteName string, filenames []string, toDir string,
	force bool, callback SyncProgressCallback) error {

	srcpath, err := self.getRemoteRootPath(remoteName)
	if err != nil {
		return err
	}

	// Check dir exists & also extract permissions to use
	srcpathfi, err := os.Stat(srcpath)
	if err != nil || !srcpathfi.IsDir() {
		return fmt.Errorf("git-lob-path '%v' for remote '%v' is not a valid directory", srcpath, remoteName)
	}

	var errorList []string
	for _, filename := range filenames {
		// Allow aborting
		newerrs, abort := self.downloadSingleFile(remoteName, filename, srcpath, toDir, force, callback)
		errorList = append(errorList, newerrs...)
		if abort {
			break
		}
	}

	if len(errorList) > 0 {
		return errors.New(strings.Join(errorList, "\n"))
	}

	return nil
}

func (*FileSystemSyncProvider) getRemoteRootPath(remoteName string) (string, error) {
	// Check config
	path, ok := util.GlobalOptions.GitConfig[fmt.Sprintf("remote.%v.git-lob-path", remoteName)]
	if !ok {
		return "", fmt.Errorf("Missing git-lob-path config parameter for remote '%v'", remoteName)
	}

	// clean up the path
	path = filepath.Clean(path)

	return path, nil
}

func (self *FileSystemSyncProvider) FileExists(remoteName, filename string) bool {
	root, err := self.getRemoteRootPath(remoteName)
	if err != nil {
		return false
	}
	fullpath := filepath.Join(root, filename)
	_, err = os.Stat(fullpath)

	return err == nil
}
func (self *FileSystemSyncProvider) FileExistsAndIsOfSize(remoteName, filename string, sz int64) bool {
	root, err := self.getRemoteRootPath(remoteName)
	if err != nil {
		return false
	}
	fullpath := filepath.Join(root, filename)
	stat, err := os.Stat(fullpath)

	return err == nil && stat.Size() == sz

}
