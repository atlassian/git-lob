package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
)

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
	return filepath.Join(GetGitDir(), "git-lob")
}

// Gets the containing folder for a given LOB SHA & creates if necessary
// LOBs are 'splayed' based on first 2 chars of SHA
func GetLOBDir(sha string) string {
	if len(sha) != 40 {
		LogErrorf("Invalid SHA format: %v\n", sha)
		return ""
	}
	return filepath.Join(GetLOBRoot(), sha[:2])
}

func getLOBMetaFilename(lobfld string, sha string) string {
	return filepath.Join(lobfld, sha+"_meta")
}

// Retrieve information about an existing stored LOB
func GetLOBInfo(sha string) (*LOBInfo, error) {
	fld := GetLOBDir(sha)
	meta := getLOBMetaFilename(fld, sha)
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
		} else {
			// A problem
			LogErrorf("Unable to retrieve LOB with SHA %v: %v\n", sha, err)
			return nil, err
		}
	}

	for i := 0; i < info.NumChunks; i++ {
		// Check each chunk file exists, and is correct size
		// if not, maybe download (again)
		// TODO
	}

	return

}

// Store a LOB from a file stream
func StoreLOB(in io.Reader) (info *LOBInfo, err error) {
	// TODO
	return

}
