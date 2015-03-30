package util

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"testing"
)

func TestAll(t *testing.T) {
	// Connect Ginkgo to Gomega
	RegisterFailHandler(Fail)

	// Set manual logging off
	loggingOff := true
	//loggingOff = false
	if loggingOff {
		LogSuppressAllConsoleOutput()
	}

	// Run everything
	RunSpecs(t, "Git Lob Util Test Suite")
}
