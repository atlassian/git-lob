package util

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

var (
	parseSizeRegex *regexp.Regexp
)

var cachedRepoRoot string
var cachedRepoRootIsSeparate bool
var cachedRepoRootWorkingDir string

// Gets the root folder of this git repository (the one containing .git)
func GetRepoRoot() (path string, isSeparateGitDir bool, reterr error) {
	// We could call 'git rev-parse --git-dir' but this requires shelling out = slow, especially on Windows
	// We should try to avoid that whenever we can
	// So let's just find it ourselves; first containing folder with a .git folder/file
	curDir, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	origCurDir := curDir
	// Use the cached value if known
	if cachedRepoRootWorkingDir == curDir && cachedRepoRoot != "" {
		return cachedRepoRoot, cachedRepoRootIsSeparate, nil
	}

	for {
		exists, isDir := FileOrDirExists(filepath.Join(curDir, ".git"))
		if exists {
			// Store in cache to speed up
			cachedRepoRoot = curDir
			cachedRepoRootWorkingDir = origCurDir
			cachedRepoRootIsSeparate = !isDir
			return curDir, !isDir, nil
		}
		curDir = filepath.Dir(curDir)
		if len(curDir) == 0 || curDir[len(curDir)-1] == filepath.Separator || curDir == "." {
			// Not a repo
			return "", false, errors.New("Couldn't find repo root, not a git folder")
		}
	}
}

// Gets the git data dir of git repository (the .git dir, or where .git file points)
func GetGitDir() string {
	root, isSeparate, err := GetRepoRoot()
	if err != nil {
		return ""
	}
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
		// The text in the git dir will use cygwin-style separators, so normalise
		return filepath.Clean(match[1])
	} else {
		// Regular git dir
		return git
	}

}

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

func FormatBytes(sz int64) (suffix string, scaled float32) {
	switch {
	case sz >= (1 << 50):
		return "PB", float32(sz) / float32(1<<50)
	case sz >= (1 << 40):
		return "TB", float32(sz) / float32(1<<40)
	case sz >= (1 << 30):
		return "GB", float32(sz) / float32(1<<30)
	case sz >= (1 << 20):
		return "MB", float32(sz) / float32(1<<20)
	case sz >= (1 << 10):
		return "KB", float32(sz) / float32(1<<10)
	default:
		return "B", float32(sz)
	}

}

func FormatFloat(f float32) string {
	// Just adjust width & precision based on scale to be friendly
	switch {
	case f < 1000:
		// Need %g to make after decimal place optional
		return fmt.Sprintf("%.3g", f)
	default:
		// Need %f here to kill exponent
		return fmt.Sprintf("%4.0f", f)
	}
}

// Format a number of bytes into a display format
func FormatSize(sz int64) string {

	suffix, num := FormatBytes(sz)
	return FormatFloat(num) + suffix
}

// Format a bytes per second transfer rate into a display format
func FormatTransferRate(bytesPerSecond int64) string {

	suffix, num := FormatBytes(bytesPerSecond)
	return fmt.Sprintf("%v%v/s", FormatFloat(num), suffix)
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

// Remove duplicates from a slice of strings (in place)
// Linear to logarithmic time, doesn't change the ordering of the slice
// allocates/frees a new map of up to the size of the slice though
func StringRemoveDuplicates(s *[]string) {
	if s == nil || *s == nil {
		return
	}
	uniques := NewStringSet()
	insertidx := 0
	for _, x := range *s {
		if !uniques.Contains(x) {
			uniques.Add(x)
			(*s)[insertidx] = x // could do this only when x != insertidx but prob wasteful compare
			insertidx++
		}
	}
	// If any were eliminated it will now be shorter
	*s = (*s)[:insertidx]
}

// Return whether a given filename passes the include / exclude path filters
// Only paths that are in includePaths and outside excludePaths are passed
// If includePaths is empty that filter always passes and the same with excludePaths
// Both path lists support wildcard matches
func FilenamePassesIncludeExcludeFilter(filename string, includePaths, excludePaths []string) bool {
	if len(includePaths) == 0 && len(excludePaths) == 0 {
		return true
	}

	// For Win32, becuase git reports files with / separators
	cleanfilename := filepath.Clean(filename)
	if len(includePaths) > 0 {
		matched := false
		for _, inc := range includePaths {
			matched, _ = filepath.Match(inc, filename)
			if !matched && IsWindows() {
				// Also Win32 match
				matched, _ = filepath.Match(inc, cleanfilename)
			}
			if !matched {
				// Also support matching a parent directory without a wildcard
				if strings.HasPrefix(cleanfilename, inc+string(filepath.Separator)) {
					matched = true
				}
			}
			if matched {
				break
			}

		}
		if !matched {
			return false
		}
	}

	if len(excludePaths) > 0 {
		for _, ex := range excludePaths {
			matched, _ := filepath.Match(ex, filename)
			if !matched && IsWindows() {
				// Also Win32 match
				matched, _ = filepath.Match(ex, cleanfilename)
			}
			if matched {
				return false
			}
			// Also support matching a parent directory without a wildcard
			if strings.HasPrefix(cleanfilename, ex+string(filepath.Separator)) {
				return false
			}

		}
	}

	return true

}

// Execute 1:n os.exec.Command instances for a list of files, splitting where the command line might
// get too long. name is the command name as per exec.Command
// Files are appended to the end of the argument list
// errorCallback is called for any errors so caller can decide whether to abort
func ExecForManyFilesSplitIfRequired(files []string,
	errorCallback func(args []string, output string, err error) (abort bool),
	name string, baseargs ...string) {

	// How many characters have we used in base args?
	baseLen := len(name)
	for _, arg := range baseargs {
		// +1 for separator (in practice might be +3 with quoting but we'll allow a little legroom)
		baseLen += len(arg) + 1
	}

	lenLeft := GetMaxCommandLineLength() - baseLen - 1
	argsLeft := GetMaxCommandLineArguments() - len(baseargs) - 1

	if lenLeft <= 0 || argsLeft <= 0 {
		errorCallback(baseargs, "",
			fmt.Errorf("Base arguments were too long to include anything else in ExecForManyFilesSplitIfRequired: %v %v", name, baseargs))
		return
	}

	for filesLeft := files; len(filesLeft) > 0; {
		newargs := baseargs
		var filesUsed int
		for _, file := range filesLeft {
			lenadded := len(file)
			if strings.Contains(file, " \t") {
				// 2 for quoting
				lenadded += 2
			}
			if lenadded > lenLeft || argsLeft == 0 {
				break
			}
			argsLeft--
			lenLeft -= (lenadded + 1) // +1 for space separator
			newargs = append(newargs, file)
			filesUsed++
		}
		// Issue this command
		cmd := exec.Command(name, newargs...)
		outp, err := cmd.CombinedOutput()
		if err != nil {
			abort := errorCallback(newargs, string(outp), err)
			if abort {
				return
			}
		}

		if filesUsed == len(filesLeft) {
			break
		} else {
			filesLeft = filesLeft[filesUsed:]
		}

	}

}

// Make a list of filenames expressed relative to the root of the repo relative to the
// current working dir. This is useful when needing to call out to git, but the user
// may be in a subdir of their repo
func MakeRepoFileListRelativeToCwd(repofiles []string) []string {
	root, _, err := GetRepoRoot()
	if err != nil {
		LogError("Unable to get repo root: ", err.Error())
		return repofiles
	}
	wd, err := os.Getwd()
	if err != nil {
		LogError("Unable to get working dir: ", err.Error())
		return repofiles
	}

	// Early-out if working dir is root dir, same result
	if root == wd {
		return repofiles
	}

	var ret []string
	for _, f := range repofiles {
		abs := filepath.Join(root, f)
		rel, err := filepath.Rel(wd, abs)
		if err != nil {
			LogErrorf("Unable to convert %v to path relative to working dir %v: %v\n", abs, wd, err.Error())
			// Use absolute file instead (longer)
			ret = append(ret, abs)
		} else {
			ret = append(ret, rel)
		}
	}

	return ret

}

// Are we running on Windows? Need to handle some extra path shenanigans
func IsWindows() bool {
	return runtime.GOOS == "windows"
}
