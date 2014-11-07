package main

import (
	"io"
	"os"
	"regexp"
)

// Prefix which identifies file contents as a git-lob SHA entry
// Use this prefix rather than just the SHA in case by accident a file's content
// actually is a 40-char pattern
const SHAPrefix = "git-lob: "
const SHALineLen = len(SHAPrefix) + 40

func SmudgeFilter() int {
	LogDebug("Running smudge filter")
	shaRegex := regexp.MustCompile("^git-lob: ([0-9A-Fa-f]{40})$")
	// read committed content from stdin
	// write actual file content to stdout if a git-lob SHA
	buf := make([]byte, SHALineLen)
	c, err := os.Stdin.Read(buf)
	if c == SHALineLen {
		if match := shaRegex.FindStringSubmatch(string(buf)); match != nil {
			sha := match[1]
			lobinfo, err := RetrieveLOB(sha, os.Stdout)
			if err == nil {
				LogDebugf("Retrieved LOB for %v from %v chunks\n", sha, lobinfo.NumChunks)
				return 0
			} else {
				LogErrorf("Error obtaining LOB data: %v\n", err)
			}

		}
	}
	// Otherwise, pass through content
	os.Stdout.Write(buf[:c])
	_, err = io.Copy(os.Stdout, os.Stdin)
	if err == nil {
		return 0
	}

	LogErrorf("Error copying stdin->stdout: %v\n", err)
	return 3
}

func CleanFilter() int {
	LogDebug("Running clean filter")
	// stdin / stdout
	return -1
}

func retrieveLOB(sha string, out io.Writer) int {
	return -1
}
