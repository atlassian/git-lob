package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"runtime/debug"
)

var (
	// Console output (can be disabled by changing)
	consoleOut io.Writer = os.Stderr
	// Loggers for file output
	debugLog  *log.Logger
	errorLog  *log.Logger
	outputLog *log.Logger
	logFile   *os.File
)

// Log error with format (no implicit newline)
func LogErrorf(format string, v ...interface{}) {
	fmt.Fprintf(consoleOut, format, v...)

	if errorLog != nil {
		errorLog.Printf(format, v...)
		// Also dump stack trace to log
		errorLog.Println(string(debug.Stack()))
	}
}

// Log debug message with format (if verbose)
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

// Log output message with format (if not quiet)
func Logf(format string, v ...interface{}) {
	if !GlobalOptions.Quiet {
		fmt.Fprintf(consoleOut, format, v...)

		if outputLog != nil {
			outputLog.Printf(format, v...)
		}
	}
}

// Log error message with newline
func LogError(msg string) {
	fmt.Fprintln(consoleOut, msg)

	if errorLog != nil {
		errorLog.Println(msg)
		// Also dump stack trace to log
		errorLog.Println(debug.Stack())
	}

}

// Log debug message with newline (if verbose)
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

// Log output message with newline (if not quiet)
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
	logFile, err = os.OpenFile(logFileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
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
