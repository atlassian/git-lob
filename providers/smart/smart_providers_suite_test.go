package smart

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	. "bitbucket.org/sinbad/git-lob/util"
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
	RunSpecs(t, "Git Lob Smart Provider Test Suite")
}
