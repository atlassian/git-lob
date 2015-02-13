// +build !windows

package main

import (
	"os"
	"strconv"
)

// Get the maximum number of arguments we want to try passing to the command line
func GetMaxCommandLineArguments() int {
	// Git doesn't allow more than 4096 file arguments so use that as a low-water mark
	// ARG_MAX isn't that useful, suggests a much higher limit than actually works in practice
	// but in case it's lower, use it
	ret := int64(4096)
	argMax, err := strconv.ParseInt(os.Getenv("ARG_MAX"), 10, 16)
	if err != nil && argMax < ret && argMax != 0 {
		ret = argMax
	}
	return int(ret)
}

// Get the maximum length of a command on the command line
func GetMaxCommandLineLength() int {
	// from experience, safe limit
	return 128000
}
