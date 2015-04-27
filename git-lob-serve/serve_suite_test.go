package main

import (
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "bitbucket.org/sinbad/git-lob/Godeps/_workspace/src/github.com/onsi/gomega"
	"testing"
)

func TestAll(t *testing.T) {
	// Connect Ginkgo to Gomega
	RegisterFailHandler(Fail)

	// Run everything
	RunSpecs(t, "Git Lob Serve Test Suite")
}
