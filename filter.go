package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
)

// Prefix which identifies file contents as a git-lob SHA entry
// Use this prefix rather than just the SHA in case by accident a file's content
// actually is a 40-char pattern
const SHAPrefix = "git-lob: "
const SHALen = 40
const SHALineLen = len(SHAPrefix) + SHALen

func cmdSmudgeFilter() int {
	return SmudgeFilterWithReaderWriter(os.Stdin, os.Stdout)
}

func SmudgeFilterWithReaderWriter(in io.Reader, out io.Writer) int {
	LogDebug("Running smudge filter")
	shaRegex := regexp.MustCompile("^git-lob: ([0-9A-Fa-f]{40})$")
	// read committed content from stdin
	// write actual file content to stdout if a git-lob SHA
	buf := make([]byte, SHALineLen)
	c, err := in.Read(buf)
	if c == SHALineLen {
		if match := shaRegex.FindStringSubmatch(string(buf)); match != nil {
			sha := match[1]
			lobinfo, err := RetrieveLOB(sha, out)
			if err == nil {
				LogDebugf("Retrieved LOB for %v from %v chunks\n", sha, lobinfo.NumChunks)
				return 0
			} else {
				LogErrorf("Error obtaining LOB data for %v: %v\n", sha, err)
				// fall through to below which will just write the SHA line to the working copy
			}

		}
	}
	// Otherwise, pass through content
	out.Write(buf[:c])
	_, err = io.Copy(out, in)
	if err == nil {
		return 0
	}

	LogErrorf("Error copying stdin->stdout: %v\n", err)
	return 3
}

func cmdCleanFilter() int {
	return CleanFilterWithReaderWriter(os.Stdin, os.Stdout)
}

func CleanFilterWithReaderWriter(in io.Reader, out io.Writer) int {
	LogDebug("Running clean filter")
	shaRegex := regexp.MustCompile("^git-lob: ([0-9A-Fa-f]{40})$")
	// read working copy content from stdin
	// First check if this is an unexpanded LOB SHA (not downloaded)
	buf := make([]byte, SHALineLen)
	c, err := in.Read(buf)
	if c == SHALineLen {
		if match := shaRegex.FindStringSubmatch(string(buf)); match != nil {
			sha := match[1]
			LogDebugf("Unexpanded LOB SHA in file content (%v), clean filter will not change\n", sha)
			// Yes, unexpanded SHA, just write
			out.Write(buf[:c])
			_, err = io.Copy(out, in)
			if err == nil {
				return 0
			} else {
				LogErrorf("Error writing unexpanded LOB in clean filter: %v\n", err)
				return 3
			}

		}
	}
	// Otherwise if we got here, this is just binary data we need to hash
	lobinfo, err := StoreLOB(in, buf[:c])

	if err != nil {
		LogErrorf("Error storing LOB in clean filter: %v\n", err)
		return 4
	}

	// Write SHA code to output
	shaLine := SHAPrefix + lobinfo.SHA
	_, err = io.WriteString(out, shaLine)
	if err != nil {
		LogErrorf("Error writing LOB SHA to index in clean filter: %v\n", err)
		return 5
	}

	LogDebugf("Successfully stored/checked LOB data for SHA %v, %d chunks, total size %v\n", lobinfo.SHA, lobinfo.NumChunks, lobinfo.Size)

	return 0
}

func cmdSmudgeFilterHelp() {
	fmt.Println(`Usage: git-lob filter-smudge [options]

  The smudge filter converts a file stored in git to a file in the working
  directory. In this case we look for files containing the git-lob marker
  and replace the content with real binary data from the binary store.

  Not intended to be called directly, see README.md for how to configure
  the filter for your repository.

Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't actually delete anything, just report
`)
}
func cmdCleanFilterHelp() {
	fmt.Println(`Usage: git-lob filter-clean [options]

  The clean filter converts a file in the working directory to a form which
  will be stored in git. In this case we calculate the SHA-1 of the binary
  content and write this to git, while storing the real data in the separate
  binary store.

  Not intended to be called directly, see README.md for how to configure
  the filter for your repository.

Options:
  --quiet, -q          Print less output
  --verbose, -v        Print more output
  --dry-run            Don't actually delete anything, just report
`)
}
