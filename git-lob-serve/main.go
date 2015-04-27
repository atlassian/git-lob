package main

import (
	"bitbucket.org/sinbad/git-lob/util"
	"fmt"
	"os"
	"runtime/debug"
)

func main() {
	// Need to send the result code to the OS but also need to support 'defer'
	// os.Exit would finish before any defers, so wrap everything in mainImpl()
	os.Exit(MainImpl())

}

func MainImpl() int {

	// Generic panic handler so we get stack trace
	defer func() {
		if e := recover(); e != nil {
			fmt.Fprintf(os.Stderr, "git-lob-serve panic: %v\n", e)
			fmt.Fprint(os.Stderr, string(debug.Stack()))
			os.Exit(99)
		}

	}()

	// Get set up
	cfg := LoadConfig()

	if cfg.BasePath == "" {
		fmt.Fprintf(os.Stderr, "Missing required configuration setting: base-path\n")
		return 12
	}
	if util.DirExists(cfg.BasePath) {
		fmt.Fprintf(os.Stderr, "Invalid value for base-path: %v\nDirectory must exist.\n", cfg.BasePath)
		return 14
	}
	if cfg.DeltaCachePath != "" && !util.DirExists(cfg.DeltaCachePath) {
		// Create delta cache if doesn't exist, use same permissions as base path
		s, err := os.Stat(cfg.BasePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid value for base-path: %v\nCannot stat: %v\n", cfg.BasePath, err.Error())
			return 16
		}
		err = os.MkdirAll(cfg.DeltaCachePath, s.Mode())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating delta cache path %v: %v\n", cfg.DeltaCachePath, err.Error())
			return 16
		}
	}

	return Serve(cfg)

	return 0
}
