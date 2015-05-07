package core

import (
	"bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/cloudflare/bm"
	"bitbucket.org/sinbad/git-lob/util"
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

const BUFSIZE = 131072

// Chunk size that we split stored data into so it's easier to resume uploads/downloads
// This used to be configurable, but it caused too many issues if different people had different
// settings in a shared repository
// This is only 'var' rather than 'const' to allow tests to modify
var ChunkSize = int64(32 * 1024 * 1024)

// Information about a LOB
type LOBInfo struct {
	// SHA of the LOB
	SHA string
	// Total size of the LOB (all chunks)
	Size int64
	// Number of chunks that make up the whole LOB (integrity check)
	NumChunks int
}

// Gets the root directory for local LOB files & creates if necessary
func GetLocalLOBRoot() string {
	ret := filepath.Join(util.GetGitDir(), "git-lob", "content")
	err := os.MkdirAll(ret, 0755)
	if err != nil {
		util.LogErrorf("Unable to create LOB root folder at %v: %v", ret, err)
		panic(err)
	}
	return ret
}

// Gets the root directory for shared LOB files & creates if necessary
func GetSharedLOBRoot() string {
	// We create shared store when loading config if specified
	return util.GlobalOptions.SharedStore
}

// Get relative directory for some base dir for a given sha
func getLOBRelativeDir(sha string) string {
	return filepath.Join(sha[:3], sha[3:6])
}

// Get a relative file name for a meta file (no dirs created as not rooted)
func GetLOBMetaRelativePath(sha string) string {
	return filepath.Join(getLOBRelativeDir(sha), getLOBMetaFilename(sha))
}

// Get a relative file name for a meta file (no dirs created as not rooted)
func GetLOBChunkRelativePath(sha string, chunkIdx int) string {
	return filepath.Join(getLOBRelativeDir(sha), getLOBChunkFilename(sha, chunkIdx))
}

// Get absolute directory for a sha & creates it
func getLOBSubDir(base, sha string) string {
	ret := filepath.Join(base, getLOBRelativeDir(sha))
	err := os.MkdirAll(ret, 0755)
	if err != nil {
		util.LogErrorf("Unable to create LOB 2nd-level folder at %v: %v", ret, err)
		panic(err)
	}
	return ret

}

// Gets the containing local folder for a given LOB SHA & creates if necessary
// LOBs are 'splayed' 2-levels deep based on first 6 chars of SHA (3 for each dir)
// We splay by 2 levels and by 3 each (4096 dirs) because we don't pack like git
// so need to ensure directory contents remain practical at high numbers of files
func GetLocalLOBDir(sha string) string {
	if len(sha) != 40 {
		util.LogErrorf("Invalid SHA format: %v\n", sha)
		return ""
	}
	return getLOBSubDir(GetLocalLOBRoot(), sha)
}

// Gets the containing shared folder for a given LOB SHA & creates if necessary
// LOBs are 'splayed' 2-levels deep based on first 6 chars of SHA (3 for each dir)
// We splay by 2 levels and by 3 each (4096 dirs) because we don't pack like git
// so need to ensure directory contents remain practical at high numbers of files
func GetSharedLOBDir(sha string) string {
	if len(sha) != 40 {
		util.LogErrorf("Invalid SHA format: %v\n", sha)
		return ""
	}
	return getLOBSubDir(GetSharedLOBRoot(), sha)
}

// get the filename for a meta file (no dir)
func getLOBMetaFilename(sha string) string {
	return sha + "_meta"
}

// get the filename for a chunk file (no dir)
func getLOBChunkFilename(sha string, chunkIdx int) string {
	return fmt.Sprintf("%v_%d", sha, chunkIdx)
}

// Gets the absolute path to the meta file for a LOB from a base dir
func GetLOBMetaPathInBaseDir(basedir, sha string) string {
	fld := getLOBSubDir(basedir, sha)
	return filepath.Join(fld, getLOBMetaFilename(sha))
}

// Gets the absolute path to the chunk file for a LOB from a base dir
func GetLOBChunkPathInBaseDir(basedir, sha string, chunkIdx int) string {
	fld := getLOBSubDir(basedir, sha)
	return filepath.Join(fld, getLOBChunkFilename(sha, chunkIdx))
}

// Gets the absolute path to the meta file for a LOB in local store
func GetLocalLOBMetaPath(sha string) string {
	return GetLOBMetaPathInBaseDir(GetLocalLOBRoot(), sha)
}

// Gets the absolute path to the chunk file for a LOB in local store
func GetLocalLOBChunkPath(sha string, chunkIdx int) string {
	return GetLOBChunkPathInBaseDir(GetLocalLOBRoot(), sha, chunkIdx)
}

// Gets the absolute path to the meta file for a LOB in shared store
func getSharedLOBMetaPath(sha string) string {
	return GetLOBMetaPathInBaseDir(GetSharedLOBRoot(), sha)
}

// Gets the absolute path to the chunk file for a LOB in local store
func GetSharedLOBChunkPath(sha string, chunkIdx int) string {
	return GetLOBChunkPathInBaseDir(GetSharedLOBRoot(), sha, chunkIdx)
}

// Retrieve information about an existing stored LOB, from a base dir
func getLOBInfoInBaseDir(sha, basedir string) (*LOBInfo, error) {
	file := GetLOBMetaPathInBaseDir(basedir, sha)
	_, err := os.Stat(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewNotFoundError(err.Error(), file)
		}
		return nil, err
	}

	info, err := parseLOBInfoFromFile(file)
	if err != nil {
		return nil, NewIntegrityErrorWithAdditionalMessage([]string{sha}, err.Error())
	}
	return info, nil
}

// Retrieve information about an existing stored LOB (local)
func GetLOBInfo(sha string) (*LOBInfo, error) {
	info, err := getLOBInfoInBaseDir(sha, GetLocalLOBRoot())
	if err != nil {
		if IsNotFoundError(err) {
			// Try to recover from shared
			if recoverLocalLOBFilesFromSharedStore(sha) {
				info, err = getLOBInfoInBaseDir(sha, GetLocalLOBRoot())
				if err != nil {
					// Dang
					return nil, err
				}
				// otherwise we recovered!
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return info, nil
}

// Parse a LOB meta file
func parseLOBInfoFromFile(file string) (*LOBInfo, error) {
	infobytes, err := ioutil.ReadFile(file)

	if err != nil {
		return nil, err
	}
	// Read JSON metadata
	info := &LOBInfo{}
	err = json.Unmarshal(infobytes, info)
	if err != nil {
		// Fatal, corruption
		return nil, errors.New(fmt.Sprintf("Unable to interpret meta file %v: %v", file, err))
	}

	return info, nil

}

// If files are missing in the local repo but available in the shared
// store, returns true after re-establishing the link
// Note: this doesn't validate sizes of any files because it's assumed
// because of hardlinking the files are either missing entirely or the
// same as the shared store
func recoverLocalLOBFilesFromSharedStore(sha string) bool {
	if !IsUsingSharedStorage() {
		return false
	}

	metalocal := GetLocalLOBMetaPath(sha)
	if !util.FileExists(metalocal) {
		metashared := getSharedLOBMetaPath(sha)
		if util.FileExists(metashared) {
			err := linkSharedLOBFilename(metashared)
			if err != nil {
				util.LogErrorf("Failed to link shared file %v into local repo: %v\n", metashared, err.Error())
				return false
			}
		} else {
			return false
		}
	}
	// Meta should be complete & local now
	info, err := GetLOBInfo(sha)
	if err != nil {
		return false
	}
	for i := 0; i < info.NumChunks; i++ {
		local := GetLocalLOBChunkPath(sha, i)
		expectedSize := getLOBExpectedChunkSize(info, i)
		if !util.FileExistsAndIsOfSize(local, expectedSize) {
			shared := GetSharedLOBChunkPath(sha, i)
			if util.FileExistsAndIsOfSize(shared, expectedSize) {
				err := linkSharedLOBFilename(shared)
				if err != nil {
					util.LogErrorf("Failed to link shared file %v into local repo: %v\n", shared, err.Error())
					return false
				}
			} else {
				return false
			}
		}
	}

	return true
}

// Retrieve LOB from storage
func RetrieveLOB(sha string, out io.Writer) (info *LOBInfo, err error) {
	info, err = GetLOBInfo(sha)

	if err != nil {
		if IsNotFoundError(err) && util.GlobalOptions.AutoFetchEnabled {
			err = AutoFetch(sha, true)
			if err == nil {
				info, err = GetLOBInfo(sha)
			}
		}
		if err != nil {
			if IsNotFoundError(err) {
				// Still not found after possible recovery?
				return nil, err
			} else {
				// Some other issue
				return nil, errors.New(fmt.Sprintf("Unable to retrieve LOB with SHA %v: %v", sha, err.Error()))
			}
		}
	}

	var totalBytesRead = int64(0)
	fileSize := info.Size
	// Pre-validate all the files BEFORE we start streaming data to out
	// if we fail part way through we don't want to have written partial
	// data, should be all or nothing
	lastChunkSize := fileSize - (int64(info.NumChunks-1) * ChunkSize)
	// Check all files
	for i := 0; i < info.NumChunks; i++ {
		chunkFilename := GetLocalLOBChunkPath(sha, i)
		var expectedSize int64
		if i+1 < info.NumChunks {
			expectedSize = ChunkSize
		} else {
			if info.NumChunks == 1 {
				expectedSize = fileSize
			} else {
				expectedSize = lastChunkSize
			}
		}
		if !util.FileExistsAndIsOfSize(chunkFilename, expectedSize) {
			// Try to recover from shared store
			recoveredFromShared := false
			if recoverLocalLOBFilesFromSharedStore(sha) {
				recoveredFromShared = util.FileExistsAndIsOfSize(chunkFilename, expectedSize)
			}

			if !recoveredFromShared {
				if util.GlobalOptions.AutoFetchEnabled {
					err = AutoFetch(sha, true)
					if err != nil {
						if IsNotFoundError(err) {
							return info, NewNotFoundError(fmt.Sprintf("Missing chunk %d for %v & not on remote", i, sha), chunkFilename)
						} else {
							return info, errors.New(fmt.Sprintf("Missing chunk %d for %v & failed fetch: %v", i, sha, err.Error()))
						}
					}
				} else {
					return info, NewNotFoundError(fmt.Sprintf("Missing chunk %d for %v", i, sha), chunkFilename)
				}
			}
		}
	}
	// If all was well, start reading & streaming content
	for i := 0; i < info.NumChunks; i++ {
		// Check each chunk file exists
		chunkFilename := GetLocalLOBChunkPath(info.SHA, i)
		in, err := os.OpenFile(chunkFilename, os.O_RDONLY, 0644)
		if err != nil {
			return info, errors.New(fmt.Sprintf("Error reading LOB file %v: %v", chunkFilename, err))
		}
		c, err := io.Copy(out, in)
		if err != nil {
			return info, errors.New(fmt.Sprintf("I/O error while copying LOB file %v, check working copy state", chunkFilename))
		}
		totalBytesRead += c
	}

	// Final check
	if totalBytesRead != fileSize {
		err = errors.New(fmt.Sprintf("Error, file length does not match expected in LOB %v, expected %d, total size %d", sha, fileSize, totalBytesRead))
		return info, err
	}

	util.LogDebugf("Successfully retrieved LOB %v from %d chunks, total size %v\n", sha, info.NumChunks, util.FormatSize(totalBytesRead))

	return info, nil

}

// Link a file from shared storage into the local repo
// The hard link means we only ever have one copy of the data
// but it appears under each repo's git-lob folder
// destFile should be a full path of shared file location
func linkSharedLOBFilename(destSharedFile string) error {
	// Get path relative to shared store root, then translate it to local path
	relPath, err := filepath.Rel(util.GlobalOptions.SharedStore, destSharedFile)
	if err != nil {
		return err
	}
	linkPath := filepath.Join(GetLocalLOBRoot(), relPath)

	// Make sure path exists since we're not using utility method to link
	os.MkdirAll(filepath.Dir(linkPath), 0755)

	os.Remove(linkPath)
	err = CreateHardLink(destSharedFile, linkPath)
	if err != nil {
		return errors.New(fmt.Sprintf("Error creating hard link from %v to %v: %v", linkPath, destSharedFile, err))
	}
	return nil
}

// Store the metadata for a given sha
// If it already exists and is of the right size, will do nothing
func StoreLOBInfo(info *LOBInfo) error {
	var root string
	if IsUsingSharedStorage() {
		root = GetSharedLOBRoot()
	} else {
		root = GetLocalLOBRoot()
	}
	return StoreLOBInfoInBaseDir(root, info)
}

// Store the metadata for a given sha in a relative path
// If it already exists and is of the right size, will do nothing
func StoreLOBInfoInBaseDir(basedir string, info *LOBInfo) error {
	infoBytes, err := json.Marshal(info)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to convert LOB info to JSON: %v", err))
	}
	infoFilename := GetLOBMetaPathInBaseDir(basedir, info.SHA)
	if !util.FileExistsAndIsOfSize(infoFilename, int64(len(infoBytes))) {
		// Since all the details are derived from the SHA the only variant is chunking or incomplete writes so
		// we don't need to worry about needing to update the content (it must be correct)
		util.LogDebugf("Writing LOB metadata file: %v\n", infoFilename)
		err = ioutil.WriteFile(infoFilename, infoBytes, 0644)
		if err != nil {
			return err
		}
	} else {
		util.LogDebugf("LOB metadata file already exists & is valid: %v\n", infoFilename)
	}

	// This may have stored in shared storage, so link if required
	if IsUsingSharedStorage() && basedir == GetSharedLOBRoot() {
		return linkSharedLOBFilename(infoFilename)
	} else {
		return nil
	}

}

func IsUsingSharedStorage() bool {
	if util.GlobalOptions.SharedStore != "" {
		// We create the folder on loading config
		return util.DirExists(util.GlobalOptions.SharedStore)
	}
	return false
}

// Write the contents of fromFile to final storage with sha, checking the size
// If file already exists and is of the right size, will do nothing
// fromChunkFile will be moved into its final location or deleted if the data is already valid,
// so the file will not exist after this call (renamed to final location or deleted), unless error
func StoreLOBChunk(sha string, chunkNo int, fromChunkFile string, sz int64) error {
	var root string
	if IsUsingSharedStorage() {
		root = GetSharedLOBRoot()
	} else {
		root = GetLocalLOBRoot()
	}
	return StoreLOBChunkInBaseDir(root, sha, chunkNo, fromChunkFile, sz)
}

// Write the contents of fromFile to final storage with sha, checking the size, to a relative root
// If file already exists and is of the right size, will do nothing
// fromChunkFile will be moved into its final location or deleted if the data is already valid,
// so the file will not exist after this call (renamed to final location or deleted), unless error
func StoreLOBChunkInBaseDir(basedir, sha string, chunkNo int, fromChunkFile string, sz int64) error {
	destFile := GetLOBChunkPathInBaseDir(basedir, sha, chunkNo)

	if !util.FileExistsAndIsOfSize(destFile, int64(sz)) {
		util.LogDebugf("Saving final LOB metadata file: %v\n", destFile)
		// delete any existing (incorrectly sized) file since will probably not be allowed to rename over it
		// ignore any errors
		os.Remove(destFile)
		err := os.Rename(fromChunkFile, destFile)
		if err != nil {
			return err
		}
	} else {
		util.LogDebugf("LOB chunk file already exists & is valid: %v\n", destFile)
		// Remove file that would have been moved
		os.Remove(fromChunkFile)
	}

	// This may have stored in shared storage, so link if required
	if IsUsingSharedStorage() && basedir == GetSharedLOBRoot() {
		return linkSharedLOBFilename(destFile)
	}
	return nil

}

// Read from a stream and calculate SHA, while also writing content to chunked content
// leader is a slice of bytes that has already been read (probe for SHA)
func StoreLOB(in io.Reader, leader []byte) (*LOBInfo, error) {
	var root string
	if IsUsingSharedStorage() {
		root = GetSharedLOBRoot()
	} else {
		root = GetLocalLOBRoot()
	}
	return StoreLOBInBaseDir(root, in, leader)
}

// Read from a stream and calculate SHA, while also writing content to chunked content
// leader is a slice of bytes that has already been read (probe for SHA)
// Store underneath a specified LOB root
func StoreLOBInBaseDir(basedir string, in io.Reader, leader []byte) (*LOBInfo, error) {
	sha := sha1.New()
	// Write chunks to temporary files, then move based on SHA filename once calculated
	chunkFilenames := make([]string, 0, 5)

	var outf *os.File
	var err error
	writeLeader := true
	buf := make([]byte, BUFSIZE)
	var fatalError error
	var currentChunkSize int64 = 0
	var totalSize int64 = 0

	for {
		var dataToWrite []byte

		if writeLeader && len(leader) > 0 {
			dataToWrite = leader
			writeLeader = false
		} else {
			var bytesToRead int64 = BUFSIZE
			if BUFSIZE+currentChunkSize > ChunkSize {
				// Read less than BUFSIZE so we stick to CHUNKLIMIT
				bytesToRead = ChunkSize - currentChunkSize
			}
			c, err := in.Read(buf[:bytesToRead])
			// Write any data to SHA & output
			if c > 0 {
				dataToWrite = buf[:c]
			} else if err != nil {
				if err == io.EOF {
					// End of input
					outf.Close()
					break
				} else {
					outf.Close()
					fatalError = errors.New(fmt.Sprintf("I/O error reading chunk %d: %v", len(chunkFilenames), err))
					break
				}
			}

		}

		// Write data
		if len(dataToWrite) > 0 {
			// New chunk file?
			if outf == nil {
				outf, err = ioutil.TempFile("", "tempchunk")
				if err != nil {
					fatalError = errors.New(fmt.Sprintf("Unable to create chunk %d: %v", len(chunkFilenames), err))
					break
				}
				chunkFilenames = append(chunkFilenames, outf.Name())
				currentChunkSize = 0
			}
			sha.Write(dataToWrite)
			c, err := outf.Write(dataToWrite)
			if err != nil {
				fatalError = errors.New(fmt.Sprintf("I/O error writing chunk: %v wrote %d bytes of %d", err, c, len(dataToWrite)))
				break
			}
			currentChunkSize += int64(c)
			totalSize += int64(c)

			// Read from incoming
			// Deal with chunk limit
			if currentChunkSize >= ChunkSize {
				// Close this output, next iteration will create the next file
				outf.Close()
				outf = nil
				currentChunkSize = 0
			}
		} else {
			// No data to write
			outf.Close()
			break
		}
	}
	if outf != nil {
		// Close any dangling chunk
		outf.Close()
	}
	defer func() {
		// Clean up any temporaries on error or not used
		for _, f := range chunkFilenames {
			os.Remove(f)
		}
	}()

	if fatalError != nil {
		return nil, fatalError
	}

	shaStr := fmt.Sprintf("%x", string(sha.Sum(nil)))

	// We *may* now move the data to LOB dir
	// We won't if it already exists & is the correct size
	// Construct LOBInfo & write to final location
	info := &LOBInfo{SHA: shaStr, Size: totalSize, NumChunks: len(chunkFilenames)}
	err = StoreLOBInfoInBaseDir(basedir, info)
	if err != nil {
		return nil, err
	}

	// Check each chunk file
	for i, f := range chunkFilenames {
		sz := ChunkSize
		if i+1 == len(chunkFilenames) {
			// Last chunk, get size
			sz = currentChunkSize
		}
		err = StoreLOBChunkInBaseDir(basedir, shaStr, i, f, sz)
		if err != nil {
			return nil, err
		}
	}

	return info, nil

}

// Delete all files associated with a given LOB SHA from the local store
func DeleteLOB(sha string) error {
	// Delete from local always (either only copy, or hard link)
	return DeleteLOBInBaseDir(sha, GetLocalLOBRoot())
}

// Delete all files associated with a given LOB SHA from a specified root dir
func DeleteLOBInBaseDir(sha, basedir string) error {

	dir := getLOBSubDir(basedir, sha)
	names, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("%v*", sha)))
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to glob local files for %v: %v", sha, err))
	}
	for _, n := range names {
		err = os.Remove(n)
		if err != nil {
			return errors.New(fmt.Sprintf("Unable to delete file %v: %v", n, err))
		}
	}

	if IsUsingSharedStorage() && basedir != GetSharedLOBRoot() {
		// If we're using shared storage, then also check the number of links in
		// shared storage for this SHA. See PruneSharedStore for a more general
		// sweep for files that don't go through DeleteLOB (e.g. repo deleted manually)
		shareddir := GetSharedLOBDir(sha)
		names, err := filepath.Glob(filepath.Join(shareddir, fmt.Sprintf("%v*", sha)))
		if err != nil {
			return errors.New(fmt.Sprintf("Unable to glob shared files for %v: %v", sha, err))
		}
		for _, n := range names {
			links, err := GetHardLinkCount(n)
			if err == nil && links == 1 {
				// only 1 hard link means no other repo refers to this shared LOB
				// so it's safe to delete it
				err = os.Remove(n)
				if err != nil {
					return errors.New(fmt.Sprintf("Unable to delete file %v: %v", n, err))
				}
			}

		}

	}

	return nil

}

// Get the local/shared storage of a LOB with a given SHA
// Returns the list of files (relative to basedir) & checks for
// integrity if check = true
// If check = true and checkHash = true, reads all the data in the files and re-calculates
// the SHA for a deep validation of content
// If check = true and checkHash = false, just checks the presence & size of all files
// If there are any errors the returned list may not be correct
// In the rare case that a break has occurred between shared storage
// and the local hardlink, this method will re-link if the shared
// store has it
func GetLOBFilesForSHA(sha, basedir string, check bool, checkHash bool) (files []string, size int64, _err error) {
	var ret []string
	info, err := getLOBInfoInBaseDir(sha, basedir)
	if err != nil {
		return []string{}, 0, err
	}
	// add meta file (relative) - already checked by GetLOBInfo above
	relmeta := GetLOBMetaRelativePath(sha)
	ret = append(ret, relmeta)

	var shaRecalc hash.Hash
	if checkHash {
		shaRecalc = sha1.New()
	}
	lastChunkSize := info.Size - (int64(info.NumChunks-1) * ChunkSize)
	for i := 0; i < info.NumChunks; i++ {
		relchunk := GetLOBChunkRelativePath(sha, i)
		ret = append(ret, relchunk)
		if check {
			abschunk := filepath.Join(basedir, relchunk)
			// Check size first
			var expectedSize int64
			if i+1 < info.NumChunks {
				expectedSize = ChunkSize
			} else {
				if info.NumChunks == 1 {
					expectedSize = info.Size
				} else {
					expectedSize = lastChunkSize
				}
			}
			if !util.FileExistsAndIsOfSize(abschunk, expectedSize) {
				// Try to recover from shared store
				recoveredFromShared := false
				if recoverLocalLOBFilesFromSharedStore(sha) {
					recoveredFromShared = util.FileExistsAndIsOfSize(abschunk, expectedSize)
				}

				if !recoveredFromShared {
					msg := fmt.Sprintf("LOB file not found or wrong size: %v expected to be %d bytes", abschunk, expectedSize)
					wrongSize := util.FileExists(abschunk)
					var err error
					if wrongSize {
						err = NewWrongSizeError(msg, abschunk)
					} else {
						err = NewNotFoundError(msg, abschunk)
					}
					return ret, info.Size, err
				}
			}

			// Check SHA content?
			if checkHash {
				f, err := os.OpenFile(abschunk, os.O_RDONLY, 0644)
				if err != nil {
					msg := fmt.Sprintf("Error opening LOB file %v to check SHA: %v", abschunk, err)
					return ret, info.Size, errors.New(msg)
				}
				_, err = io.Copy(shaRecalc, f)
				if err != nil {
					msg := fmt.Sprintf("Error copying LOB file %v into SHA calculator: %v", abschunk, err)
					return ret, info.Size, errors.New(msg)
				}
				f.Close()
			}

		}
	}

	if check && checkHash {
		shaRecalcStr := fmt.Sprintf("%x", string(shaRecalc.Sum(nil)))
		if sha != shaRecalcStr {
			return ret, info.Size, NewIntegrityError([]string{sha})
		}
	}

	return ret, info.Size, nil

}

// Check the integrity of the files for a given sha in the attached basedir
// If checkHash = true, reads all the data in the files and re-calculates
// the SHA for a deep validation of content (slower but complete)
// If checkHash = false, just checks the presence & size of all files (quick & most likely correct)
// Note that if basedir is the local root, will try to recover missing files from shared store
func CheckLOBFilesForSHA(sha, basedir string, checkHash bool) error {
	_, _, err := GetLOBFilesForSHA(sha, basedir, true, checkHash)
	return err
}

// Check the presence & integrity of the files for a given list of shas in this repo
// and return a list of those which failed the check
// If checkHash = true, reads all the data in the files and re-calculates
// the SHA for a deep validation of content (slower but complete)
// If checkHash = false, just checks the presence & size of all files (quick & most likely correct)
func GetMissingLOBs(lobshas []string, checkHash bool) []string {
	localroot := GetLocalLOBRoot()
	var missing []string
	for _, sha := range lobshas {
		err := CheckLOBFilesForSHA(sha, localroot, checkHash)
		if err != nil {
			// Recover from shared storage if possible
			if IsUsingSharedStorage() && recoverLocalLOBFilesFromSharedStore(sha) {
				// then we're OK
			} else {
				missing = append(missing, sha)
			}
		}
	}
	return missing
}

// Return whether a single LOB is missing
func IsLOBMissing(sha string, checkHash bool) bool {
	localroot := GetLocalLOBRoot()
	err := CheckLOBFilesForSHA(sha, localroot, checkHash)
	if err != nil {
		// Recover from shared storage if possible
		if IsUsingSharedStorage() && recoverLocalLOBFilesFromSharedStore(sha) {
			// then we're OK
		} else {
			return true
		}
	}

	return false
}

// Get the correct size of a given chunk
func getLOBExpectedChunkSize(info *LOBInfo, chunkIdx int) int64 {
	if chunkIdx+1 < info.NumChunks {
		return ChunkSize
	} else {
		if info.NumChunks == 1 {
			return info.Size
		} else {
			return info.Size - (int64(info.NumChunks-1) * ChunkSize)
		}
	}

}

// returns whether the local store has any binaries in it
func IsLocalLOBStoreEmpty() bool {
	root := GetLocalLOBRoot()
	rootf, err := os.Open(root)
	if err != nil {
		return true
	}
	defer rootf.Close()
	// Max 3 entries
	dirs, err := rootf.Readdirnames(3)
	if err != nil {
		return true
	}
	// Will be no entries if this is new
	return len(dirs) == 0
}

// Generates a diff between the contents of 2 LOBs
// Automatically copes with chunking, the diff is one file across the entire content
// Returns the size of the compressed delta
func GenerateLOBDelta(basesha, targetsha string, out io.Writer) (int64, error) {
	return GenerateLOBDeltaInBaseDir(GetLocalLOBRoot(), basesha, targetsha, out)
}

// Applies a diff to basesha and generates a LOB which should have targetsha (will be checked, error returned if disagrees)
func ApplyLOBDelta(basesha, targetsha string, delta io.Reader) error {
	var root string
	if IsUsingSharedStorage() {
		root = GetSharedLOBRoot()
	} else {
		root = GetLocalLOBRoot()
	}
	err := ApplyLOBDeltaInBaseDir(root, basesha, targetsha, delta)
	if err != nil {
		// This may have stored in shared storage, so link if required
		if IsUsingSharedStorage() {
			recoverLocalLOBFilesFromSharedStore(targetsha)
		}
	}
	return err
}

// Retrieve the entire content for all chunks of a LOB and write to 'out'
func GetLOBCompleteContent(sha string, out io.Writer) error {
	return GetLOBCompleteContentInBaseDir(GetLocalLOBRoot(), sha, out)
}

// Retrieve the entire content for all chunks of a LOB within a base root, and write to 'out'
func GetLOBCompleteContentInBaseDir(basedir, sha string, out io.Writer) error {
	info, err := getLOBInfoInBaseDir(sha, basedir)
	if err != nil {
		return err
	}
	var bytesread int64
	for i := 0; i < info.NumChunks; i++ {
		chunkfile := filepath.Join(basedir, GetLOBChunkRelativePath(sha, i))
		cf, err := os.OpenFile(chunkfile, os.O_RDONLY, 0644)
		if err != nil {
			return err
		}
		defer cf.Close()
		n, err := io.Copy(out, cf)
		if err != nil {
			return fmt.Errorf("Error while copying data from content: %v", err.Error())
		}
		bytesread += n
	}
	if bytesread != info.Size {
		return fmt.Errorf("Incorrect number of bytes read for LOB - expected %d actual %d", info.Size, bytesread)
	}
	return nil
}

// Generates a diff between the contents of 2 LOBs, with a specified root storage
// Automatically copes with chunking, the diff is one file across the entire content
// Returns the size of the compressed delta
func GenerateLOBDeltaInBaseDir(basedir, basesha, targetsha string, out io.Writer) (int64, error) {
	// Read all of base file into memory to use as dictionary (pre-size from info)
	baseinfo, err := getLOBInfoInBaseDir(basesha, basedir)
	basebuf := bytes.NewBuffer(make([]byte, 0, baseinfo.Size))
	if err != nil {
		return 0, err
	}
	err = GetLOBCompleteContentInBaseDir(basedir, basesha, basebuf)
	if err != nil {
		return 0, fmt.Errorf("Error getting base file content for delta: %v", err.Error())
	}
	comp := bm.NewCompressor()
	baseDict := &bm.Dictionary{Dict: basebuf.Bytes()}
	// Use SetDictionary to set on compressor, this computes the hashes
	comp.SetDictionary(baseDict)
	// Set the delta buffer as the output
	comp.SetWriter(out)

	// Now we read all the targetsha's content and copy it into the compressor
	targetinfo, err := getLOBInfoInBaseDir(targetsha, basedir)
	if err != nil {
		return 0, err
	}
	var targetbytesread int64
	for i := 0; i < targetinfo.NumChunks; i++ {
		chunkfile := filepath.Join(basedir, GetLOBChunkRelativePath(targetsha, i))
		cf, err := os.OpenFile(chunkfile, os.O_RDONLY, 0644)
		if err != nil {
			return 0, err
		}
		defer cf.Close()
		n, err := io.Copy(comp, cf)
		if err != nil {
			return 0, fmt.Errorf("Error while copying data from target into compressor: %v", err.Error())
		}
		targetbytesread += n
	}
	if targetbytesread != targetinfo.Size {
		return 0, fmt.Errorf("Incorrect number of bytes read for target file - expected %d actual %d", targetinfo.Size, targetbytesread)
	}

	// Now do the actual compression
	// Maybe we can improve bm later so that it does it on the fly (less memory)
	err = comp.Close()
	if err != nil {
		return 0, fmt.Errorf("Error during compression of delta: %v", err.Error())
	}
	// This has been written to out now so we're done

	return int64(comp.CompressedSize()), nil
}

// Applies a diff to basesha and generates a LOB, with a specified root storage,
// which should have targetsha (will be checked, error returned if disagrees)
func ApplyLOBDeltaInBaseDir(basedir, basesha, targetsha string, delta io.Reader) error {
	// Read all of base file into memory to use as dictionary (pre-size from info)
	baseinfo, err := getLOBInfoInBaseDir(basesha, basedir)
	basebuf := bytes.NewBuffer(make([]byte, 0, baseinfo.Size))
	if err != nil {
		return err
	}
	err = GetLOBCompleteContentInBaseDir(basedir, basesha, basebuf)
	if err != nil {
		return fmt.Errorf("Error getting base file content for delta: %v", err.Error())
	}

	exp := bm.NewExpander(delta, basebuf.Bytes())

	// output result to temp file
	outf, err := ioutil.TempFile("", fmt.Sprintf("tempdelta%v_%v", basesha, targetsha))
	if err != nil {
		return fmt.Errorf("Error opening temp file for writing: %v\n", err)
	}
	defer outf.Close()
	defer os.Remove(outf.Name()) // always remove temp file if not moved

	// bm.Expander claims to support io.Reader but it doesn't
	// so can't use the same io.Copy approach we use for Compressor
	// Probably because it doesn't currently implement the buffering required to support arbitrary Read() calls
	// Do it all in memory right now - we should probably enhance bm to make this more efficient
	// Reading the code, the input to Expand is the current slice if you want it
	// good to pre-allocate some space here, estimate same size as base
	outbytes, err := exp.Expand(make([]byte, 0, basebuf.Len()))
	if err != nil {
		return fmt.Errorf("Error applying LOB delta: %v", err)
	}
	// Check the SHA
	shacalc := sha1.New()
	shacalc.Write(outbytes)
	testsha := fmt.Sprintf("%x", string(shacalc.Sum(nil)))
	if testsha != targetsha {
		return fmt.Errorf("Integrity error applying delta, SHA does not agree (expected: %v actual %v)", targetsha, testsha)
	}
	// Otherwise, we're good. Store this data
	targetinfo, err := StoreLOBInBaseDir(basedir, bytes.NewReader(outbytes), nil)
	if err != nil {
		return fmt.Errorf("Error storing target LOB %v: %v", targetsha, err.Error())
	} else if targetinfo.SHA != targetsha {
		return fmt.Errorf("Integrity error saving applied delta, SHA does not agree (expected: %v actual %v)", targetsha, targetinfo)
	}

	return nil
}

// Record of a LOB delta (calculated but still to be done)
type LOBDelta struct {
	BaseSHA, TargetSHA string
	DeltaSize          int64
	// Optional already present delta filename, can be blank
	DeltaFilename string
}
