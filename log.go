package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"runtime/debug"
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
	if errorFileLog != nil && errorConsoleLog != nil {
		errorFileLog.Printf(format, v...)
		errorConsoleLog.Printf(format, v...)
		// Also dump stack trace to log
		errorFileLog.Println(debug.Stack())
	} else {
		fmt.Fprintf(os.Stderr, format, v...)
	}
}

// Log debug message with format (if verbose)
func LogDebugf(format string, v ...interface{}) {
	if debugLog != nil {
		debugLog.Printf(format, v...)
	} else {
		fmt.Fprintf(os.Stderr, format, v...)
	}
}

// Log output message with format (if not quiet)
func Logf(format string, v ...interface{}) {
	if outputLog != nil {
		outputLog.Printf(format, v...)
	} else {
		fmt.Fprintf(os.Stderr, format, v...)
	}
}

// Log error message with newline
func LogError(msg string) {
	if errorFileLog != nil && errorConsoleLog != nil {
		errorFileLog.Println(msg)
		errorConsoleLog.Println(msg)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// Log debug message with newline (if verbose)
func LogDebug(msg string) {
	if debugLog != nil {
		debugLog.Println(msg)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// Log output message with newline (if not quiet)
func Log(msg string) {
	if outputLog != nil {
		outputLog.Println(msg)
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
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
