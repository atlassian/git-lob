package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	parseSizeRegex *regexp.Regexp
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

// Utility method to determine if a file/dir exists and is of a specific size
func FileExistsAndIsOfSize(path string, sz int64) bool {
	fi, err := os.Stat(path)

	if err != nil && os.IsNotExist(err) {
		return false
	}

	return fi.Size() == sz
}

// Parse a string representing a size into a number of bytes
// supports m/mb = megabytes, g/gb = gigabytes etc (case insensitive)
func ParseSize(str string) (int64, error) {
	if parseSizeRegex == nil {
		parseSizeRegex = regexp.MustCompile(`(?i)^\s*([\d\.]+)\s*([KMGTP]?B?)\s*$`)
	}

	if match := parseSizeRegex.FindStringSubmatch(str); match != nil {
		value, err := strconv.ParseFloat(match[1], 32)
		if err != nil {
			return 0, err
		}
		strUnits := strings.ToUpper(match[2])
		switch strUnits {
		case "KB", "K":
			return int64(value * (1 << 10)), nil
		case "MB", "M":
			return int64(value * (1 << 20)), nil
		case "GB", "G":
			return int64(value * (1 << 30)), nil
		case "TB", "T":
			return int64(value * (1 << 40)), nil
		case "PB", "P":
			return int64(value * (1 << 50)), nil
		default:
			return int64(value), nil

		}

	} else {
		return 0, errors.New(fmt.Sprintf("Invalid size: %v", str))
	}

}

// Format a number of bytes into a display format
func FormatSize(sz int64) string {

	switch {
	case sz >= (1 << 50):
		return fmt.Sprintf("%.3gPB", float32(sz)/float32(1<<50))
	case sz >= (1 << 40):
		return fmt.Sprintf("%.3gTB", float32(sz)/float32(1<<40))
	case sz >= (1 << 30):
		return fmt.Sprintf("%.3gGB", float32(sz)/float32(1<<30))
	case sz >= (1 << 20):
		return fmt.Sprintf("%.3gMB", float32(sz)/float32(1<<20))
	case sz >= (1 << 10):
		return fmt.Sprintf("%.3gKB", float32(sz)/float32(1<<10))
	default:
		return fmt.Sprintf("%d", sz)
	}
}

// Search a sorted slice of strings for a specific string
// Returns boolean for if found, and either location or insertion point
func StringBinarySearch(sortedSlice []string, searchTerm string) (bool, int) {
	// Convenience method to easily provide boolean of whether to insert or not
	idx := sort.SearchStrings(sortedSlice, searchTerm)
	found := idx < len(sortedSlice) && sortedSlice[idx] == searchTerm
	return found, idx
}

// Walk first parents starting from startSHA and call callback
// First call will be startSHA & its parent
// Parent will be blank string if there are no more parents & walk will stop after
// Optimises internally to call Git only for batches of 50
func WalkGitHistory(startSHA string, callback func(currentSHA, parentSHA string) (quit bool, err error)) error {

	quit := false
	currentLogHEAD := startSHA
	var callbackError error
	for !quit {
		// get 50 parents
		// format as <SHA> <PARENT> so we can detect the end of history
		cmd := exec.Command("git", "log", "--first-parent", "--topo-order",
			"-n", "50", "--format=%H %P", currentLogHEAD)

		outp, err := cmd.StdoutPipe()
		if err != nil {
			LogErrorf("Unable to list commits from %v: %v", currentLogHEAD, err.Error())
			return err
		}
		cmd.Start()
		scanner := bufio.NewScanner(outp)
		var currentLine string
		var parentSHA string
		for scanner.Scan() {
			currentLine = scanner.Text()
			currentSHA := currentLine[:40]
			// If we got here, we still haven't found an ancestor that was already marked
			// check next batch, provided there's a parent on the last one
			// 81 chars long, 2x40 SHAs + space
			if len(currentLine) >= 81 {
				parentSHA = strings.TrimSpace(currentLine[41:81])
			}
			quit, callbackError = callback(currentSHA, parentSHA)
			if quit {
				break
			}
		}
		cmd.Wait()
		// End of history
		if parentSHA == "" {
			break
		} else {
			currentLogHEAD = parentSHA
		}
	}
	return callbackError
}
