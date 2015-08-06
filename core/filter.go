package core

import (
	"io"
	"regexp"

	"github.com/atlassian/git-lob/util"
)

// Prefix which identifies file contents as a git-lob SHA entry
// Use this prefix rather than just the SHA in case by accident a file's content
// actually is a 40-char pattern
const SHAPrefix = "git-lob: "
const SHALen = 40
const SHALineLen = len(SHAPrefix) + SHALen
const SHALineRegexStr = "^git-lob: [A-Fa-f0-9]{40}$"
const SHALineMatchRegexStr = "^git-lob: ([0-9A-Fa-f]{40})$"

func getLOBPlaceholderContent(sha string) string {
	return SHAPrefix + sha
}

func SmudgeFilterWithReaderWriter(in io.Reader, out io.Writer, filename string) int {
	util.LogDebug("Running smudge filter for ", filename)

	shaRegex := regexp.MustCompile(SHALineMatchRegexStr)
	// read committed content from stdin
	// write actual file content to stdout if a git-lob SHA
	buf := make([]byte, SHALineLen)
	c, err := in.Read(buf)
	if c == SHALineLen {
		if match := shaRegex.FindStringSubmatch(string(buf)); match != nil {
			sha := match[1]
			lobinfo, err := RetrieveLOB(sha, out)
			if err == nil {
				util.LogDebugf("Successfully smudged %v: %v in %v chunks from %v\n", filename, util.FormatSize(lobinfo.Size), lobinfo.NumChunks, sha)
				return 0
			} else {
				if IsNotFoundError(err) {
					util.LogErrorf("%v: content not available, placeholder used [%v]\n", filename, sha[:7])
				} else {
					util.LogErrorf("Error obtaining %v for %v: %v\n", sha, filename, err)
				}
				// fall through to below which will just write the SHA line to the working copy
			}

		}
	}
	// Otherwise, pass through content
	out.Write(buf[:c])
	_, err = io.Copy(out, in)
	if err != nil {
		util.LogErrorf("Error copying stdin->stdout for %v: %v\n", filename, err)
		return 3
	}

	return 0
}

func CleanFilterWithReaderWriter(in io.Reader, out io.Writer, filename string) int {
	util.LogDebug("Running clean filter for ", filename)
	shaRegex := regexp.MustCompile(SHALineMatchRegexStr)
	// read working copy content from stdin
	// First check if this is an unexpanded LOB SHA (not downloaded)
	buf := make([]byte, SHALineLen)
	c, err := in.Read(buf)
	if c == SHALineLen {
		if match := shaRegex.FindStringSubmatch(string(buf)); match != nil {
			sha := match[1]
			util.LogDebugf("Unexpanded LOB file content at %v, not storing\n", filename)
			// Yes, unexpanded SHA, just write
			out.Write(buf[:c])
			_, err = io.Copy(out, in)
			if err == nil {
				util.LogDebug("Successful clean filter for ", filename)
				return 0
			} else {
				util.LogErrorf("Error writing unexpanded LOB for %v/%v in clean filter: %v\n", filename, sha, err)
				return 3
			}

		}
	}
	// Otherwise if we got here, this is just binary data we need to hash
	lobinfo, err := StoreLOB(in, buf[:c])

	if err != nil {
		util.LogErrorf("Error storing LOB from %v in clean filter: %v\n", filename, err)
		return 4
	}

	// Write SHA code to output
	shaLine := getLOBPlaceholderContent(lobinfo.SHA)
	_, err = io.WriteString(out, shaLine)
	if err != nil {
		util.LogErrorf("Error writing LOB SHA for %v to index in clean filter: %v\n", filename, err)
		return 5
	}

	util.LogDebug("Successful clean filter for ", filename)

	return 0
}
