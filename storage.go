package main

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
)

const BUFSIZE = 8192

// TODO make chunking user-configurable, default to 32MB
// chunk limit should be a multiple of BUFSIZE for max efficiency
const CHUNKLIMIT = BUFSIZE * 4086

// Information about a LOB
type LOBInfo struct {
	// SHA of the LOB
	SHA string
	// Total size of the LOB (all chunks)
	Size int64
	// Number of chunks that make up the whole LOB (integrity check)
	NumChunks int
}

// Gets the root folder of this git repository (the one containing .git)
func GetRepoRoot() (path string, isSeparateGitDir bool) {
	// We could call 'git rev-parse --git-dir' but this requires shelling out = slow, especially on Windows
	// We should try to avoid that whenever we can
	// So let's just find it ourselves; first containing folder with a .git folder/file
	curDir, err := os.Getwd()
	if err != nil {
		LogErrorf("Getwd failed: %v\n", err)
		return "", false
	}
	for {
		exists, isDir := FileOrDirExists(filepath.Join(curDir, ".git"))
		if exists {
			return curDir, !isDir
		}
		curDir = filepath.Dir(curDir)
		if curDir == string(filepath.Separator) || curDir == "." {
			// Not a repo
			LogError("Couldn't find repo root, not a git folder")
			return "", false
		}
	}
}

// Gets the git data dir of git repository (the .git dir, or where .git file points)
func GetGitDir() string {
	root, isSeparate := GetRepoRoot()
	git := filepath.Join(root, ".git")
	if isSeparate {
		// Git repo folder is separate, read location from file
		filebytes, err := ioutil.ReadFile(git)
		if err != nil {
			LogErrorf("Can't read .git file %v: %v\n", git, err)
			return ""
		}
		filestr := string(filebytes)
		match := regexp.MustCompile("gitdir:[\\s]+([^\\r\\n]+)").FindStringSubmatch(filestr)
		if match == nil {
			LogErrorf("Unexpected contents of .git file %v: %v\n", git, filestr)
			return ""
		}
		return match[1]
	} else {
		// Regular git dir
		return git
	}

}

// Gets the root directory for LOB files & creates if necessary
func GetLOBRoot() string {
	ret := filepath.Join(GetGitDir(), "git-lob")
	err := os.MkdirAll(ret, 0777)
	if err != nil {
		LogErrorf("Unable to create LOB root folder at %v: %v", ret, err)
		panic(err)
	}
	return ret
}

// Gets the containing folder for a given LOB SHA & creates if necessary
// LOBs are 'splayed' based on first 2 chars of SHA
func GetLOBDir(sha string) string {
	if len(sha) != 40 {
		LogErrorf("Invalid SHA format: %v\n", sha)
		return ""
	}
	ret := filepath.Join(GetLOBRoot(), sha[:2])
	err := os.MkdirAll(ret, 0777)
	if err != nil {
		LogErrorf("Unable to create LOB 2nd-levle folder at %v: %v", ret, err)
		panic(err)
	}
	return ret
}

func getLOBMetaFilename(sha string) string {
	fld := GetLOBDir(sha)
	return filepath.Join(fld, sha+"_meta")
}
func getLOBChunkFilename(sha string, chunkIdx int) string {
	fld := GetLOBDir(sha)
	return filepath.Join(fld, fmt.Sprintf("%v_%d", sha, chunkIdx))
}

// Retrieve information about an existing stored LOB
func GetLOBInfo(sha string) (*LOBInfo, error) {
	meta := getLOBMetaFilename(sha)
	infobytes, err := ioutil.ReadFile(meta)

	if err != nil {
		// Maybe just that it's not been downloaded yet
		// Let caller decide
		return nil, err
	}
	// Read JSON metadata
	info := &LOBInfo{}
	err = json.Unmarshal(infobytes, info)
	if err != nil {
		// Fatal, corruption
		LogErrorf("Unable to interpret meta file %v: %v\n", meta, err)
		return nil, err
	}

	return info, nil

}

// Retrieve LOB from storage
func RetrieveLOB(sha string, out io.Writer) (info *LOBInfo, err error) {
	info, err = GetLOBInfo(sha)

	if err != nil {
		if os.IsNotExist(err) {
			// We don't have this file yet
			// Potentially auto-download
			// TODO
			LogErrorf("LOB meta not found TODO AUTODOWNLOAD %v: %v\n", sha, err)
			return nil, err
		} else {
			// A problem
			LogErrorf("Unable to retrieve LOB with SHA %v: %v\n", sha, err)
			return nil, err
		}
	}

	var totalBytesRead = int64(0)
	fileSize := info.Size
	// Pre-validate all the files BEFORE we start streaming data to out
	// if we fail part way through we don't want to have written partial
	// data, should be all or nothing
	// Don't assume size of stored file chunks, maybe we want to allow chunk size
	// to change; calculate the total size, and the chunk size so that if
	// one differs we know it's faulty
	lastChunkIdx := info.NumChunks - 1
	lastChunkStat, err := os.Stat(getLOBChunkFilename(sha, lastChunkIdx))
	lastChunkSize := lastChunkStat.Size()
	otherChunksSize := fileSize - lastChunkSize
	var chunkSize int64
	if otherChunksSize > 0 && info.NumChunks > 1 {
		chunkSize = otherChunksSize / int64(info.NumChunks-1)
	} else {
		chunkSize = lastChunkSize
	}
	// Check all files
	for i := 0; i < info.NumChunks; i++ {
		chunkFilename := getLOBChunkFilename(sha, i)
		var expectedSize int64
		if i+1 < info.NumChunks {
			expectedSize = chunkSize
		} else {
			if info.NumChunks == 1 {
				expectedSize = fileSize
			} else {
				expectedSize = fileSize - (chunkSize * int64(info.NumChunks-1))
			}
		}
		if !FileExistsAndIsOfSize(chunkFilename, expectedSize) {
			// TODO auto-download?
			LogErrorf("LOB file not found or wrong size TODO AUTODOWNLOAD: %v expected to be %d bytes\n", chunkFilename, expectedSize)
			return info, err
		}
	}
	// If all was well, start reading & streaming content
	for i := 0; i < info.NumChunks; i++ {
		// Check each chunk file exists
		chunkFilename := getLOBChunkFilename(info.SHA, i)
		in, err := os.OpenFile(chunkFilename, os.O_RDONLY, 0666)
		if err != nil {
			LogErrorf("Error reading LOB file %v: %v\n", chunkFilename, err)
			return info, err
		}
		c, err := io.Copy(out, in)
		if err != nil {
			LogErrorf("I/O error while copying LOB file %v, check working copy state\n", chunkFilename)
			return info, err
		}
		totalBytesRead += c
	}

	// Final check
	if totalBytesRead != fileSize {
		err = errors.New(fmt.Sprintf("Error, file length does not match expected in LOB %v, expected %d, total size %d", sha, fileSize, totalBytesRead))
		LogErrorf(err.Error())
		return info, err
	}

	LogDebugf("Successfully retrieved LOB %v from %d chunks, total size ", sha, info.NumChunks, totalBytesRead)

	return info, nil

}

func storeLOBInfo(info *LOBInfo) error {
	infoBytes, err := json.Marshal(info)
	if err != nil {
		LogErrorf("Unable to convert LOB info to JSON: %v\n", err)
		return err
	}
	infoFilename := getLOBMetaFilename(info.SHA)
	if !FileExistsAndIsOfSize(infoFilename, int64(len(infoBytes))) {
		// Since all the details are derived from the SHA the only variant is chunking or incomplete writes so
		// we don't need to worry about needing to update the content (it must be correct)
		LogDebugf("Writing LOB metadata file: %v\n", infoFilename)
		ioutil.WriteFile(infoFilename, infoBytes, 0666)
	} else {
		LogDebugf("LOB metadata file already exists & is valid: %v\n", infoFilename)
	}
	return nil
}

// Read from a stream and calculate SHA, while also writing content to chunked content
// leader is a slice of bytes that has already been read (probe for SHA)
func StoreLOB(in io.Reader, leader []byte) (*LOBInfo, error) {

	sha := sha1.New()
	// Write chunks to temporary files, then move based on SHA filename once calculated
	chunkFilenames := make([]string, 0, 5)

	var outf *os.File
	var err error
	writeLeader := true
	buf := make([]byte, BUFSIZE)
	var fatalError error
	currentChunkSize := 0
	var totalSize int64 = 0

	for {
		// New chunk file?
		if outf == nil {
			outf, err = ioutil.TempFile("", "tempchunk")
			if err != nil {
				LogErrorf("Unable to create chunk %d: %v\n", len(chunkFilenames), err)
				fatalError = err
				break
			}
			LogDebugf("Creating temporary chunk file #%d: %v\n", len(chunkFilenames), outf.Name())
			chunkFilenames = append(chunkFilenames, outf.Name())
			currentChunkSize = 0
		}
		if writeLeader {
			LogDebugf("Writing leader of size %d to %v\n", len(leader), outf.Name())
			sha.Write(leader)
			c, err := outf.Write(leader)
			if err != nil {
				LogErrorf("I/O error writing leader: %v wrote %d bytes of %d\n", err, c, len(leader))
				fatalError = err
				break
			}
			currentChunkSize += c
			totalSize += int64(c)
			writeLeader = false
		}
		// Read from incoming
		var bytesToRead = BUFSIZE
		if BUFSIZE+currentChunkSize > CHUNKLIMIT {
			// Read less than BUFSIZE so we stick to CHUNKLIMIT
			bytesToRead = CHUNKLIMIT - currentChunkSize
		}
		c, err := in.Read(buf[:bytesToRead])
		// Write any data to SHA & output
		if c > 0 {
			currentChunkSize += c
			totalSize += int64(c)
			sha.Write(buf[:c])
			cw, err := outf.Write(buf[:c])
			if err != nil || cw != c {
				LogErrorf("I/O error writing chunk %d: %v\n", len(chunkFilenames), err)
				fatalError = err
				break
			}
		}
		if err != nil {
			if err == io.EOF {
				// End of input
				outf.Close()
				break
			} else {
				LogErrorf("I/O error reading chunk %d: %v", len(chunkFilenames), err)
				outf.Close()
				fatalError = err
				break
			}
		}
		// Deal with chunk limit
		// NB right now assumes BUFSIZE is an exact divisor of CHUNKSIZE
		if currentChunkSize >= CHUNKLIMIT {
			// Close this output, next iteration will create the next file
			outf.Close()
			outf = nil
		}

	}

	if fatalError != nil {
		// Clean up temporaries
		for _, f := range chunkFilenames {
			os.Remove(f)
		}
		return nil, fatalError
	}

	shaStr := fmt.Sprintf("%x", string(sha.Sum(nil)))

	// We *may* now move the data to LOB dir
	// We won't if it already exists & is the correct size
	// Construct LOBInfo & write to final location
	info := &LOBInfo{SHA: shaStr, Size: totalSize, NumChunks: len(chunkFilenames)}
	err = storeLOBInfo(info)

	// Check each chunk file
	for i, f := range chunkFilenames {
		sz := CHUNKLIMIT
		if i+1 == len(chunkFilenames) {
			// Last chunk, get size
			sz = currentChunkSize
		}
		destFile := getLOBChunkFilename(shaStr, i)
		if !FileExistsAndIsOfSize(destFile, int64(sz)) {
			LogDebugf("Saving final LOB metadata file: %v\n", destFile)
			// delete any existing (incorrectly sized) file since will probably not be allowed to rename over it
			// ignore any errors
			os.Remove(destFile)
			os.Rename(f, destFile)
		} else {
			LogDebugf("LOB chunk file already exists & is valid: %v\n", destFile)
		}
	}

	return info, nil

}
