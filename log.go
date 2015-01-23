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
)

var (
	// Console output (can be overridden by changing)
	consoleErr io.Writer = os.Stderr
	consoleOut io.Writer = os.Stdout
	// Loggers for file output
	debugLog  *log.Logger
	errorLog  *log.Logger
	outputLog *log.Logger
	logFile   *os.File
)

// Always send all console output to stderr, including info/debug messages
// This is mostly useful when stdout is reserved for piping content
func LogAllConsoleOutputToStdErr() {
	consoleOut = os.Stderr
	consoleErr = os.Stderr
}

// Suppress all console output
func LogSuppressAllConsoleOutput() {
	consoleOut = ioutil.Discard
	consoleErr = ioutil.Discard
	errorLog = log.New(ioutil.Discard, "", 0)
	debugLog = log.New(ioutil.Discard, "", 0)
	outputLog = log.New(ioutil.Discard, "", 0)
}

// Log error to console and log with format (no implicit newline)
func LogErrorf(format string, v ...interface{}) {
	fmt.Fprintf(consoleErr, format, v...)

	if errorLog != nil {
		errorLog.Printf(format, v...)
		// Also dump stack trace to log
		errorLog.Println(string(debug.Stack()))
	}
}

// Log debug message to console and log with format (if verbose)
func LogDebugf(format string, v ...interface{}) {
	if GlobalOptions.Verbose {
		fmt.Fprintf(consoleOut, format, v...)
	}

	if GlobalOptions.Verbose || GlobalOptions.VerboseLog {
		if debugLog != nil {
			debugLog.Printf(format, v...)
		}
	}

}

// Log output message to console and log with format (if not quiet)
func Logf(format string, v ...interface{}) {
	if !GlobalOptions.Quiet {
		fmt.Fprintf(consoleOut, format, v...)

		if outputLog != nil {
			outputLog.Printf(format, v...)
		}
	}
}

// Log error message to console and log with newline
func LogError(msg string) {
	fmt.Fprintln(consoleErr, msg)

	if errorLog != nil {
		errorLog.Println(msg)
		// Also dump stack trace to log
		errorLog.Println(debug.Stack())
	}

}

// Log debug message to console and log with newline (if verbose)
func LogDebug(msg string) {
	if GlobalOptions.Verbose {
		fmt.Fprintln(consoleOut, msg)
	}

	if GlobalOptions.Verbose || GlobalOptions.VerboseLog {
		if debugLog != nil {
			debugLog.Println(msg)
		}
	}
}

// Log output message to console and log with newline (if not quiet)
func Log(msg string) {
	if !GlobalOptions.Quiet {
		fmt.Fprintln(consoleOut, msg)

		if outputLog != nil {
			outputLog.Println(msg)
		}
	}
}

func getLogFileHandle() *os.File {
	var logFileName string
	if GlobalOptions.LogFile != "" {
		logFileName = GlobalOptions.LogFile
	} else {
		usr, err := user.Current()
		if err != nil {
			log.Fatal(err)
		}
		logFileName = filepath.Join(usr.HomeDir, "git-lob.log")
	}
	var err error
	logFile, err = os.OpenFile(logFileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	return logFile
}

// Initialise logging, make sure GlobalOptions is initialised
func InitLogging() {

	if GlobalOptions.LogEnabled {
		const logFlags = log.Ldate | log.Ltime | log.Lshortfile
		f := getLogFileHandle()
		outputLog = log.New(f, "", logFlags)
		errorLog = log.New(f, "ERROR: ", logFlags)
		debugLog = log.New(f, "", logFlags)
	}
}
func ShutDownLogging() {
	if logFile != nil {
		logFile.Close()
	}

}
