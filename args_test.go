package main

import (
	"testing"
)

func TestParseCommandLine(t *testing.T) {
	var args []string
	var opts *CommandLineOptions
	var ok bool

	// Command required
	args = []string{"git-lob"}
	opts, ok = parseCommandLine(args)
	if ok {
		t.Error("Should have failed because no command specified")
	}
	// Command required, with other options
	args = []string{"git-lob", "--force", "-q"}
	opts, ok = parseCommandLine(args)
	if ok {
		t.Error("Should have failed because no command specified")
	}

	_ = opts

}
