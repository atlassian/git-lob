package util

import (
	"testing"

	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/atlassian/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
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
