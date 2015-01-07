package main

import (
	"errors"
	"fmt"
	"io"
	"os"
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

// Utility method to determine if a file (NOT dir) exists
func FileExists(path string) bool {
	ret, isDir := FileOrDirExists(path)
	return ret && !isDir
}

// Utility method to determine if a dir (NOT file) exists
func DirExists(path string) bool {
	ret, isDir := FileOrDirExists(path)
	return ret && isDir
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

// Format a bytes per second transfer rate into a display format
func FormatTransferRate(bytesPerSecond int64) string {
	switch {
	case bytesPerSecond >= (1 << 50): // yeah, right ;)
		return fmt.Sprintf("%.3gPB/s", float32(bytesPerSecond)/float32(1<<50))
	case bytesPerSecond >= (1 << 40):
		return fmt.Sprintf("%.3gTB/s", float32(bytesPerSecond)/float32(1<<40))
	case bytesPerSecond >= (1 << 30):
		return fmt.Sprintf("%.3gGB/s", float32(bytesPerSecond)/float32(1<<30))
	case bytesPerSecond >= (1 << 20):
		return fmt.Sprintf("%.3gMB/s", float32(bytesPerSecond)/float32(1<<20))
	case bytesPerSecond >= (1 << 10):
		return fmt.Sprintf("%.3gKB/s", float32(bytesPerSecond)/float32(1<<10))
	default:
		return fmt.Sprintf("%dBytes/s", bytesPerSecond)
	}
}

// Calculates transfer rates by averaging over n samples
type TransferRateCalculator struct {
	numSamples      int
	samples         []int64 // bytesPerSecond samples
	sampleInsertIdx int
}

func NewTransferRateCalculator(numSamples int) *TransferRateCalculator {
	return &TransferRateCalculator{numSamples, make([]int64, numSamples), 0}
}
func (t *TransferRateCalculator) AddSample(bytesPerSecond int64) {
	t.samples[t.sampleInsertIdx] = bytesPerSecond
	t.sampleInsertIdx = (t.sampleInsertIdx + 1) % t.numSamples
}
func (t *TransferRateCalculator) Average() int64 {
	var sum int64
	for _, s := range t.samples {
		sum += s
	}
	return sum / int64(t.numSamples)
}

// Search a sorted slice of strings for a specific string
// Returns boolean for if found, and either location or insertion point
func StringBinarySearch(sortedSlice []string, searchTerm string) (bool, int) {
	// Convenience method to easily provide boolean of whether to insert or not
	idx := sort.SearchStrings(sortedSlice, searchTerm)
	found := idx < len(sortedSlice) && sortedSlice[idx] == searchTerm
	return found, idx
}

// Overwrite the current line in the console (e.g. for progressive update)
// Requires the previous line length so that it can clear it with spaces
func OverwriteConsoleLine(newString string, lastLineLength int, iobuf io.Writer) {
	if len(newString) < lastLineLength {
		fmt.Fprintf(iobuf, "\r%v%v", newString, strings.Repeat(" ", lastLineLength-len(newString)))
	} else {
		fmt.Fprintf(iobuf, "\r%v", newString)
	}

}
