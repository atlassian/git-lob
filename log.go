package main

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

var (
	debugLog        *log.Logger
	errorFileLog    *log.Logger
	errorConsoleLog *log.Logger
	outputLog       *log.Logger
	logFile         *os.File
)

// Log error with format (no implicit newline)
func LogErrorf(format string, v ...interface{}) {
	errorFileLog.Printf(format, v...)
	errorConsoleLog.Printf(format, v...)
}

// Log debug message with format (if verbose)
func LogDebugf(format string, v ...interface{}) {
	debugLog.Printf(format, v...)
}

// Log output message with format (if not quiet)
func Logf(format string, v ...interface{}) {
	outputLog.Printf(format, v...)
}

// Log error message with newline
func LogError(msg string) {
	errorFileLog.Println(msg)
	errorConsoleLog.Println(msg)
}

// Log debug message with newline (if verbose)
func LogDebug(msg string) {
	debugLog.Println(msg)
}

// Log output message with newline (if not quiet)
func Log(msg string) {
	outputLog.Println(msg)
}

func getLogFileHandle() *os.File {
	if logFile == nil {
		usr, err := user.Current()
		if err != nil {
			log.Fatal(err)
		}
		logFileName := filepath.Join(usr.HomeDir, "git-lob.log")
		logFile, err = os.OpenFile(logFileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			log.Fatal(err)
		}
	}
	return logFile
}

// Initialise logging, make sure GlobalOptions is initialised
func InitLogging() {

	var outputW, debugW io.Writer
	var flags int
	const logFlags = log.Ldate | log.Ltime | log.Lshortfile
	const consoleFlags = 0
	// Must call this after initialising GlobalOptions
	if GlobalOptions.Quiet {
		outputW = ioutil.Discard
	} else {
		// Filters can't use stdout
		if strings.HasPrefix(GlobalOptions.Command, "filter-") {
			outputW = getLogFileHandle()
			flags = logFlags
		} else {
			outputW = os.Stdout
			flags = consoleFlags
		}
	}
	outputLog = log.New(outputW, "", flags)

	if GlobalOptions.Verbose {
		// Filters can't use stdout
		if strings.HasPrefix(GlobalOptions.Command, "filter-") {
			debugW = getLogFileHandle()
			flags = logFlags
		} else {
			debugW = os.Stdout
			flags = consoleFlags
		}
	} else {
		debugW = ioutil.Discard
	}
	debugLog = log.New(debugW, "DEBUG: ", flags)

	// Always log errors to both the log file and stderr, but with different prefixes
	errorFileLog = log.New(getLogFileHandle(), "ERROR: ", logFlags)
	errorConsoleLog = log.New(os.Stderr, "", consoleFlags)

}
func ShutDownLogging() {
	if logFile != nil {
		logFile.Close()
	}

}
