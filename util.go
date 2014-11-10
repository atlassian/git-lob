package main

import (
	"os"
)

// Utility method to determine if a file/dir exists
func FileOrDirExists(path string) (exists bool, isDir bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, false
	} else {
		return true, fi.IsDir()
	}
}

func FileExistsAndIsOfSize(path string, sz int64) bool {
	fi, err := os.Stat(path)

	if err != nil && os.IsNotExist(err) {
		return false
	}

	return fi.Size() == sz
}
