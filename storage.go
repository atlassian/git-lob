package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
)

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
			return curDir, isDir
		}
		curDir = filepath.Dir(curDir)
		if curDir == string(filepath.Separator) || curDir == "." {
			// Not a repo
			LogError("Couldn't find repo root, not a git folder")
			return "", false
		}
	}
}

// Gets the git data folder of git repository (the .git folder, or where .git file points)
func GetGitFolder() string {
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
	return filepath.Join(GetGitFolder(), "git-lob")
}

// Gets the containing folder for a given LOB SHA & creates if necessary
// LOBs are 'splayed' based on first 2 chars of SHA
func GetLOBFolder(sha string) string {
	if len(sha) != 40 {
		LogErrorf("Invalid SHA format: %v\n", sha)
		return ""
	}
	return filepath.Join(GetLOBRoot(), sha[:2])
}
