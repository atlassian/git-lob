// +build windows
package core

// Get the maximum number of arguments we want to try passing to the command line
func GetMaxCommandLineArguments() int {
	// Git doesn't allow more than 4096 file arguments so use that as a low-water mark
	// No other OS limits on Windows, just shorter command line limits
	return 4096
}

// Get the maximum length of a command on the command line
func GetMaxCommandLineLength() int {
	// < Win7 = 8191 - not supported?
	// >= Win7 = 32768 (sub a a little for padding)
	return 32000
}
